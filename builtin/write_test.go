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

func TestWriteTool(t *testing.T) {
	t.Parallel()

	t.Run("returns tool definition with correct schema", func(t *testing.T) {
		t.Parallel()
		tool := builtin.WriteTool()
		assert.Equal(t, "write", tool.Name)
		assert.NotEmpty(t, tool.Description)

		var schema map[string]any
		err := json.Unmarshal(tool.Parameters, &schema)
		require.NoError(t, err)

		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		_, hasPath := props["file_path"]
		assert.True(t, hasPath)
		_, hasContent := props["content"]
		assert.True(t, hasContent)
	})

	t.Run("creates a new file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "new.txt")

		args, _ := json.Marshal(map[string]any{"file_path": path, "content": "hello world"})
		result, err := builtin.ExecuteWrite(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(data))
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "existing.txt")
		require.NoError(t, os.WriteFile(path, []byte("old content"), 0o644))

		args, _ := json.Marshal(map[string]any{"file_path": path, "content": "new content"})
		result, err := builtin.ExecuteWrite(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "new content", string(data))
	})

	t.Run("creates intermediate directories", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "a", "b", "c", "file.txt")

		args, _ := json.Marshal(map[string]any{"file_path": path, "content": "deep file"})
		result, err := builtin.ExecuteWrite(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "deep file", string(data))
	})

	t.Run("preserves permissions of existing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "script.sh")
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/bash\necho old\n"), 0o755))

		args, _ := json.Marshal(map[string]any{"file_path": path, "content": "#!/bin/bash\necho new\n"})
		result, err := builtin.ExecuteWrite(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})

	t.Run("returns domain error for missing file_path", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"content": "hello"}`)
		result, err := builtin.ExecuteWrite(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "file_path")
	})

	t.Run("returns domain error for invalid JSON", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{invalid`)
		result, err := builtin.ExecuteWrite(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns success message", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "msg.txt")

		args, _ := json.Marshal(map[string]any{"file_path": path, "content": "test"})
		result, err := builtin.ExecuteWrite(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Len(t, result.Content, 1)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, path)
	})
}
