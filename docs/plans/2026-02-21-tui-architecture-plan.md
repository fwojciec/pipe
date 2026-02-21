# TUI Architecture Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor the monolithic TUI into a tree-of-models architecture with themed message blocks, markdown rendering, and multi-line input.

**Architecture:** Tree-of-models with a MessageBlock interface. Root model owns a flat `[]MessageBlock` list rendered into a viewport. Each block type (user message, assistant text, thinking, tool call, tool result, error) is its own model with `View(width int) string`. Custom goldmark ANSI renderer for markdown. Forked textarea for multi-line input. ANSI-derived theming mapped to terminal's base 16 colors. Event-to-block correlation uses Index/ID fields from events, not last-block-type heuristics.

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

	assert.Equal(t, 4, theme.UserMsg)
	assert.Equal(t, 1, theme.Error)
	assert.Equal(t, 3, theme.ToolCall)
	assert.Equal(t, 8, theme.Thinking)
	assert.Equal(t, 2, theme.Success)
	assert.Equal(t, 8, theme.Muted)
	assert.Equal(t, 0, theme.CodeBg)
	assert.Equal(t, 5, theme.Accent)
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
		Foreground: -1,
		Background: -1,
		UserMsg:    4,
		Thinking:   8,
		ToolCall:   3,
		Error:      1,
		Success:    2,
		Muted:      8,
		CodeBg:     0,
		Accent:     5,
	}
}
```

**Step 4: Run test to verify it passes**

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

	assert.NotEmpty(t, styles.UserMsg.Render("test"))
	assert.NotEmpty(t, styles.Error.Render("test"))
	assert.NotEmpty(t, styles.Thinking.Render("test"))
	assert.NotEmpty(t, styles.ToolCall.Render("test"))
	assert.NotEmpty(t, styles.Muted.Render("test"))
	assert.NotEmpty(t, styles.Accent.Render("test"))
}
```

**Step 2: Run test to verify it fails**

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

No test needed — interface definition only. Compliance checks added per concrete block.

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

// ToggleMsg tells a collapsible block to toggle its collapsed state.
// Sent by the root model when the user presses the toggle key on a focused block.
type ToggleMsg struct{}
```

**Step 2: Run `make validate`**

**Step 3: Commit**

```bash
git add bubbletea/block.go
git commit -m "Add MessageBlock interface and ToggleMsg"
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

var _ MessageBlock = (*UserMessageBlock)(nil)

type UserMessageBlock struct {
	text   string
	styles Styles
}

func NewUserMessageBlock(text string, styles Styles) *UserMessageBlock {
	return &UserMessageBlock{text: text, styles: styles}
}

func (b *UserMessageBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

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
	"strings"
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

	t.Run("toggle via ToggleMsg", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("thoughts")
		// Starts collapsed.
		assert.NotContains(t, block.View(80), "thoughts")
		// ToggleMsg expands it.
		updated, _ := block.Update(bt.ToggleMsg{})
		block = updated.(*bt.ThinkingBlock)
		assert.Contains(t, block.View(80), "thoughts")
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
	if _, ok := msg.(ToggleMsg); ok {
		b.collapsed = !b.collapsed
	}
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
git commit -m "Add collapsible ThinkingBlock with ToggleMsg support"
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
	"encoding/json"
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
		block := bt.NewToolCallBlock("read", "tc-1", styles)
		view := block.View(80)
		assert.Contains(t, view, "▶")
		assert.Contains(t, view, "read")
	})

	t.Run("expanded shows arguments", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", "tc-1", styles)
		block.AppendArgs(`{"path": "/tmp/foo"}`)
		block.Toggle()
		view := block.View(80)
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "/tmp/foo")
	})

	t.Run("finalize with call applies arguments from EventToolCallEnd", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		// Simulate Gemini pattern: begin + end with no deltas.
		block := bt.NewToolCallBlock("bash", "tc-2", styles)
		block.FinalizeWithCall(pipe.ToolCallBlock{
			ID:        "tc-2",
			Name:      "bash",
			Arguments: json.RawMessage(`{"command":"ls"}`),
		})
		block.Toggle()
		view := block.View(80)
		assert.Contains(t, view, "ls")
	})

	t.Run("toggle via ToggleMsg", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", "tc-1", styles)
		block.AppendArgs(`{"path":"x"}`)
		// Starts collapsed.
		assert.NotContains(t, block.View(80), "path")
		updated, _ := block.Update(bt.ToggleMsg{})
		block = updated.(*bt.ToolCallBlock)
		assert.Contains(t, block.View(80), "path")
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
	"github.com/fwojciec/pipe"
)

