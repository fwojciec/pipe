# Building a Bubble Tea TUI for an AI coding agent

**Charmbracelet's ecosystem is mature enough to build a production AI agent TUI**, but streaming markdown and collapsible content blocks require custom work — no off-the-shelf solution exists. The flagship reference implementation is **Crush** (charmbracelet/crush, ~20K stars), which evolved from the open-source OpenCode project and implements virtually every feature pipe needs: streaming markdown, tool call rendering, collapsible thinking blocks, and session management. Below is a deep technical analysis across all seven research questions, grounded in real-world source code from Charm's own applications and the broader ecosystem.

---

## 1. The tree-of-models pattern dominates component architecture

Mature Bubble Tea applications universally follow a **tree-of-models** pattern: the root model acts as a message router and screen compositor, with child models embedded as struct fields. Each child implements `Init()`, `Update()`, and `View()`. The root's `Update()` routes messages to children using three strategies: **handle directly** (global keys like quit), **route to focused child** (input keys), or **broadcast to all** (`tea.WindowSizeMsg`).

```go
type rootModel struct {
    header   headerModel
    content  contentModel
    footer   footerModel
    focused  int
    width, height int
}

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        // Broadcast to ALL children
        m.header, cmd = m.header.Update(msg)
        cmds = append(cmds, cmd)
        m.content, cmd = m.content.Update(msg)
        cmds = append(cmds, cmd)
    case tea.KeyMsg:
        // Route to FOCUSED child only
        m.content, cmd = m.content.Update(msg)
        cmds = append(cmds, cmd)
    }
    return m, tea.Batch(cmds...)
}
```

**Soft-serve** (charmbracelet/soft-serve) is the most instructive reference for component architecture. It defines a `common.Component` interface that extends `tea.Model` with `SetSize(width, height int)`, and passes a shared `common.Common` context (terminal dimensions, styles, config) to all components at construction. Its package structure cleanly separates `components/` (reusable widgets like tabs, viewport, statusbar, code viewer) from `pages/` (composed screens like repo view, selection). Custom message types like `SelectTabMsg` and `ActiveTabMsg` handle parent-child communication through Bubble Tea's command system.

**Crush** (charmbracelet/crush) takes this further with a page-based architecture: a root `appModel` coordinates pages, dialogs, and status bars. The chat page alone has **five sub-components** — editor, sidebar, header, message list, and status — with focus management via a `focusedPane` field and automatic compact-mode switching based on terminal dimensions.

### Value semantics vs. pointer receivers — resolved in practice

The theoretical conflict between Bubble Tea's Elm-style value semantics and components needing mutable internal state (like textarea's memoization cache) is **resolved in practice**: pointer receivers are fully supported and preferred for non-trivial applications. You pass `&model{}` to `tea.NewProgram()`. Bubbles v2 explicitly moved to pointer receivers (`fix(textarea): use pointer receiver for Model methods`). The one iron rule: **all model mutations must happen inside `Update()` or `Init()`** — never from goroutines or `tea.Cmd` functions, which would create race conditions.

Note that bubbles components (list, textarea, viewport) don't implement `tea.Model` directly — they return `(Model, tea.Cmd)` from `Update`, not `(tea.Model, tea.Cmd)`. This is intentional: they're meant to be embedded in your models, not used standalone.

### Composing styled blocks into a scrollable viewport

The standard pattern is straightforward: render each block independently with lipgloss, join them with `lipgloss.JoinVertical`, and feed the composed string to `viewport.SetContent()`. The critical lesson from the PUG developer's blog: **never hard-code height arithmetic**. Always use `lipgloss.Height()` to measure rendered content dynamically, because adding a border or changing padding will silently break fixed calculations.

```go
header := headerStyle.Render("Title")
footer := footerStyle.Render("Status")
contentHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
m.viewport.Height = contentHeight
```

### Community component libraries

Beyond the standard bubbles package, notable community libraries include **tree-bubble** (savannahostrowski/tree-bubble) for hierarchical views, **bubbletea-overlay** (rmhubbert) for modal compositing, **BubbleZone** (lrstanley/bubblezone) for mouse event tracking zones, **stickers** (76creates) for responsive flexbox layouts, **ntcharts** (NimbleMarkets) for terminal charts, and **bubbleboxer** (treilik) for side-by-side layout trees. The community maintains a curated list at **charm-and-friends/additional-bubbles**.

---

## 2. Streaming markdown remains the hardest unsolved problem

