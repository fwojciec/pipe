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

func TestGlobTool(t *testing.T) {
	t.Parallel()

	t.Run("returns tool definition with correct schema", func(t *testing.T) {
		t.Parallel()
		tool := fs.GlobTool()
		assert.Equal(t, "glob", tool.Name)
		assert.NotEmpty(t, tool.Description)

		var schema map[string]any
		err := json.Unmarshal(tool.Parameters, &schema)
		require.NoError(t, err)

		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		_, hasPattern := props["pattern"]
		assert.True(t, hasPattern)
		_, hasPath := props["path"]
		assert.True(t, hasPath)
	})

	t.Run("matches files with simple pattern", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "*.go", "path": dir})
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "a.go")
		assert.Contains(t, text.Text, "b.go")
		assert.NotContains(t, text.Text, "c.txt")
	})

	t.Run("matches files recursively with doublestar", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		require.NoError(t, os.MkdirAll(sub, 0o755))

		require.NoError(t, os.WriteFile(filepath.Join(dir, "root.go"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(sub, "nested.go"), []byte(""), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "**/*.go", "path": dir})
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "root.go")
		assert.Contains(t, text.Text, "nested.go")
	})

	t.Run("returns no matches message when nothing found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte(""), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "*.go", "path": dir})
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "no matches")
	})

	t.Run("returns domain error for invalid pattern", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		args, _ := json.Marshal(map[string]any{"pattern": "[invalid", "path": dir})
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns domain error for missing pattern", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"path": "/tmp"}`)
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns domain error for invalid JSON", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{invalid`)
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns domain error for nonexistent path", func(t *testing.T) {
		t.Parallel()
		args, _ := json.Marshal(map[string]any{"pattern": "*.go", "path": "/nonexistent/dir"})
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns relative paths from base directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(sub, "file.go"), []byte(""), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "**/*.go", "path": dir})
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, filepath.Join("sub", "file.go"))
		assert.NotContains(t, text.Text, dir)
	})

	t.Run("matches deeply nested files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		deep := filepath.Join(dir, "a", "b", "c")
		require.NoError(t, os.MkdirAll(deep, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(deep, "deep.go"), []byte(""), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "**/*.go", "path": dir})
		result, err := fs.ExecuteGlob(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "deep.go")
	})
}
