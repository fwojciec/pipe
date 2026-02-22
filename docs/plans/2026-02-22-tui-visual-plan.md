# TUI Visual Design Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement tinted-block visual system, dense status bar with separators, ASCII welcome screen, and collapsible tool results for the pipe TUI.

**Architecture:** Extend Theme with background color fields, add background styles, update each block's View method to apply tints, restructure status bar with cwd/model info and box-drawing separators, add FIGlet welcome screen. ToolResultBlock becomes collapsible.

**Tech Stack:** Go, Bubble Tea, Lipgloss, ANSI colors

**Design doc:** `docs/plans/2026-02-22-tui-visual-design.md`

---

### Task 1: Extend Theme with background color fields

**Files:**
- Modify: `theme.go`
- Test: `bubbletea/styles_test.go`

**Step 1: Write failing test**

Add test in `bubbletea/styles_test.go` that verifies the new theme fields exist and DefaultTheme returns sensible values:

```go
func TestDefaultTheme_BackgroundFields(t *testing.T) {
	t.Parallel()
	theme := pipe.DefaultTheme()
	// Background fields should have valid ANSI indices (>= 0).
	assert.GreaterOrEqual(t, theme.UserBg, 0)
	assert.GreaterOrEqual(t, theme.ToolCallBg, 0)
	assert.GreaterOrEqual(t, theme.ToolResultBg, 0)
	assert.GreaterOrEqual(t, theme.ErrorBg, 0)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./bubbletea/ -run TestDefaultTheme_BackgroundFields -v`
Expected: FAIL — `theme.UserBg` field doesn't exist

**Step 3: Implement**

In `theme.go`, add fields to Theme and DefaultTheme:

```go
type Theme struct {
	UserMsg  int
	Thinking int
	ToolCall int
	Error    int
	Success  int
	Muted    int
	CodeBg   int
	Accent   int
	// Block background colors.
	UserBg       int
	ToolCallBg   int
	ToolResultBg int
	ErrorBg      int
}

func DefaultTheme() Theme {
	return Theme{
		UserMsg:      4,
		Thinking:     8,
		ToolCall:     3,
		Error:        1,
		Success:      2,
		Muted:        8,
		CodeBg:       0,
		Accent:       5,
		UserBg:       4,
		ToolCallBg:   3,
		ToolResultBg: 8,
		ErrorBg:      1,
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./bubbletea/ -run TestDefaultTheme_BackgroundFields -v`
Expected: PASS

**Step 5: Commit**

```bash
git add theme.go bubbletea/styles_test.go
git commit -m "feat: add background color fields to Theme"
```

---

### Task 2: Add background styles to Styles

**Files:**
- Modify: `bubbletea/styles.go`
- Modify: `bubbletea/styles_test.go`

**Step 1: Write failing test**

```go
func TestNewStyles_BackgroundStyles(t *testing.T) {
	t.Parallel()
	theme := pipe.DefaultTheme()
	styles := bt.NewStyles(theme)
	plain := "test"
	// Background styles should produce output longer than plain text
	// (ANSI escape codes for background color are injected).
	assert.Greater(t, len(styles.UserBg.Render(plain)), len(plain))
	assert.Greater(t, len(styles.ToolCallBg.Render(plain)), len(plain))
	assert.Greater(t, len(styles.ToolResultBg.Render(plain)), len(plain))
	assert.Greater(t, len(styles.ErrorBg.Render(plain)), len(plain))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./bubbletea/ -run TestNewStyles_BackgroundStyles -v`
Expected: FAIL — `styles.UserBg` doesn't exist

**Step 3: Implement**

In `bubbletea/styles.go`:

