package anthropic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseResponse is a helper to build SSE responses for tests.
type sseResponse struct {
	events []sseEvent
}

type sseEvent struct {
	event string
	data  string
}

func (s sseResponse) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, evt := range s.events {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.event, evt.data)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// textStreamResponse returns a simple text streaming SSE response.
func textStreamResponse() sseResponse {
	return sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"ping", `{"type":"ping"}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}
}

func streamFromSSE(t *testing.T, resp sseResponse) pipe.Stream {
	t.Helper()
	srv := httptest.NewServer(resp.handler())
	t.Cleanup(srv.Close)
	client := anthropic.New("test-key", anthropic.WithBaseURL(srv.URL))
	stream, err := client.Stream(context.Background(), pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { stream.Close() })
	return stream
}

func collectEvents(t *testing.T, s pipe.Stream) []pipe.Event {
	t.Helper()
	var events []pipe.Event
	for {
		evt, err := s.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		events = append(events, evt)
	}
	return events
}

func TestStream_TextResponse(t *testing.T) {
	t.Parallel()
	s := streamFromSSE(t, textStreamResponse())

	events := collectEvents(t, s)

	assert.Len(t, events, 2)
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: "Hello"}, events[0])
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: " world"}, events[1])

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopEndTurn, msg.StopReason)
	assert.Equal(t, "end_turn", msg.RawStopReason)
	assert.Equal(t, 10, msg.Usage.InputTokens)
	assert.Equal(t, 5, msg.Usage.OutputTokens)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, pipe.TextBlock{Text: "Hello world"}, msg.Content[0])
}

func TestStream_ToolUse(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":1}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check."}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read","input":{}}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":" \"foo.go\"}"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	events := collectEvents(t, s)

	require.Len(t, events, 6)
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: "Let me check."}, events[0])
	assert.Equal(t, pipe.EventToolCallBegin{ID: "toolu_1", Name: "read"}, events[1])
	assert.Equal(t, pipe.EventToolCallDelta{ID: "toolu_1", Delta: ""}, events[2])
	assert.Equal(t, pipe.EventToolCallDelta{ID: "toolu_1", Delta: `{"path":`}, events[3])
	assert.Equal(t, pipe.EventToolCallDelta{ID: "toolu_1", Delta: ` "foo.go"}`}, events[4])
	assert.Equal(t, pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{
		ID:        "toolu_1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path": "foo.go"}`),
	}}, events[5])

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopToolUse, msg.StopReason)
	assert.Equal(t, "tool_use", msg.RawStopReason)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, pipe.TextBlock{Text: "Let me check."}, msg.Content[0])
	assert.Equal(t, pipe.ToolCallBlock{
		ID:        "toolu_1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path": "foo.go"}`),
	}, msg.Content[1])
}

func TestStream_Thinking(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":50,"output_tokens":1}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think..."}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" step 2"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig123"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"The answer is 42."}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":20}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	events := collectEvents(t, s)

	require.Len(t, events, 3)
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: "Let me think..."}, events[0])
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: " step 2"}, events[1])
	assert.Equal(t, pipe.EventTextDelta{Index: 1, Delta: "The answer is 42."}, events[2])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, pipe.ThinkingBlock{Thinking: "Let me think... step 2"}, msg.Content[0])
	assert.Equal(t, pipe.TextBlock{Text: "The answer is 42."}, msg.Content[1])
}

func TestStream_MultipleToolCalls(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":200,"output_tokens":1}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tc_1","name":"read","input":{}}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"a.go\"}"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tc_2","name":"read","input":{}}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"b.go\"}"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":30}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	events := collectEvents(t, s)

	require.Len(t, events, 6)
	assert.IsType(t, pipe.EventToolCallBegin{}, events[0])
	assert.IsType(t, pipe.EventToolCallDelta{}, events[1])
	assert.IsType(t, pipe.EventToolCallEnd{}, events[2])
	assert.IsType(t, pipe.EventToolCallBegin{}, events[3])
	assert.IsType(t, pipe.EventToolCallDelta{}, events[4])
	assert.IsType(t, pipe.EventToolCallEnd{}, events[5])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{"path": "a.go"}`)}, msg.Content[0])
	assert.Equal(t, pipe.ToolCallBlock{ID: "tc_2", Name: "read", Arguments: json.RawMessage(`{"path": "b.go"}`)}, msg.Content[1])
}

