# TUI Architecture Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor the monolithic TUI into a tree-of-models architecture with themed message blocks, markdown rendering, and multi-line input.

**Architecture:** Tree-of-models with a MessageBlock interface. Root model owns a flat `[]MessageBlock` list rendered into a viewport. Each block type (user message, assistant text, thinking, tool call, error) is its own model with `View(width int) string`. Custom goldmark ANSI renderer for markdown. Forked textarea for multi-line input. ANSI-derived theming mapped to terminal's base 16 colors.

**Tech Stack:** Go 1.24, Bubble Tea v1.3, bubbles v1.0, lipgloss v1.1, goldmark (new dep), teatest (new dep)

**Design doc:** `docs/plans/2026-02-21-tui-architecture-design.md`

**Constraints:**
- All tests in external `_test` packages (`testpackage` linter)
- All tests call `t.Parallel()` (`paralleltest` linter)
- No globals (`gochecknoglobals` linter)
- Interface compliance checks in production code, not tests
- `make validate` must pass after each commit

---

### Task 1: Theme Domain Type

**Files:**
- Create: `theme.go`
- Test: `theme_test.go`

**Step 1: Write the failing test**

```go
package pipe_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
)

func TestDefaultTheme(t *testing.T) {
	t.Parallel()

	theme := pipe.DefaultTheme()

	// Verify all fields are set (non-zero ANSI indices or explicit 0 for black).
	assert.Equal(t, 4, theme.UserMsg)    // blue
	assert.Equal(t, 1, theme.Error)      // red
	assert.Equal(t, 3, theme.ToolCall)   // yellow
	assert.Equal(t, 8, theme.Thinking)   // bright black
	assert.Equal(t, 2, theme.Success)    // green
	assert.Equal(t, 8, theme.Muted)      // bright black
	assert.Equal(t, 0, theme.CodeBg)     // black
	assert.Equal(t, 5, theme.Accent)     // magenta
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run TestDefaultTheme -v`
Expected: FAIL — `DefaultTheme` not defined

**Step 3: Write minimal implementation**

```go
package pipe

// Theme defines semantic color mappings using ANSI color indices (0-15).
// The user's terminal theme determines the actual RGB values, so the app
// automatically matches any color scheme. Optimized for Ghostty's coherent
// 256-color generation from the base 16 palette.
type Theme struct {
	Foreground int // ANSI index for default text (-1 = terminal default)
	Background int // ANSI index for app background (-1 = terminal default)
	UserMsg    int // User message accent
	Thinking   int // Thinking block text
	ToolCall   int // Tool call header
	Error      int // Error messages
	Success    int // Success indicators
	Muted      int // Status bar, placeholders
	CodeBg     int // Code block background
	Accent     int // Headings, links
}

// DefaultTheme returns the default ANSI color mapping.
func DefaultTheme() Theme {
	return Theme{
		Foreground: -1, // terminal default
		Background: -1, // terminal default
		UserMsg:    4,  // blue
		Thinking:   8,  // bright black (gray)
		ToolCall:   3,  // yellow
		Error:      1,  // red
		Success:    2,  // green
		Muted:      8,  // bright black (gray)
		CodeBg:     0,  // black
		Accent:     5,  // magenta
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -run TestDefaultTheme -v`
Expected: PASS

**Step 5: Run `make validate`**

**Step 6: Commit**

```bash
git add theme.go theme_test.go
git commit -m "Add Theme domain type with ANSI color mappings"
```

---

### Task 2: Styles in bubbletea Package

**Files:**
- Create: `bubbletea/styles.go`
- Test: `bubbletea/styles_test.go`

**Step 1: Write the failing test**

```go
package bubbletea_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewStyles(t *testing.T) {
	t.Parallel()

	theme := pipe.DefaultTheme()
	styles := bt.NewStyles(theme)

	// Styles should produce non-empty rendered strings.
	assert.NotEmpty(t, styles.UserMsg.Render("test"))
	assert.NotEmpty(t, styles.Error.Render("test"))
	assert.NotEmpty(t, styles.Thinking.Render("test"))
	assert.NotEmpty(t, styles.ToolCall.Render("test"))
	assert.NotEmpty(t, styles.Muted.Render("test"))
	assert.NotEmpty(t, styles.Accent.Render("test"))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./bubbletea/... -run TestNewStyles -v`
Expected: FAIL — `NewStyles` not defined

**Step 3: Write minimal implementation**

