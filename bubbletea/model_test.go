package bubbletea_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initModel creates a model and sends a WindowSizeMsg to initialize the viewport.
func initModel(t *testing.T, run bt.AgentFunc) bt.Model {
	t.Helper()
	session := &pipe.Session{}
	m := bt.New(run, session)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, ok := updated.(bt.Model)
	require.True(t, ok)
	return model
}

// nopAgent is a mock agent that does nothing.
func nopAgent(_ context.Context, _ *pipe.Session, _ func(pipe.Event)) error {
	return nil
}

func TestNew(t *testing.T) {
	t.Parallel()

	session := &pipe.Session{}
	m := bt.New(nopAgent, session)

	assert.False(t, m.Running())
	assert.NoError(t, m.Err())
}

func TestModel_Update(t *testing.T) {
	t.Parallel()

	t.Run("window size initializes viewport", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{}
		m := bt.New(nopAgent, session)
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
		updated, _ := m.Update(bt.StreamEventMsg{Event: pipe.EventTextDelta{Delta: "hello"}})
		model := updated.(bt.Model)

		assert.Contains(t, model.View(), "hello")
	})

	t.Run("tool call begin shows tool name", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		updated, _ := m.Update(bt.StreamEventMsg{Event: pipe.EventToolCallBegin{Name: "read"}})
		model := updated.(bt.Model)

		assert.Contains(t, model.View(), "read")
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

	t.Run("agent done with context canceled is not an error", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		m, _ = bt.SetRunning(m)

		updated, _ := m.Update(bt.AgentDoneMsg{Err: context.Canceled})
		model := updated.(bt.Model)

		assert.False(t, model.Running())
		assert.NoError(t, model.Err())
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

func TestModel_Integration(t *testing.T) {
	t.Parallel()

	t.Run("submit creates user message and returns cmd", func(t *testing.T) {
		t.Parallel()

		session := &pipe.Session{}
		m := bt.New(nopAgent, session)

		// Initialize viewport.
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = updated.(bt.Model)

		// Type and submit.
		m.Textarea.SetValue("hi")
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

	t.Run("full agent cycle with event delivery", func(t *testing.T) {
		t.Parallel()

		// Create a mock agent that sends events and completes.
		agent := func(_ context.Context, session *pipe.Session, onEvent func(pipe.Event)) error {
			onEvent(pipe.EventTextDelta{Index: 0, Delta: "Hello!"})
			session.Messages = append(session.Messages, pipe.AssistantMessage{
				Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "Hello!"}},
				StopReason: pipe.StopEndTurn,
			})
			return nil
		}

		session := &pipe.Session{}
		m := bt.New(agent, session)

		// Initialize.
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = updated.(bt.Model)

		// Submit input.
		m.Textarea.SetValue("hi")
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(bt.Model)
		require.NotNil(t, cmd)

		// Execute the batch cmd. tea.Batch returns a BatchMsg containing sub-cmds.
		batchResult := cmd()
		batchMsg, ok := batchResult.(tea.BatchMsg)
		require.True(t, ok, "expected BatchMsg, got %T", batchResult)

		// Execute sub-cmds in goroutines and collect results.
		results := make(chan tea.Msg, len(batchMsg))
		for _, subCmd := range batchMsg {
			go func() {
				if subCmd != nil {
					results <- subCmd()
				}
			}()
		}

		// Collect all results with timeout.
		timeout := time.After(5 * time.Second)
		for range len(batchMsg) {
			select {
			case msg := <-results:
				if msg == nil {
					continue
				}
				updated, cmd = m.Update(msg)
				m = updated.(bt.Model)

				// If a StreamEventMsg, chain the next listen.
				if _, isSE := msg.(bt.StreamEventMsg); isSE && cmd != nil {
					nextMsg := cmd()
					if nextMsg != nil {
						updated, _ = m.Update(nextMsg)
						m = updated.(bt.Model)
					}
				}
			case <-timeout:
				t.Fatal("timeout waiting for cmd results")
			}
		}

		// Agent should have completed.
		assert.False(t, m.Running())
		assert.NoError(t, m.Err())

		// Session should have user message + assistant message.
		require.Len(t, session.Messages, 2)
	})

	t.Run("viewport scrolls long output", func(t *testing.T) {
		t.Parallel()

		m := initModel(t, nopAgent)
		// Set viewport to small height.
		m.Viewport = viewport.New(80, 5)

		// Add many lines of output.
		var updated tea.Model
		for i := 0; i < 50; i++ {
			updated, _ = m.Update(bt.StreamEventMsg{Event: pipe.EventTextDelta{Delta: "line\n"}})
			m = updated.(bt.Model)
		}

		// The viewport should have scrollable content.
		view := m.View()
		assert.NotEmpty(t, view)
		lines := strings.Split(view, "\n")
		// View should be constrained to viewport height, not all 50 lines.
		assert.Less(t, len(lines), 50)
	})
}
