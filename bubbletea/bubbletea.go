// Package bubbletea provides a Bubble Tea TUI for the pipe agent.
package bubbletea

import (
	"context"

	"github.com/fwojciec/pipe"
)

// AgentFunc runs the agent loop. The onEvent callback is called for each
// streaming event. The function blocks until the agent completes or the
// context is cancelled.
type AgentFunc func(ctx context.Context, session *pipe.Session, onEvent func(pipe.Event)) error

// StreamEventMsg wraps a streaming event for delivery to the Bubble Tea model.
type StreamEventMsg struct {
	Event pipe.Event
}

// AgentDoneMsg signals that the agent loop has completed.
type AgentDoneMsg struct {
	Err error
}