```go
package bubbletea

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/fwojciec/pipe"
)

// Styles maps a Theme to lipgloss styles for TUI rendering.
type Styles struct {
	UserMsg  lipgloss.Style
	Thinking lipgloss.Style
	ToolCall lipgloss.Style
	Error    lipgloss.Style
	Success  lipgloss.Style
	Muted    lipgloss.Style
	Accent   lipgloss.Style
	CodeBg   lipgloss.Style
}

// NewStyles creates Styles from a Theme.
func NewStyles(t pipe.Theme) Styles {
	return Styles{
		UserMsg:  lipgloss.NewStyle().Foreground(ansiColor(t.UserMsg)).Bold(true),
		Thinking: lipgloss.NewStyle().Foreground(ansiColor(t.Thinking)).Faint(true),
		ToolCall: lipgloss.NewStyle().Foreground(ansiColor(t.ToolCall)),
		Error:    lipgloss.NewStyle().Foreground(ansiColor(t.Error)),
		Success:  lipgloss.NewStyle().Foreground(ansiColor(t.Success)),
		Muted:    lipgloss.NewStyle().Foreground(ansiColor(t.Muted)).Faint(true),
		Accent:   lipgloss.NewStyle().Foreground(ansiColor(t.Accent)).Bold(true),
		CodeBg:   lipgloss.NewStyle().Background(ansiColor(t.CodeBg)),
	}
}

func ansiColor(index int) lipgloss.TerminalColor {
	if index < 0 {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(fmt.Sprintf("%d", index))
}
```

**Step 4: Run test, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/styles.go bubbletea/styles_test.go
git commit -m "Add Styles mapping from Theme to lipgloss"
```

---

### Task 3: MessageBlock Interface

**Files:**
- Create: `bubbletea/block.go`

No test needed — this is an interface definition. Compliance checks will be added
in each concrete block's production file.

**Step 1: Write the interface**

```go
package bubbletea

import tea "github.com/charmbracelet/bubbletea"

// MessageBlock is a renderable element in the conversation.
// Unlike tea.Model, View takes a width parameter so the root model
// controls layout and blocks are testable in isolation.
type MessageBlock interface {
	Update(tea.Msg) (MessageBlock, tea.Cmd)
	View(width int) string
}
```

Note: `Init()` and `Height()` omitted from MVP. Init is unnecessary — blocks don't
have commands to run on startup. Height can be computed from `View()` output when
needed for scroll math. Keep the interface minimal; add methods only when tests
demand them.

**Step 2: Run `make validate`**

**Step 3: Commit**

```bash
git add bubbletea/block.go
git commit -m "Add MessageBlock interface"
```

---

### Task 4: UserMessageBlock

**Files:**
- Create: `bubbletea/block_user.go`
- Test: `bubbletea/block_user_test.go`

**Step 1: Write the failing test**

```go
package bubbletea_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestUserMessageBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders text with prompt prefix", func(t *testing.T) {
		t.Parallel()

		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewUserMessageBlock("hello world", styles)
		view := block.View(80)

		assert.Contains(t, view, ">")
		assert.Contains(t, view, "hello world")
	})

	t.Run("wraps long text to width", func(t *testing.T) {
		t.Parallel()

		styles := bt.NewStyles(pipe.DefaultTheme())
		longText := "short words that keep going and going beyond the viewport width easily"
		block := bt.NewUserMessageBlock(longText, styles)
		view := block.View(30)

		assert.Contains(t, view, "easily")
	})
}
```

**Step 2: Run test to verify it fails**

**Step 3: Write minimal implementation**

```go
package bubbletea

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Interface compliance.
var _ MessageBlock = (*UserMessageBlock)(nil)

// UserMessageBlock renders a user message with a ">" prefix.
type UserMessageBlock struct {
	text   string
	styles Styles
}

// NewUserMessageBlock creates a UserMessageBlock.
func NewUserMessageBlock(text string, styles Styles) *UserMessageBlock {
	return &UserMessageBlock{text: text, styles: styles}
}

