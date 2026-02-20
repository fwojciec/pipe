package mock

import "github.com/fwojciec/pipe"

// Interface compliance check.
var _ pipe.Stream = (*Stream)(nil)

// Stream is a test double for pipe.Stream.
// Set the function fields for the methods you need. NextFn and MessageFn
// panic when nil to catch missing setup. CloseFn and StateFn are nil-safe
// (no-op and zero value) because test code commonly calls defer stream.Close()
// and these methods rarely need custom behavior.
type Stream struct {
	NextFn    func() (pipe.Event, error)
	StateFn   func() pipe.StreamState
	MessageFn func() (pipe.AssistantMessage, error)
	CloseFn   func() error
}

// Next delegates to NextFn.
func (s *Stream) Next() (pipe.Event, error) {
	return s.NextFn()
}

// State delegates to StateFn. Returns StreamStateNew when StateFn is nil.
func (s *Stream) State() pipe.StreamState {
	if s.StateFn == nil {
		return pipe.StreamStateNew
	}
	return s.StateFn()
}

// Message delegates to MessageFn.
func (s *Stream) Message() (pipe.AssistantMessage, error) {
	return s.MessageFn()
}

// Close delegates to CloseFn. Returns nil when CloseFn is not set.
func (s *Stream) Close() error {
	if s.CloseFn == nil {
		return nil
	}
	return s.CloseFn()
}