var _ MessageBlock = (*ToolCallBlock)(nil)

type ToolCallBlock struct {
	name      string
	id        string
	args      strings.Builder
	collapsed bool
	finalized bool
	styles    Styles
}

func NewToolCallBlock(name, id string, styles Styles) *ToolCallBlock {
	return &ToolCallBlock{name: name, id: id, collapsed: true, styles: styles}
}

// ID returns the tool call ID for event correlation.
func (b *ToolCallBlock) ID() string { return b.id }

func (b *ToolCallBlock) AppendArgs(text string) {
	b.args.WriteString(text)
}

func (b *ToolCallBlock) Toggle() {
	b.collapsed = !b.collapsed
}

// FinalizeWithCall applies the completed tool call, including arguments
// from EventToolCallEnd. This handles providers like Gemini that emit
// begin+end without intermediate deltas.
func (b *ToolCallBlock) FinalizeWithCall(call pipe.ToolCallBlock) {
	if b.args.Len() == 0 && len(call.Arguments) > 0 {
		b.args.Write(call.Arguments)
	}
	b.finalized = true
}

func (b *ToolCallBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	if _, ok := msg.(ToggleMsg); ok {
		b.collapsed = !b.collapsed
	}
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
git commit -m "Add collapsible ToolCallBlock with ID correlation and FinalizeWithCall"
```

---

### Task 7b: ToolResultBlock

**Files:**
- Create: `bubbletea/block_toolresult.go`
- Test: `bubbletea/block_toolresult_test.go`

The design specifies full scope including tool results. This block renders below the
corresponding ToolCallBlock in the flat list.

**Step 1: Write the failing tests**

```go
package bubbletea_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestToolResultBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders result content", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "file contents here", false, styles)
		view := block.View(80)
		assert.Contains(t, view, "file contents here")
	})

	t.Run("error result shows error styling", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "command failed", true, styles)
		view := block.View(80)
		assert.Contains(t, view, "command failed")
	})

	t.Run("long result wraps to width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		long := "this is a very long result that should wrap properly within the viewport"
		block := bt.NewToolResultBlock("read", long, false, styles)
		view := block.View(30)
		assert.Contains(t, view, "viewport")
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

var _ MessageBlock = (*ToolResultBlock)(nil)

type ToolResultBlock struct {
	toolName string
	content  string
	isError  bool
	styles   Styles
}

func NewToolResultBlock(toolName, content string, isError bool, styles Styles) *ToolResultBlock {
	return &ToolResultBlock{toolName: toolName, content: content, isError: isError, styles: styles}
}

func (b *ToolResultBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *ToolResultBlock) View(width int) string {
	style := b.styles.Muted
	if b.isError {
		style = b.styles.Error
	}
	rendered := style.Render(b.content)
	return lipgloss.NewStyle().Width(width).Render(rendered)
}
```

**Step 4: Run test, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/block_toolresult.go bubbletea/block_toolresult_test.go
git commit -m "Add ToolResultBlock"
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
go get github.com/yuin/goldmark
```

**Step 2: Write the failing tests**

```go
package markdown_test

import (
	"strings"
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

	t.Run("fenced code block preserves content without reflow", func(t *testing.T) {
		t.Parallel()
		// Code blocks must NOT be word-wrapped — only paragraphs are.
		src := "```go\nfmt.Println(\"hello world\")\n```"
		result := markdown.Render(src, 20, theme)
		// The code line must remain intact despite width=20.
		assert.Contains(t, result, `fmt.Println("hello world")`)
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

	t.Run("link shows text and URL", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("[click](https://example.com)", 80, theme)
		assert.Contains(t, result, "click")
		assert.Contains(t, result, "example.com")
	})

	t.Run("paragraph wraps to width", func(t *testing.T) {
		t.Parallel()
		long := "word1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12"
		result := markdown.Render(long, 30, theme)
		// All words present (wrapped, not truncated).
		assert.Contains(t, result, "word1")
		assert.Contains(t, result, "word12")
		// Verify it actually wrapped (output has multiple lines).
		lines := strings.Split(result, "\n")
		assert.Greater(t, len(lines), 1)
	})
}
```

**Step 3: Run tests to verify they fail**

**Step 4: Implement the renderer**

`markdown/markdown.go` — public API:

```go
// Package markdown renders markdown text to ANSI-styled terminal output
// using goldmark for parsing and lipgloss for styling.
package markdown