// Update implements MessageBlock.
func (b *UserMessageBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

// View implements MessageBlock.
func (b *UserMessageBlock) View(width int) string {
	content := b.styles.UserMsg.Render("> ") + b.text
	return lipgloss.NewStyle().Width(width).Render(content)
}
```

**Step 4: Run test, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/block_user.go bubbletea/block_user_test.go
git commit -m "Add UserMessageBlock"
```

---

### Task 5: ErrorBlock

**Files:**
- Create: `bubbletea/block_error.go`
- Test: `bubbletea/block_error_test.go`

**Step 1: Write the failing test**

```go
package bubbletea_test

import (
	"errors"
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestErrorBlock_View(t *testing.T) {
	t.Parallel()

	styles := bt.NewStyles(pipe.DefaultTheme())
	block := bt.NewErrorBlock(errors.New("something broke"), styles)
	view := block.View(80)

	assert.Contains(t, view, "Error")
	assert.Contains(t, view, "something broke")
}
```

**Step 2: Run test to verify it fails**

**Step 3: Write minimal implementation**

```go
package bubbletea

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ MessageBlock = (*ErrorBlock)(nil)

// ErrorBlock renders an error message.
type ErrorBlock struct {
	err    error
	styles Styles
}

func NewErrorBlock(err error, styles Styles) *ErrorBlock {
	return &ErrorBlock{err: err, styles: styles}
}

func (b *ErrorBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *ErrorBlock) View(width int) string {
	content := b.styles.Error.Render(fmt.Sprintf("Error: %v", b.err))
	return lipgloss.NewStyle().Width(width).Render(content)
}
```

**Step 4: Run test, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/block_error.go bubbletea/block_error_test.go
git commit -m "Add ErrorBlock"
```

---

### Task 6: ThinkingBlock (Collapsible)

**Files:**
- Create: `bubbletea/block_thinking.go`
- Test: `bubbletea/block_thinking_test.go`

**Step 1: Write the failing tests**

```go
package bubbletea_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestThinkingBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("collapsed shows indicator and label", func(t *testing.T) {
		t.Parallel()

		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("deep thoughts here")

		view := block.View(80)
		assert.Contains(t, view, "▶")
		assert.Contains(t, view, "Thinking")
		assert.NotContains(t, view, "deep thoughts here")
	})

	t.Run("expanded shows content", func(t *testing.T) {
		t.Parallel()

		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("deep thoughts here")
		block.Toggle()

		view := block.View(80)
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "deep thoughts here")
	})

	t.Run("append accumulates text", func(t *testing.T) {
		t.Parallel()

		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("hello ")
		block.Append("world")
		block.Toggle()

		view := block.View(80)
		assert.Contains(t, view, "hello world")
	})
}
```

**Step 2: Run test to verify it fails**

**Step 3: Write minimal implementation**

```go
package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ MessageBlock = (*ThinkingBlock)(nil)

// ThinkingBlock renders a collapsible thinking section.
type ThinkingBlock struct {
	content   strings.Builder
	collapsed bool
	styles    Styles
}

func NewThinkingBlock(styles Styles) *ThinkingBlock {
	return &ThinkingBlock{collapsed: true, styles: styles}
}

func (b *ThinkingBlock) Append(text string) {
	b.content.WriteString(text)
}

func (b *ThinkingBlock) Toggle() {
	b.collapsed = !b.collapsed
}

func (b *ThinkingBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *ThinkingBlock) View(width int) string {
	indicator := "▶"
	if !b.collapsed {
		indicator = "▼"
	}

	header := b.styles.Thinking.Render(indicator + " Thinking")

	if b.collapsed {
		return lipgloss.NewStyle().Width(width).Render(header)
	}

	content := b.styles.Thinking.Render(b.content.String())
	full := header + "\n" + content
	return lipgloss.NewStyle().Width(width).Render(full)
}
```

**Step 4: Run test, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/block_thinking.go bubbletea/block_thinking_test.go
git commit -m "Add collapsible ThinkingBlock"
```

---

### Task 7: ToolCallBlock (Collapsible)

**Files:**
- Create: `bubbletea/block_toolcall.go`
- Test: `bubbletea/block_toolcall_test.go`

**Step 1: Write the failing tests**

```go
package bubbletea_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestToolCallBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("collapsed shows tool name", func(t *testing.T) {
		t.Parallel()

		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", styles)

		view := block.View(80)
		assert.Contains(t, view, "▶")
		assert.Contains(t, view, "read")
	})

	t.Run("expanded shows arguments", func(t *testing.T) {
		t.Parallel()

		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", styles)
		block.AppendArgs(`{"path": "/tmp/foo"}`)
		block.Toggle()

		view := block.View(80)
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "/tmp/foo")
	})

	t.Run("finalized shows result summary", func(t *testing.T) {
		t.Parallel()

		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("bash", styles)
		block.Finalize()

		view := block.View(80)
		assert.Contains(t, view, "bash")
		// Finalized blocks start collapsed.
		assert.Contains(t, view, "▶")
	})
}
```

**Step 2: Run test to verify it fails**

**Step 3: Write minimal implementation**

