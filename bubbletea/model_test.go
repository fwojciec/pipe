package bubbletea_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/fwojciec/pipe/bubbletea/textarea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	session := &pipe.Session{}
	theme := pipe.DefaultTheme()
	m := bt.New(nopAgent, session, theme)

	assert.False(t, m.Running())
	assert.NoError(t, m.Err())
}

func TestModel_Update(t *testing.T) {
	t.Parallel()

	t.Run("window size initializes viewport", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, ok := updated.(bt.Model)
		require.True(t, ok)

		// View should render without error after initialization.
		view := model.View()
		assert.NotEmpty(t, view)
	})

	t.Run("window size resize updates viewport dimensions", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)

		// Verify initial dimensions differ from resize target.
		assert.Equal(t, 80, m.Viewport.Width)
		assert.Equal(t, 20, m.Viewport.Height) // 24 - 1 - 1 - 2 = 20

		// Send a second WindowSizeMsg with different dimensions.
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		model, ok := updated.(bt.Model)
		require.True(t, ok)

		assert.Equal(t, 120, model.Viewport.Width)
		// Height = 40 - inputHeight(1) - statusHeight(1) - borderHeight(2) = 36
		assert.Equal(t, 36, model.Viewport.Height)

		view := model.View()
		assert.NotEmpty(t, view)
	})

	t.Run("window size resize re-renders viewport content", func(t *testing.T) {
		t.Parallel()

		// Start with a narrow viewport so word-wrapping is visible.
		m := initModelWithSize(t, nopAgent, 30, 20)

		// Add content that wraps at 30 columns.
		longLine := "word1 word2 word3 word4 word5 word6 word7 word8"
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Delta: longLine}})

		// Widen the viewport. Content should be re-rendered at new width.
		m = updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 20})

		// At 120 columns the entire line fits on one row. If content was
		// not re-rendered, the old 30-column wrapping would split the text
		// across multiple lines and "word8" wouldn't appear on the same
		// line as "word1".
		viewportContent := m.Viewport.View()
		lines := strings.Split(viewportContent, "\n")
		// Find the line containing word1 — word8 should be on that same line.
		found := false
		for _, line := range lines {
			if strings.Contains(line, "word1") && strings.Contains(line, "word8") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected word1 and word8 on the same line after resize, got:\n%s", viewportContent)
	})

	t.Run("ctrl+c when idle quits", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

		// Execute the cmd and check for quit message.
		require.NotNil(t, cmd)
		msg := cmd()
		_, isQuit := msg.(tea.QuitMsg)
		assert.True(t, isQuit)
	})

	t.Run("enter with empty input does nothing", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := updated.(bt.Model)

		assert.False(t, model.Running())
		assert.Nil(t, cmd)
	})

	t.Run("text event updates output", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Delta: "hello"}})

		assert.Contains(t, m.View(), "hello")
	})

	t.Run("long lines are word-wrapped to viewport width", func(t *testing.T) {
		t.Parallel()

		// Use a narrow viewport so wrapping is obvious.
		m := initModelWithSize(t, nopAgent, 30, 20)

		// Build a line much wider than 30 columns.
		longLine := "short words that keep going and going beyond the viewport width easily"
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Delta: longLine}})

		view := m.View()
		// Without wrapping, "easily" is truncated at column 30.
		// With wrapping, it flows to the next line and remains visible.
		assert.Contains(t, view, "easily")
	})

	t.Run("tool call begin shows tool name", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{Name: "read"}})

		assert.Contains(t, m.View(), "read")
	})

	t.Run("agent done re-enables input", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		// Simulate running state.
		m, _ = bt.SetRunning(m)
		require.True(t, m.Running())

		updated, _ := m.Update(bt.AgentDoneMsg{})
		model := updated.(bt.Model)

		assert.False(t, model.Running())
	})

	t.Run("agent done with error shows error", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m, _ = bt.SetRunning(m)

		updated, _ := m.Update(bt.AgentDoneMsg{Err: assert.AnError})
		model := updated.(bt.Model)

		assert.False(t, model.Running())
		assert.Error(t, model.Err())
		assert.Contains(t, model.View(), "Error")
	})

	t.Run("input accepts text after agent error", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m, _ = bt.SetRunning(m)

		// Agent completes with error.
		m = updateModel(t, m, bt.AgentDoneMsg{Err: assert.AnError})
		require.Error(t, m.Err())
		require.False(t, m.Running())

		// Type into input — should work since input was re-focused.
		m.Input = typeInputString(t, m.Input, "retry")
		assert.Equal(t, "retry", m.Input.Value())
	})

	t.Run("submit after error clears error and starts new run", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m, _ = bt.SetRunning(m)

		// Agent completes with error.
		m = updateModel(t, m, bt.AgentDoneMsg{Err: assert.AnError})
		require.Error(t, m.Err())

		// Type and submit.
		m.Input = typeInputString(t, m.Input, "retry")
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})

		assert.True(t, m.Running())
		assert.NoError(t, m.Err())
		assert.Contains(t, m.View(), "retry")
	})

	t.Run("ctrl+c quits after agent error", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m, _ = bt.SetRunning(m)

		// Agent completes with error.
		m = updateModel(t, m, bt.AgentDoneMsg{Err: assert.AnError})
		require.Error(t, m.Err())
		require.False(t, m.Running())

		// Ctrl+C should quit (not just cancel).
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		require.NotNil(t, cmd)
		msg := cmd()
		_, isQuit := msg.(tea.QuitMsg)
		assert.True(t, isQuit)
	})

	t.Run("agent done with long error wraps to viewport width", func(t *testing.T) {
		t.Parallel()

		m := initModelWithSize(t, nopAgent, 40, 20)
		m, _ = bt.SetRunning(m)

		longErr := fmt.Errorf("this is a very long error message that should wrap within the viewport width limit")
		updated, _ := m.Update(bt.AgentDoneMsg{Err: longErr})
		model := updated.(bt.Model)

		view := model.View()
		// The full error text must be visible (wrapped, not truncated).
		assert.Contains(t, view, "width limit")
		// No line should visually exceed the viewport width.
		for _, line := range strings.Split(view, "\n") {
			assert.LessOrEqual(t, lipgloss.Width(line), 40, "line exceeds viewport width: %q", line)
		}
	})

	t.Run("agent done with context canceled is not an error", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m, _ = bt.SetRunning(m)

		updated, _ := m.Update(bt.AgentDoneMsg{Err: context.Canceled})
		model := updated.(bt.Model)

		assert.False(t, model.Running())
		assert.NoError(t, model.Err())
	})

	t.Run("InputHeightMsg adjusts viewport height", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		// Initial viewport height: 24 - 1(input) - 1(status) - 2(borders) = 20
		assert.Equal(t, 20, m.Viewport.Height)

		// Simulate input growing to 3 lines.
		m = updateModel(t, m, textarea.InputHeightMsg{Height: 3})
		// Viewport should shrink: 24 - 3(input) - 1(status) - 2(borders) = 18
		assert.Equal(t, 18, m.Viewport.Height)
	})

	t.Run("enter during agent run is ignored", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m, _ = bt.SetRunning(m)

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := updated.(bt.Model)

		assert.True(t, model.Running())
		assert.Nil(t, cmd)
	})

	t.Run("ctrl+c during agent run cancels operation", func(t *testing.T) {
		t.Parallel()

		var cancelCalled bool
		m := initModel(t, nopAgent)
		m, _ = bt.SetRunningWithCancel(m, func() { cancelCalled = true })

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		model := updated.(bt.Model)

		assert.True(t, cancelCalled)
		// Should not quit the program.
		assert.Nil(t, cmd)
		// Still running (agent hasn't responded to cancellation yet).
		assert.True(t, model.Running())
	})
}

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