import "github.com/fwojciec/pipe"

// Render parses markdown source and returns ANSI-styled terminal output.
// Paragraphs and list items are word-wrapped to width. Code blocks are
// rendered at full width without reflow.
func Render(source string, width int, theme pipe.Theme) string {
	r := newRenderer(theme)
	return r.render([]byte(source), width)
}
```

`markdown/renderer.go` — goldmark AST walker producing styled output:

- Use `goldmark.New()` to parse source into AST
- Custom `renderer.NodeRenderer` implementation
- Per-node rendering:
  - `ast.Heading` → bold + accent color, no width wrapping
  - `ast.Paragraph` → `lipgloss.NewStyle().Width(width).Render(text)` for wrapping
  - `ast.FencedCodeBlock` → code background style, **NO word-wrapping** (preserve whitespace)
  - `ast.Emphasis` (level 1) → italic, (level 2) → bold
  - `ast.CodeSpan` → inline code background
  - `ast.List` / `ast.ListItem` → indented with `- ` or `N. ` markers, item text wrapped to `width - indent`
  - `ast.Link` → underlined text + dimmed `(URL)`

**Critical**: Wrapping happens per-block (paragraph, list item), NOT as a
document-level pass. Code blocks are never reflowed.

Reference: `jira4claude/markdown/to_markdown.go` for AST walking pattern.

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
	"strings"
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

	t.Run("wraps paragraphs to width", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("short words that keep going and going beyond thirty columns easily")
		view := block.View(30)
		assert.Contains(t, view, "easily")
	})

	t.Run("finalized paragraph stays while trailing text streams", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("first paragraph\n\n")
		block.Append("trailing")
		view := block.View(80)
		assert.Contains(t, view, "first paragraph")
		assert.Contains(t, view, "trailing")
	})

	t.Run("width change re-renders cached finalized content", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("word1 word2 word3 word4 word5 word6\n\ntail")
		narrow := block.View(20)
		wide := block.View(80)
		assert.NotEqual(t, strings.Count(narrow, "\n"), strings.Count(wide, "\n"))
	})

	t.Run("unclosed fenced code block renders safely", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("```go\nfmt.Println(\"x\")")
		view := block.View(80)
		assert.Contains(t, view, "fmt.Println")
	})
}
```

**Step 2: Run test to verify it fails**

**Step 3: Implement AssistantTextBlock with streaming optimization**

```go
package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/markdown"
)

var _ MessageBlock = (*AssistantTextBlock)(nil)

type AssistantTextBlock struct {
	content strings.Builder
	theme   pipe.Theme
	styles  Styles

	// finalizedRaw is the stable prefix ending at the last double newline.
	// It's rendered once per width and cached in finalizedByWidth.
	finalizedRaw     string
	finalizedByWidth map[int]string
}

func NewAssistantTextBlock(theme pipe.Theme, styles Styles) *AssistantTextBlock {
	return &AssistantTextBlock{
		theme:           theme,
		styles:          styles,
		finalizedByWidth: make(map[int]string),
	}
}

func (b *AssistantTextBlock) Append(text string) {
	b.content.WriteString(text)
	b.promoteFinalized()
}

func (b *AssistantTextBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *AssistantTextBlock) View(width int) string {
	finalizedRendered := b.renderFinalized(width)
	trailing := b.trailingRaw()
	if hasUnclosedFence(trailing) {
		// Close fence only for rendering so partial streams display safely.
		trailing += "\n```"
	}
	trailingRendered := markdown.Render(trailing, width, b.theme)
	switch {
	case finalizedRendered == "":
		return trailingRendered
	case trailingRendered == "":
		return finalizedRendered
	default:
		return finalizedRendered + "\n\n" + trailingRendered
	}
}

func (b *AssistantTextBlock) promoteFinalized() {
	raw := b.content.String()
	idx := strings.LastIndex(raw, "\n\n")
	if idx <= 0 {
		return
	}
	next := raw[:idx]
	if next != b.finalizedRaw {
		b.finalizedRaw = next
		// Width-sensitive cache must be invalidated when finalized text grows.
		clear(b.finalizedByWidth)
	}
}

func (b *AssistantTextBlock) renderFinalized(width int) string {
	if width <= 0 || b.finalizedRaw == "" {
		return ""
	}
	if cached, ok := b.finalizedByWidth[width]; ok {
		return cached
	}
	rendered := markdown.Render(b.finalizedRaw, width, b.theme)
	b.finalizedByWidth[width] = rendered
	return rendered
}

