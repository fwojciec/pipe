package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fwojciec/pipe"
	pipeexec "github.com/fwojciec/pipe/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutor(t *testing.T) {
	t.Parallel()

	t.Run("dispatches bash tool", func(t *testing.T) {
		t.Parallel()
		exec := &executor{bash: pipeexec.NewBashExecutor()}
		args := json.RawMessage(`{"command": "echo dispatched"}`)
		result, err := exec.Execute(context.Background(), "bash", args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "dispatched")
	})

	t.Run("dispatches read tool", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("read me"), 0o644))

		exec := &executor{bash: pipeexec.NewBashExecutor()}
		args, _ := json.Marshal(map[string]any{"file_path": path})
		result, err := exec.Execute(context.Background(), "read", args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "read me")
	})

	t.Run("dispatches write tool", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "out.txt")

		exec := &executor{bash: pipeexec.NewBashExecutor()}
		args, _ := json.Marshal(map[string]any{"file_path": path, "content": "written"})
		result, err := exec.Execute(context.Background(), "write", args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "written", string(data))
	})

	t.Run("dispatches edit tool", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "edit.txt")
		require.NoError(t, os.WriteFile(path, []byte("old value"), 0o644))

		exec := &executor{bash: pipeexec.NewBashExecutor()}
		args, _ := json.Marshal(map[string]any{
			"file_path":  path,
			"old_string": "old value",
			"new_string": "new value",
		})
		result, err := exec.Execute(context.Background(), "edit", args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "new value", string(data))
	})

	t.Run("dispatches grep tool", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("findme\n"), 0o644))

		exec := &executor{bash: pipeexec.NewBashExecutor()}
		args, _ := json.Marshal(map[string]any{"pattern": "findme", "path": dir})
		result, err := exec.Execute(context.Background(), "grep", args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "findme")
	})

	t.Run("dispatches glob tool", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.go"), []byte(""), 0o644))

		exec := &executor{bash: pipeexec.NewBashExecutor()}
		args, _ := json.Marshal(map[string]any{"pattern": "*.go", "path": dir})
		result, err := exec.Execute(context.Background(), "glob", args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "test.go")
	})

	t.Run("returns tool error for unknown tool", func(t *testing.T) {
		t.Parallel()
		exec := &executor{bash: pipeexec.NewBashExecutor()}
		result, err := exec.Execute(context.Background(), "nonexistent", json.RawMessage(`{}`))
		require.NoError(t, err)
		require.True(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "nonexistent")
	})

	t.Run("every tool in tools() is dispatchable", func(t *testing.T) {
		t.Parallel()
		exec := &executor{bash: pipeexec.NewBashExecutor()}
		for _, tool := range tools() {
			t.Run(tool.Name, func(t *testing.T) {
				t.Parallel()
				// Dispatch with empty args — we only need dispatch to reach the
				// tool handler, not to succeed. An unknown-tool result would have
				// IsError=true and contain "unknown tool".
				result, err := exec.Execute(context.Background(), tool.Name, json.RawMessage(`{}`))
				require.NoError(t, err)
				if result.IsError {
					text, ok := result.Content[0].(pipe.TextBlock)
					require.True(t, ok)
					assert.NotContains(t, text.Text, "unknown tool", "tool %q not dispatched", tool.Name)
				}
			})
		}
	})
}