```go
package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ MessageBlock = (*ToolCallBlock)(nil)

// ToolCallBlock renders a collapsible tool call section.
type ToolCallBlock struct {
	name      string
	args      strings.Builder
	collapsed bool
	finalized bool
	styles    Styles
}

func NewToolCallBlock(name string, styles Styles) *ToolCallBlock {
	return &ToolCallBlock{name: name, collapsed: true, styles: styles}
}

func (b *ToolCallBlock) AppendArgs(text string) {
	b.args.WriteString(text)
}

func (b *ToolCallBlock) Toggle() {
	b.collapsed = !b.collapsed
}

func (b *ToolCallBlock) Finalize() {
	b.finalized = true
}

func (b *ToolCallBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *ToolCallBlock) View(width int) string {
	indicator := "▶"
	if !b.collapsed {
		indicator = "▼"
	}

	header := b.styles.ToolCall.Render(indicator + " " + b.name)

	if b.collapsed {
		return lipgloss.NewStyle().Width(width).Render(header)
	}

	content := b.args.String()
	full := header + "\n" + content
	return lipgloss.NewStyle().Width(width).Render(full)
}
```

**Step 4: Run test, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/block_toolcall.go bubbletea/block_toolcall_test.go
git commit -m "Add collapsible ToolCallBlock"
```

---

### Task 8: Custom Goldmark ANSI Renderer

**Files:**
- Create: `markdown/markdown.go`
- Create: `markdown/renderer.go`
- Test: `markdown/markdown_test.go`

This task adds `github.com/yuin/goldmark` as a dependency.

**Step 1: Add goldmark dependency**

```bash
cd /Users/filip/code/go/pipe && go get github.com/yuin/goldmark
```

**Step 2: Write the failing tests**

```go
package markdown_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/markdown"
	"github.com/stretchr/testify/assert"
)

func TestRender(t *testing.T) {
	t.Parallel()

	theme := pipe.DefaultTheme()

	t.Run("plain paragraph", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("hello world", 80, theme)
		assert.Contains(t, result, "hello world")
	})

	t.Run("heading", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("# Title", 80, theme)
		assert.Contains(t, result, "Title")
	})

	t.Run("bold text", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("**bold**", 80, theme)
		assert.Contains(t, result, "bold")
	})

	t.Run("italic text", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("*italic*", 80, theme)
		assert.Contains(t, result, "italic")
	})

	t.Run("inline code", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("`code`", 80, theme)
		assert.Contains(t, result, "code")
	})

	t.Run("fenced code block", func(t *testing.T) {
		t.Parallel()
		src := "```go\nfmt.Println(\"hi\")\n```"
		result := markdown.Render(src, 80, theme)
		assert.Contains(t, result, "fmt.Println")
	})

	t.Run("bullet list", func(t *testing.T) {
		t.Parallel()
		src := "- one\n- two\n- three"
		result := markdown.Render(src, 80, theme)
		assert.Contains(t, result, "one")
		assert.Contains(t, result, "two")
	})

	t.Run("ordered list", func(t *testing.T) {
		t.Parallel()
		src := "1. first\n2. second"
		result := markdown.Render(src, 80, theme)
		assert.Contains(t, result, "first")
		assert.Contains(t, result, "second")
	})

	t.Run("link", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("[click](https://example.com)", 80, theme)
		assert.Contains(t, result, "click")
		assert.Contains(t, result, "example.com")
	})

	t.Run("wraps to width", func(t *testing.T) {
		t.Parallel()
		long := "word word word word word word word word word word word word word"
		result := markdown.Render(long, 20, theme)
		assert.Contains(t, result, "word")
		// All words should be present (wrapped, not truncated).
		assert.Contains(t, result, "word")
	})
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./markdown/... -v`
Expected: FAIL — package doesn't exist

**Step 4: Implement the renderer**

`markdown/markdown.go` — public API:

```go
// Package markdown renders markdown text to ANSI-styled terminal output
// using goldmark for parsing and lipgloss for styling.
package markdown

import "github.com/fwojciec/pipe"

// Render parses markdown source and returns ANSI-styled terminal output
// wrapped to the given width.
func Render(source string, width int, theme pipe.Theme) string {
	r := newRenderer(theme)
	return r.render([]byte(source), width)
}
```

`markdown/renderer.go` — internal renderer implementation. This is the core work:
a goldmark AST walker that produces ANSI-styled output. Key approach:

- Use `goldmark.New()` to parse source into AST
- Walk AST nodes with a custom `renderer.NodeRenderer` implementation
- For each node kind, emit styled text using lipgloss:
  - `ast.Heading` → bold + accent color
  - `ast.Paragraph` → plain text
  - `ast.FencedCodeBlock` → code background style, preserve whitespace
  - `ast.Emphasis` (level 1) → italic, (level 2) → bold
  - `ast.CodeSpan` → inline code background
  - `ast.List` → indented with `- ` or `1. ` markers
  - `ast.Link` → underlined text + dimmed URL
- Wrap final output to width using `lipgloss.NewStyle().Width(width)`

Reference implementation: `jira4claude/markdown/to_markdown.go` (~278 lines) for
the AST walking pattern. Our renderer will be similar complexity but output styled
strings instead of plain markdown.

**Step 5: Run tests, then `make validate`**

**Step 6: Commit**

```bash
git add markdown/ go.mod go.sum
git commit -m "Add custom goldmark ANSI markdown renderer"
```

---

### Task 9: AssistantTextBlock

**Files:**
- Create: `bubbletea/block_assistant.go`
- Test: `bubbletea/block_assistant_test.go`

**Step 1: Write the failing tests**

```go
package bubbletea_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestAssistantTextBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders markdown", func(t *testing.T) {
		t.Parallel()

		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("hello **world**")

		view := block.View(80)
		assert.Contains(t, view, "hello")
		assert.Contains(t, view, "world")
	})

	t.Run("append accumulates deltas", func(t *testing.T) {
		t.Parallel()

		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("hello ")
		block.Append("world")

		view := block.View(80)
		assert.Contains(t, view, "hello world")
	})

	t.Run("wraps to width", func(t *testing.T) {
		t.Parallel()

		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("short words that keep going and going beyond thirty columns easily")

		view := block.View(30)
		assert.Contains(t, view, "easily")
	})
}
```

**Step 2: Run test to verify it fails**

**Step 3: Write minimal implementation**

```go
package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/markdown"
)