func (b *AssistantTextBlock) trailingRaw() string {
	raw := b.content.String()
	if b.finalizedRaw == "" {
		return raw
	}
	prefix := b.finalizedRaw + "\n\n"
	return strings.TrimPrefix(raw, prefix)
}

func hasUnclosedFence(s string) bool {
	return strings.Count(s, "```")%2 == 1
}
```

McGugan-style block finalization is included in scope for this task:
- finalized paragraphs (up to the last `\n\n`) are cached
- only trailing unfinalized text is re-rendered per delta
- cache is width-aware (`map[width]rendered`) to avoid stale wraps on resize
- trailing unclosed fences are auto-closed for rendering safety

**Step 4: Run test, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/block_assistant.go bubbletea/block_assistant_test.go
git commit -m "Add AssistantTextBlock with markdown rendering and streaming cache"
```

---

### Task 10: Forked Textarea

**Files:**
- Create: `bubbletea/textarea/textarea.go`
- Create: `bubbletea/textarea/wrap.go`
- Test: `bubbletea/textarea/textarea_test.go`

Port bubbles textarea, strip it, fix it.

**Source:** `~/go/pkg/mod/github.com/charmbracelet/bubbles@v1.0.0/textarea/textarea.go`

**Strip:**
- `ShowLineNumbers` and all line number rendering
- `Prompt` variations (always empty)
- `FocusedStyle` / `BlurredStyle` (single style)
- Placeholder animation
- The existing Styles struct

**Keep:**
- `[][]rune` text storage
- Cursor positioning (`row`, `col`, `lastCharOffset`)
- Word-wrap logic (`wrap()` → `wrap.go`)
- `SetWidth()` / `SetHeight()` / `MaxHeight`
- Key handling (arrows, backspace, delete, home/end, word movement)
- Viewport scrolling
- `Value()` / `SetValue()` / `Reset()`
- `Focus()` / `Blur()`

**Fix:**
- **Cache invalidation**: In `SetWidth()`, after updating `m.width`, reset
  the memoization cache. This is the root cause of the bubbles textarea bug.

**Add:**
- `CheckInputComplete func(value string) bool` — callback. When set, Enter
  calls this. If false, inserts newline. If true (or nil), normal Enter behavior.
- Auto-grow: When content changes, compute total visible lines. Set visible
  height to `min(totalLines, MaxHeight)`. Return an `InputHeightMsg{Height int}`
  command so the root model can recompute viewport layout.
- Newline insertion via Ctrl+J (portable across all terminals, maps to `\n`).

**Step 1: Write the failing tests**

```go
package textarea_test

import (
	"strings"
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
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
		assert.Equal(t, "hi", ta.Value())
	})

	t.Run("enter inserts newline", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		ta.SetValue("line1")
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyEnter})
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
		assert.Equal(t, "line1\n2", ta.Value())
	})

	t.Run("ctrl+j inserts newline", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		ta.SetValue("line1")
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyCtrlJ})
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
		assert.Equal(t, "line1\n2", ta.Value())
	})

	t.Run("backspace deletes character", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		ta.SetValue("abc")
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyEnd})
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyBackspace})
		assert.Equal(t, "ab", ta.Value())
	})
}

func TestTextarea_CheckInputComplete(t *testing.T) {
	t.Parallel()

	t.Run("enter inserts newline when CheckInputComplete returns false", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		ta.CheckInputComplete = func(value string) bool { return false }
		ta.SetValue("incomplete")
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyEnter})
		// Newline inserted, not submitted.
		assert.Equal(t, "incomplete\n", ta.Value())
	})

	t.Run("enter submits when CheckInputComplete returns true", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		ta.CheckInputComplete = func(value string) bool { return true }
		ta.SetValue("complete")
		ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyEnter})
		// Should NOT insert newline — the root model handles submission.
		assert.Equal(t, "complete", ta.Value())
	})
}

func TestTextarea_AutoGrow(t *testing.T) {
	t.Parallel()

	t.Run("height increases with content and emits InputHeightMsg", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.SetHeight(1)
		ta.MaxHeight = 3
		ta.Focus()

		// Type first line — no height change expected.
		for _, r := range "line1" {
			ta = applyKey(t, ta, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}

		// Insert newline — triggers auto-grow from height 1 to 2.
		updated, cmd := ta.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
		ta = updated.(textarea.Model)

		// Must emit InputHeightMsg with new height.
		require.NotNil(t, cmd)
		msg := cmd()
		heightMsg, ok := msg.(textarea.InputHeightMsg)
		require.True(t, ok, "expected InputHeightMsg, got %T", msg)
		assert.Equal(t, 2, heightMsg.Height)

		// Both lines visible.
		view := ta.View()
		assert.Contains(t, view, "line1")
	})

	t.Run("does not exceed MaxHeight", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.SetHeight(1)
		ta.MaxHeight = 2
		ta.Focus()
		ta.SetValue("line1\nline2\nline3")
		// View should show content but height capped at MaxHeight.
		view := ta.View()
		lines := strings.Split(view, "\n")
		assert.LessOrEqual(t, len(lines), 2)
	})
}

func TestTextarea_SetWidth_InvalidatesCache(t *testing.T) {
	t.Parallel()

	ta := textarea.New()
	ta.SetWidth(80)
	ta.Focus()
	ta.SetValue("a long line that should wrap at different widths depending on the setting")

	view80 := ta.View()

	ta.SetWidth(20)
	view20 := ta.View()

	require.NotEqual(t, view80, view20)
}

func applyKey(t *testing.T, ta textarea.Model, msg tea.KeyMsg) textarea.Model {
	t.Helper()
	updated, _ := ta.Update(msg)
	model, ok := updated.(textarea.Model)
	require.True(t, ok)
	return model
}
```