**Glamour has no streaming API.** Every call to `glamour.Render()` parses and renders the entire document from scratch using goldmark internally. There is no `RenderDelta()`, no cursor-based continuation, no incremental mode. For unclosed code fences, glamour treats everything after the opening backticks as code, producing incorrect output until the fence closes.

However, **re-rendering the full document on each token is feasible for typical LLM responses**. Goldmark parses at ~4.2ms for a benchmark document; a few KB of LLM output parses in sub-millisecond time. At 50–100 tokens/sec, re-rendering a 10KB document 50x/sec is viable but wasteful. For conversations exceeding **100KB**, this becomes prohibitive.

### The glow streaming PR offers a proven architecture

**Glow PR #823** (by @anthonyrisinger, September 2025) implements streaming markdown rendering with three configurable modes: aggressive (render every chunk), bounded (buffer N bytes), and disabled (wait for completion). The key insight from the PR author: *"It'd be much easier if chunking/continuation support landed in glamour... streaming the interiors of fences and tables was pretty much impossible without support from glamour."* The PR works around this by detecting **markdown block boundaries** and feeding glamour complete blocks. It was closed but the approach is well-documented and extractable.

### Will McGugan's four optimizations are directly applicable

Will McGugan (creator of Python's Textual framework) published a detailed blog post on streaming markdown optimizations that maps perfectly to Go:

1. **Block-level finalization**: Markdown divides into top-level blocks (paragraphs, code fences, tables). When appending, only the **last block** can change. Prior blocks are finalized and cached.
2. **In-place updates**: If the last block doesn't change type (stays a paragraph), update it in-place. Watch for type shifts (e.g., "paragraph" → "table" as header rows arrive).
3. **Partial parsing**: Store the byte offset where the last block began. Only feed the parser data from that point forward, making parsing always sub-1ms regardless of document size.
4. **Token buffering**: Buffer between LLM producer and TUI consumer. When tokens arrive faster than the display can update, concatenate them and render when ready.

### Practical recommendation for pipe

The pragmatic approach: **use glamour with full re-render, but apply McGugan's block-finalization optimization**. Track finalized blocks by detecting paragraph breaks and fence closures. Cache rendered output for finalized blocks. Temporarily close unclosed code fences before rendering (inject a closing ```` ``` ```` before passing to glamour). Throttle renders to ~20–30fps max. This gives acceptable performance for conversations under ~50KB with minimal custom code. For longer conversations, consider virtualizing old messages (render only recent N messages in detail, summarize older ones).

Alternatives to glamour include **MichaelMure/go-term-markdown** (uses gomarkdown, same full-render limitation) and building a custom goldmark ANSI renderer via goldmark's `renderer.NodeRenderer` interface, which would allow true incremental rendering but requires significant effort.

**Charmbracelet/mods**, Charm's own LLM CLI tool, sidesteps this entirely: it streams raw text to stdout during generation, then renders formatted markdown with glamour only after completion. **Crush** re-renders via glamour during streaming, accepting the performance cost.

---

## 3. Crush is the primary reference implementation — plus a rich ecosystem

### Tier 1: Charmbracelet's own projects

**Crush** (charmbracelet/crush, ~20K stars) is Charm's flagship agentic coding agent and the single most relevant reference for pipe. It evolved from **opencode-ai/opencode** (~10.9K stars, archived September 2025). Crush uses Bubble Tea for its full TUI with streaming markdown via glamour, click-to-expand code blocks and thinking blocks, session management with SQLite, LSP integration, and MCP tool support. Its AI backend is powered by **charmbracelet/fantasy** (~582 stars), a multi-provider Go agent SDK with a streaming API (`agent.Stream()` returning `fantasy.StreamPart` iterators). The codebase lives in `internal/tui` (UI) and `internal/agent` (AI logic), demonstrating clean separation of concerns.

**Mods** (charmbracelet/mods, ~4.4K stars) is a pipe-friendly CLI tool that streams LLM output to stdout and uses Bubble Tea minimally — only for conversation listing via the `huh` forms library. It's architecturally simpler but demonstrates provider abstraction (OpenAI, Anthropic, Gemini, Ollama via OpenAI-compat) and SQLite-backed conversation history.

### Tier 2: Community AI TUI projects

**chatgpt-tui/nekot** (tearingItUp786/chatgpt-tui) implements a full split-pane chat TUI with Bubble Tea — conversation list on the left, chat on the right, SQLite persistence, and streaming. The author documented streaming as the hardest part, using `tea.Cmd` goroutines that read SSE streams and send messages back via `p.Send()`. **yai** (ekkinox/yai, ~849 stars) is a simpler inline TUI for generating shell commands from natural language. **agent-deck** (asheshgoplani/agent-deck) manages tmux sessions for multiple AI coding agents from a single Bubble Tea TUI.

### The universal streaming pattern in Bubble Tea

Every project follows the same approach:

1. A `tea.Cmd` launches a goroutine that reads from the streaming API
2. Each chunk is sent back as a custom message (e.g., `StreamChunkMsg{Delta string}`)
3. `Update()` appends the delta to an accumulated buffer
4. `View()` renders the full accumulated markdown via glamour
5. A viewport component handles scrolling, auto-scrolling to bottom during streaming

### Component ecosystem beyond bubbles

The **charm-and-friends/additional-bubbles** repository curates community components: **stickers** (responsive flexbox/table), **bubble-table** (interactive paginated tables from both calyptia and evertras), **bubbleup** (notifications/alerts), **promptkit** (selection and text input prompts), **bubbletea-overlay** (modal windows), **bubbleboxer** (layout trees), and **bubblelister** (scrollable lists that can contain other bubbles). **charmbracelet/huh** is the official forms/dialogs library, used by mods for model and conversation selection.

---

## 4. Collapsible content blocks must be built from scratch

**No native collapsible, accordion, or disclosure component exists** in bubbles or the wider Bubble Tea ecosystem. This is a gap that pipe must fill with custom code. The closest existing component is **tree-bubble** (savannahostrowski/tree-bubble, 31 stars), which provides hierarchical navigation with section jumping, but it renders static tree structures rather than expandable content.

### Recommended implementation pattern

Model each collapsible block as a struct with a `Collapsed bool` field. Maintain a flat slice of blocks with a cursor for keyboard navigation. On toggle, flip the boolean and recompute the full content string, then call `viewport.SetContent()`:

```go
type ContentBlock struct {
    Kind      string // "thinking", "tool_call", "bash_output"
    Title     string
    Content   string
    Collapsed bool
}

type conversationModel struct {
    blocks   []ContentBlock
    viewport viewport.Model
    cursor   int
}

func (m *conversationModel) renderContent() string {
    var sections []string
    for i, b := range m.blocks {
        indicator := "▶"
        if !b.Collapsed { indicator = "▼" }
        header := fmt.Sprintf("%s %s", indicator, b.Title)
        if i == m.cursor { header = selectedStyle.Render(header) }
        if b.Collapsed {
            sections = append(sections, header)
        } else {
            sections = append(sections, header+"\n"+b.Content)
        }
    }
    return strings.Join(sections, "\n")
}
```

### Viewport height recalculation

The viewport's scrollable window height stays constant (it's set by the layout). Only the **content height** changes when blocks expand/collapse, and the viewport handles scrolling automatically. Call `viewport.SetContent()` in `Update()`, never in `View()`. After expanding a block, ensure it remains visible by calculating the line offset of the toggled block header and calling `viewport.SetYOffset()` if needed.

