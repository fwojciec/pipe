package anthropic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_RequestFormat(t *testing.T) {
	t.Parallel()

	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)

		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-api-key", r.Header.Get("X-Api-Key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("Anthropic-Version"))
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/messages", r.URL.Path)

		// Return minimal valid SSE response.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":0}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	temp := 0.7
	client := anthropic.New("test-api-key", anthropic.WithBaseURL(srv.URL))
	s, err := client.Stream(context.Background(), pipe.Request{
		Model:        "claude-opus-4-20250514",
		SystemPrompt: "You are helpful.",
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hello"}}},
			pipe.AssistantMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Thanks"}}},
		},
		Tools: []pipe.Tool{
			{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
		MaxTokens:   1024,
		Temperature: &temp,
	})
	require.NoError(t, err)
	defer s.Close()

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(captured, &body))

	assert.Equal(t, "claude-opus-4-20250514", body["model"])
	assert.Equal(t, float64(1024), body["max_tokens"])
	assert.Equal(t, true, body["stream"])
	assert.Equal(t, "You are helpful.", body["system"])
	assert.Equal(t, 0.7, body["temperature"])

	msgs := body["messages"].([]interface{})
	require.Len(t, msgs, 3)

	msg0 := msgs[0].(map[string]interface{})
	assert.Equal(t, "user", msg0["role"])
	content0 := msg0["content"].([]interface{})
	require.Len(t, content0, 1)
	block0 := content0[0].(map[string]interface{})
	assert.Equal(t, "text", block0["type"])
	assert.Equal(t, "Hello", block0["text"])

	tools := body["tools"].([]interface{})
	require.Len(t, tools, 1)
	tool0 := tools[0].(map[string]interface{})
	assert.Equal(t, "read", tool0["name"])
	assert.Equal(t, "Read a file", tool0["description"])
}

func TestClient_DefaultModelAndMaxTokens(t *testing.T) {
	t.Parallel()

	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":0}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
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

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(captured, &body))

	assert.Equal(t, "claude-sonnet-4-20250514", body["model"])
	assert.Equal(t, float64(8192), body["max_tokens"])
}

func TestClient_ToolResultMessagesMerged(t *testing.T) {
	t.Parallel()

	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":0}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	client := anthropic.New("test-key", anthropic.WithBaseURL(srv.URL))
	s, err := client.Stream(context.Background(), pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
			pipe.AssistantMessage{Content: []pipe.ContentBlock{
				pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{"path":"a.go"}`)},
				pipe.ToolCallBlock{ID: "tc_2", Name: "read", Arguments: json.RawMessage(`{"path":"b.go"}`)},
			}},
			pipe.ToolResultMessage{ToolCallID: "tc_1", ToolName: "read", Content: []pipe.ContentBlock{pipe.TextBlock{Text: "file a"}}},
			pipe.ToolResultMessage{ToolCallID: "tc_2", ToolName: "read", Content: []pipe.ContentBlock{pipe.TextBlock{Text: "file b"}}},
		},
	})
	require.NoError(t, err)
	defer s.Close()

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(captured, &body))

	msgs := body["messages"].([]interface{})
	// UserMessage, AssistantMessage, merged ToolResultMessage = 3 messages
	require.Len(t, msgs, 3)

	toolResultMsg := msgs[2].(map[string]interface{})
	assert.Equal(t, "user", toolResultMsg["role"])
	blocks := toolResultMsg["content"].([]interface{})
	require.Len(t, blocks, 2)

	block0 := blocks[0].(map[string]interface{})
	assert.Equal(t, "tool_result", block0["type"])
	assert.Equal(t, "tc_1", block0["tool_use_id"])

	block1 := blocks[1].(map[string]interface{})
	assert.Equal(t, "tool_result", block1["type"])
	assert.Equal(t, "tc_2", block1["tool_use_id"])
}

func TestClient_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: integer above 1 expected"}}`))
	}))
	defer srv.Close()

	client := anthropic.New("test-key", anthropic.WithBaseURL(srv.URL))
	_, err := client.Stream(context.Background(), pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_request_error")
	assert.Contains(t, err.Error(), "max_tokens")
}

func TestClient_HTTPErrorNonJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client := anthropic.New("test-key", anthropic.WithBaseURL(srv.URL))
	_, err := client.Stream(context.Background(), pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