func TestModel_BlockFocusCycle(t *testing.T) {
	t.Parallel()

	t.Run("shift+tab cycles focus to previous collapsible block", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		// Create two thinking blocks.
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventThinkingDelta{Index: 0, Delta: "thought-1"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "answer"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventThinkingDelta{Index: 1, Delta: "thought-2"}})
		// Focus is on the last thinking block (thought-2).
		// Tab toggles thought-2.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
		assert.Contains(t, m.View(), "thought-2")
		assert.NotContains(t, m.View(), "thought-1")
		// Shift+Tab moves focus to thought-1.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
		// Tab now toggles thought-1.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
		assert.Contains(t, m.View(), "thought-1")
	})

	t.Run("tab without collapsible blocks is a no-op", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "hello"}})
		// No collapsible blocks — Tab should not insert a tab character.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
		assert.NotContains(t, m.View(), "\t")
	})
}

func TestModel_ToolResultEvent(t *testing.T) {
	t.Parallel()

	t.Run("EventToolResult creates ToolResultBlock during streaming", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		// Tool call first, then tool result event.
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "read"}}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "read", Content: "file contents here", IsError: false}})
		assert.Contains(t, m.View(), "file contents here")
	})

	t.Run("EventToolResult with error shows error styling", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "bash"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "bash"}}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "bash", Content: "command failed", IsError: true}})
		assert.Contains(t, m.View(), "command failed")
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

	t.Run("tool calls after turn reset create fresh blocks", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		// Turn 1: tool call.
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallDelta{ID: "tc-1", Delta: `{"path":"/old"}`}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "read"}}})
		// Turn 2: text delta triggers turn state reset, clearing activeToolCall.
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "answer"}})
		// New tool call with a different ID — must create a fresh block.
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-2", Name: "bash"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallDelta{ID: "tc-2", Delta: `{"cmd":"ls"}`}})
		view := m.View()
		assert.Contains(t, view, "read")
		assert.Contains(t, view, "bash")
		assert.Contains(t, view, "answer")
	})
}