```go
type Styles struct {
	UserMsg      lipgloss.Style
	Thinking     lipgloss.Style
	ToolCall     lipgloss.Style
	Error        lipgloss.Style
	Success      lipgloss.Style
	Muted        lipgloss.Style
	Accent       lipgloss.Style
	CodeBg       lipgloss.Style
	UserBg       lipgloss.Style
	ToolCallBg   lipgloss.Style
	ToolResultBg lipgloss.Style
	ErrorBg      lipgloss.Style
}

func NewStyles(t pipe.Theme) Styles {
	return Styles{
		UserMsg:      lipgloss.NewStyle().Foreground(ansiColor(t.UserMsg)).Bold(true),
		Thinking:     lipgloss.NewStyle().Foreground(ansiColor(t.Thinking)).Faint(true),
		ToolCall:     lipgloss.NewStyle().Foreground(ansiColor(t.ToolCall)),
		Error:        lipgloss.NewStyle().Foreground(ansiColor(t.Error)),
		Success:      lipgloss.NewStyle().Foreground(ansiColor(t.Success)),
		Muted:        lipgloss.NewStyle().Foreground(ansiColor(t.Muted)).Faint(true),
		Accent:       lipgloss.NewStyle().Foreground(ansiColor(t.Accent)).Bold(true),
		CodeBg:       lipgloss.NewStyle().Background(ansiColor(t.CodeBg)),
		UserBg:       lipgloss.NewStyle().Background(ansiColor(t.UserBg)),
		ToolCallBg:   lipgloss.NewStyle().Background(ansiColor(t.ToolCallBg)),
		ToolResultBg: lipgloss.NewStyle().Background(ansiColor(t.ToolResultBg)),
		ErrorBg:      lipgloss.NewStyle().Background(ansiColor(t.ErrorBg)),
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./bubbletea/ -run TestNewStyles_BackgroundStyles -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `make validate`
Expected: All pass (additive change, no breakage)

**Step 6: Commit**

```bash
git add bubbletea/styles.go bubbletea/styles_test.go
git commit -m "feat: add background styles to Styles"
```

---

### Task 3: UserMessageBlock — background tint, remove prefix

**Files:**
- Modify: `bubbletea/block_user.go`
- Modify: `bubbletea/block_user_test.go`

**Step 1: Update test expectations for new behavior**

Replace `TestUserMessageBlock_View` in `bubbletea/block_user_test.go`:

```go
func TestUserMessageBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders text without prefix", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewUserMessageBlock("hello world", styles)
		view := block.View(80)
		// Text is present. No "> " prefix — background tint is the visual indicator.
		assert.Contains(t, view, "hello world")
	})

	t.Run("wraps long text to width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		longText := "short words that keep going and going beyond the viewport width easily"
		block := bt.NewUserMessageBlock(longText, styles)
		view := block.View(30)
		assert.Contains(t, view, "easily")
		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1)
	})
}
```

**Step 2: Run test to verify current behavior fails the new test**

Run: `go test ./bubbletea/ -run TestUserMessageBlock_View -v`
Expected: Tests still pass (we only removed the `">"` assertion which was a subset check)

**Step 3: Implement background tint rendering**

Update `bubbletea/block_user.go`:

```go
func (b *UserMessageBlock) View(width int) string {
	content := " " + b.styles.UserMsg.Render(b.text)
	wrapped := lipgloss.NewStyle().Width(width).Render(content)
	return b.styles.UserBg.Width(width).Render(wrapped)
}
```

The `" "` prefix adds 1-space left padding. `UserMsg` applies bold + foreground
color to the text. `UserBg` wraps the whole block with background tint.

**Step 4: Run test to verify it passes**

Run: `go test ./bubbletea/ -run TestUserMessageBlock_View -v`
Expected: PASS

**Step 5: Update model test that checks for ">" prefix**

In `bubbletea/model_test.go`, the `TestModel_BlockAssembly/"submit creates user block"` test checks `assert.Contains(t, m.View(), "hi")` — this still works. No changes needed there.

**Step 6: Run full test suite**

Run: `make validate`
Expected: All pass

**Step 7: Commit**

```bash
git add bubbletea/block_user.go bubbletea/block_user_test.go
git commit -m "feat: user message block with background tint"
```

---

### Task 4: ToolCallBlock — background tint

**Files:**
- Modify: `bubbletea/block_toolcall.go`
- Test: `bubbletea/block_toolcall_test.go` (existing tests should still pass)

**Step 1: Verify existing tests pass before changes**

Run: `go test ./bubbletea/ -run TestToolCallBlock_View -v`
Expected: PASS

**Step 2: Implement background tint**

Update `View` method in `bubbletea/block_toolcall.go`:

```go
func (b *ToolCallBlock) View(width int) string {
	indicator := "▶"
	if !b.collapsed {
		indicator = "▼"
	}
	header := b.styles.ToolCall.Render(indicator + " " + b.name)
	var content string
	if b.collapsed || b.args.Len() == 0 {
		content = header
	} else {
		content = header + "\n" + b.styles.Muted.Render(b.args.String())
	}
	wrapped := lipgloss.NewStyle().Width(width).Render(content)
	return b.styles.ToolCallBg.Width(width).Render(wrapped)
}
```

**Step 3: Run tests to verify no breakage**

Run: `go test ./bubbletea/ -run TestToolCallBlock_View -v`
Expected: PASS (tests check Contains, background wrapping doesn't affect content)

**Step 4: Run full test suite**

Run: `make validate`
Expected: All pass

**Step 5: Commit**

```bash
git add bubbletea/block_toolcall.go
git commit -m "feat: tool call block with background tint"
```

---

### Task 5: ToolResultBlock — collapsible with summary and background tint

This is the most complex task. ToolResultBlock becomes collapsible (like ThinkingBlock), shows a summary line when collapsed, and gets a background tint.

**Files:**
- Modify: `bubbletea/block_toolresult.go`
- Modify: `bubbletea/block_toolresult_test.go`
- Modify: `bubbletea/model.go` (updateBlockFocus, cycleFocusPrev)

**Step 1: Update tests for new collapsible behavior**

Replace `TestToolResultBlock_View` in `bubbletea/block_toolresult_test.go`:

```go
func TestToolResultBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("collapsed shows tool name and success indicator", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "file contents here", false, styles)
		view := block.View(80)
		assert.Contains(t, view, "read")
		assert.Contains(t, view, "✓")
		// Content is hidden when collapsed.
		assert.NotContains(t, view, "file contents here")
	})

	t.Run("collapsed shows first line as summary", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "hello world\nmore output", false, styles)
		view := block.View(80)
		assert.Contains(t, view, "hello world")
		assert.NotContains(t, view, "more output")
	})

	t.Run("error result expands automatically", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "command failed", true, styles)
		view := block.View(80)
		assert.Contains(t, view, "bash")
		assert.Contains(t, view, "✗")
		assert.Contains(t, view, "command failed")
	})

	t.Run("toggle expands to show full content", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "file contents here", false, styles)
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolResultBlock).View(80)
		assert.Contains(t, view, "file contents here")
	})

	t.Run("empty content renders header only", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "", false, styles)
		view := block.View(80)
		assert.Contains(t, view, "read")
		assert.Contains(t, view, "✓")
	})

	t.Run("long result wraps to width when expanded", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		long := "this is a very long result that should wrap properly within the viewport"
		block := bt.NewToolResultBlock("read", long, false, styles)
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolResultBlock).View(30)
		assert.Contains(t, view, "viewport")
		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 2)
	})
}
```

**Step 2: Run tests to verify failure**

Run: `go test ./bubbletea/ -run TestToolResultBlock_View -v`
Expected: FAIL — collapsed state doesn't exist yet, "✓" not rendered

**Step 3: Implement collapsible ToolResultBlock**

Replace `bubbletea/block_toolresult.go`:

```go
package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ MessageBlock = (*ToolResultBlock)(nil)

