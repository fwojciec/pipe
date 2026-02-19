package pipe_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_Validate_ValidDefaults(t *testing.T) {
	t.Parallel()
	r := pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}}},
		},
	}
	assert.NoError(t, r.Validate())
}

func TestRequest_Validate_ValidWithAllFields(t *testing.T) {
	t.Parallel()
	temp := 1.0
	r := pipe.Request{
		Model:        "claude-3-opus",
		SystemPrompt: "You are helpful.",
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}}},
		},
		Tools:       []pipe.Tool{{Name: "read", Description: "Read a file"}},
		MaxTokens:   4096,
		Temperature: &temp,
	}
	assert.NoError(t, r.Validate())
}

func TestRequest_Validate_TemperatureBounds(t *testing.T) {
	t.Parallel()

	t.Run("nil temperature is valid", func(t *testing.T) {
		t.Parallel()
		r := pipe.Request{
			Messages: []pipe.Message{
				pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}},
			},
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("temperature 0 is valid", func(t *testing.T) {
		t.Parallel()
		temp := 0.0
		r := pipe.Request{
			Messages:    []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}}},
			Temperature: &temp,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("temperature 2 is valid", func(t *testing.T) {
		t.Parallel()
		temp := 2.0
		r := pipe.Request{
			Messages:    []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}}},
			Temperature: &temp,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("temperature 1.5 is valid", func(t *testing.T) {
		t.Parallel()
		temp := 1.5
		r := pipe.Request{
			Messages:    []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}}},
			Temperature: &temp,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("negative temperature is invalid", func(t *testing.T) {
		t.Parallel()
		temp := -0.1
		r := pipe.Request{
			Messages:    []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}}},
			Temperature: &temp,
		}
		err := r.Validate()
		require.Error(t, err)
		assert.True(t, errors.Is(err, pipe.ErrValidation))
		assert.Contains(t, err.Error(), "temperature")
	})

	t.Run("temperature above 2 is invalid", func(t *testing.T) {
		t.Parallel()
		temp := 2.1
		r := pipe.Request{
			Messages:    []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}}},
			Temperature: &temp,
		}
		err := r.Validate()
		require.Error(t, err)
		assert.True(t, errors.Is(err, pipe.ErrValidation))
		assert.Contains(t, err.Error(), "temperature")
	})
}

func TestRequest_Validate_MaxTokens(t *testing.T) {
	t.Parallel()

	t.Run("zero max_tokens is valid (provider default)", func(t *testing.T) {
		t.Parallel()
		r := pipe.Request{
			Messages: []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}}},
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("positive max_tokens is valid", func(t *testing.T) {
		t.Parallel()
		r := pipe.Request{
			Messages:  []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}}},
			MaxTokens: 1024,
		}
		assert.NoError(t, r.Validate())
	})

	t.Run("negative max_tokens is invalid", func(t *testing.T) {
		t.Parallel()
		r := pipe.Request{
			Messages:  []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}}},
			MaxTokens: -1,
		}
		err := r.Validate()
		require.Error(t, err)
		assert.True(t, errors.Is(err, pipe.ErrValidation))
		assert.Contains(t, err.Error(), "max_tokens")
	})
}

func TestValidateMessage_UserMessage(t *testing.T) {
	t.Parallel()

	t.Run("text block is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}}}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("image block is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.UserMessage{Content: []pipe.ContentBlock{pipe.ImageBlock{Data: []byte{0x89}, MimeType: "image/png"}}}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("tool call block is invalid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.UserMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{}`)},
		}}
		err := pipe.ValidateMessage(msg)
		require.Error(t, err)
		assert.True(t, errors.Is(err, pipe.ErrValidation))
		assert.Contains(t, err.Error(), "ToolCallBlock")
		assert.Contains(t, err.Error(), "user")
	})

	t.Run("thinking block is invalid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.UserMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "hmm"},
		}}
		err := pipe.ValidateMessage(msg)
		require.Error(t, err)
		assert.True(t, errors.Is(err, pipe.ErrValidation))
		assert.Contains(t, err.Error(), "ThinkingBlock")
		assert.Contains(t, err.Error(), "user")
	})
}

func TestValidateMessage_AssistantMessage(t *testing.T) {
	t.Parallel()

	t.Run("text block is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.AssistantMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}}}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("tool call block is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{}`)},
		}}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("thinking block is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "reasoning..."},
		}}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("image block is invalid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ImageBlock{Data: []byte{0x89}, MimeType: "image/png"},
		}}
		err := pipe.ValidateMessage(msg)
		require.Error(t, err)
		assert.True(t, errors.Is(err, pipe.ErrValidation))
		assert.Contains(t, err.Error(), "ImageBlock")
		assert.Contains(t, err.Error(), "assistant")
	})
}

func TestValidateMessage_ToolResultMessage(t *testing.T) {
	t.Parallel()

	t.Run("text block is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.ToolResultMessage{ToolCallID: "tc_1", ToolName: "read", Content: []pipe.ContentBlock{pipe.TextBlock{Text: "contents"}}}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("image block is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.ToolResultMessage{ToolCallID: "tc_1", ToolName: "read", Content: []pipe.ContentBlock{
			pipe.ImageBlock{Data: []byte{0x89}, MimeType: "image/png"},
		}}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("tool call block is invalid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.ToolResultMessage{ToolCallID: "tc_1", ToolName: "read", Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "tc_2", Name: "write", Arguments: json.RawMessage(`{}`)},
		}}
		err := pipe.ValidateMessage(msg)
		require.Error(t, err)
		assert.True(t, errors.Is(err, pipe.ErrValidation))
		assert.Contains(t, err.Error(), "ToolCallBlock")
		assert.Contains(t, err.Error(), "tool_result")
	})

	t.Run("thinking block is invalid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.ToolResultMessage{ToolCallID: "tc_1", ToolName: "read", Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "hmm"},
		}}
		err := pipe.ValidateMessage(msg)
		require.Error(t, err)
		assert.True(t, errors.Is(err, pipe.ErrValidation))
		assert.Contains(t, err.Error(), "ThinkingBlock")
		assert.Contains(t, err.Error(), "tool_result")
	})
}

func TestValidateMessage_EmptyContent(t *testing.T) {
	t.Parallel()

	t.Run("user message with empty content is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.UserMessage{}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("assistant message with empty content is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.AssistantMessage{}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})

	t.Run("tool result message with empty content is valid", func(t *testing.T) {
		t.Parallel()
		msg := pipe.ToolResultMessage{ToolCallID: "tc_1", ToolName: "read"}
		assert.NoError(t, pipe.ValidateMessage(msg))
	})
}
