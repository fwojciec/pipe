# TUI Architecture Design

Date: 2026-02-21

## Problem

The current TUI is a monolithic Model with a textinput, viewport, and strings.Builder.
It lacks structured message rendering, collapsible blocks, markdown formatting, theming,
and multi-line input. Tests use direct model.Update() calls and field inspection rather
than rendered output verification.

## Decisions

- **Tree-of-models** component architecture (each UI element is its own model)
- **teatest** for all TUI testing (rendered output, not field inspection)
- **Full 8-element** UI scope (user msg, assistant text, thinking, tool call, tool result, error, status bar, input)
- **Custom goldmark ANSI renderer** for markdown (~300-400 LOC)
- **ANSI-derived theming** mapped to terminal's base 16 colors (Ghostty-optimized)
- **Forked textarea** stripped and fixed for multi-line chat input (~600-800 LOC)
- **Flat message list** (not turn-based grouping)

## Component Architecture

### Root Model

The existing `bubbletea/model.go`, refactored. Owns:

- `[]MessageBlock` — the conversation as a flat list of renderable blocks
- Viewport — scrollable output area composing all block views
- Input — the forked textarea component
- Status bar — lipgloss-styled string

Routes messages: WindowSizeMsg broadcast to all, keyboard to focused component or
viewport. Manages agent lifecycle (channels, cancel, running state).

### MessageBlock Interface

```go
type MessageBlock interface {
    Init() tea.Cmd
    Update(tea.Msg) (MessageBlock, tea.Cmd)
    View(width int) string
    Height() int
}
```

`View(width int)` takes a width parameter (unlike `tea.Model.View()`) so the root
controls layout width and blocks are testable in isolation.

### Concrete Blocks

| Block | Renders as | Collapsible |
|-------|-----------|-------------|
| UserMessageBlock | `> user text` in user color | No |
| AssistantTextBlock | Markdown via goldmark renderer | No |
| ThinkingBlock | `▶ Thinking...` / `▼` + content | Yes |
| ToolCallBlock | `▶ tool: name` / `▼` + args + result | Yes |
| ErrorBlock | Error text in red | No |

Each block is ~50-100 LOC.

## Streaming & Message Assembly

Events arrive one at a time during agent execution. The root model assembles them
into MessageBlocks incrementally:

- `EventTextDelta` — append to last AssistantTextBlock (or create new one)
- `EventThinkingDelta` — append to last ThinkingBlock (or create new one)
- `EventToolCallBegin` — create new ToolCallBlock with tool name
- `EventToolCallDelta` — append to current ToolCallBlock arguments
- `EventToolCallEnd` — finalize current ToolCallBlock

The `*strings.Builder` output buffer goes away. The `[]MessageBlock` list is the
source of truth. Viewport content is rebuilt by joining `block.View(width)` for all
blocks. During streaming, only the last block changes — earlier blocks cache their
rendered output.

Session reload (`renderSession()`) walks `session.Messages` and creates the
appropriate MessageBlock for each.

## Custom Goldmark ANSI Renderer

New package `markdown/` following Ben Johnson layout (wraps goldmark dependency).

**Architecture:**
- Goldmark parses markdown string to AST
- Custom `goldmark.Renderer` walks AST, outputs lipgloss-styled text
- Single public function: `Render(source string, width int, theme pipe.Theme) string`

**Supported elements** (what LLMs produce):
- Paragraphs (word-wrapped), fenced code blocks (background color + language label)
- Inline code, bold, italic, bold+italic
- Headings (bold + accent color), bullet and numbered lists, links

