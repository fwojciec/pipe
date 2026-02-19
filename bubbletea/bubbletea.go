// Package bubbletea provides a Bubble Tea TUI for the pipe agent.
package bubbletea

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
)

// AgentFunc runs the agent loop. The onEvent callback is called for each
// streaming event. The function blocks until the agent completes or the
// context is cancelled.
type AgentFunc func(ctx context.Context, session *pipe.Session, onEvent func(pipe.Event)) error

// Run creates and runs the Bubble Tea TUI program. It blocks until the program
// exits. The context is used for graceful shutdown — when cancelled, the
// program quits.
func Run(ctx context.Context, m Model) error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	go func() {
		<-ctx.Done()
		p.Quit()
	}()
	_, err := p.Run()
	return err
}

// StreamEventMsg wraps a streaming event for delivery to the Bubble Tea model.
type StreamEventMsg struct {
	Event pipe.Event
}

// AgentDoneMsg signals that the agent loop has completed.
type AgentDoneMsg struct {
	Err error
}