var _ MessageBlock = (*AssistantTextBlock)(nil)

// AssistantTextBlock renders assistant text with markdown formatting.
type AssistantTextBlock struct {
	content strings.Builder
	theme   pipe.Theme
	styles  Styles
}

func NewAssistantTextBlock(theme pipe.Theme, styles Styles) *AssistantTextBlock {
	return &AssistantTextBlock{theme: theme, styles: styles}
}

func (b *AssistantTextBlock) Append(text string) {
	b.content.WriteString(text)
}

func (b *AssistantTextBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *AssistantTextBlock) View(width int) string {
	return markdown.Render(b.content.String(), width, b.theme)
}
```

Note: McGugan's block-finalization optimization (cache finalized paragraphs, only
re-render trailing text) is deferred to a follow-up. The initial implementation
re-renders everything — this is fine for conversations under ~50KB per the research.

**Step 4: Run test, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/block_assistant.go bubbletea/block_assistant_test.go
git commit -m "Add AssistantTextBlock with markdown rendering"
```

---

### Task 10: Forked Textarea

**Files:**
- Create: `bubbletea/textarea/textarea.go`
- Create: `bubbletea/textarea/wrap.go`
- Test: `bubbletea/textarea/textarea_test.go`

This is the largest single task. Port the bubbles textarea, strip it, fix it.

**Source:** `~/go/pkg/mod/github.com/charmbracelet/bubbles@v1.0.0/textarea/textarea.go`

**Step 1: Copy and strip textarea source**

Port `textarea.go` from bubbles v1.0.0 into `bubbletea/textarea/`. Strip:
- `ShowLineNumbers` and all line number rendering logic
- `Prompt` variations (use empty string always)
- `FocusedStyle` / `BlurredStyle` complexity (single style)
- Placeholder animation
- The existing Styles struct (we apply our own theme externally)

Keep:
- `[][]rune` text storage
- Cursor positioning (`row`, `col`, `lastCharOffset`)
- Word-wrap logic (`wrap()` function → `wrap.go`)
- `SetWidth()` / `SetHeight()` / `MaxHeight`
- Key handling (arrows, backspace, delete, home/end, word movement)
- Viewport scrolling for content taller than visible area
- `Value()` / `SetValue()` / `Reset()`
- `Focus()` / `Blur()`

Fix:
- **Cache invalidation**: In `SetWidth()`, after updating `m.width`, clear the
  memoization cache: `m.cache = newMemoCache(m.MaxHeight)`. This is the one-line
  fix for the stale cache bug.

Add:
- `CheckInputComplete func(value string) bool` — callback field. When set and
  Enter is pressed, if `CheckInputComplete(m.Value())` returns false, insert a
  newline instead of sending. The root model sets this to always return true
  (Enter sends), but the infrastructure supports future conditional behavior.
- Auto-grow: when content changes, if total soft-wrapped lines < MaxHeight,
  set visible height to match. Cap at MaxHeight.

**Step 2: Write the failing tests**

