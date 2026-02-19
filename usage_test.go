package pipe_test

import (
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
)

func TestRole_Values(t *testing.T) {
	t.Parallel()
	assert.Equal(t, pipe.Role("user"), pipe.RoleUser)
	assert.Equal(t, pipe.Role("assistant"), pipe.RoleAssistant)
	assert.Equal(t, pipe.Role("tool_result"), pipe.RoleToolResult)
}

func TestStopReason_Values(t *testing.T) {
	t.Parallel()
	assert.Equal(t, pipe.StopReason("end_turn"), pipe.StopEndTurn)
	assert.Equal(t, pipe.StopReason("length"), pipe.StopLength)
	assert.Equal(t, pipe.StopReason("tool_use"), pipe.StopToolUse)
	assert.Equal(t, pipe.StopReason("error"), pipe.StopError)
	assert.Equal(t, pipe.StopReason("aborted"), pipe.StopAborted)
	assert.Equal(t, pipe.StopReason("unknown"), pipe.StopUnknown)
}

func TestUsage_ZeroValue(t *testing.T) {
	t.Parallel()
	var u pipe.Usage
	assert.Equal(t, 0, u.InputTokens)
	assert.Equal(t, 0, u.OutputTokens)
}
