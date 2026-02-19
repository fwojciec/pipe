package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/builtin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutor(t *testing.T) {
	t.Parallel()

	t.Run("implements ToolExecutor interface", func(t *testing.T) {
		t.Parallel()
		var _ pipe.ToolExecutor = (*builtin.Executor)(nil)
	})

	t.Run("dispatches bash tool", func(t *testing.T) {
		t.Parallel()
		exec := builtin.NewExecutor()
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

		exec := builtin.NewExecutor()
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

		exec := builtin.NewExecutor()
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

		exec := builtin.NewExecutor()
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

	t.Run("returns error for unknown tool", func(t *testing.T) {
		t.Parallel()
		exec := builtin.NewExecutor()
		_, err := exec.Execute(context.Background(), "nonexistent", json.RawMessage(`{}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent")
	})

	t.Run("Tools returns all tool definitions", func(t *testing.T) {
		t.Parallel()
		exec := builtin.NewExecutor()
		tools := exec.Tools()
		assert.Len(t, tools, 4)

		names := make(map[string]bool)
		for _, tool := range tools {
			names[tool.Name] = true
		}
		assert.True(t, names["bash"])
		assert.True(t, names["read"])
		assert.True(t, names["write"])
		assert.True(t, names["edit"])
	})
}
