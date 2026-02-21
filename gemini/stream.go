package gemini

import (
	"context"
	"fmt"
	"io"
	"iter"

	"github.com/fwojciec/pipe"
	"google.golang.org/genai"
)

// stream implements [pipe.Stream] by wrapping the genai SDK's streaming iterator.
// TODO(#29): Implement full event parsing. Currently a compilation stub that
// drains the iterator without processing response chunks into events.
type stream struct {
	pull  func() (*genai.GenerateContentResponse, error, bool)
	stop  func()
	state pipe.StreamState
	msg   pipe.AssistantMessage
	err   error
}

// Interface compliance check.
var _ pipe.Stream = (*stream)(nil)

func newStream(_ context.Context, iterFn iter.Seq2[*genai.GenerateContentResponse, error], _ []*genai.Content) *stream {
	next, stop := iter.Pull2(iterFn)
	return &stream{
		pull:  next,
		stop:  stop,
		state: pipe.StreamStateNew,
	}
}

func (s *stream) Next() (pipe.Event, error) {
	switch s.state {
	case pipe.StreamStateComplete:
		return nil, io.EOF
	case pipe.StreamStateError:
		return nil, s.err
	case pipe.StreamStateClosed:
		return nil, fmt.Errorf("gemini: stream closed")
	}
	// Stub: drain the iterator and return EOF.
	for {
		_, err, ok := s.pull()
		if !ok {
			s.state = pipe.StreamStateComplete
			return nil, io.EOF
		}
		if err != nil {
			s.state = pipe.StreamStateError
			s.err = fmt.Errorf("gemini: %w", err)
			return nil, s.err
		}
	}
}

func (s *stream) State() pipe.StreamState {
	return s.state
}

func (s *stream) Message() (pipe.AssistantMessage, error) {
	if s.state == pipe.StreamStateNew {
		return pipe.AssistantMessage{}, fmt.Errorf("gemini: no data received yet")
	}
	return s.msg, nil
}

func (s *stream) Close() error {
	if s.state != pipe.StreamStateComplete && s.state != pipe.StreamStateError {
		s.state = pipe.StreamStateClosed
		s.msg.StopReason = pipe.StopAborted
		s.msg.RawStopReason = "aborted"
	}
	s.stop()
	return nil
}