func TestModel_SessionReloadBlockFocus(t *testing.T) {
	t.Parallel()

	t.Run("tab toggles collapsible block from session reload", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{
			Messages: []pipe.Message{
				pipe.AssistantMessage{Content: []pipe.ContentBlock{
					pipe.ThinkingBlock{Thinking: "deep thoughts"},
					pipe.TextBlock{Text: "answer"},
				}},
			},
		}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)
		m = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

		// Thinking block should start collapsed — content not visible.
		assert.NotContains(t, m.View(), "deep thoughts")

		// Tab should toggle the thinking block (focus was set by renderSession).
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyTab})
		assert.Contains(t, m.View(), "deep thoughts")
	})
}

func TestModel_MouseToggle(t *testing.T) {
	t.Parallel()

	t.Run("mouse enabled by default", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		assert.True(t, m.MouseEnabled())
	})

	t.Run("alt+m toggles mouse off", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
		assert.False(t, m.MouseEnabled())
	})

	t.Run("alt+m twice toggles mouse back on", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
		assert.True(t, m.MouseEnabled())
	})

	t.Run("alt+m returns disable then enable commands", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)

		// First toggle: disable mouse.
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
		m = updated.(bt.Model)
		require.NotNil(t, cmd)
		disableMsg := cmd()
		disableType := fmt.Sprintf("%T", disableMsg)

		// Second toggle: re-enable mouse.
		_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
		require.NotNil(t, cmd)
		enableMsg := cmd()
		enableType := fmt.Sprintf("%T", enableMsg)

		// The two commands should produce different message types.
		assert.NotEqual(t, disableType, enableType)
	})

	t.Run("alt+m during agent run is ignored", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m, _ = bt.SetRunning(m)
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
		// Mouse should still be enabled — toggle was ignored.
		assert.True(t, m.MouseEnabled())
	})

	t.Run("status line shows mouse hint when disabled", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		// Default state: no mouse hint.
		assert.NotContains(t, m.View(), "Alt+M")
		// Disable mouse: hint appears.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
		assert.Contains(t, m.View(), "Alt+M")
	})
}

func TestModel_InputHeightResetOnSubmit(t *testing.T) {
	t.Parallel()

	t.Run("input height resets to 1 after submit", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)
		m = updateModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})

		// Type multi-line input.
		m.Input.SetValue("line1")
		m.Input, _ = m.Input.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
		m.Input = typeInputString(t, m.Input, "line2")

		// Submit.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyEnter})

		// Input should be back to height 1.
		assert.Equal(t, 1, m.Input.Height())
	})
}