**Step 2: Run tests to verify they fail**

**Step 3: Implement the forked textarea**

Port from bubbles source, apply changes above. Target ~600-800 LOC.

The auto-grow feature returns an `InputHeightMsg` command when height changes:

```go
// InputHeightMsg is sent when the textarea's visible height changes due to
// auto-grow. The root model uses this to recompute viewport layout.
type InputHeightMsg struct {
	Height int
}
```

**Step 4: Run tests, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/textarea/
git commit -m "Add forked textarea with cache fix, auto-grow, and Ctrl+J newline"
```

---

### Task 11: Refactor Root Model

**Files:**
- Modify: `bubbletea/model.go`
- Modify: `bubbletea/bubbletea_test.go`
- Modify: `bubbletea/model_test.go`
- Modify: `cmd/pipe/main.go` (wiring for `make validate`)

Replace the monolithic strings.Builder with `[]MessageBlock` and wire all components.

**Step 1: Write failing tests for block assembly with correct event correlation**

Update test helpers in `bubbletea/bubbletea_test.go`:

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

func initModelWithSize(t *testing.T, run bt.AgentFunc, width, height int) bt.Model {
	t.Helper()
	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(run, session, theme)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	model, ok := updated.(bt.Model)
	require.True(t, ok)
	return model
}
```

New tests for block assembly and event correlation:

```go
func TestModel_BlockAssembly(t *testing.T) {
	t.Parallel()

	t.Run("text deltas with same index append to same block", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "hello "}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "world"}})
		assert.Contains(t, m.View(), "hello world")
	})

	t.Run("text deltas with different index create separate blocks", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "first"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 1, Delta: "second"}})
		view := m.View()
		assert.Contains(t, view, "first")
		assert.Contains(t, view, "second")
	})

	t.Run("thinking then text creates two blocks", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventThinkingDelta{Index: 0, Delta: "hmm"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "answer"}})
		assert.Contains(t, m.View(), "answer")
		// Thinking is collapsed so "hmm" is not visible.
		assert.NotContains(t, m.View(), "hmm")
	})

	t.Run("tool call correlated by ID", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallDelta{ID: "tc-1", Delta: `{"path":"/tmp"}`}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{
			ID: "tc-1", Name: "read", Arguments: json.RawMessage(`{"path":"/tmp"}`),
		}}})
		assert.Contains(t, m.View(), "read")
	})

	t.Run("interleaved tool calls stay separate", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-2", Name: "bash"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallDelta{ID: "tc-1", Delta: "args1"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallDelta{ID: "tc-2", Delta: "args2"}})
		view := m.View()
		assert.Contains(t, view, "read")
		assert.Contains(t, view, "bash")
	})

	t.Run("submit creates user block", func(t *testing.T) {
		t.Parallel()
		session := &pipe.Session{}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)
		m = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
		m.Input.SetValue("hi")
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		assert.Contains(t, m.View(), "hi")
	})
}

func TestModel_BlockToggle(t *testing.T) {
	t.Parallel()

	t.Run("tab toggles focused collapsible block", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		// Create a thinking block (starts collapsed).
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventThinkingDelta{Index: 0, Delta: "thoughts"}})
		assert.NotContains(t, m.View(), "thoughts")
		// Send Tab to toggle the focused block.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
		assert.Contains(t, m.View(), "thoughts")
	})
}

func TestModel_MultiTurnReset(t *testing.T) {
	t.Parallel()

	t.Run("second turn text index 0 creates new block", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		// Turn 1: text at index 0.
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "turn1"}})
		// Tool call ends turn 1.
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "read"}}})
		// Turn 2: text at index 0 again — must create a NEW block, not append to turn 1's.
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "turn2"}})
		view := m.View()
		assert.Contains(t, view, "turn1")
		assert.Contains(t, view, "turn2")
	})
}

// updateModel is a test helper that sends a message and returns the updated Model.
func updateModel(t *testing.T, m bt.Model, msg tea.Msg) bt.Model {
	t.Helper()
	updated, _ := m.Update(msg)
	model, ok := updated.(bt.Model)
	require.True(t, ok)
	return model
}
```

