package builtin_test

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/builtin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBashTool(t *testing.T) {
	t.Parallel()

	t.Run("returns tool definition with correct schema", func(t *testing.T) {
		t.Parallel()
		tool := builtin.BashTool()
		assert.Equal(t, "bash", tool.Name)
		assert.NotEmpty(t, tool.Description)

		var schema map[string]any
		err := json.Unmarshal(tool.Parameters, &schema)
		require.NoError(t, err)

		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)

		_, hasCommand := props["command"]
		assert.True(t, hasCommand, "schema should have command property")

		_, hasTimeout := props["timeout"]
		assert.True(t, hasTimeout, "schema should have timeout property")
	})

	t.Run("executes simple command and returns output", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"command": "echo hello"}`)
		result, err := builtin.ExecuteBash(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Len(t, result.Content, 1)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Equal(t, "hello\n", text.Text)
	})

	t.Run("captures stderr in output", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"command": "echo error >&2"}`)
		result, err := builtin.ExecuteBash(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Len(t, result.Content, 1)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "error")
	})

	t.Run("returns domain error for non-zero exit code", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"command": "exit 1"}`)
		result, err := builtin.ExecuteBash(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		args := json.RawMessage(`{"command": "sleep 10"}`)
		result, err := builtin.ExecuteBash(ctx, args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("respects timeout argument", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("sleep command differs on Windows")
		}
		args := json.RawMessage(`{"command": "sleep 10", "timeout": 100}`)
		start := time.Now()
		result, err := builtin.ExecuteBash(context.Background(), args)
		elapsed := time.Since(start)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Less(t, elapsed, 5*time.Second, "should timeout quickly")
	})

	t.Run("returns error for missing command", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{}`)
		result, err := builtin.ExecuteBash(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "command")
	})

	t.Run("returns error for invalid JSON args", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{invalid`)
		result, err := builtin.ExecuteBash(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("captures combined stdout and stderr", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"command": "echo out && echo err >&2"}`)
		result, err := builtin.ExecuteBash(context.Background(), args)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "out")
		assert.Contains(t, text.Text, "err")
	})

	t.Run("handles multi-line output", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"command": "echo line1 && echo line2 && echo line3"}`)
		result, err := builtin.ExecuteBash(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		lines := strings.Split(strings.TrimSpace(text.Text), "\n")
		assert.Len(t, lines, 3)
	})
}
