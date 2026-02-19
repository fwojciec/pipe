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
