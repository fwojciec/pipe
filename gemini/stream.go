package gemini

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"

	"github.com/fwojciec/pipe"
	"google.golang.org/genai"
)

type streamError string

func (e streamError) Error() string { return string(e) }

// ErrStreamClosed is returned by [stream.Next] after [stream.Close] has been
// called. Callers can distinguish a closed stream from a completed one
// (io.EOF) or an error (wrapped in the returned error).
const ErrStreamClosed streamError = "gemini: stream closed"

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
	textBuf   strings.Builder
	signature []byte
}

// Interface compliance check.
var _ pipe.Stream = (*stream)(nil)

func newStream(ctx context.Context, iterFn iter.Seq2[*genai.GenerateContentResponse, error]) *stream {
	next, stop := iter.Pull2(iterFn)
	return &stream{
		ctx:   ctx,
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
		return nil, ErrStreamClosed
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

		if err := s.processChunk(resp); err != nil {
			s.terminate(err)
			return nil, s.err
		}
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
	if s.state != pipe.StreamStateComplete && s.state != pipe.StreamStateError && s.state != pipe.StreamStateClosed {
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
	s.stop() // Release iter.Pull2 goroutine.
	if s.ctx.Err() != nil {
		s.msg.StopReason = pipe.StopAborted
		s.msg.RawStopReason = "aborted"
	} else if s.msg.StopReason != pipe.StopError {
		// Preserve StopError if already set (e.g. blocked prompt), but
		// overwrite non-error reasons like StopEndTurn.
		s.msg.StopReason = pipe.StopError
		s.msg.RawStopReason = "error"
	}
}

func (s *stream) finalize() {
	s.state = pipe.StreamStateComplete
	s.stop() // Release iter.Pull2 goroutine (idempotent).
	if s.hasToolCall && (s.msg.StopReason == "" || s.msg.StopReason == pipe.StopEndTurn) {
		s.msg.StopReason = pipe.StopToolUse
		s.msg.RawStopReason = "tool_use"
	} else if s.msg.StopReason == "" {
		s.msg.StopReason = pipe.StopEndTurn
		s.msg.RawStopReason = "end_turn"
	}
}

func (s *stream) processChunk(resp *genai.GenerateContentResponse) error {
	// UsageMetadata is overwritten (not accumulated) because the Gemini SDK
	// provides cumulative totals in the final chunk, not incremental deltas.
	if resp.UsageMetadata != nil {
		cached := int(resp.UsageMetadata.CachedContentTokenCount)
		// PromptTokenCount includes CachedContentTokenCount; subtract to get
		// non-cached input tokens. Guard below handles SDK semantic changes.
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

	// A blocked prompt arrives with PromptFeedback and zero candidates.
	if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != "" && len(resp.Candidates) == 0 {
		s.msg.StopReason = pipe.StopError
		s.msg.RawStopReason = string(resp.PromptFeedback.BlockReason)
		return fmt.Errorf("prompt blocked: %s", resp.PromptFeedback.BlockReason)
	}

	if len(resp.Candidates) == 0 {
		return nil
	}
	candidate := resp.Candidates[0]

	if candidate.FinishReason != "" {
		s.msg.RawStopReason = string(candidate.FinishReason)
		s.msg.StopReason = mapFinishReason(candidate.FinishReason)
	}

	if candidate.Content == nil {
		return nil
	}

	for _, part := range candidate.Content.Parts {
		if err := s.processPart(part); err != nil {
			return err
		}
	}
	return nil
}

func (s *stream) processPart(part *genai.Part) error {
	switch {
	case part.FunctionCall != nil:
		s.hasToolCall = true
		args := part.FunctionCall.Args
		if args == nil {
			args = map[string]any{}
		}
		rawArgs, err := json.Marshal(args)
		if err != nil {
			return fmt.Errorf("invalid tool call arguments: %w", err)
		}
		id := part.FunctionCall.ID
		if id == "" {
			var err error
			id, err = generateToolCallID()
			if err != nil {
				return fmt.Errorf("processing function call: %w", err)
			}
		}
		call := pipe.ToolCallBlock{
			ID:        id,
			Name:      part.FunctionCall.Name,
			Arguments: json.RawMessage(rawArgs),
		}
		s.msg.Content = append(s.msg.Content, call)
		s.blocks = append(s.blocks, &blockState{blockType: "tool_call"})
		s.pending = append(s.pending,
			pipe.EventToolCallBegin{ID: id, Name: part.FunctionCall.Name},
			pipe.EventToolCallEnd{Call: call},
		)

	case part.Thought:
		idx := s.currentBlockIndex("thinking")
		bs := s.blocks[idx]
		bs.textBuf.WriteString(part.Text)
		if len(part.ThoughtSignature) > 0 {
			bs.signature = append(bs.signature, part.ThoughtSignature...)
		}
		s.msg.Content[idx] = pipe.ThinkingBlock{Thinking: bs.textBuf.String(), Signature: slices.Clone(bs.signature)}
		if part.Text != "" {
			s.pending = append(s.pending, pipe.EventThinkingDelta{Index: idx, Delta: part.Text})
		}

	case part.Text != "":
		idx := s.currentBlockIndex("text")
		bs := s.blocks[idx]
		bs.textBuf.WriteString(part.Text)
		s.msg.Content[idx] = pipe.TextBlock{Text: bs.textBuf.String()}
		s.pending = append(s.pending, pipe.EventTextDelta{Index: idx, Delta: part.Text})
	}
	return nil
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
func generateToolCallID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating tool call ID: %w", err)
	}
	return "call_" + hex.EncodeToString(b), nil
}
