package anthropic_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/anthropic"
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
