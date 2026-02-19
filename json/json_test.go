package json_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fwojciec/pipe"
	pipejson "github.com/fwojciec/pipe/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalSession_RoundTrip(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 2, 18, 12, 5, 0, 0, time.UTC)
	ts1 := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 2, 18, 12, 0, 1, 0, time.UTC)
	ts3 := time.Date(2026, 2, 18, 12, 0, 2, 0, time.UTC)

	session := pipe.Session{
		ID:           "sess-123",
		SystemPrompt: "You are helpful.",
		CreatedAt:    created,
		UpdatedAt:    updated,
		Messages: []pipe.Message{
			pipe.UserMessage{
				Content:   []pipe.ContentBlock{pipe.TextBlock{Text: "Fix the login bug"}},
				Timestamp: ts1,
			},
			pipe.AssistantMessage{
				Content: []pipe.ContentBlock{
					pipe.TextBlock{Text: "I'll look at the auth module."},
					pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{"path":"auth.go"}`)},
				},
				StopReason:    pipe.StopToolUse,
				RawStopReason: "tool_use",
				Usage:         pipe.Usage{InputTokens: 150, OutputTokens: 42},
				Timestamp:     ts2,
			},
			pipe.ToolResultMessage{
				ToolCallID: "tc_1",
				ToolName:   "read",
				Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "package auth\n..."}},
				IsError:    false,
				Timestamp:  ts3,
			},
		},
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	got, err := pipejson.UnmarshalSession(data)
	require.NoError(t, err)

	assert.Equal(t, session.ID, got.ID)
	assert.Equal(t, session.SystemPrompt, got.SystemPrompt)
	assert.True(t, session.CreatedAt.Equal(got.CreatedAt), "CreatedAt mismatch")
	assert.True(t, session.UpdatedAt.Equal(got.UpdatedAt), "UpdatedAt mismatch")
	require.Len(t, got.Messages, 3)

	// User message
	um, ok := got.Messages[0].(pipe.UserMessage)
	require.True(t, ok, "expected UserMessage")
	require.Len(t, um.Content, 1)
	assert.Equal(t, "Fix the login bug", um.Content[0].(pipe.TextBlock).Text)
	assert.True(t, ts1.Equal(um.Timestamp))

	// Assistant message
	am, ok := got.Messages[1].(pipe.AssistantMessage)
	require.True(t, ok, "expected AssistantMessage")
	require.Len(t, am.Content, 2)
	assert.Equal(t, "I'll look at the auth module.", am.Content[0].(pipe.TextBlock).Text)
	tc := am.Content[1].(pipe.ToolCallBlock)
	assert.Equal(t, "tc_1", tc.ID)
	assert.Equal(t, "read", tc.Name)
	assert.JSONEq(t, `{"path":"auth.go"}`, string(tc.Arguments))
	assert.Equal(t, pipe.StopToolUse, am.StopReason)
	assert.Equal(t, "tool_use", am.RawStopReason)
	assert.Equal(t, 150, am.Usage.InputTokens)
	assert.Equal(t, 42, am.Usage.OutputTokens)
	assert.True(t, ts2.Equal(am.Timestamp))

	// Tool result message
	trm, ok := got.Messages[2].(pipe.ToolResultMessage)
	require.True(t, ok, "expected ToolResultMessage")
	assert.Equal(t, "tc_1", trm.ToolCallID)
	assert.Equal(t, "read", trm.ToolName)
	require.Len(t, trm.Content, 1)
	assert.Equal(t, "package auth\n...", trm.Content[0].(pipe.TextBlock).Text)
	assert.False(t, trm.IsError)
	assert.True(t, ts3.Equal(trm.Timestamp))
}

func TestMarshalSession_V1Envelope(t *testing.T) {
	t.Parallel()
	session := pipe.Session{
		ID:           "test-id",
		SystemPrompt: "prompt",
		CreatedAt:    time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 2, 18, 12, 5, 0, 0, time.UTC),
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &envelope))

	// Version field must be present and equal to 1
	var version int
	require.NoError(t, json.Unmarshal(envelope["version"], &version))
	assert.Equal(t, 1, version)

	// ID field
	var id string
	require.NoError(t, json.Unmarshal(envelope["id"], &id))
	assert.Equal(t, "test-id", id)

	// system_prompt field (snake_case)
	_, ok := envelope["system_prompt"]
	assert.True(t, ok, "expected system_prompt key in JSON")
}

func TestMarshalSession_AllContentBlockTypes(t *testing.T) {
	t.Parallel()
	session := pipe.Session{
		ID:        "all-blocks",
		CreatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		Messages: []pipe.Message{
			pipe.AssistantMessage{
				Content: []pipe.ContentBlock{
					pipe.TextBlock{Text: "hello"},
					pipe.ThinkingBlock{Thinking: "let me think..."},
					pipe.ToolCallBlock{ID: "tc_1", Name: "bash", Arguments: json.RawMessage(`{"cmd":"ls"}`)},
				},
				StopReason: pipe.StopToolUse,
				Timestamp:  time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
			},
			pipe.UserMessage{
				Content: []pipe.ContentBlock{
					pipe.TextBlock{Text: "with image"},
					pipe.ImageBlock{Data: []byte("fakepng"), MimeType: "image/png"},
				},
				Timestamp: time.Date(2026, 2, 18, 12, 0, 1, 0, time.UTC),
			},
		},
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	got, err := pipejson.UnmarshalSession(data)
	require.NoError(t, err)

	require.Len(t, got.Messages, 2)

	// Assistant message with text, thinking, tool_call
	am, ok := got.Messages[0].(pipe.AssistantMessage)
	require.True(t, ok)
	require.Len(t, am.Content, 3)
	assert.Equal(t, "hello", am.Content[0].(pipe.TextBlock).Text)
	assert.Equal(t, "let me think...", am.Content[1].(pipe.ThinkingBlock).Thinking)
	tc := am.Content[2].(pipe.ToolCallBlock)
	assert.Equal(t, "tc_1", tc.ID)

	// User message with text and image
	um, ok := got.Messages[1].(pipe.UserMessage)
	require.True(t, ok)
	require.Len(t, um.Content, 2)
	assert.Equal(t, "with image", um.Content[0].(pipe.TextBlock).Text)
	img := um.Content[1].(pipe.ImageBlock)
	assert.Equal(t, []byte("fakepng"), img.Data)
	assert.Equal(t, "image/png", img.MimeType)
}

func TestMarshalSession_EmptySession(t *testing.T) {
	t.Parallel()
	session := pipe.Session{
		ID:        "empty",
		CreatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	got, err := pipejson.UnmarshalSession(data)
	require.NoError(t, err)

	assert.Equal(t, "empty", got.ID)
	assert.Empty(t, got.Messages)
}

func TestMarshalSession_ToolResultWithError(t *testing.T) {
	t.Parallel()
	session := pipe.Session{
		ID:        "error-result",
		CreatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		Messages: []pipe.Message{
			pipe.ToolResultMessage{
				ToolCallID: "tc_err",
				ToolName:   "bash",
				Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "command not found"}},
				IsError:    true,
				Timestamp:  time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	got, err := pipejson.UnmarshalSession(data)
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	trm, ok := got.Messages[0].(pipe.ToolResultMessage)
	require.True(t, ok)
	assert.True(t, trm.IsError)
	assert.Equal(t, "tc_err", trm.ToolCallID)
}

func TestMarshalSession_JSONFieldNames(t *testing.T) {
	t.Parallel()
	session := pipe.Session{
		ID:           "field-names",
		SystemPrompt: "test",
		CreatedAt:    time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 2, 18, 12, 5, 0, 0, time.UTC),
		Messages: []pipe.Message{
			pipe.AssistantMessage{
				Content:       []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}},
				StopReason:    pipe.StopEndTurn,
				RawStopReason: "end_turn",
				Usage:         pipe.Usage{InputTokens: 10, OutputTokens: 5},
				Timestamp:     time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	// Verify snake_case field names in JSON
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	// Top-level fields
	assert.Contains(t, raw, "version")
	assert.Contains(t, raw, "id")
	assert.Contains(t, raw, "system_prompt")
	assert.Contains(t, raw, "created_at")
	assert.Contains(t, raw, "updated_at")
	assert.Contains(t, raw, "messages")

	// Message-level fields
	var msgs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["messages"], &msgs))
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "type")
	assert.Contains(t, msgs[0], "stop_reason")
	assert.Contains(t, msgs[0], "raw_stop_reason")
	assert.Contains(t, msgs[0], "usage")
	assert.Contains(t, msgs[0], "timestamp")

	// Usage fields
	var usage map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(msgs[0]["usage"], &usage))
	assert.Contains(t, usage, "input_tokens")
	assert.Contains(t, usage, "output_tokens")
}

func TestSave_And_Load(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")

	session := pipe.Session{
		ID:           "save-load",
		SystemPrompt: "You are helpful.",
		CreatedAt:    time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 2, 18, 12, 5, 0, 0, time.UTC),
		Messages: []pipe.Message{
			pipe.UserMessage{
				Content:   []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}},
				Timestamp: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	err := pipejson.Save(path, session)
	require.NoError(t, err)

	// File should exist
	_, err = os.Stat(path)
	require.NoError(t, err)

	got, err := pipejson.Load(path)
	require.NoError(t, err)

	assert.Equal(t, session.ID, got.ID)
	assert.Equal(t, session.SystemPrompt, got.SystemPrompt)
	require.Len(t, got.Messages, 1)
}

func TestLoad_NonexistentFile(t *testing.T) {
	t.Parallel()
	_, err := pipejson.Load("/nonexistent/path/session.json")
	assert.Error(t, err)
}

func TestSave_CreatesParentDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "session.json")

	session := pipe.Session{
		ID:        "nested-save",
		CreatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
	}

	err := pipejson.Save(path, session)
	require.NoError(t, err)

	got, err := pipejson.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "nested-save", got.ID)
}

func TestUnmarshalSession_UnknownMessageType(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"version": 1,
		"id": "test",
		"created_at": "2026-02-18T12:00:00Z",
		"updated_at": "2026-02-18T12:00:00Z",
		"messages": [
			{"type": "unknown_type", "content": []}
		]
	}`)
	_, err := pipejson.UnmarshalSession(data)
	assert.Error(t, err)
}

func TestUnmarshalSession_UnknownContentBlockType(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"version": 1,
		"id": "test",
		"created_at": "2026-02-18T12:00:00Z",
		"updated_at": "2026-02-18T12:00:00Z",
		"messages": [
			{"type": "user", "content": [{"type": "unknown_block"}], "timestamp": "2026-02-18T12:00:00Z"}
		]
	}`)
	_, err := pipejson.UnmarshalSession(data)
	assert.Error(t, err)
}

func TestUnmarshalSession_UnsupportedVersion(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"version": 99,
		"id": "test",
		"created_at": "2026-02-18T12:00:00Z",
		"updated_at": "2026-02-18T12:00:00Z",
		"messages": []
	}`)
	_, err := pipejson.UnmarshalSession(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported envelope version")
}

func TestMarshalSession_ImageBase64Encoding(t *testing.T) {
	t.Parallel()
	imgData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header bytes
	session := pipe.Session{
		ID:        "image-test",
		CreatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		Messages: []pipe.Message{
			pipe.UserMessage{
				Content: []pipe.ContentBlock{
					pipe.ImageBlock{Data: imgData, MimeType: "image/png"},
				},
				Timestamp: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	got, err := pipejson.UnmarshalSession(data)
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	um := got.Messages[0].(pipe.UserMessage)
	require.Len(t, um.Content, 1)
	img := um.Content[0].(pipe.ImageBlock)
	assert.Equal(t, imgData, img.Data)
	assert.Equal(t, "image/png", img.MimeType)
}

func TestMarshalSession_ToolCallComplexArguments(t *testing.T) {
	t.Parallel()
	complexArgs := json.RawMessage(`{"nested":{"key":"value"},"array":[1,2,3],"bool":true}`)
	session := pipe.Session{
		ID:        "complex-args",
		CreatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		Messages: []pipe.Message{
			pipe.AssistantMessage{
				Content: []pipe.ContentBlock{
					pipe.ToolCallBlock{ID: "tc_1", Name: "test", Arguments: complexArgs},
				},
				StopReason: pipe.StopToolUse,
				Timestamp:  time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	got, err := pipejson.UnmarshalSession(data)
	require.NoError(t, err)

	am := got.Messages[0].(pipe.AssistantMessage)
	tc := am.Content[0].(pipe.ToolCallBlock)
	assert.JSONEq(t, string(complexArgs), string(tc.Arguments))
}
