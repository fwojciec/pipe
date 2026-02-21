package pipe_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
)

func TestDefaultTheme(t *testing.T) {
	t.Parallel()

	theme := pipe.DefaultTheme()

	assert.Equal(t, 4, theme.UserMsg)
	assert.Equal(t, 1, theme.Error)
	assert.Equal(t, 3, theme.ToolCall)
	assert.Equal(t, 8, theme.Thinking)
	assert.Equal(t, 2, theme.Success)
	assert.Equal(t, 8, theme.Muted)
	assert.Equal(t, 0, theme.CodeBg)
	assert.Equal(t, 5, theme.Accent)
}