```go
package textarea_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe/bubbletea/textarea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextarea_Basic(t *testing.T) {
	t.Parallel()

	t.Run("new textarea is empty", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		assert.Equal(t, "", ta.Value())
	})

	t.Run("set and get value", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.SetValue("hello")
		assert.Equal(t, "hello", ta.Value())
	})

	t.Run("typing adds characters", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		ta, _ = applyKey(ta, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		ta, _ = applyKey(ta, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
		assert.Equal(t, "hi", ta.Value())
	})

	t.Run("enter inserts newline", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		ta.SetValue("line1")
		ta, _ = applyKey(ta, tea.KeyMsg{Type: tea.KeyEnter})
		ta, _ = applyKey(ta, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
		assert.Equal(t, "line1\n2", ta.Value())
	})

	t.Run("backspace deletes character", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		ta.SetValue("abc")
		// Move cursor to end.
		ta, _ = applyKey(ta, tea.KeyMsg{Type: tea.KeyEnd})
		ta, _ = applyKey(ta, tea.KeyMsg{Type: tea.KeyBackspace})
		assert.Equal(t, "ab", ta.Value())
	})
}

func TestTextarea_SetWidth_InvalidatesCache(t *testing.T) {
	t.Parallel()

	// This is the regression test for the bubbles textarea cache bug.
	// Changing width must produce correct wrapping immediately.
	ta := textarea.New()
	ta.SetWidth(80)
	ta.Focus()
	ta.SetValue("a long line that should wrap at different widths depending on the setting")

	// Render at width 80 (no wrap expected).
	view80 := ta.View()

	// Change to narrow width.
	ta.SetWidth(20)
	view20 := ta.View()

	// The narrow view should be taller (more wrapped lines).
	require.NotEqual(t, view80, view20)
}

// applyKey is a test helper that sends a key message and returns the updated model.
func applyKey(ta textarea.Model, msg tea.KeyMsg) (textarea.Model, tea.Cmd) {
	updated, cmd := ta.Update(msg)
	return updated.(textarea.Model), cmd
}
```

**Step 3: Run tests to verify they fail**

**Step 4: Implement the forked textarea**

Port from bubbles source, apply the changes described above. Target ~600-800 LOC.

**Step 5: Run tests, then `make validate`**

**Step 6: Commit**

```bash
git add bubbletea/textarea/
git commit -m "Add forked textarea with cache fix and auto-grow"
```

---

### Task 11: Refactor Root Model

**Files:**
- Modify: `bubbletea/model.go`
- Modify: `bubbletea/bubbletea.go` (add Styles to New signature)

This is the core refactoring task. Replace the monolithic strings.Builder with
`[]MessageBlock` and wire all the new components together.

**Step 1: Write failing tests for the new architecture**

Update the test helpers in `bubbletea/bubbletea_test.go` to pass a theme:

```go
func initModel(t *testing.T, run bt.AgentFunc) bt.Model {
	t.Helper()
	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(run, session, theme)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, ok := updated.(bt.Model)
	require.True(t, ok)
	return model
}
```

Write new tests for block-based behavior:

```go
func TestModel_BlockAssembly(t *testing.T) {
	t.Parallel()

	t.Run("text delta creates assistant block", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		updated, _ := m.Update(bt.StreamEventMsg{Event: pipe.EventTextDelta{Delta: "hello"}})
		model := updated.(bt.Model)
		assert.Contains(t, model.View(), "hello")
		assert.Equal(t, 1, model.BlockCount())
	})

	t.Run("thinking delta creates thinking block", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		updated, _ := m.Update(bt.StreamEventMsg{Event: pipe.EventThinkingDelta{Delta: "hmm"}})
		model := updated.(bt.Model)
		// Thinking blocks start collapsed, so content is hidden.
		assert.Equal(t, 1, model.BlockCount())
	})

	t.Run("tool call begin creates tool block", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		updated, _ := m.Update(bt.StreamEventMsg{Event: pipe.EventToolCallBegin{Name: "read"}})
		model := updated.(bt.Model)
		assert.Contains(t, model.View(), "read")
		assert.Equal(t, 1, model.BlockCount())
	})

	t.Run("submit creates user block", func(t *testing.T) {
		t.Parallel()
		session := &pipe.Session{}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = updated.(bt.Model)
		m.Input.SetValue("hi")
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(bt.Model)
		assert.Contains(t, m.View(), "hi")
		assert.GreaterOrEqual(t, m.BlockCount(), 1)
	})
}
```

**Step 2: Run tests to verify they fail**

**Step 3: Refactor model.go**

Key changes to `Model` struct:

```go
type Model struct {
	Input    textinput.Model  // TODO: replace with forked textarea in Task 12
	Viewport viewport.Model

	run     AgentFunc
	session *pipe.Session
	theme   pipe.Theme
	styles  Styles

	blocks  []MessageBlock
	running bool
	cancel  context.CancelFunc
	eventCh chan pipe.Event
	doneCh  chan error
	err     error
	ready   bool
}
```

Remove: `output *strings.Builder`
Add: `blocks []MessageBlock`, `theme pipe.Theme`, `styles Styles`

Change `New()` signature:

```go
func New(run AgentFunc, session *pipe.Session, theme pipe.Theme) Model {
	styles := NewStyles(theme)
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Prompt = ""
	ti.Focus()
	ti.CharLimit = 0
	return Model{
		Input:   ti,
		run:     run,
		session: session,
		theme:   theme,
		styles:  styles,
	}
}
```

Add: `BlockCount() int` method for test access.

Replace `processEvent()` with block-based assembly:

```go
func (m *Model) processEvent(evt pipe.Event) {
	switch e := evt.(type) {
	case pipe.EventTextDelta:
		if b, ok := m.lastBlock().(*AssistantTextBlock); ok {
			b.Append(e.Delta)
		} else {
			b := NewAssistantTextBlock(m.theme, m.styles)
			b.Append(e.Delta)
			m.blocks = append(m.blocks, b)
		}
	case pipe.EventThinkingDelta:
		if b, ok := m.lastBlock().(*ThinkingBlock); ok {
			b.Append(e.Delta)
		} else {
			b := NewThinkingBlock(m.styles)
			b.Append(e.Delta)
			m.blocks = append(m.blocks, b)
		}
	case pipe.EventToolCallBegin:
		b := NewToolCallBlock(e.Name, m.styles)
		m.blocks = append(m.blocks, b)
	case pipe.EventToolCallDelta:
		if b, ok := m.lastBlock().(*ToolCallBlock); ok {
			b.AppendArgs(e.Delta)
		}
	case pipe.EventToolCallEnd:
		if b, ok := m.lastBlock().(*ToolCallBlock); ok {
			b.Finalize()
		}
	}
}

func (m Model) lastBlock() MessageBlock {
	if len(m.blocks) == 0 {
		return nil
	}
	return m.blocks[len(m.blocks)-1]
}
```

Replace `renderContent()`:

```go
func (m Model) renderContent() string {
	var parts []string
	for _, b := range m.blocks {
		parts = append(parts, b.View(m.Viewport.Width))
	}
	return strings.Join(parts, "\n\n")
}
```

Replace `renderSession()`:

```go
func (m *Model) renderSession() {
	for _, msg := range m.session.Messages {
		switch msg := msg.(type) {
		case pipe.UserMessage:
			for _, b := range msg.Content {
				if tb, ok := b.(pipe.TextBlock); ok {
					m.blocks = append(m.blocks, NewUserMessageBlock(tb.Text, m.styles))
				}
			}
		case pipe.AssistantMessage:
			for _, b := range msg.Content {
				switch cb := b.(type) {
				case pipe.TextBlock:
					block := NewAssistantTextBlock(m.theme, m.styles)
					block.Append(cb.Text)
					m.blocks = append(m.blocks, block)
				case pipe.ThinkingBlock:
					block := NewThinkingBlock(m.styles)
					block.Append(cb.Thinking)
					m.blocks = append(m.blocks, block)
				case pipe.ToolCallBlock:
					block := NewToolCallBlock(cb.Name, m.styles)
					block.AppendArgs(string(cb.Arguments))
					block.Finalize()
					m.blocks = append(m.blocks, block)
				}
			}
		case pipe.ToolResultMessage:
			// Skip tool results in output for now.
		}
	}
}
```

Replace `submitInput()` to create UserMessageBlock:

```go
// In submitInput, replace m.output.WriteString with:
m.blocks = append(m.blocks, NewUserMessageBlock(text, m.styles))
```

Replace `statusLine()` to use themed styles:

```go
func (m Model) statusLine() string {
	if m.err != nil {
		return m.styles.Error.Render(fmt.Sprintf("Error: %v", m.err))
	}
	if m.running {
		return m.styles.Muted.Render("Generating...")
	}
	return m.styles.Muted.Render("Enter to send, Ctrl+C to quit")
}
```

**Step 4: Update all existing tests**

Update `bubbletea/bubbletea_test.go` helpers and `bubbletea/model_test.go` to pass
the theme parameter. Most tests should pass with minimal changes since they assert
on `View()` content which still contains the same text. Key changes:

- `bt.New(nopAgent, session)` → `bt.New(nopAgent, session, pipe.DefaultTheme())`
- Remove any tests that reference `m.Viewport.Width` or `m.Viewport.Height`
  directly (internal state) — replace with View() assertions if needed
- The `Output` field and `SetRunning` helpers need updating for the new struct

**Step 5: Run all tests, then `make validate`**

**Step 6: Commit**

```bash
git add bubbletea/model.go bubbletea/bubbletea.go bubbletea/bubbletea_test.go bubbletea/model_test.go
git commit -m "Refactor root model to tree-of-models with MessageBlock list"
```

---

### Task 12: Swap Input to Forked Textarea

**Files:**
- Modify: `bubbletea/model.go` — replace `textinput.Model` with `textarea.Model`