// ToolResultBlock renders the result of a tool execution.
// Collapsed by default; error results expand automatically.
type ToolResultBlock struct {
	toolName  string
	content   string
	isError   bool
	collapsed bool
	styles    Styles
}

// NewToolResultBlock creates a ToolResultBlock. Error results start expanded;
// success results start collapsed with a summary line.
func NewToolResultBlock(toolName, content string, isError bool, styles Styles) *ToolResultBlock {
	return &ToolResultBlock{
		toolName:  toolName,
		content:   content,
		isError:   isError,
		collapsed: !isError,
		styles:    styles,
	}
}

func (b *ToolResultBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	if _, ok := msg.(ToggleMsg); ok {
		b.collapsed = !b.collapsed
	}
	return b, nil
}

func (b *ToolResultBlock) View(width int) string {
	var content string
	if b.collapsed || b.content == "" {
		// Collapsed: tool name + ✓/✗ + first-line preview.
		content = b.headerLine(true)
	} else {
		// Expanded: tool name + ✓/✗ (no preview), then full content.
		contentStyle := b.styles.Muted
		if b.isError {
			contentStyle = b.styles.Error
		}
		content = b.headerLine(false) + "\n" + contentStyle.Render(b.content)
	}
	wrapped := lipgloss.NewStyle().Width(width).Render(content)
	return b.styles.ToolResultBg.Width(width).Render(wrapped)
}

