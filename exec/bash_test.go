package exec_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fwojciec/pipe"
	pipeexec "github.com/fwojciec/pipe/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustJSON marshals v to json.RawMessage, failing the test on error.
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// resultText extracts the text from the first content block of a tool result.
func resultText(t *testing.T, r *pipe.ToolResult) string {
	t.Helper()
	require.NotEmpty(t, r.Content)
	text, ok := r.Content[0].(pipe.TextBlock)
	require.True(t, ok)
	return text.Text
}

func TestBashExecutorTool(t *testing.T) {
	t.Parallel()
	tool := pipeexec.BashExecutorTool()
	assert.Equal(t, "bash", tool.Name)
	assert.NotEmpty(t, tool.Description)

	var schema map[string]any
	err := json.Unmarshal(tool.Parameters, &schema)
	require.NoError(t, err)

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "command")
	assert.Contains(t, props, "timeout")
}

func TestBashExecutor(t *testing.T) {
	t.Parallel()

	t.Run("separates stdout and stderr", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo out && echo err >&2",
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.Contains(t, text, "stdout:\nout\n")
		assert.Contains(t, text, "stderr:\nerr\n")
	})

	t.Run("strips ANSI codes from output", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": `printf '\033[31mred\033[0m\n'`,
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.Contains(t, text, "red")
		assert.NotContains(t, text, "\033")
		assert.NotContains(t, text, "\x1b")
	})

	t.Run("reports non-zero exit code", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo fail && exit 42",
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		text := resultText(t, result)
		assert.Contains(t, text, "exit code: 42")
		assert.Contains(t, text, "fail")
	})

	t.Run("truncates large stdout by line count", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": fmt.Sprintf("seq 1 %d", pipeexec.DefaultMaxLines+1000),
		}))
		require.NoError(t, err)
		require.False(t, result.IsError)
		text := resultText(t, result)
		assert.Contains(t, text, "Showing last")
		assert.NotContains(t, text, "Full output:", "no file offload for line-only truncation")
		assert.Contains(t, text, fmt.Sprintf("%d", pipeexec.DefaultMaxLines+1000))
	})

	t.Run("offloads to file when output exceeds byte threshold", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": `for i in $(seq 1 1000); do printf '%099d\n' $i; done`,
		}))
		require.NoError(t, err)
		require.False(t, result.IsError)
		text := resultText(t, result)

		assert.Contains(t, text, "Showing last")
		assert.Contains(t, text, "Full output:")

		found := false
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "Full output:") {
				path := strings.TrimSpace(strings.Split(line, "Full output:")[1])
				path = strings.TrimSuffix(path, "]")
				path = strings.TrimSpace(path)
				t.Cleanup(func() { os.Remove(path) })
				_, statErr := os.Stat(path)
				assert.NoError(t, statErr, "temp file should exist")
				found = true
				break
			}
		}
		assert.True(t, found, "should have found and verified temp file path")
	})

	t.Run("returns error for invalid JSON args", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), json.RawMessage(`{invalid`))
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("omits empty stderr section", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo hello",
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.NotContains(t, text, "stderr:")
	})

	t.Run("omits empty stdout section", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo err >&2",
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.NotContains(t, text, "stdout:")
		assert.Contains(t, text, "stderr:\nerr\n")
	})

	t.Run("executes simple command", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo hello",
		}))
		require.NoError(t, err)
		require.False(t, result.IsError)
		text := resultText(t, result)
		assert.Contains(t, text, "stdout:\nhello\n")
		assert.Contains(t, text, "exit code: 0")
	})

	t.Run("returns error on cancelled context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(ctx, mustJSON(t, map[string]any{
			"command": "sleep 10",
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		text := resultText(t, result)
		assert.Contains(t, text, "context canceled")
	})

	t.Run("returns error for missing command", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		text := resultText(t, result)
		assert.Contains(t, text, "command")
	})
}
