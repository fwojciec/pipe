package exec_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	pipeexec "github.com/fwojciec/pipe/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackgroundExecution(t *testing.T) {
	t.Parallel()

	t.Run("auto-backgrounds command on timeout", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		start := time.Now()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "sleep 30",
			"timeout": 200, // 200ms timeout
		}))
		elapsed := time.Since(start)
		require.NoError(t, err)
		assert.Less(t, elapsed, 2*time.Second)

		text := resultText(t, result)
		assert.Contains(t, text, "backgrounded")
		assert.Contains(t, text, "pid")
		assert.False(t, result.IsError)

		// Clean up: kill the backgrounded process.
		pid := extractPID(t, text)
		e.Execute(context.Background(), mustJSON(t, map[string]any{"kill_pid": pid}))
	})

	t.Run("check_pid returns status of running process", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()

		// Start a command that will be backgrounded.
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo started && sleep 30",
			"timeout": 200,
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		require.Contains(t, text, "backgrounded")

		pid := extractPID(t, text)
		require.Greater(t, pid, 0)

		// Check on it.
		result, err = e.Execute(context.Background(), mustJSON(t, map[string]any{
			"check_pid": pid,
		}))
		require.NoError(t, err)
		text = resultText(t, result)
		assert.Contains(t, text, "still running")

		// Clean up.
		e.Execute(context.Background(), mustJSON(t, map[string]any{"kill_pid": pid}))
	})

	t.Run("kill_pid terminates process and returns output", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()

		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo started && sleep 30",
			"timeout": 200,
		}))
		require.NoError(t, err)
		pid := extractPID(t, resultText(t, result))

		// Kill it.
		result, err = e.Execute(context.Background(), mustJSON(t, map[string]any{
			"kill_pid": pid,
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.Contains(t, text, "killed")

		// check_pid should now return unknown.
		result, err = e.Execute(context.Background(), mustJSON(t, map[string]any{
			"check_pid": pid,
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, resultText(t, result), "no background process")
	})

	t.Run("check_pid on completed process returns final output", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()

		// sleep 0.1 (100ms) with 1ms timeout — reliably backgrounds,
		// then completes quickly so we can test the completed-process path.
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "sleep 0.1",
			"timeout": 1,
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		require.Contains(t, text, "backgrounded")

		pid := extractPID(t, text)

		// Poll check_pid until the process completes — same as a real caller would.
		text = pollUntilDone(t, e, pid)
		assert.Contains(t, text, "exited")

		// Completed process should be removed from registry after Check.
		result, err = e.Execute(context.Background(), mustJSON(t, map[string]any{
			"check_pid": pid,
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, resultText(t, result), "no background process")
	})

	t.Run("check_pid on unknown pid returns error", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"check_pid": 99999,
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, resultText(t, result), "no background process")
	})

	t.Run("command completing before timeout returns normal result", func(t *testing.T) {
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

	t.Run("context cancellation kills process", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := e.Execute(ctx, mustJSON(t, map[string]any{
			"command": "sleep 30",
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, resultText(t, result), "cancelled")
	})

	t.Run("requires one of command check_pid or kill_pid", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

// pollUntilDone polls check_pid until the process reports completion.
// Returns the final check_pid result text. Single-use per pid: check_pid
// removes completed processes from the registry.
func pollUntilDone(t *testing.T, e *pipeexec.BashExecutor, pid int) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"check_pid": pid,
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		if strings.Contains(text, "exited") {
			return text
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for background process to complete")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// extractPID parses "pid NNNN" from the background notice.
func extractPID(t *testing.T, text string) int {
	t.Helper()
	var pid int
	idx := strings.Index(text, "pid ")
	if idx >= 0 {
		fmt.Sscanf(text[idx:], "pid %d", &pid)
	}
	require.Greater(t, pid, 0, "could not extract pid from: %s", text)
	return pid
}