// headerLine renders the tool name and status indicator. When withPreview is
// true, the first line of content is appended as a summary (collapsed state).
func (b *ToolResultBlock) headerLine(withPreview bool) string {
	indicator := "✓"
	indicatorStyle := b.styles.Success
	if b.isError {
		indicator = "✗"
		indicatorStyle = b.styles.Error
	}
	header := b.styles.ToolCall.Render(b.toolName) + " " + indicatorStyle.Render(indicator)
	if withPreview {
		if first := firstLine(b.content); first != "" {
			header += "  " + b.styles.Muted.Render(first)
		}
	}
	return header
}

// firstLine returns the first non-empty line of s, truncated to 60 chars.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}
```

**Step 4: Run tests to verify pass**

Run: `go test ./bubbletea/ -run TestToolResultBlock_View -v`
Expected: PASS

**Step 5: Update focus cycling to include ToolResultBlock**

In `bubbletea/model.go`, update `updateBlockFocus` and `cycleFocusPrev` type switches to include `*ToolResultBlock`:

```go
// In updateBlockFocus:
case *ThinkingBlock, *ToolCallBlock, *ToolResultBlock:

// In cycleFocusPrev:
case *ThinkingBlock, *ToolCallBlock, *ToolResultBlock:
```

**Step 6: Update model tests that expect tool result content to be visible**

In `bubbletea/model_test.go`, `TestModel_ToolResultEvent/"EventToolResult creates ToolResultBlock during streaming"` asserts `Contains(m.View(), "file contents here")`. This will fail because tool results are now collapsed. Update to check for the summary instead:

```go
t.Run("EventToolResult creates ToolResultBlock during streaming", func(t *testing.T) {
	t.Parallel()
	m := initModel(t, nopAgent)
	m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
	m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "read"}}})
	m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "read", Content: "file contents here", IsError: false}})
	// Tool result is collapsed — summary visible, full content hidden.
	assert.Contains(t, m.View(), "read")
	assert.Contains(t, m.View(), "✓")
})
```

Error result test still passes because errors expand automatically:

```go
t.Run("EventToolResult with error shows error styling", func(t *testing.T) {
	t.Parallel()
	m := initModel(t, nopAgent)
	m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "bash"}})
	m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "bash"}}})
	m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "bash", Content: "command failed", IsError: true}})
	assert.Contains(t, m.View(), "command failed")
})
```

Also update the teatest that checks for "file contents here" in
`TestModel_Teatest/"existing session with tool result renders on init"`:

```go
teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
	return bytes.Contains(out, []byte("read")) &&
		bytes.Contains(out, []byte("✓"))
}, teatest.WithDuration(5*time.Second))
```

And the teatest `"tool result event appears during agent run"`:

```go
teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
	return bytes.Contains(out, []byte("bash")) &&
		bytes.Contains(out, []byte("Done!")) &&
		bytes.Contains(out, []byte("Enter to send"))
}, teatest.WithDuration(5*time.Second))
```

**Step 7: Run full test suite**

Run: `make validate`
Expected: All pass

**Step 8: Commit**

```bash
git add bubbletea/block_toolresult.go bubbletea/block_toolresult_test.go bubbletea/model.go bubbletea/model_test.go
git commit -m "feat: collapsible tool result block with summary and background tint"
```

---

### Task 6: ErrorBlock — background tint

**Files:**
- Modify: `bubbletea/block_error.go`
- Test: `bubbletea/block_error_test.go` (existing tests check Contains — should still pass)

**Step 1: Verify existing tests pass**

Run: `go test ./bubbletea/ -run TestErrorBlock -v`
Expected: PASS (or no test exists yet — check)

**Step 2: Implement background tint**

Update `bubbletea/block_error.go`:

```go
func (b *ErrorBlock) View(width int) string {
	content := b.styles.Error.Render(fmt.Sprintf("Error: %v", b.err))
	wrapped := lipgloss.NewStyle().Width(width).Render(content)
	return b.styles.ErrorBg.Width(width).Render(wrapped)
}
```

**Step 3: Run full test suite**

Run: `make validate`
Expected: All pass

**Step 4: Commit**

```bash
git add bubbletea/block_error.go
git commit -m "feat: error block with background tint"
```

---

### Task 7: Status bar with separators and model config

**Files:**
- Modify: `bubbletea/model.go` (Model struct, New, View, statusLine, viewportHeight)
- Modify: `bubbletea/model_test.go`
- Modify: `bubbletea/bubbletea_test.go` (helpers)
- Modify: `cmd/pipe/main.go`

**Step 1: Write test for new status bar format**

Add to `bubbletea/model_test.go`:

```go
func TestModel_StatusBar(t *testing.T) {
	t.Parallel()

	t.Run("shows working directory and model name", func(t *testing.T) {
		t.Parallel()
		session := &pipe.Session{}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme, bt.Config{
			WorkDir:   "~/code/pipe",
			GitBranch: "main",
			ModelName: "claude-opus-4-5",
		})
		m = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
		view := m.View()
		assert.Contains(t, view, "~/code/pipe")
		assert.Contains(t, view, "main")
		assert.Contains(t, view, "claude-opus-4-5")
	})

	t.Run("shows generating indicator when running", func(t *testing.T) {
		t.Parallel()
		session := &pipe.Session{}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme, bt.Config{
			WorkDir:   "~/code/pipe",
			ModelName: "claude-opus-4-5",
		})
		m = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
		m, _ = bt.SetRunning(m)
		view := m.View()
		assert.Contains(t, view, "●")
	})

	t.Run("separator lines present", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		view := m.View()
		assert.Contains(t, view, "─")
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./bubbletea/ -run TestModel_StatusBar -v`
Expected: FAIL — Config type doesn't exist, New signature wrong

**Step 3: Add Config type and update New**

In `bubbletea/model.go`:

```go
// Config holds optional display configuration for the TUI.
type Config struct {
	WorkDir   string
	GitBranch string
	ModelName string
}
```

Update Model struct to add config field:

```go
type Model struct {
	// ... existing fields ...
	config Config
}
```

Update `New` signature:

```go
func New(run AgentFunc, session *pipe.Session, theme pipe.Theme, cfg Config) Model {
	// ... existing code ...
	return Model{
		// ... existing fields ...
		config: cfg,
	}
}
```

**Step 4: Update View to include separators**

```go
func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder
	b.WriteString(m.Viewport.View())
	b.WriteString("\n")
	b.WriteString(m.separator())
	b.WriteString("\n")
	b.WriteString(m.statusLine())
	b.WriteString("\n")
	b.WriteString(m.separator())
	b.WriteString("\n")
	b.WriteString(m.Input.View())
	return b.String()
}

