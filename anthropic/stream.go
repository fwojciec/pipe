package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/fwojciec/pipe"
)

// stream implements [pipe.Stream] by parsing SSE events from an HTTP response body.
type stream struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	ctx     context.Context
	state   pipe.StreamState
	msg     pipe.AssistantMessage
	blocks  map[int]*blockState
	err     error // terminal error, if any
}

// blockState tracks the state of a content block being assembled.
type blockState struct {
	blockType   string
	toolID      string
	toolName    string
	inputBuf    strings.Builder
	textBuf     strings.Builder
	thinkingBuf strings.Builder
}

// Interface compliance check.
var _ pipe.Stream = (*stream)(nil)

func newStream(ctx context.Context, body io.ReadCloser) *stream {
	return &stream{
		body:    body,
		scanner: bufio.NewScanner(body),
		ctx:     ctx,
		state:   pipe.StreamStateNew,
		blocks:  make(map[int]*blockState),
	}
}

// Next reads the next semantic event from the SSE stream.
// Returns io.EOF when the stream completes normally.
func (s *stream) Next() (pipe.Event, error) {
	switch s.state {
	case pipe.StreamStateComplete:
		return nil, io.EOF
	case pipe.StreamStateError:
		return nil, s.err
	case pipe.StreamStateClosed:
		return nil, fmt.Errorf("anthropic: stream closed")
	}

	for {
		eventType, data, err := s.readSSEEvent()
		if err != nil {
			s.terminate(err)
			return nil, s.err
		}

		s.state = pipe.StreamStateStreaming

		evt, err := s.processEvent(eventType, data)
		if err != nil {
			s.terminate(err)
			return nil, s.err
		}

		// processEvent may set a terminal state (e.g. message_stop).
		if s.state == pipe.StreamStateComplete {
			return nil, io.EOF
		}

		if evt != nil {
			return evt, nil
		}
		// Non-semantic event (ping, message_start, etc.) - keep reading.
	}
}

// State returns the current stream state.
func (s *stream) State() pipe.StreamState {
	return s.state
}

// Message returns the assembled AssistantMessage.
func (s *stream) Message() (pipe.AssistantMessage, error) {
	if s.state == pipe.StreamStateNew {
		return pipe.AssistantMessage{}, fmt.Errorf("anthropic: no data received yet")
	}
	return s.msg, nil
}

// Close closes the underlying HTTP response body.
func (s *stream) Close() error {
	if s.state != pipe.StreamStateComplete && s.state != pipe.StreamStateError {
		s.state = pipe.StreamStateClosed
		s.msg.StopReason = pipe.StopAborted
		s.msg.RawStopReason = "aborted"
	}
	return s.body.Close()
}

// terminate records a terminal error and sets the appropriate state and stop reason.
func (s *stream) terminate(err error) {
	if err == io.EOF {
		// Normal completion via message_stop should set StreamStateComplete
		// before we reach here. If we get raw EOF, the stream ended unexpectedly.
		s.state = pipe.StreamStateError
		s.err = fmt.Errorf("anthropic: unexpected end of stream")
		s.msg.StopReason = pipe.StopError
		s.msg.RawStopReason = "error"
		return
	}
	s.state = pipe.StreamStateError
	s.err = err
	if s.ctx.Err() != nil {
		s.msg.StopReason = pipe.StopAborted
		s.msg.RawStopReason = "aborted"
	} else {
		s.msg.StopReason = pipe.StopError
		s.msg.RawStopReason = "error"
	}
}

// readSSEEvent reads lines until a complete SSE event is assembled.
// Returns the event type and the data payload.
func (s *stream) readSSEEvent() (string, string, error) {
	var eventType string
	var dataBuf strings.Builder

	for s.scanner.Scan() {
		line := s.scanner.Text()

		if line == "" {
			// Empty line signals end of event.
			if dataBuf.Len() > 0 {
				return eventType, dataBuf.String(), nil
			}
			// Empty event, keep reading.
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimPrefix(line, "data: "))
		}
		// Ignore comments (lines starting with ':') and unknown fields.
	}

	if err := s.scanner.Err(); err != nil {
		return "", "", fmt.Errorf("anthropic: %w", err)
	}

	// Scanner exhausted without error = EOF.
	if dataBuf.Len() > 0 {
		return eventType, dataBuf.String(), nil
	}
	return "", "", io.EOF
}

// processEvent maps an SSE event to a semantic pipe.Event.
// Returns nil event for non-semantic events (ping, message_start, etc.).
func (s *stream) processEvent(eventType, data string) (pipe.Event, error) {
	switch eventType {
	case "message_start":
		return nil, s.handleMessageStart(data)
	case "content_block_start":
		return s.handleContentBlockStart(data)
	case "content_block_delta":
		return s.handleContentBlockDelta(data)
	case "content_block_stop":
		return s.handleContentBlockStop(data)
	case "message_delta":
		return nil, s.handleMessageDelta(data)
	case "message_stop":
		s.state = pipe.StreamStateComplete
		return nil, nil
	case "ping":
		return nil, nil
	case "error":
		return nil, s.handleError(data)
	default:
		// Unknown event types are ignored per the API spec.
		return nil, nil
	}
}

