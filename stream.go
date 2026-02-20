package pipe

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