func (m Model) separator() string {
	return m.styles.Muted.Render(strings.Repeat("─", m.Viewport.Width))
}
```

**Step 5: Update viewportHeight**

```go
func (m Model) viewportHeight(inputH int) int {
	const statusHeight = 3 // separator + status text + separator
	h := m.windowHeight - inputH - statusHeight
	if h < 1 {
		h = 1
	}
	return h
}
```

Current: `windowHeight - inputH - 1 - 2 = windowHeight - inputH - 3`
New: `windowHeight - inputH - 3`
Same math — existing height assertions still pass.

**Step 6: Restructure statusLine**

```go
func (m Model) statusLine() string {
	if m.err != nil {
		content := m.styles.Error.Render(fmt.Sprintf("Error: %v", m.err))
		return lipgloss.NewStyle().Width(m.Viewport.Width).Render(content)
	}

	// Build left side: workdir (branch)
	left := m.config.WorkDir
	if m.config.GitBranch != "" {
		left += " (" + m.config.GitBranch + ")"
	}
	if m.running {
		left += " " + m.styles.Accent.Render("●")
	}

	// Build right side: model name
	right := m.config.ModelName

	// Pad to fill width.
	gap := m.Viewport.Width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return m.styles.Muted.Render(line)
}
```

**Step 7: Update all existing callers of New to pass Config**

In `bubbletea/bubbletea_test.go`, update helpers:

```go
func initModel(t *testing.T, run bt.AgentFunc) bt.Model {
	t.Helper()
	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(run, session, theme, bt.Config{})
	// ...
}