func TestStream_State(t *testing.T) {
	t.Parallel()

	t.Run("new before first next", func(t *testing.T) {
		t.Parallel()
		s := streamFromSSE(t, textStreamResponse())
		assert.Equal(t, pipe.StreamStateNew, s.State())
	})

	t.Run("streaming after first next", func(t *testing.T) {
		t.Parallel()
		s := streamFromSSE(t, textStreamResponse())
		_, err := s.Next()
		require.NoError(t, err)
		assert.Equal(t, pipe.StreamStateStreaming, s.State())
	})

	t.Run("complete after EOF", func(t *testing.T) {
		t.Parallel()
		s := streamFromSSE(t, textStreamResponse())
		collectEvents(t, s)
		assert.Equal(t, pipe.StreamStateComplete, s.State())
	})

	t.Run("closed after close mid-stream", func(t *testing.T) {
		t.Parallel()
		s := streamFromSSE(t, textStreamResponse())
		_, err := s.Next()
		require.NoError(t, err)
		require.NoError(t, s.Close())
		assert.Equal(t, pipe.StreamStateClosed, s.State())
	})
}

func TestStream_MessageBeforeNext(t *testing.T) {
	t.Parallel()
	s := streamFromSSE(t, textStreamResponse())
	_, err := s.Message()
	assert.Error(t, err)
}

func TestStream_MessageMidStream(t *testing.T) {
	t.Parallel()
	s := streamFromSSE(t, textStreamResponse())

	_, err := s.Next() // first text delta
	require.NoError(t, err)

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, pipe.TextBlock{Text: "Hello"}, msg.Content[0])
}

func TestStream_CloseAbortsMessage(t *testing.T) {
	t.Parallel()
	s := streamFromSSE(t, textStreamResponse())

	_, err := s.Next()
	require.NoError(t, err)
	require.NoError(t, s.Close())

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopAborted, msg.StopReason)
}

func TestStream_ClosePreservesTerminalState(t *testing.T) {
	t.Parallel()
	s := streamFromSSE(t, textStreamResponse())
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopEndTurn, msg.StopReason)

	// Close after terminal state should preserve the stop reason.
	require.NoError(t, s.Close())
	msg, err = s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopEndTurn, msg.StopReason)
}

func TestStream_NextAfterClose(t *testing.T) {
	t.Parallel()
	s := streamFromSSE(t, textStreamResponse())
	require.NoError(t, s.Close())

	_, err := s.Next()
	assert.Error(t, err)
}