**Step 1: Write a failing test for multi-line input**

```go
t.Run("shift+enter inserts newline in input", func(t *testing.T) {
	t.Parallel()

	m := initModel(t, nopAgent)
	// Type "line1", press shift+enter, type "line2".
	m.Input.SetValue("line1")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = updated.(bt.Model)
	// ... type "line2" and verify Value() contains newline.
})
```

**Step 2: Run test to verify it fails**

**Step 3: Swap the input component**

In `model.go`:
- Replace `"github.com/charmbracelet/bubbles/textinput"` import with
  `"github.com/fwojciec/pipe/bubbletea/textarea"`
- Change `Input textinput.Model` to `Input textarea.Model`
- In `New()`: replace `textinput.New()` setup with `textarea.New()` setup
- In `Init()`: return `textarea.Blink` or `nil`
- In `handleKey()`: Enter handling checks `CheckInputComplete` callback
- In `submitInput()`: use `m.Input.Value()` / `m.Input.Reset()`
- In `handleWindowSize()`: use `m.Input.SetWidth(msg.Width)`

**Step 4: Run all tests, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/model.go
git commit -m "Swap input to forked textarea for multi-line support"
```

---

### Task 13: Update cmd/pipe Wiring

**Files:**
- Modify: `cmd/pipe/main.go`

**Step 1: Update the New() call to pass theme**

```go
// In run():
theme := pipe.DefaultTheme()
tuiModel := bt.New(agentFn, &session, theme)
```

**Step 2: Run `make validate`**

**Step 3: Manual smoke test**

```bash
cd /Users/filip/code/go/pipe && go run ./cmd/pipe/
```

Verify:
- App starts, shows input
- Can type and submit a message
- Agent responses render with markdown styling
- Thinking blocks show collapsed
- Tool calls show collapsed with tool name
- Ctrl+C quits when idle
- Ctrl+C cancels when running

**Step 4: Commit**

```bash
git add cmd/pipe/main.go
git commit -m "Wire theme into TUI startup"
```

---

### Task 14: Migrate Tests to teatest (Optional Enhancement)

**Files:**
- Modify: `bubbletea/model_test.go`
- Modify: `bubbletea/bubbletea_test.go`

This task migrates integration tests to teatest. It's optional — the direct
Update/View testing from earlier tasks already provides good coverage. teatest
adds value for tests that involve async behavior and full rendered output
verification.

**Step 1: Add teatest dependency**

```bash
go get github.com/charmbracelet/x/exp/teatest
```

**Step 2: Add deterministic renderer helper**

```go
// In bubbletea_test.go:
func trueColorRenderer() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	return r
}
```

Note: This requires the model to accept a renderer option. Add
`WithRenderer(*lipgloss.Renderer)` option to `New()` if needed, or defer
this task until the renderer option is architecturally justified.

**Step 3: Rewrite integration test with teatest**

```go
func TestModel_FullCycle_Teatest(t *testing.T) {
	t.Parallel()

	agent := func(_ context.Context, session *pipe.Session, onEvent func(pipe.Event)) error {
		onEvent(pipe.EventTextDelta{Index: 0, Delta: "Hello!"})
		session.Messages = append(session.Messages, pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "Hello!"}},
			StopReason: pipe.StopEndTurn,
		})
		return nil
	}

	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(agent, session, theme)

	tm := teatest.NewTestModel(t, m,
		teatest.WithInitialTermSize(80, 24),
	)

	// Type and submit.
	tm.Type("hi")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	// Wait for agent response.
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Hello!"))
	})

	// Quit.
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}
```

**Step 4: Run tests, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/model_test.go bubbletea/bubbletea_test.go go.mod go.sum
git commit -m "Migrate integration tests to teatest"
```

---

## Task Dependency Graph

```
Task 1 (Theme) ─────┬── Task 2 (Styles) ──┬── Task 4 (UserMsg)
                     │                      ├── Task 5 (Error)
Task 3 (Interface) ──┤                      ├── Task 6 (Thinking)
                     │                      ├── Task 7 (ToolCall)
                     │                      └── Task 9 (Assistant) ── requires Task 8
                     │
                     ├── Task 8 (Markdown) ─── independent, needs goldmark
                     │
                     └── Task 10 (Textarea) ── independent

Task 11 (Root Refactor) ── requires Tasks 1-9
Task 12 (Input Swap) ── requires Tasks 10, 11
Task 13 (Wiring) ── requires Task 11
Task 14 (teatest) ── optional, requires Task 11
```

Tasks 4-7 can be done in any order (all depend on Tasks 1-3).
Task 8 can be done in parallel with Tasks 4-7.
Task 10 can be done in parallel with everything before Task 11.