**Streaming optimization** (McGugan's block-finalization):
- Lives in AssistantTextBlock, not the renderer
- Block tracks finalized text (complete paragraphs separated by `\n\n`)
- Finalized portions are rendered once and cached
- Only trailing unfinalized text is re-rendered on each delta
- Unclosed code fences: detect odd ` ``` ` count, append closing fence before render

~300-400 LOC production, ~400-500 LOC tests.

## ANSI-Derived Theming

### Domain Type (root package)

```go
// theme.go — no dependencies
type Theme struct {
    Foreground int // ANSI color index
    Background int
    UserMsg    int
    Thinking   int
    ToolCall   int
    Error      int
    Success    int
    Muted      int
    CodeBg     int
    Accent     int
}

func DefaultTheme() Theme { ... }
```

### Lipgloss Mapping (bubbletea package)

```go
// styles.go
type Styles struct {
    UserMsg   lipgloss.Style
    Thinking  lipgloss.Style
    ToolCall  lipgloss.Style
    Error     lipgloss.Style
    // ...
}

func NewStyles(t pipe.Theme) Styles { ... }
```

Maps semantic colors to ANSI indices 0-15. The user's terminal theme (Ghostty or
otherwise) determines actual RGB values. Extended 256 palette (indices 232-255
grayscale ramp) used for subtle backgrounds.

| Semantic | ANSI Index | Used for |
|----------|-----------|----------|
| UserMsg | 4 (blue) | User message prefix |
| Thinking | 8 (bright black) | Thinking block text |
| ToolCall | 3 (yellow) | Tool name header |
| Error | 1 (red) | Error messages |
| Success | 2 (green) | Success indicators |
| Muted | 8 (bright black) | Status bar, placeholders |
| CodeBg | 0 (black) | Code block background |
| Accent | 5 (magenta) | Headings, links |

## Multi-line Chat Input

Fork bubbles textarea (~1400 LOC), strip to ~600-800 LOC, fix cache bug.

**Keep:** `[][]rune` storage, cursor positioning, word-wrap logic, key handling,
SetWidth/SetHeight, viewport scrolling.

**Strip:** Line numbers, ShowLineNumbers, complex prompt rendering, placeholder
animation, the Styles system.

**Fix:** Cache invalidation in SetWidth() — the root cause of the textarea bugs.
The memoizedWrap cache hashes `(runes, width)` but SetWidth() never invalidates
stale entries.

**Add:**
- `CheckInputComplete` callback (from bubbline's concept) — Enter sends when
  callback returns true, otherwise inserts newline
- Auto-grow up to MaxHeight (1-3 lines)
- Shift+Enter / Ctrl+J always inserts newline

Lives in `bubbletea/textarea/` as owned code.

## Testing Strategy

All tests use **teatest** with deterministic rendering.

**Infrastructure:**
- `trueColorRenderer()` — forces deterministic color output
- `TestTheme()` — stable ANSI color mapping
- Helpers to initialize model, send messages, wait for output

**Per-component tests:**

| Component | Test focus |
|-----------|-----------|
| Root Model | Message routing, agent lifecycle, viewport scroll, session reload |
| UserMessageBlock | `>` prefix, user color, word-wrapping |
| AssistantTextBlock | Markdown rendering, streaming deltas, finalized block caching |
| ThinkingBlock | Collapsible toggle, collapsed/expanded rendering |
| ToolCallBlock | Collapsible toggle, tool name, arguments, result |
| ErrorBlock | Error color, error text |
| Chat Input | Typing, Enter sends, Shift+Enter newline, wrap, auto-grow |
| Markdown renderer | Each element: headings, code, bold, italic, lists, links |
| Status bar | Idle, generating, error states |

**Integration tests:** Full agent cycle with events, session reload with history,
collapsible block interaction.

Current `model_test.go` tests get rewritten from direct Update + field inspection
to teatest output assertions.

## References

- [Bubble Tea TUI Research](../research/bubbletea-tui.md)
- [Ghostty 256-color generation](https://gist.github.com/jake-stewart/0a8ea46159a7da2c808e5be2177e1783)
- Crush (charmbracelet/crush) — tree-of-models, streaming markdown
- Diffstory — teatest patterns, deterministic rendering, pure render functions
- jira4claude — goldmark AST-based markdown processing
