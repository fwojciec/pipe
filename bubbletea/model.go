package bubbletea

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/bubbletea/textarea"
)

var _ tea.Model = Model{}

// Model is the Bubble Tea model for the pipe TUI.
type Model struct {
	// Input is the multi-line text input component. Exported for test access.
	Input textarea.Model
	// Viewport is the scrollable output area. Exported for test access.
	Viewport viewport.Model

	run     AgentFunc
	session *pipe.Session
	theme   pipe.Theme
	styles  Styles

	blocks     []MessageBlock
	blockFocus int // index of focused collapsible block (-1 = none)

	// Active block maps for event correlation within the current turn.
	// Text/thinking indices restart at 0 each assistant turn. Tool call
	// IDs are globally unique and never cleared.
	activeText     map[int]*AssistantTextBlock // keyed by EventTextDelta.Index
	activeThinking map[int]*ThinkingBlock      // keyed by EventThinkingDelta.Index
	activeToolCall map[string]*ToolCallBlock   // keyed by EventToolCall*.ID

	// hadToolCalls is set on EventToolCallBegin. When text/thinking arrives
	// after tool calls, it signals a new assistant turn — the text and
	// thinking maps are cleared. This works because Anthropic and Gemini
	// always emit tool use blocks last within an assistant message.
	hadToolCalls bool

	windowHeight int // stored for viewport recomputation on InputHeightMsg

	running bool
	cancel  context.CancelFunc
	eventCh chan pipe.Event
	doneCh  chan error
	err     error
	ready   bool
}

// New creates a new TUI Model with the given agent function, session, and theme.
func New(run AgentFunc, session *pipe.Session, theme pipe.Theme) Model {
	ta := textarea.New()
	ta.MaxHeight = 3
	// Defensive fallback: handleKey intercepts Enter at line 225 before the
	// textarea sees it, so this callback is normally never invoked. It exists
	// as a safety net — if a code path ever lets Enter through, this prevents
	// accidental newline insertion. Ctrl+J inserts newlines.
	ta.CheckInputComplete = func(_ string) bool { return true }
	ta.Focus()

	return Model{
		Input:          ta,
		run:            run,
		session:        session,
		theme:          theme,
		styles:         NewStyles(theme),
		blockFocus:     -1,
		activeText:     make(map[int]*AssistantTextBlock),
		activeThinking: make(map[int]*ThinkingBlock),
		activeToolCall: make(map[string]*ToolCallBlock),
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
	return cursor.Blink
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m = m.handleWindowSize(msg)
		return m, nil

	case textarea.InputHeightMsg:
		if m.windowHeight == 0 {
			return m, nil
		}
		m.Viewport.Height = m.viewportHeight(msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case StreamEventMsg:
		m = m.processEvent(msg.Event)
		m.Viewport.SetContent(m.renderContent())
		m.Viewport.GotoBottom()
		if m.eventCh != nil {
			return m, listenForEvent(m.eventCh, m.doneCh)
		}
		return m, nil

	case AgentDoneMsg:
		m.running = false
		m.cancel = nil
		m.eventCh = nil
		m.doneCh = nil
		if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) {
			m.err = msg.Err
		}
		m = m.updateBlockFocus()
		cmd := m.Input.Focus()
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Pass remaining messages to sub-components.
	// Viewport always receives messages for scrolling (keyboard and mouse).
	var cmd tea.Cmd
	m.Viewport, cmd = m.Viewport.Update(msg)
	cmds = append(cmds, cmd)

	if !m.running {
		m.Input, cmd = m.Input.Update(msg)
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
	b.WriteString(m.Input.View())

	return b.String()
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) Model {
	m.windowHeight = msg.Height
	vpHeight := m.viewportHeight(m.Input.Height())

	if !m.ready {
		m.Viewport = viewport.New(msg.Width, vpHeight)
		m = m.renderSession()
		m = m.updateBlockFocus()
		m.Viewport.SetContent(m.renderContent())
		m.Viewport.GotoBottom()
		m.ready = true
	} else {
		m.Viewport.Width = msg.Width
		m.Viewport.Height = vpHeight
		m.Viewport.SetContent(m.renderContent())
	}

	m.Input.SetWidth(msg.Width)
	return m
}

// viewportHeight computes the viewport height given the current input height.
func (m Model) viewportHeight(inputH int) int {
	const statusHeight = 1
	const borderHeight = 2 // newlines between sections
	h := m.windowHeight - inputH - statusHeight - borderHeight
	if h < 1 {
		h = 1
	}
	return h
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
		text := strings.TrimSpace(m.Input.Value())
		if text == "" {
			return m, nil
		}
		return m.submitInput(text)

	case tea.KeyTab:
		if !m.running && m.blockFocus >= 0 && m.blockFocus < len(m.blocks) {
			block, cmd := m.blocks[m.blockFocus].Update(ToggleMsg{})
			m.blocks[m.blockFocus] = block
			m.Viewport.SetContent(m.renderContent())
			return m, cmd
		}
		return m, nil

	case tea.KeyShiftTab:
		if !m.running {
			m = m.cycleFocusPrev()
			m.Viewport.SetContent(m.renderContent())
		}
		return m, nil
	}

	// When idle, pass keys to both textarea (for typing) and viewport
	// (for scrolling). Only forward non-character keys to viewport to avoid
	// conflicts (e.g. 'j'/'k' are viewport scroll AND text characters).
	if !m.running {
		var cmd tea.Cmd
		var cmds []tea.Cmd

		if msg.Type != tea.KeyRunes {
			m.Viewport, cmd = m.Viewport.Update(msg)
			cmds = append(cmds, cmd)
		}

		m.Input, cmd = m.Input.Update(msg)
		cmds = append(cmds, cmd)

		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m Model) submitInput(text string) (tea.Model, tea.Cmd) {
	m.Input.SetValue("")
	m.Input.SetHeight(1)
	m.Viewport.Height = m.viewportHeight(1)
	m.err = nil

	// Append user message to session.
	userMsg := pipe.UserMessage{
		Content:   []pipe.ContentBlock{pipe.TextBlock{Text: text}},
		Timestamp: time.Now(),
	}
	m.session.Messages = append(m.session.Messages, userMsg)

	// Add user message block.
	m.blocks = append(m.blocks, NewUserMessageBlock(text, m.styles))
	m.Viewport.SetContent(m.renderContent())
	m.Viewport.GotoBottom()

	// Reset active maps for new conversation turn.
	m = m.resetTurnState()

	// Set up channels and context for agent run.
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.eventCh = make(chan pipe.Event, 256)
	m.doneCh = make(chan error, 1)
	m.running = true

	m.Input.Blur()

	return m, tea.Batch(
		startAgent(m.run, ctx, m.session, m.eventCh, m.doneCh),
		listenForEvent(m.eventCh, m.doneCh),
	)
}

// renderSession creates blocks from existing session messages.
func (m Model) renderSession() Model {
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
					block := NewAssistantTextBlock(m.theme)
					block.Append(cb.Text)
					m.blocks = append(m.blocks, block)
				case pipe.ThinkingBlock:
					block := NewThinkingBlock(m.styles)
					block.Append(cb.Thinking)
					m.blocks = append(m.blocks, block)
				case pipe.ToolCallBlock:
					block := NewToolCallBlock(cb.Name, cb.ID, m.styles)
					block.FinalizeWithCall(cb)
					m.blocks = append(m.blocks, block)
				}
			}
		case pipe.ToolResultMessage:
			var content strings.Builder
			for _, b := range msg.Content {
				if tb, ok := b.(pipe.TextBlock); ok {
					content.WriteString(tb.Text)
				}
			}
			m.blocks = append(m.blocks, NewToolResultBlock(msg.ToolName, content.String(), msg.IsError, m.styles))
		}
	}
	return m
}