Crush (charmbracelet/crush) implements click-to-expand for code blocks, diffs, and thinking blocks — making it the primary reference for this pattern, though its source is under a custom license.

---

## 5. The standard textarea works for chat input — with known workarounds

### The memoization cache bug is documented, not fixed

The textarea's rendering memoization (introduced in bubbles v0.18.0, PR #427) creates stale output when config properties change after initialization. The official API documentation explicitly warns: *"When changing the value of Prompt after the model has been initialized, ensure that SetWidth() gets called afterwards."* This applies to `Prompt`, `ShowLineNumbers`, and any property affecting layout. **The fix is procedural**: always call `SetWidth()` after any config change. This remains true in both v1 and v2.

```go
// CORRECT order — config changes THEN SetWidth:
ta.Prompt = "│ "
ta.ShowLineNumbers = false
ta.SetWidth(termWidth) // Must come AFTER config changes

// On every WindowSizeMsg:
m.textarea.SetWidth(msg.Width) // Invalidates memoization cache
```

### Notable open issues

**Viewport line wrapping** (#644) causes `GotoBottom()` to misbehave with soft-wrapped text — the viewport counts logical lines, not visual lines. **v2 viewport panic** (#820) occurs when enabling `SoftWrap` before dimensions are set. **Paste performance** (#831) is slow because characters are written one-by-one. **Width overflow** (#812) in v2 causes panics when input overflows the set width.

### v2 textarea improvements

Bubbles v2 adds **`MaxHeight`** (auto-grow up to N lines), **`VirtualCursor`** toggle for real vs. virtual cursor rendering, a restructured **`Styles`** struct replacing `FocusedStyle`/`BlurredStyle`, **`DefaultDarkStyles()`/`DefaultLightStyles()`** constructors, and real cursor support via `tea.Cursor`. The `SetCursor()` method is renamed to `SetCursorColumn()`.

### The best alternative: knz/bubbline

**knz/bubbline** (90 stars, Apache-2.0) is a readline-style line editor built on Bubble Tea with features the standard textarea lacks: **auto-resize vertically** as input grows, **history navigation and search**, **tab completion** with a fancy menu, a **`CheckInputComplete` callback** for conditional Enter behavior (send on single line, allow multiline via Ctrl+O), text reflow to fit width, and external editor support. The limitation: bubbline runs its own `tea.Program`, making it harder to embed in a larger TUI layout. For pipe, the **standard textarea with workarounds** is the safer choice, with a potential future migration to bubbline's `editline` internals if needed.

### Recommended chat input setup

```go
func newChatInput(width int) textarea.Model {
    ta := textarea.New()
    ta.Placeholder = "Type a message..."
    ta.Prompt = "│ "
    ta.ShowLineNumbers = false
    ta.CharLimit = 0                            // No limit
    ta.SetWidth(width)
    ta.SetHeight(3)
    ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // No cursor line highlight
    ta.KeyMap.InsertNewline.SetEnabled(false)    // Enter sends, Shift+Enter for newline (v2)
    ta.Focus()
    return ta
}
```

---

## 6. Bubble Tea already diffs — v2 makes it cell-level

A common misconception is that Bubble Tea re-renders everything on every frame. **The v1 standard renderer already performs line-level diffing**: it maintains `lastRenderedLines []string` and compares each line of new output against the previous frame, skipping unchanged lines entirely. This was introduced in v0.13.3. A regression in this logic was caught and fixed in PR #1233 (November 2024). The renderer is also **framerate-based** — it doesn't render on every state change but batches multiple updates into single render passes at ~60fps.

For streaming LLM content, this diffing is particularly effective: when new content appends at the bottom, all lines above remain unchanged and are skipped. The viewport component additionally only returns visible lines (`m.lines[top:bottom]`), so content outside the viewport window never enters the diffing pipeline.

### v2's cell-based renderer is a major advancement

Bubble Tea v2 (currently in beta) replaces the line-based renderer with a **cell-based renderer** (`cellbuf.Screen`) that tracks changes at the individual character cell level. Only dirty cells are written to the terminal. For streaming scenarios where a few characters change per frame, this is dramatically more efficient. v2 also introduces a declarative `tea.View` struct (replacing the string return from `View()`) and eliminates race conditions between lipgloss and Bubble Tea by centralizing all I/O management.

### Practical streaming performance recommendations

- **Throttle `viewport.SetContent()` calls** — buffer tokens and update every 30–50ms, not on every single token arrival
- **Keep the viewport scrolled to bottom** during streaming for optimal diff performance (only the last few lines change)
- **Cache rendered markdown blocks** — only re-render the last (potentially changing) block; append cached output for finalized blocks
- **Run glamour rendering off the main thread** — use a `tea.Cmd` goroutine to render markdown, send the result back as a message, so the event loop isn't blocked by parsing
- **For very long conversations** (100K+ characters), consider virtualizing: render only the last N messages in full detail, collapse older messages to summaries

The deprecated `HighPerformanceRendering` mode used terminal scroll regions for partial updates but was removed because the standard renderer's line-diffing proved sufficient for most use cases.

---

## 7. Lipgloss theming follows a styles-struct pattern — no built-in theme system

Lipgloss has **no built-in theme system**, `themes/` directory, or official theming abstraction. The idiomatic approach across Charm's own applications follows a consistent two-layer pattern: define named colors as variables, then build a styles struct from those colors.

### Pattern from Charm's own projects

**Glow** uses flat `AdaptiveColor` variables — the simplest approach:

```go
var (
    normalDim   = lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"}
    gray        = lipgloss.AdaptiveColor{Light: "#909090", Dark: "#626262"}
    fuchsia     = lipgloss.AdaptiveColor{Light: "#EE6FF8", Dark: "#EE6FF8"}
    yellowGreen = lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#ECFD65"}
)
```

**Mods** uses a styles struct with a constructor accepting `*lipgloss.Renderer` — critical for SSH/multi-output support:

```go
type styles struct {
    AppName, Comment, Flag, Quote, Link lipgloss.Style
}

func makeStyles(r *lipgloss.Renderer) (s styles) {
    s.AppName = r.NewStyle().Bold(true)
    s.Flag = r.NewStyle().Foreground(lipgloss.AdaptiveColor{
        Light: "#00B594", Dark: "#3EEFCF",
    }).Bold(true)
    return s
}
```

**Soft-serve** uses the most elaborate pattern — a deeply nested `Styles` struct with **60+ style fields** organized by component (RepoSelector.Normal.Title, LogItem.Active.Hash, etc.) and a `DefaultStyles()` constructor.

### Implementing a 48-color theme system for pipe

The recommended architecture separates palette definition from style construction:

```go
// Layer 1: Color palette
type Palette struct {
    Background, Foreground    lipgloss.AdaptiveColor
    Primary, Secondary, Accent lipgloss.AdaptiveColor
    Success, Warning, Error   lipgloss.AdaptiveColor
    Gray50, Gray100, Gray200  lipgloss.AdaptiveColor // ... through Gray900
    // Semantic AI colors
    ThinkingBg, ToolCallBg, UserMsgBg, AssistantMsgBg lipgloss.AdaptiveColor
}

// Layer 2: Styles built from palette
type Styles struct {
    App, Title, Heading, Body, Muted lipgloss.Style
    UserMessage, AssistantMessage     lipgloss.Style
    ThinkingBlock, ToolCallBlock      lipgloss.Style
    CodeBlock, Border, StatusBar      lipgloss.Style
}

func NewStyles(p Palette) *Styles {
    return &Styles{
        Title: lipgloss.NewStyle().Foreground(p.Primary).Bold(true),
        ThinkingBlock: lipgloss.NewStyle().
            Background(p.ThinkingBg).
            Foreground(p.Foreground).
            Padding(0, 1),
        // ...
    }
}
```

### Dark/light mode detection

`lipgloss.AdaptiveColor` is the v1 mechanism — the terminal's background color is auto-detected via termenv at runtime, and `HasDarkBackground()` determines which variant to use. `lipgloss.CompleteAdaptiveColor` adds per-color-profile variants (TrueColor, ANSI256, ANSI) on top of light/dark switching for maximum terminal compatibility.

In **Bubble Tea v2**, the model explicitly requests background color detection:

```go
func (m model) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.BackgroundColorMsg:
        m.styles = NewStyles(msg.IsDark())
    }
    return m, nil
}
```

### Third-party theme packages

- **catppuccin/go** (official port): 4 flavors (Latte, Frappé, Macchiato, Mocha) with **26 named colors** each. Returns `lipgloss.TerminalColor` directly. Supports adaptive switching via `catppuccin.Adaptive(catppuccin.Latte, catppuccin.Mocha)`.
- **go.withmatt.com/themes**: **450+ terminal color schemes** from iTerm2-Color-Schemes, all Hex strings compatible with `lipgloss.Color()`.
- **willyv3/gogh-themes/lipgloss**: 361 themes with `lipgloss.Color`-wrapped values.
- **purpleclay/lipgloss-theme**: Pre-built styles (H1–H6, code, links) with shade palettes (S50–S950) using `AdaptiveColor`.

For pipe's minimal dependency philosophy, **catppuccin/go** is the best fit — a single dependency providing a well-designed 26-color palette with semantic color names (Text, Subtext, Surface, Overlay, Base, Mantle, Crust) that map cleanly to UI concerns.

---

## Conclusion: architectural decisions for pipe

The Bubble Tea ecosystem provides a solid foundation, but pipe will need custom work in three areas. First, **streaming markdown** requires building a block-finalization layer over glamour — cache rendered output for completed blocks, only re-render the last active block, and temporarily close unclosed fences before each render pass. Second, **collapsible content blocks** (thinking, tool calls, bash output) must be built from scratch as a custom component wrapping a viewport with toggle state per block. Third, the **textarea** works for chat input with documented workarounds (always call `SetWidth()` after config changes), but monitor bubbles v2 for `MaxHeight` auto-grow support.

The strongest architectural signal comes from Crush and soft-serve: define a `Component` interface extending `tea.Model` with `SetSize(width, height int)`, use pointer receivers throughout, pass a shared context struct to all components, and use custom message types for parent-child communication. For theming, the mods-style styles struct with a `*lipgloss.Renderer` parameter and `AdaptiveColor` throughout provides the right balance of simplicity and flexibility. Target Bubble Tea **v1.3+** for stability today, but architect with v2 migration in mind — the cell-based renderer alone will eliminate most streaming performance concerns.
