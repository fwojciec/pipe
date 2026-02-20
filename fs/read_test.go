package fs_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTool(t *testing.T) {
	t.Parallel()

	t.Run("returns tool definition with correct schema", func(t *testing.T) {
		t.Parallel()
		tool := fs.ReadTool()
		assert.Equal(t, "read", tool.Name)
		assert.NotEmpty(t, tool.Description)

		var schema map[string]any
		err := json.Unmarshal(tool.Parameters, &schema)
		require.NoError(t, err)

		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		_, hasPath := props["file_path"]
		assert.True(t, hasPath)
		_, hasOffset := props["offset"]
		assert.True(t, hasOffset)
		_, hasLimit := props["limit"]
		assert.True(t, hasLimit)
	})

	t.Run("reads entire file contents", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"file_path": path})
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Len(t, result.Content, 1)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "line1")
		assert.Contains(t, text.Text, "line2")
		assert.Contains(t, text.Text, "line3")
	})

	t.Run("supports line offset", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("line1\nline2\nline3\nline4\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"file_path": path, "offset": 2})
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.NotContains(t, text.Text, "line1\n")
		assert.Contains(t, text.Text, "line2")
	})

	t.Run("supports line limit", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("line1\nline2\nline3\nline4\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"file_path": path, "limit": 2})
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "line1")
		assert.Contains(t, text.Text, "line2")
		assert.NotContains(t, text.Text, "line3")
	})

	t.Run("supports offset and limit together", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("line1\nline2\nline3\nline4\nline5\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"file_path": path, "offset": 2, "limit": 2})
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "line2")
		assert.Contains(t, text.Text, "line3")
		assert.NotContains(t, text.Text, "line1\n")
		assert.NotContains(t, text.Text, "line4")
	})

	t.Run("returns domain error for missing file", func(t *testing.T) {
		t.Parallel()
		args, _ := json.Marshal(map[string]any{"file_path": "/nonexistent/file.txt"})
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns domain error for missing file_path", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{}`)
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "file_path")
	})

	t.Run("returns domain error for invalid JSON", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{invalid`)
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("reads empty file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")
		require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

		args, _ := json.Marshal(map[string]any{"file_path": path})
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Empty(t, text.Text)
	})

	t.Run("offset beyond file length returns empty content", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "short.txt")
		require.NoError(t, os.WriteFile(path, []byte("line1\nline2\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"file_path": path, "offset": 100})
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Empty(t, text.Text)
	})

	t.Run("includes line numbers in output", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"file_path": path})
		result, err := fs.ExecuteRead(context.Background(), args)
		require.NoError(t, err)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "1\talpha")
	})
}
