package gemini

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"iter"

	"github.com/fwojciec/pipe"
	"google.golang.org/genai"
)

// stream implements [pipe.Stream] by wrapping the genai SDK's streaming iterator.
// Each SDK chunk can contain multiple Parts that map to different event types
// (text, thinking, tool calls). The stream uses append-based block assembly:
// new block when part type changes, accumulate into current block when
// consecutive same-type.
type stream struct {
	ctx     context.Context
	pull    func() (*genai.GenerateContentResponse, error, bool)
	stop    func()
	state   pipe.StreamState
	msg     pipe.AssistantMessage
	pending []pipe.Event
	err     error

	blocks      []*blockState
	hasToolCall bool
}

// blockState tracks accumulation for a single content block.
type blockState struct {
	blockType string // "thinking", "text", "tool_call"
	textBuf   string
	signature []byte
}

// Interface compliance check.
var _ pipe.Stream = (*stream)(nil)

func newStream(ctx context.Context, iterFn iter.Seq2[*genai.GenerateContentResponse, error], _ []*genai.Content) *stream {
	next, stop := iter.Pull2(iterFn)
	return &stream{
		ctx:   ctx,
		pull:  next,
		stop:  stop,
		state: pipe.StreamStateNew,
	}
}

// NewStreamFromIter creates a stream from an iter.Seq2. Exported for testing.
func NewStreamFromIter(ctx context.Context, iterFn iter.Seq2[*genai.GenerateContentResponse, error], contents []*genai.Content) pipe.Stream {
	return newStream(ctx, iterFn, contents)
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

	for {
		// Drain pending events first.
		if len(s.pending) > 0 {
			evt := s.pending[0]
			s.pending = s.pending[1:]
			return evt, nil
		}

		// Check context before pulling.
		if s.ctx.Err() != nil {
			s.terminate(s.ctx.Err())
			return nil, s.err
		}

		// Pull next chunk from SDK iterator.
		resp, err, ok := s.pull()
		if !ok {
			s.finalize()
			return nil, io.EOF
		}
		if err != nil {
			s.terminate(err)
			return nil, s.err
		}

		s.state = pipe.StreamStateStreaming

		if resp == nil {
			continue
		}

		s.processChunk(resp)
		// Loop back to check pending events.
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

func (s *stream) terminate(err error) {
	s.state = pipe.StreamStateError
	s.err = fmt.Errorf("gemini: %w", err)
	if s.ctx.Err() != nil {
		s.msg.StopReason = pipe.StopAborted
		s.msg.RawStopReason = "aborted"
	} else {
		s.msg.StopReason = pipe.StopError
		s.msg.RawStopReason = "error"
	}
}

func (s *stream) finalize() {
	s.state = pipe.StreamStateComplete
	if s.hasToolCall {
		s.msg.StopReason = pipe.StopToolUse
		s.msg.RawStopReason = "tool_use"
	}
}

func (s *stream) processChunk(resp *genai.GenerateContentResponse) {
	if resp.UsageMetadata != nil {
		cached := int(resp.UsageMetadata.CachedContentTokenCount)
		input := int(resp.UsageMetadata.PromptTokenCount) - cached
		if input < 0 {
			input = 0
		}
		s.msg.Usage = pipe.Usage{
			InputTokens:     input,
			OutputTokens:    int(resp.UsageMetadata.CandidatesTokenCount),
			CacheReadTokens: cached,
		}
	}

	if len(resp.Candidates) == 0 {
		return
	}
	candidate := resp.Candidates[0]

	if candidate.FinishReason != "" {
		s.msg.RawStopReason = string(candidate.FinishReason)
		s.msg.StopReason = mapFinishReason(candidate.FinishReason)
	}

	if candidate.Content == nil {
		return
	}

	for _, part := range candidate.Content.Parts {
		s.processPart(part)
	}
}

func (s *stream) processPart(part *genai.Part) {
	switch {
	case part.FunctionCall != nil:
		s.hasToolCall = true
		args, _ := json.Marshal(part.FunctionCall.Args)
		id := part.FunctionCall.ID
		if id == "" {
			id = generateToolCallID()
		}
		call := pipe.ToolCallBlock{
			ID:        id,
			Name:      part.FunctionCall.Name,
			Arguments: json.RawMessage(args),
		}
		s.msg.Content = append(s.msg.Content, call)
		s.blocks = append(s.blocks, &blockState{blockType: "tool_call"})
		s.pending = append(s.pending,
			pipe.EventToolCallBegin{ID: id, Name: part.FunctionCall.Name},
			pipe.EventToolCallEnd{Call: call},
		)

	case part.Thought && (part.Text != "" || len(part.ThoughtSignature) > 0):
		idx := s.currentBlockIndex("thinking")
		bs := s.blocks[idx]
		bs.textBuf += part.Text
		if len(part.ThoughtSignature) > 0 {
			bs.signature = append(bs.signature, part.ThoughtSignature...)
		}
		var sig []byte
		if len(bs.signature) > 0 {
			sig = bs.signature
		}
		s.msg.Content[idx] = pipe.ThinkingBlock{Thinking: bs.textBuf, Signature: sig}
		if part.Text != "" {
			s.pending = append(s.pending, pipe.EventThinkingDelta{Index: idx, Delta: part.Text})
		}

	case part.Text != "":
		idx := s.currentBlockIndex("text")
		bs := s.blocks[idx]
		bs.textBuf += part.Text
		s.msg.Content[idx] = pipe.TextBlock{Text: bs.textBuf}
		s.pending = append(s.pending, pipe.EventTextDelta{Index: idx, Delta: part.Text})
	}
}

// currentBlockIndex returns the index of the current block if it matches the
// given type. If the last block is a different type (or no blocks exist), a new
// block is appended.
func (s *stream) currentBlockIndex(blockType string) int {
	if n := len(s.blocks); n > 0 && s.blocks[n-1].blockType == blockType {
		return n - 1
	}
	idx := len(s.blocks)
	s.blocks = append(s.blocks, &blockState{blockType: blockType})
	switch blockType {
	case "thinking":
		s.msg.Content = append(s.msg.Content, pipe.ThinkingBlock{})
	case "text":
		s.msg.Content = append(s.msg.Content, pipe.TextBlock{})
	}
	return idx
}

func mapFinishReason(reason genai.FinishReason) pipe.StopReason {
	switch reason {
	case genai.FinishReasonStop:
		return pipe.StopEndTurn
	case genai.FinishReasonMaxTokens:
		return pipe.StopLength
	case genai.FinishReasonSafety, genai.FinishReasonRecitation,
		genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII, genai.FinishReasonMalformedFunctionCall:
		return pipe.StopError
	default:
		return pipe.StopUnknown
	}
}

// generateToolCallID generates a unique fallback ID for tool calls
// when the SDK doesn't provide one.
func generateToolCallID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "call_" + hex.EncodeToString(b)
}