func typeInputString(t *testing.T, ta textarea.Model, s string) textarea.Model {
	t.Helper()
	for _, r := range s {
		ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return ta
}

func TestModel_Integration(t *testing.T) {
	t.Parallel()

	t.Run("submit creates user message and returns cmd", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)

		// Initialize viewport.
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = updated.(bt.Model)

		// Type and submit.
		m.Input.SetValue("hi")
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(bt.Model)

		assert.True(t, m.Running())
		require.NotNil(t, cmd)

		// Verify user message was added to session.
		require.Len(t, session.Messages, 1)
		um, ok := session.Messages[0].(pipe.UserMessage)
		require.True(t, ok)
		require.Len(t, um.Content, 1)
		tb, ok := um.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Equal(t, "hi", tb.Text)

		// Verify user message appears in view.
		assert.Contains(t, m.View(), "hi")
	})

	t.Run("viewport scrolls long output", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		// Set viewport to small height.
		m.Viewport = viewport.New(80, 5)

		// Add many blocks of output. Each delta uses a different index
		// to create separate blocks that produce distinct visible lines.
		for i := range 50 {
			m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: i, Delta: "line"}})
		}

		// The viewport should have scrollable content.
		view := m.View()
		assert.NotEmpty(t, view)
		lines := strings.Split(view, "\n")
		// View should be constrained to viewport height, not all 50 lines.
		assert.Less(t, len(lines), 50)
	})

	t.Run("viewport accepts scroll keys when idle", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		require.False(t, m.Running())

		// Fill viewport with numbered blocks. Each delta uses a different
		// index to create a separate block that appears on its own line.
		for i := range 30 {
			m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{
				Index: i,
				Delta: fmt.Sprintf("line-%d", i),
			}})
		}

		// Viewport should be at the bottom (auto-scroll).
		viewBefore := m.Viewport.View()
		assert.Contains(t, viewBefore, "line-29")

		// Send page-up to scroll up while idle.
		m = updateModel(t, m, tea.KeyMsg{Type: tea.KeyPgUp})

		// After scrolling up, the last line should no longer be visible.
		viewAfter := m.Viewport.View()
		assert.NotContains(t, viewAfter, "line-29")
	})

}