func (m Model) renderContent() string {
	if len(m.blocks) == 0 {
		return ""
	}
	var b strings.Builder
	for i, block := range m.blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(block.View(m.Viewport.Width))
	}
	return b.String()
}

// resetTurnState clears the active block maps and hadToolCalls flag, preparing
// the model for a new assistant turn.
func (m Model) resetTurnState() Model {
	m.activeText = make(map[int]*AssistantTextBlock)
	m.activeThinking = make(map[int]*ThinkingBlock)
	m.activeToolCall = make(map[string]*ToolCallBlock)
	m.hadToolCalls = false
	return m
}

// processEvent routes a streaming event to the appropriate block.
func (m Model) processEvent(evt pipe.Event) Model {
	switch e := evt.(type) {
	case pipe.EventTextDelta:
		if m.hadToolCalls {
			m = m.resetTurnState()
		}
		if b, ok := m.activeText[e.Index]; ok {
			b.Append(e.Delta)
		} else {
			b := NewAssistantTextBlock(m.theme)
			b.Append(e.Delta)
			m.blocks = append(m.blocks, b)
			m.activeText[e.Index] = b
			m = m.updateBlockFocus()
		}
	case pipe.EventThinkingDelta:
		if m.hadToolCalls {
			m = m.resetTurnState()
		}
		if b, ok := m.activeThinking[e.Index]; ok {
			b.Append(e.Delta)
		} else {
			b := NewThinkingBlock(m.styles)
			b.Append(e.Delta)
			m.blocks = append(m.blocks, b)
			m.activeThinking[e.Index] = b
			m = m.updateBlockFocus()
		}
	case pipe.EventToolCallBegin:
		m.hadToolCalls = true
		b := NewToolCallBlock(e.Name, e.ID, m.styles)
		m.blocks = append(m.blocks, b)
		m.activeToolCall[e.ID] = b
		m = m.updateBlockFocus()
	case pipe.EventToolCallDelta:
		if b, ok := m.activeToolCall[e.ID]; ok {
			b.AppendArgs(e.Delta)
		}
	case pipe.EventToolCallEnd:
		if b, ok := m.activeToolCall[e.Call.ID]; ok {
			b.FinalizeWithCall(e.Call)
		}
	}
	return m
}

// updateBlockFocus scans backwards to find the last collapsible block.
// Only the focused block responds to Tab. ShiftTab cycles to the previous
// collapsible block. Full arrow-key navigation is deferred to a follow-up.
func (m Model) updateBlockFocus() Model {
	m.blockFocus = -1
	for i := len(m.blocks) - 1; i >= 0; i-- {
		switch m.blocks[i].(type) {
		case *ThinkingBlock, *ToolCallBlock:
			m.blockFocus = i
			return m
		}
	}
	return m
}

// cycleFocusPrev moves blockFocus to the previous collapsible block, wrapping around.
func (m Model) cycleFocusPrev() Model {
	start := m.blockFocus - 1
	if start < 0 {
		start = len(m.blocks) - 1
	}
	for i := range len(m.blocks) {
		idx := (start - i + len(m.blocks)) % len(m.blocks)
		switch m.blocks[idx].(type) {
		case *ThinkingBlock, *ToolCallBlock:
			m.blockFocus = idx
			return m
		}
	}
	m.blockFocus = -1
	return m
}

func (m Model) statusLine() string {
	if m.err != nil {
		return m.styles.Error.Render(fmt.Sprintf("Error: %v", m.err))
	}
	if m.running {
		return m.styles.Muted.Render("Generating...")
	}
	return m.styles.Muted.Render("Enter to send, Ctrl+C to quit")
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
