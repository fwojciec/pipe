package pipe_test

import (
	"errors"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamState_ZeroValue(t *testing.T) {
	t.Parallel()
	var s pipe.StreamState
	assert.Equal(t, pipe.StreamStateNew, s, "zero-value StreamState should be StreamStateNew")
}

func TestRequest_ZeroValue(t *testing.T) {
	t.Parallel()
	var r pipe.Request
	assert.Empty(t, r.Model)
	assert.Empty(t, r.SystemPrompt)
	assert.Nil(t, r.Messages)
	assert.Nil(t, r.Tools)
	assert.Equal(t, 0, r.MaxTokens)
	assert.Nil(t, r.Temperature)
}

func TestRequest_ValuePassingPreventsAppendMutation(t *testing.T) {
	t.Parallel()
	original := pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}}},
		},
		Tools: []pipe.Tool{
			{Name: "read", Description: "Read a file"},
		},
	}

	// Simulate what a provider receiving Request by value would do.
	mutate := func(req pipe.Request) {
		req.Messages = append(req.Messages, pipe.AssistantMessage{
			Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}},
		})
		req.Tools = append(req.Tools, pipe.Tool{Name: "write", Description: "Write a file"})
	}
	mutate(original)

	assert.Len(t, original.Messages, 1, "caller's Messages slice must not grow after provider appends")
	assert.Len(t, original.Tools, 1, "caller's Tools slice must not grow after provider appends")
}

func TestRequest_ValuePassingSharesUnderlyingArray(t *testing.T) {
	t.Parallel()
	original := pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}}},
		},
		Tools: []pipe.Tool{
			{Name: "read", Description: "Read a file"},
		},
	}

	// Modifying existing elements through a by-value copy mutates the
	// caller's data because slice headers share the underlying array.
	// This test documents the caveat noted on the Provider interface.
	mutate := func(req pipe.Request) {
		req.Messages[0] = pipe.UserMessage{
			Content: []pipe.ContentBlock{pipe.TextBlock{Text: "replaced"}},
		}
		req.Tools[0] = pipe.Tool{Name: "write", Description: "Write a file"}
	}
	mutate(original)

	msg, ok := original.Messages[0].(pipe.UserMessage)
	assert.True(t, ok, "Messages[0] should still be a UserMessage")
	tb, ok := msg.Content[0].(pipe.TextBlock)
	assert.True(t, ok, "Content[0] should still be a TextBlock")
	assert.Equal(t, "replaced", tb.Text, "existing element mutation leaks through shared backing array")
	assert.Equal(t, "write", original.Tools[0].Name, "existing element mutation leaks through shared backing array")
}

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
