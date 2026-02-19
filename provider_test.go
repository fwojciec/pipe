package pipe_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
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