func (s *stream) handleMessageStart(data string) error {
	var evt sseMessageStart
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return fmt.Errorf("anthropic: failed to parse message_start: %w", err)
	}
	s.msg.Usage.InputTokens = evt.Message.Usage.InputTokens
	return nil
}

func (s *stream) handleContentBlockStart(data string) (pipe.Event, error) {
	var evt sseContentBlockStart
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse content_block_start: %w", err)
	}

	bs := &blockState{blockType: evt.ContentBlock.Type}
	s.blocks[evt.Index] = bs

	// Grow content slice to accommodate this index.
	for len(s.msg.Content) <= evt.Index {
		s.msg.Content = append(s.msg.Content, nil)
	}

	switch evt.ContentBlock.Type {
	case "tool_use":
		bs.toolID = evt.ContentBlock.ID
		bs.toolName = evt.ContentBlock.Name
		s.msg.Content[evt.Index] = pipe.ToolCallBlock{ID: bs.toolID, Name: bs.toolName}
		return pipe.EventToolCallBegin{ID: evt.ContentBlock.ID, Name: evt.ContentBlock.Name}, nil
	case "text":
		// No semantic event for text block start.
		return nil, nil
	case "thinking":
		// No semantic event for thinking block start.
		return nil, nil
	default:
		return nil, nil
	}
}

func (s *stream) handleContentBlockDelta(data string) (pipe.Event, error) {
	var evt sseContentBlockDelta
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse content_block_delta: %w", err)
	}

	bs := s.blocks[evt.Index]
	if bs == nil {
		return nil, fmt.Errorf("anthropic: delta for unknown block index %d", evt.Index)
	}

	switch evt.Delta.Type {
	case "text_delta":
		bs.textBuf.WriteString(evt.Delta.Text)
		s.msg.Content[evt.Index] = pipe.TextBlock{Text: bs.textBuf.String()}
		return pipe.EventTextDelta{Index: evt.Index, Delta: evt.Delta.Text}, nil
	case "input_json_delta":
		bs.inputBuf.WriteString(evt.Delta.PartialJSON)
		return pipe.EventToolCallDelta{ID: bs.toolID, Delta: evt.Delta.PartialJSON}, nil
	case "thinking_delta":
		bs.thinkingBuf.WriteString(evt.Delta.Thinking)
		s.msg.Content[evt.Index] = pipe.ThinkingBlock{Thinking: bs.thinkingBuf.String()}
		return pipe.EventThinkingDelta{Index: evt.Index, Delta: evt.Delta.Thinking}, nil
	case "signature_delta":
		// Internal use only; not exposed as a semantic event.
		return nil, nil
	default:
		return nil, nil
	}
}

func (s *stream) handleContentBlockStop(data string) (pipe.Event, error) {
	var evt sseContentBlockStop
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse content_block_stop: %w", err)
	}

	bs := s.blocks[evt.Index]
	if bs == nil {
		return nil, fmt.Errorf("anthropic: stop for unknown block index %d", evt.Index)
	}

	switch bs.blockType {
	case "tool_use":
		raw := bs.inputBuf.String()
		if raw == "" {
			raw = "{}"
		}
		call := pipe.ToolCallBlock{
			ID:        bs.toolID,
			Name:      bs.toolName,
			Arguments: json.RawMessage(raw),
		}
		s.msg.Content[evt.Index] = call
		return pipe.EventToolCallEnd{Call: call}, nil
	default:
		return nil, nil
	}
}

func (s *stream) handleMessageDelta(data string) error {
	var evt sseMessageDelta
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return fmt.Errorf("anthropic: failed to parse message_delta: %w", err)
	}

	s.msg.Usage.OutputTokens = evt.Usage.OutputTokens

	if evt.Delta.StopReason != nil {
		s.msg.RawStopReason = *evt.Delta.StopReason
		s.msg.StopReason = mapStopReason(*evt.Delta.StopReason)
	}

	return nil
}

func (s *stream) handleError(data string) error {
	var evt sseError
	if err := json.Unmarshal([]byte(data), &evt); err != nil {
		return fmt.Errorf("anthropic: failed to parse error event: %w", err)
	}
	return fmt.Errorf("anthropic: %s: %s", evt.Error.Type, evt.Error.Message)
}

func mapStopReason(raw string) pipe.StopReason {
	switch raw {
	case "end_turn":
		return pipe.StopEndTurn
	case "max_tokens":
		return pipe.StopLength
	case "tool_use":
		return pipe.StopToolUse
	default:
		return pipe.StopUnknown
	}
}
