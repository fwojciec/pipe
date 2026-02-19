package pipe

import "context"

// StreamState indicates the current state of a Stream.
type StreamState int

const (
	StreamStateNew       StreamState = iota // Before Next() is ever called.
	StreamStateStreaming                    // Mid-stream, receiving deltas.
	StreamStateComplete                     // Next() returned io.EOF.
	StreamStateError                        // Next() returned non-EOF error.
	StreamStateClosed                       // Close() called before terminal state.
)

// Stream uses a pull-based iterator pattern. Cancellation flows through the
// context passed to Provider.Stream().
//
// State() returns the current StreamState. Callers can use it to determine
// whether Message() will return a partial or complete message.
//
// Message() returns the assembled AssistantMessage. Behavior by stream state:
//   - StreamStateComplete: complete message, nil error.
//   - StreamStateError: partial message, nil error. StopReason is StopError
//     for transport/protocol failures, StopAborted for context cancellation.
//   - StreamStateStreaming: partial message, nil error. Content reflects
//     deltas received so far.
//   - StreamStateNew: zero-value message, non-nil error.
//   - StreamStateClosed: partial message with StopReason = StopAborted.
//     Subsequent Next() calls return error.
//   - If a terminal state (Complete/Error) was reached before Close(),
//     Message() returns the terminal-state result.
type Stream interface {
	Next() (Event, error)
	State() StreamState
	Message() (AssistantMessage, error)
	Close() error
}

// Provider is a strategy pattern interface for LLM providers.
type Provider interface {
	Stream(ctx context.Context, req Request) (Stream, error)
}

// Request carries model selection and generation parameters.
// The provider uses its own defaults when fields are zero/nil.
type Request struct {
	Model        string // model ID, provider-specific; empty = provider default
	SystemPrompt string
	Messages     []Message
	Tools        []Tool
	MaxTokens    int      // 0 = provider default
	Temperature  *float64 // nil = provider default
}
