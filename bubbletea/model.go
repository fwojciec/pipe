package bubbletea

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fwojciec/pipe"
)

// Model is the Bubble Tea model for the pipe TUI.
type Model struct {
	// Textarea is the text input component. Exported for test access.
	Textarea textarea.Model
	// Viewport is the scrollable output area. Exported for test access.
	Viewport viewport.Model

	run     AgentFunc
	session *pipe.Session

	output  *strings.Builder
	running bool
	cancel  context.CancelFunc
	eventCh chan pipe.Event
	doneCh  chan error
	err     error
	ready   bool
}

// New creates a new TUI Model with the given agent function and session.
func New(run AgentFunc, session *pipe.Session) Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.Focus()
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(1)

	return Model{
		Textarea: ta,
		run:      run,
		session:  session,
		output:   &strings.Builder{},
	}
}

// Running returns whether the agent is currently running.
func (m Model) Running() bool { return m.running }

// Err returns the last error, if any.
func (m Model) Err() error { return m.err }

// SetRunning is a test helper that puts the model in a running state.
func SetRunning(m Model) (Model, tea.Cmd) {
	m.running = true
	return m, nil
}

// SetRunningWithCancel is a test helper that puts the model in a running state
// with a cancel function.
func SetRunningWithCancel(m Model, cancel func()) (Model, tea.Cmd) {
	m.running = true
	m.cancel = cancel
	return m, nil
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.handleWindowSize(msg)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case StreamEventMsg:
		m.processEvent(msg.Event)
		m.Viewport.SetContent(m.output.String())
		m.Viewport.GotoBottom()
		if m.eventCh != nil {
			return m, listenForEvent(m.eventCh, m.doneCh)
		}
		return m, nil

	case AgentDoneMsg:
		m.running = false
		m.cancel = nil
		if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) {
			m.err = msg.Err
		}
		cmd := m.Textarea.Focus()
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	}

	// Pass remaining messages to sub-components.
	var cmd tea.Cmd
	if m.running {
		m.Viewport, cmd = m.Viewport.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		m.Textarea, cmd = m.Textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder

	// Output area.
	b.WriteString(m.Viewport.View())
	b.WriteString("\n")

	// Status line.
	b.WriteString(m.statusLine())
	b.WriteString("\n")

	// Input area.
	b.WriteString(m.Textarea.View())

	return b.String()
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) Model {
	inputHeight := 1
	statusHeight := 1
	borderHeight := 2 // newlines between sections
	vpHeight := msg.Height - inputHeight - statusHeight - borderHeight

	if vpHeight < 1 {
		vpHeight = 1
	}

	if !m.ready {
		m.Viewport = viewport.New(msg.Width, vpHeight)
		m.ready = true
	} else {
		m.Viewport.Width = msg.Width
		m.Viewport.Height = vpHeight
	}

	m.Textarea.SetWidth(msg.Width)
	return m
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.running {
			if m.cancel != nil {
				m.cancel()
			}
			return m, nil
		}
		return m, tea.Quit

	case tea.KeyEnter:
		if m.running {
			return m, nil
		}
		text := strings.TrimSpace(m.Textarea.Value())
		if text == "" {
			return m, nil
		}
		return m.submitInput(text)
	}

	// Pass key to textarea when not running.
	if !m.running {
		var cmd tea.Cmd
		m.Textarea, cmd = m.Textarea.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) submitInput(text string) (tea.Model, tea.Cmd) {
	m.Textarea.Reset()
	m.err = nil

	// Append user message to session.
	userMsg := pipe.UserMessage{
		Content:   []pipe.ContentBlock{pipe.TextBlock{Text: text}},
		Timestamp: time.Now(),
	}
	m.session.Messages = append(m.session.Messages, userMsg)

	// Show user message in output.
	m.output.WriteString("\n> " + text + "\n\n")
	m.Viewport.SetContent(m.output.String())
	m.Viewport.GotoBottom()

	// Set up channels and context for agent run.
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.eventCh = make(chan pipe.Event, 256)
	m.doneCh = make(chan error, 1)
	m.running = true

	m.Textarea.Blur()

	return m, tea.Batch(
		startAgent(m.run, ctx, m.session, m.eventCh, m.doneCh),
		listenForEvent(m.eventCh, m.doneCh),
	)
}

func (m Model) processEvent(evt pipe.Event) {
	switch e := evt.(type) {
	case pipe.EventTextDelta:
		m.output.WriteString(e.Delta)
	case pipe.EventThinkingDelta:
		// Show thinking inline for MVP.
		m.output.WriteString(e.Delta)
	case pipe.EventToolCallBegin:
		fmt.Fprintf(m.output, "\n--- tool: %s ---\n", e.Name)
	case pipe.EventToolCallDelta:
		// Skip partial JSON arguments.
	case pipe.EventToolCallEnd:
		m.output.WriteString("--- end ---\n\n")
	}
}

func (m Model) statusLine() string {
	style := lipgloss.NewStyle().Faint(true)

	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		return errStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}
	if m.running {
		return style.Render("Generating...")
	}
	return style.Render("Enter to send, Ctrl+C to quit")
}

// startAgent runs the agent loop in a goroutine and signals completion.
func startAgent(run AgentFunc, ctx context.Context, session *pipe.Session, eventCh chan<- pipe.Event, doneCh chan<- error) tea.Cmd {
	return func() tea.Msg {
		err := run(ctx, session, func(e pipe.Event) {
			select {
			case eventCh <- e:
			case <-ctx.Done():
			}
		})
		close(eventCh)
		doneCh <- err
		return nil
	}
}

// listenForEvent waits for the next event from the channel.
// When the channel closes, it reads the error from doneCh and returns AgentDoneMsg.
func listenForEvent(ch <-chan pipe.Event, doneCh <-chan error) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-ch
		if !ok {
			err := <-doneCh
			return AgentDoneMsg{Err: err}
		}
		return StreamEventMsg{Event: evt}
	}
}