**Step 2: Run tests to verify they fail**

**Step 3: Refactor model.go**

Key changes:

**Model struct** — remove `output *strings.Builder`, add blocks and theme:

```go
type Model struct {
	Input    textinput.Model  // Replaced with forked textarea in Task 12
	Viewport viewport.Model

	run     AgentFunc
	session *pipe.Session
	theme   pipe.Theme
	styles  Styles

	blocks     []MessageBlock
	blockFocus int // index of focused block for toggle (-1 = none)
	// Map of active block indices/IDs for event correlation.
	activeText     map[int]*AssistantTextBlock  // keyed by EventTextDelta.Index
	activeThinking map[int]*ThinkingBlock       // keyed by EventThinkingDelta.Index
	activeToolCall map[string]*ToolCallBlock    // keyed by EventToolCall*.ID
	hadToolCalls   bool                         // true after EventToolCallBegin, reset on new turn

	running bool
	cancel  context.CancelFunc
	eventCh chan pipe.Event
	doneCh  chan error
	err     error
	ready   bool
}
```

**New() signature** — takes theme:

```go
func New(run AgentFunc, session *pipe.Session, theme pipe.Theme) Model
```

**processEvent()** — uses Index/ID for correlation:

Active text and thinking maps are per-turn. The loop runs multiple turns per user
request (tool use → next assistant turn). Content indices restart at 0 each turn.

Turn boundary detection uses a `hadToolCalls bool` field on Model. Set to `true`
on `EventToolCallBegin`. When any `EventTextDelta` or `EventThinkingDelta`
arrives with `hadToolCalls == true`, a new turn has started — clear the text and
thinking maps, reset the flag.

**Ordering assumption:** Within a single assistant message, content ordering is
thinking → text → tool calls. Both Anthropic and Gemini enforce this — tool use
blocks are always last. If a future provider emits text/thinking after tool calls
within the same message, this logic would incorrectly treat it as a new turn.
The assumption is documented in code comments so it's easy to find if this
breaks. The tool call map is never cleared — IDs are globally unique.

```go
func (m *Model) processEvent(evt pipe.Event) {
	switch e := evt.(type) {
	case pipe.EventTextDelta:
		// Detect new turn: text after tool calls means new assistant message.
		if m.hadToolCalls {
			m.activeText = make(map[int]*AssistantTextBlock)
			m.activeThinking = make(map[int]*ThinkingBlock)
			m.hadToolCalls = false
		}
		if b, ok := m.activeText[e.Index]; ok {
			b.Append(e.Delta)
		} else {
			b := NewAssistantTextBlock(m.theme, m.styles)
			b.Append(e.Delta)
			m.blocks = append(m.blocks, b)
			m.activeText[e.Index] = b
		}
	case pipe.EventThinkingDelta:
		// Detect new turn: thinking after tool calls means new assistant message.
		if m.hadToolCalls {
			m.activeText = make(map[int]*AssistantTextBlock)
			m.activeThinking = make(map[int]*ThinkingBlock)
			m.hadToolCalls = false
		}
		if b, ok := m.activeThinking[e.Index]; ok {
			b.Append(e.Delta)
		} else {
			b := NewThinkingBlock(m.styles)
			b.Append(e.Delta)
			m.blocks = append(m.blocks, b)
			m.activeThinking[e.Index] = b
		}
	case pipe.EventToolCallBegin:
		m.hadToolCalls = true
		b := NewToolCallBlock(e.Name, e.ID, m.styles)
		m.blocks = append(m.blocks, b)
		m.activeToolCall[e.ID] = b
	case pipe.EventToolCallDelta:
		if b, ok := m.activeToolCall[e.ID]; ok {
			b.AppendArgs(e.Delta)
		}
	case pipe.EventToolCallEnd:
		if b, ok := m.activeToolCall[e.Call.ID]; ok {
			b.FinalizeWithCall(e.Call)
		}
	}
}
```