func TestModel_Teatest(t *testing.T) {
	t.Parallel()

	t.Run("full agent cycle with event delivery", func(t *testing.T) {
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
			return bytes.Contains(out, []byte("Hello!")) &&
				bytes.Contains(out, []byte("Enter to send"))
		}, teatest.WithDuration(5*time.Second))

		tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

		fm := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second))
		final, ok := fm.(bt.Model)
		require.True(t, ok)
		assert.False(t, final.Running())
		assert.NoError(t, final.Err())
		// Session should contain user message + assistant message.
		assert.Len(t, session.Messages, 2)
	})

	t.Run("existing session messages render on init", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{
			Messages: []pipe.Message{
				pipe.UserMessage{Content: []pipe.ContentBlock{
					pipe.TextBlock{Text: "hello there"},
				}},
				pipe.AssistantMessage{Content: []pipe.ContentBlock{
					pipe.TextBlock{Text: "Hi! How can I help?"},
				}},
			},
		}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)

		tm := teatest.NewTestModel(t, m,
			teatest.WithInitialTermSize(80, 24),
		)

		teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
			return bytes.Contains(out, []byte("hello there")) &&
				bytes.Contains(out, []byte("Hi! How can I help?"))
		}, teatest.WithDuration(5*time.Second))

		tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
		tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	})

	t.Run("existing session with tool result renders on init", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{
			Messages: []pipe.Message{
				pipe.UserMessage{Content: []pipe.ContentBlock{
					pipe.TextBlock{Text: "read /tmp"},
				}},
				pipe.AssistantMessage{Content: []pipe.ContentBlock{
					pipe.ToolCallBlock{ID: "tc-1", Name: "read", Arguments: json.RawMessage(`{"path":"/tmp"}`)},
				}},
				pipe.ToolResultMessage{
					ToolCallID: "tc-1",
					ToolName:   "read",
					Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "file contents here"}},
				},
			},
		}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)

		tm := teatest.NewTestModel(t, m,
			teatest.WithInitialTermSize(80, 24),
		)

		teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
			return bytes.Contains(out, []byte("file contents here")) &&
				bytes.Contains(out, []byte("read"))
		}, teatest.WithDuration(5*time.Second))

		tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
		tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	})

	t.Run("tool result event appears during agent run", func(t *testing.T) {
		t.Parallel()

		agent := func(_ context.Context, _ *pipe.Session, onEvent func(pipe.Event)) error {
			onEvent(pipe.EventToolCallBegin{ID: "tc-1", Name: "bash"})
			onEvent(pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{
				ID: "tc-1", Name: "bash", Arguments: json.RawMessage(`{"command":"echo hi"}`),
			}})
			onEvent(pipe.EventToolResult{ToolName: "bash", Content: "hi\n", IsError: false})
			onEvent(pipe.EventTextDelta{Index: 0, Delta: "Done!"})
			return nil
		}

		session := &pipe.Session{}
		theme := pipe.DefaultTheme()
		m := bt.New(agent, session, theme)

		tm := teatest.NewTestModel(t, m,
			teatest.WithInitialTermSize(80, 24),
		)

		tm.Type("run it")
		tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

		teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
			return bytes.Contains(out, []byte("hi")) &&
				bytes.Contains(out, []byte("Done!")) &&
				bytes.Contains(out, []byte("Enter to send"))
		}, teatest.WithDuration(5*time.Second))

		tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
		tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	})

	t.Run("existing session with thinking block renders on init", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{
			Messages: []pipe.Message{
				pipe.UserMessage{Content: []pipe.ContentBlock{
					pipe.TextBlock{Text: "think about this"},
				}},
				pipe.AssistantMessage{Content: []pipe.ContentBlock{
					pipe.ThinkingBlock{Thinking: "let me consider"},
					pipe.TextBlock{Text: "here is my answer"},
				}},
			},
		}
		theme := pipe.DefaultTheme()
		m := bt.New(nopAgent, session, theme)

		tm := teatest.NewTestModel(t, m,
			teatest.WithInitialTermSize(80, 24),
		)

		teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
			return bytes.Contains(out, []byte("Thinking")) &&
				bytes.Contains(out, []byte("here is my answer")) &&
				!bytes.Contains(out, []byte("let me consider"))
		}, teatest.WithDuration(5*time.Second))

		tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
		tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	})

	t.Run("conversation continues after agent error", func(t *testing.T) {
		t.Parallel()

		var callCount atomic.Int32
		// The agent mutates session.Messages directly — this mirrors the real
		// contract where both model (user messages) and agent (assistant messages)
		// append to the shared session.
		agent := func(_ context.Context, session *pipe.Session, onEvent func(pipe.Event)) error {
			n := callCount.Add(1)
			if n == 1 {
				return fmt.Errorf("simulated API error")
			}
			onEvent(pipe.EventTextDelta{Index: 0, Delta: "recovered"})
			session.Messages = append(session.Messages, pipe.AssistantMessage{
				Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "recovered"}},
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

		// First message triggers error.
		tm.Type("hello")
		tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

		teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
			return bytes.Contains(out, []byte("Error")) &&
				bytes.Contains(out, []byte("simulated API error"))
		}, teatest.WithDuration(5*time.Second))

		// Second message should succeed — conversation continues.
		tm.Type("retry")
		tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

		teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
			return bytes.Contains(out, []byte("recovered")) &&
				bytes.Contains(out, []byte("Enter to send"))
		}, teatest.WithDuration(5*time.Second))

		tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

		fm := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second))
		final, ok := fm.(bt.Model)
		require.True(t, ok)
		assert.False(t, final.Running())
		assert.NoError(t, final.Err())
		assert.Equal(t, int32(2), callCount.Load())
	})
}