func TestStream_SSEError(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
		{"error", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`},
	}}

	s := streamFromSSE(t, resp)
	_, err := s.Next()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "overloaded_error")
}

func TestStream_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Server that blocks after first event.
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-sonnet-4-20250514\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		close(started)
		// Block until request context is cancelled.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := anthropic.New("test-key", anthropic.WithBaseURL(srv.URL))
	s, err := client.Stream(ctx, pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
		},
	})
	require.NoError(t, err)
	defer s.Close()

	// Read the first event.
	evt, err := s.Next()
	require.NoError(t, err)
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: "Hi"}, evt)

	// Wait for server to block, then cancel.
	<-started
	cancel()

	// Next should return an error.
	_, err = s.Next()
	assert.Error(t, err)

	// Message should have StopAborted.
	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopAborted, msg.StopReason)
	assert.Equal(t, pipe.StreamStateError, s.State())
}

func TestStream_UnknownStopReason(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Ok"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"new_reason","stop_sequence":null},"usage":{"output_tokens":3}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopUnknown, msg.StopReason)
	assert.Equal(t, "new_reason", msg.RawStopReason)
}

func TestStream_MaxTokensStopReason(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"truncated"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null},"usage":{"output_tokens":100}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopLength, msg.StopReason)
	assert.Equal(t, "max_tokens", msg.RawStopReason)
}

func TestStream_CacheUsage(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":50,"cache_read_input_tokens":200}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, 10, msg.Usage.InputTokens)
	assert.Equal(t, 5, msg.Usage.OutputTokens)
	assert.Equal(t, 50, msg.Usage.CacheWriteTokens)
	assert.Equal(t, 200, msg.Usage.CacheReadTokens)
}

func TestStream_CacheUsageCumulative(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":50,"cache_read_input_tokens":200}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5,"cache_creation_input_tokens":10,"cache_read_input_tokens":30}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, 60, msg.Usage.CacheWriteTokens)
	assert.Equal(t, 230, msg.Usage.CacheReadTokens)
}

func TestStream_CacheUsageDeltaAbsent(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":50,"cache_read_input_tokens":200}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	// Cache values from message_start should NOT be overwritten by message_delta absence.
	assert.Equal(t, 50, msg.Usage.CacheWriteTokens)
	assert.Equal(t, 200, msg.Usage.CacheReadTokens)
}

func TestStream_CacheUsageDeltaNull(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":50,"cache_read_input_tokens":200}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5,"cache_creation_input_tokens":null,"cache_read_input_tokens":null}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	// Explicit JSON null must NOT overwrite message_start values.
	assert.Equal(t, 50, msg.Usage.CacheWriteTokens)
	assert.Equal(t, 200, msg.Usage.CacheReadTokens)
}

func TestStream_CacheUsageNull(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":null,"cache_read_input_tokens":null}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	events := collectEvents(t, s)

	// No unmarshal error — stream should complete normally.
	require.Len(t, events, 1)
	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, 0, msg.Usage.CacheWriteTokens)
	assert.Equal(t, 0, msg.Usage.CacheReadTokens)
}

func TestStream_DeltaInputTokens(t *testing.T) {
	t.Parallel()

	t.Run("delta present updates InputTokens", func(t *testing.T) {
		t.Parallel()
		resp := sseResponse{events: []sseEvent{
			{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":1}}}`},
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
			{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5,"input_tokens":100}}`},
			{"message_stop", `{"type":"message_stop"}`},
		}}

		s := streamFromSSE(t, resp)
		collectEvents(t, s)

		msg, err := s.Message()
		require.NoError(t, err)
		assert.Equal(t, 100, msg.Usage.InputTokens)
	})

	t.Run("delta absent preserves message_start InputTokens", func(t *testing.T) {
		t.Parallel()
		resp := sseResponse{events: []sseEvent{
			{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":1}}}`},
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
			{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
			{"message_stop", `{"type":"message_stop"}`},
		}}

		s := streamFromSSE(t, resp)
		collectEvents(t, s)

		msg, err := s.Message()
		require.NoError(t, err)
		assert.Equal(t, 100, msg.Usage.InputTokens)
	})

	t.Run("delta null preserves message_start InputTokens", func(t *testing.T) {
		t.Parallel()
		resp := sseResponse{events: []sseEvent{
			{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":1}}}`},
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
			{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5,"input_tokens":null}}`},
			{"message_stop", `{"type":"message_stop"}`},
		}}

		s := streamFromSSE(t, resp)
		collectEvents(t, s)

		msg, err := s.Message()
		require.NoError(t, err)
		assert.Equal(t, 100, msg.Usage.InputTokens)
	})
}

func TestStream_ReadErrorMidStream(t *testing.T) {
	t.Parallel()

	// Server that sends partial SSE then closes connection abruptly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		// Connection closes without message_stop — simulates network failure.
		// The hijack approach ensures an abrupt close.
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	defer srv.Close()

	client := anthropic.New("test-key", anthropic.WithBaseURL(srv.URL))
	s, err := client.Stream(context.Background(), pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
		},
	})
	require.NoError(t, err)
	defer s.Close()

	// First event should succeed.
	evt, err := s.Next()
	require.NoError(t, err)
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: "partial"}, evt)

	// Next should return an error (unexpected EOF or read error).
	_, err = s.Next()
	assert.Error(t, err)
	assert.Equal(t, pipe.StreamStateError, s.State())

	// Message should have partial content with StopError.
	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopError, msg.StopReason)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, pipe.TextBlock{Text: "partial"}, msg.Content[0])
}