**handleKey()** — add Tab for block toggle:

```go
case tea.KeyTab:
	if !m.running && len(m.blocks) > 0 && m.blockFocus >= 0 {
		block, cmd := m.blocks[m.blockFocus].Update(ToggleMsg{})
		m.blocks[m.blockFocus] = block
		m.Viewport.SetContent(m.renderContent())
		return m, cmd
	}
```

**Block focus rules** (deterministic, no user navigation for MVP):
- `blockFocus` is updated every time a block is appended to `m.blocks`
- After append: scan backwards from the end to find the last collapsible block
  (ThinkingBlock or ToolCallBlock). Set `blockFocus` to that index, or -1 if none.
- On `AgentDoneMsg`: recalculate `blockFocus` (same backwards scan).
- Tab only operates when `blockFocus >= 0`.
- Focus navigation (up/down arrows to move between collapsible blocks) is deferred
  to a follow-up.

**handleWindowSize()** — unchanged height calculation.

**submitInput()** — creates UserMessageBlock, resets active maps:

```go
m.blocks = append(m.blocks, NewUserMessageBlock(text, m.styles))
m.activeText = make(map[int]*AssistantTextBlock)
m.activeThinking = make(map[int]*ThinkingBlock)
m.activeToolCall = make(map[string]*ToolCallBlock)
m.hadToolCalls = false
```

**renderSession()** — creates blocks from session messages, including ToolResultMessage:

```go
case pipe.ToolResultMessage:
	content := ""
	for _, b := range msg.Content {
		if tb, ok := b.(pipe.TextBlock); ok {
			content += tb.Text
		}
	}
	m.blocks = append(m.blocks, NewToolResultBlock(msg.ToolName, content, msg.IsError, m.styles))
```

**statusLine()** — uses themed styles.

**Step 4: Update all existing tests**

- All `bt.New(agent, session)` → `bt.New(agent, session, pipe.DefaultTheme())`
- Remove `m.Viewport.Width` / `m.Viewport.Height` direct access — use `View()` assertions
- Remove `BlockCount()` — tests use `View()` content assertions instead
- Update `SetRunning` / `SetRunningWithCancel` for new struct (active maps initialized)

**Step 5: Update cmd/pipe/main.go wiring**

`make validate` includes building `cmd/pipe/`, so the wiring must update here
to keep the build green. Update the `New()` call:

```go
theme := pipe.DefaultTheme()
tuiModel := bt.New(agentFn, &session, theme)
```

**Step 6: Run all tests, then `make validate`**

**Step 7: Commit**

```bash
git add bubbletea/model.go bubbletea/bubbletea_test.go bubbletea/model_test.go cmd/pipe/main.go
git commit -m "Refactor root model to tree-of-models with Index/ID event correlation"
```

---

### Task 12: Swap Input to Forked Textarea

**Files:**
- Modify: `bubbletea/model.go`

**Step 1: Write a failing test for multi-line input**

```go
t.Run("ctrl+j inserts newline in input", func(t *testing.T) {
	t.Parallel()

	m := initModel(t, nopAgent)
	require.False(t, m.Running())

	m.Input.SetValue("line1")
	// Ctrl+J is the portable newline key.
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyCtrlJ})
	m.Input.SetValue(m.Input.Value() + "line2")

	assert.Contains(t, m.Input.Value(), "\n")
	assert.Contains(t, m.Input.Value(), "line1")
	assert.Contains(t, m.Input.Value(), "line2")
})

t.Run("enter sends message not newline", func(t *testing.T) {
	t.Parallel()

	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(nopAgent, session, theme)
	m = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Input.SetValue("hello")
	m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// Input should be cleared (message was sent).
	assert.Empty(t, m.Input.Value())
	assert.True(t, m.Running())
})
```

**Step 2: Run test to verify it fails**

**Step 3: Swap the input component**

In `model.go`:
- Replace `"github.com/charmbracelet/bubbles/textinput"` with
  `"github.com/fwojciec/pipe/bubbletea/textarea"`
