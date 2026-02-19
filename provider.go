package pipe

import "context"

// Stream uses a pull-based iterator pattern. Cancellation flows through the
// context passed to Provider.Stream().
//
// Message() returns the assembled AssistantMessage. Behavior by stream state:
//   - After Next() returns io.EOF: complete message, nil error.
//   - After Next() returns non-EOF error: partial message, nil error.
//     StopReason is StopError for transport/protocol failures, StopAborted
//     for context cancellation.
//   - Mid-stream (some deltas received, no terminal state): partial message,
//     nil error. Content reflects deltas received so far.
//   - Before Next() is ever called: zero-value message, non-nil error.
//   - After Close(): if a terminal state was reached, same as that state.
//     If Close() is called mid-stream, message is partial with
//     StopReason = StopAborted. Subsequent Next() calls return error.
type Stream interface {
	Next() (Event, error)
	Message() (AssistantMessage, error)
	Close() error
}

// Provider is a strategy pattern interface for LLM providers.
type Provider interface {
	Stream(ctx context.Context, req *Request) (Stream, error)
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
