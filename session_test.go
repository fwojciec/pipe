package pipe_test

import (
	"testing"
	"time"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
)

func TestSession_Fields(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := pipe.Session{
		ID:           "sess-123",
		Messages:     []pipe.Message{pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}}}},
		SystemPrompt: "You are helpful.",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	assert.Equal(t, "sess-123", s.ID)
	assert.Len(t, s.Messages, 1)
	assert.Equal(t, "You are helpful.", s.SystemPrompt)
	assert.Equal(t, now, s.CreatedAt)
	assert.Equal(t, now, s.UpdatedAt)
}