- `Input textinput.Model` → `Input textarea.Model`
- `New()`: `textarea.New()` setup with `MaxHeight = 3`, `SetWidth()`
- `Init()`: return appropriate blink command or nil
- `handleKey()`: Enter calls `Input.Value()`, trims, submits. The textarea's
  `CheckInputComplete` is set to always return true (Enter sends).
- `handleWindowSize()`: use `Input.SetWidth(msg.Width)`
- Handle `textarea.InputHeightMsg`: recompute viewport height when input grows/shrinks

```go
case textarea.InputHeightMsg:
	inputH := msg.Height
	statusHeight := 1
	borderHeight := 2
	vpHeight := m.windowHeight - inputH - statusHeight - borderHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.Viewport.Height = vpHeight
	return m, nil
```

Store `windowHeight int` in Model for recomputation.

**Step 4: Run all tests, then `make validate`**

**Step 5: Commit**

```bash
git add bubbletea/model.go
git commit -m "Swap input to forked textarea for multi-line support"
```

---

### Task 13: Manual Smoke Test

**Files:**
- None (wiring was done in Task 11)

**Step 1: Manual smoke test**

```bash
go run ./cmd/pipe/
```

Verify:
- App starts, shows input
- Can type and submit a message (Enter sends)
- Ctrl+J inserts newline in input
- Agent responses render with markdown styling
- Thinking blocks show collapsed with `▶ Thinking`
- Tool calls show collapsed with tool name
- Tab toggles collapsible block
- Ctrl+C quits when idle, cancels when running
- Input auto-grows with multi-line content
- Viewport adjusts when input height changes

No commit needed — this is verification only.

---

### Task 14: Migrate Integration Tests to teatest

**Files:**
- Modify: `bubbletea/model_test.go`
- Modify: `bubbletea/bubbletea_test.go`

Unit tests (individual blocks) keep direct Update/View testing — it's simpler and
sufficient. Integration tests (root model lifecycle, full agent cycle) migrate to
teatest for async behavior and full-render verification. This aligns with the
design doc's "all tests use teatest" by making teatest the standard for
integration-level testing.

**Step 1: Add teatest dependency**

```bash
go get github.com/charmbracelet/x/exp/teatest
```

**Step 2: Add deterministic renderer helper**

```go
func trueColorRenderer() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	return r
}
```

Note: Requires model to accept renderer option via `WithRenderer()`. Defer
if architecturally unjustified at this stage.

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

	tm.Type("hi")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Hello!"))
	})

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
                     │                      ├── Task 7b (ToolResult)
                     │                      └── Task 9 (Assistant) ── requires Task 8
                     │
                     ├── Task 8 (Markdown) ─── independent, needs goldmark
                     │
                     └── Task 10 (Textarea) ── independent

Task 11 (Root Refactor) ── requires Tasks 1-9, 7b
Task 12 (Input Swap) ── requires Tasks 10, 11
Task 13 (Smoke Test) ── requires Task 12
Task 14 (teatest) ── requires Task 11
```

Tasks 4-7b can be done in any order (all depend on Tasks 1-3).
Task 8 can be done in parallel with Tasks 4-7b.
Task 10 can be done in parallel with everything before Task 11.

## Review Findings Addressed

1. **Event-to-block by Index/ID** — processEvent uses `activeText[Index]`, `activeThinking[Index]`, `activeToolCall[ID]` maps instead of last-block-type.
2. **EventToolCallEnd payload** — `FinalizeWithCall(e.Call)` applies arguments from the end event, handling Gemini's begin+end pattern.
3. **Auto-grow layout recomputation** — textarea emits `InputHeightMsg`, root model recomputes viewport height using stored `windowHeight`.
4. **Interactive collapse** — `ToggleMsg` in block interface, Tab key triggers toggle on focused block in root model.
5. **Portable newline key** — Ctrl+J (maps to `\n` in all terminals). No Shift+Enter.
6. **Tool results** — Task 7b adds `ToolResultBlock`, `renderSession()` creates them from `ToolResultMessage`.
7. **Task 12 test complete** — full executable tests for Ctrl+J newline and Enter-sends.
8. **Missing test coverage** — CheckInputComplete, auto-grow, and markdown wrapping tests added.
9. **BlockCount() removed** — tests use View() content assertions.
10. **Code blocks not reflowed** — wrapping is per-block (paragraph, list item), code blocks preserve whitespace.
11. **File guidance corrected** — constructor changes in model.go (Task 11), not bubbletea.go.
12. **Local paths removed** — plan uses relative paths and `go run ./cmd/pipe/`.