func initModelWithSize(t *testing.T, run bt.AgentFunc, width, height int) bt.Model {
	t.Helper()
	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(run, session, theme, bt.Config{})
	// ...
}
```

Update every `bt.New(...)` call in `model_test.go` to include `bt.Config{}` as the fourth argument. Search for `bt.New(` and add the arg.

In `cmd/pipe/main.go`, compute config:

```go
cwd, _ := os.Getwd()
home, _ := os.UserHomeDir()
if home != "" && strings.HasPrefix(cwd, home) {
	cwd = "~" + cwd[len(home):]
}
branch, _ := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()

tuiModel := bt.New(agentFn, &session, theme, bt.Config{
	WorkDir:   cwd,
	GitBranch: strings.TrimSpace(string(branch)),
	ModelName: modelID,
})
```

Add `"os/exec"` and `"strings"` to imports.

**Step 8: Update status-line-dependent test assertions**

The new status bar drops all keyboard hints ("Enter to send", "Alt+M to
release"). Status bar shows only: cwd, branch, activity indicator, model name.

Tests to update in `model_test.go`:
- `"status line shows mouse hint when enabled"` — delete this test. Mouse
  toggle still works, but the status bar no longer advertises it.

Tests to update in teatest section of `model_test.go`:

Teatest `WaitFor` scans cumulative terminal output. Once `●` appears in any
frame it stays in the buffer forever, so `!Contains("●")` is unreliable.

Instead, use `tm.FinalModel()` to check final state after the agent completes.
Wait only for expected content, then verify idle state on the final model:

- `"full agent cycle with event delivery"` — wait for "Hello!" only:

```go
teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
	return bytes.Contains(out, []byte("Hello!"))
}, teatest.WithDuration(5*time.Second))
```

The existing `tm.FinalModel` + `assert.False(t, final.Running())` already
verifies the agent finished. No need to detect idle via output scanning.

- Other teatests waiting for `"Enter to send"` — replace with content-only
  wait conditions (the specific content each test produces).

**Step 9: Run full test suite**

Run: `make validate`
Expected: All pass

**Step 10: Commit**

```bash
git add bubbletea/model.go bubbletea/model_test.go bubbletea/bubbletea_test.go cmd/pipe/main.go
git commit -m "feat: dense status bar with separators, cwd, and model name"
```

---

### Task 8: Welcome screen with ASCII art

**Files:**
- Modify: `bubbletea/model.go`
- Modify: `bubbletea/model_test.go`

**Step 1: Write test for welcome screen**

```go
func TestModel_WelcomeScreen(t *testing.T) {
	t.Parallel()

	t.Run("empty session shows welcome art", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		view := m.View()
		assert.Contains(t, view, "pipe")
		assert.Contains(t, view, "Ceci")
	})

	t.Run("welcome disappears after first message", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Delta: "hi"}})
		view := m.View()
		assert.NotContains(t, view, "Ceci")
	})
}
```

**Step 2: Run test to verify failure**

Run: `go test ./bubbletea/ -run TestModel_WelcomeScreen -v`
Expected: FAIL — no welcome art in view

**Step 3: Implement welcome screen**

Add to `bubbletea/model.go`:

```go
func (m Model) welcomeView() string {
	art := `         _
   _ __ (_)_ __   ___
  | '_ \| | '_ \ / _ \
  | |_) | | |_) |  __/
  | .__/|_| .__/ \___|
  |_|     |_|

  Ceci n'est pas une pipe.`

	styled := m.styles.Accent.Render(art)

	// Center vertically.
	artLines := strings.Count(styled, "\n") + 1
	topPad := (m.Viewport.Height - artLines) / 2
	if topPad < 0 {
		topPad = 0
	}

	// Center horizontally: find widest art line, compute left padding.
	maxWidth := 0
	for _, line := range strings.Split(art, "\n") {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}
	leftPad := (m.Viewport.Width - maxWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	padded := lipgloss.NewStyle().PaddingLeft(leftPad).Render(styled)

	return strings.Repeat("\n", topPad) + padded
}
```

Update `renderContent` or the viewport content setting. In `handleWindowSize`, when `m.ready` is first set and blocks are empty, set viewport content to welcome:

```go
// In handleWindowSize, after m.ready = true:
if len(m.blocks) == 0 {
	m.Viewport.SetContent(m.welcomeView())
} else {
	m.Viewport.SetContent(m.renderContent())
}
```

Also update `renderContent` to return welcome when blocks are empty:

```go
func (m Model) renderContent() string {
	if len(m.blocks) == 0 {
		return m.welcomeView()
	}
	// ... existing rendering ...
}
```

**Step 4: Run test to verify pass**

Run: `go test ./bubbletea/ -run TestModel_WelcomeScreen -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `make validate`
Expected: All pass

**Step 6: Commit**

```bash
git add bubbletea/model.go bubbletea/model_test.go
git commit -m "feat: ASCII art welcome screen for empty sessions"
```

---

### Task 9: Block spacing — tool cluster grouping

Currently `renderContent` puts `"\n"` between every block. Two changes:
1. Non-tool boundaries become `"\n\n"` (blank line for visual separation).
2. Adjacent tool blocks use `"\n"` (line break only — they cluster).

This handles multi-tool turns where the block order may be:
call(1), call(2), result(1), result(2).

**Files:**
- Modify: `bubbletea/model.go` (renderContent, blockSeparator, isToolBlock)
- Create: `bubbletea/export_test.go` (test-only exports, package bubbletea)
- Add test: `bubbletea/model_test.go`

**Step 1: Extract separator logic into a testable function**

The separator between blocks is a pure function of the two adjacent block
types. Extract it so it can be unit-tested directly without rendering.

Add to `bubbletea/model.go`:

```go
// blockSeparator returns the string inserted between adjacent blocks.
// Tool-related blocks cluster with a single line break; all other
// boundaries get a blank line.
func blockSeparator(prev, curr MessageBlock) string {
	if isToolBlock(prev) && isToolBlock(curr) {
		return "\n" // line break only — no blank line
	}
	return "\n\n" // blank line
}

// isToolBlock returns true for blocks that form tool clusters.
func isToolBlock(b MessageBlock) bool {
	switch b.(type) {
	case *ToolCallBlock, *ToolResultBlock:
		return true
	}
	return false
}
```

Expose for external test package via `bubbletea/export_test.go`:

```go
package bubbletea

// BlockSeparator exposes blockSeparator for testing.
func BlockSeparator(prev, curr MessageBlock) string {
	return blockSeparator(prev, curr)
}
```

**Step 2: Write tests for separator logic**

Test the pure function directly — no rendering, no fragile string counting:

```go
func TestBlockSeparator(t *testing.T) {
	t.Parallel()
	styles := bt.NewStyles(pipe.DefaultTheme())
	theme := pipe.DefaultTheme()

	tc := bt.NewToolCallBlock("bash", "tc-1", styles)
	tr := bt.NewToolResultBlock("bash", "ok", false, styles)
	txt := bt.NewAssistantTextBlock(theme)
	usr := bt.NewUserMessageBlock("hi", styles)

	t.Run("tool-to-tool is line break only", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n", bt.BlockSeparator(tc, tr))
		assert.Equal(t, "\n", bt.BlockSeparator(tr, tc))
		assert.Equal(t, "\n", bt.BlockSeparator(tc, tc))
		assert.Equal(t, "\n", bt.BlockSeparator(tr, tr))
	})

	t.Run("non-tool boundaries get blank line", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(txt, tc))
		assert.Equal(t, "\n\n", bt.BlockSeparator(tr, txt))
		assert.Equal(t, "\n\n", bt.BlockSeparator(usr, tc))
		assert.Equal(t, "\n\n", bt.BlockSeparator(txt, usr))
	})
}
```

**Step 3: Write integration test bridging separator logic to renderContent**

Expose `renderContent` via `export_test.go` and compare two scenarios:
tool-only blocks with multiple tools (no `\n\n`) vs mixed blocks (has `\n\n`).
Both use short, single-line content so internal `\n\n` from block renderers
is impossible. The tool-only test uses two adjacent tools to exercise the
tool→tool separator path through `renderContent`.

Add to `bubbletea/export_test.go`:

```go
// RenderContent exposes renderContent for testing block spacing integration.
func (m Model) RenderContent() string { return m.renderContent() }
```

```go
func TestModel_BlockSpacing(t *testing.T) {
	t.Parallel()

	t.Run("tool-only sequence has no blank lines", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "read"}}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "read", Content: "ok", IsError: false}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-2", Name: "bash"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-2", Name: "bash"}}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "bash", Content: "ok", IsError: false}})

		raw := m.RenderContent()
		// Content must be single-line and renderers must produce compact output
		// for this assertion to hold.
		assert.NotContains(t, raw, "\n\n",
			"tool cluster should have no blank lines, got:\n%s", raw)
	})

	t.Run("text-to-tool boundary has blank line", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Delta: "hi"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "read"}}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "read", Content: "ok", IsError: false}})

		raw := m.RenderContent()
		assert.Contains(t, raw, "\n\n",
			"text→tool boundary should have blank line, got:\n%s", raw)
	})
}
```

**Step 4: Implement renderContent with blockSeparator**

```go
func (m Model) renderContent() string {
	if len(m.blocks) == 0 {
		return m.welcomeView()
	}
	var b strings.Builder
	for i, block := range m.blocks {
		if i > 0 {
			b.WriteString(blockSeparator(m.blocks[i-1], block))
		}
		b.WriteString(block.View(m.Viewport.Width))
	}
	return b.String()
}
```

**Step 5: Run tests to verify pass**

Run: `go test ./bubbletea/ -run TestBlockSeparator -v`
Run: `go test ./bubbletea/ -run TestModel_BlockSpacing -v`
Expected: PASS

**Step 6: Run full test suite**

Run: `make validate`
Expected: All pass

**Step 7: Commit**

```bash
git add bubbletea/model.go bubbletea/export_test.go bubbletea/model_test.go
git commit -m "feat: tool cluster spacing with blank-line separators"
```

---

### Task 10: Final validation and visual review

**Step 1: Run full test suite**

Run: `make validate`
Expected: All pass

**Step 2: Manual visual check**

Run: `go run ./cmd/pipe/`
Expected: See welcome screen with ASCII art, type a message to see tinted blocks, check status bar rendering.

**Step 3: Commit any fixups**

If visual review reveals issues, fix and commit.
