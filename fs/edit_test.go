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

func TestEditTool(t *testing.T) {
	t.Parallel()

	t.Run("returns tool definition with correct schema", func(t *testing.T) {
		t.Parallel()
		tool := fs.EditTool()
		assert.Equal(t, "edit", tool.Name)
		assert.NotEmpty(t, tool.Description)

		var schema map[string]any
		err := json.Unmarshal(tool.Parameters, &schema)
		require.NoError(t, err)

		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		_, hasPath := props["file_path"]
		assert.True(t, hasPath)
		_, hasOld := props["old_string"]
		assert.True(t, hasOld)
		_, hasNew := props["new_string"]
		assert.True(t, hasNew)
	})

	t.Run("replaces unique string in file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		require.NoError(t, os.WriteFile(path, []byte("func greet() {\n\treturn \"hello\"\n}\n"), 0o644))

		args, _ := json.Marshal(map[string]any{
			"file_path":  path,
			"old_string": "greet",
			"new_string": "welcome",
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "func welcome() {\n\treturn \"hello\"\n}\n", string(data))
	})

	t.Run("errors on non-unique match when replace_all is false", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		require.NoError(t, os.WriteFile(path, []byte("foo bar foo baz"), 0o644))

		args, _ := json.Marshal(map[string]any{
			"file_path":  path,
			"old_string": "foo",
			"new_string": "qux",
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "2")
	})

	t.Run("replace_all replaces all occurrences", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		require.NoError(t, os.WriteFile(path, []byte("foo bar foo baz"), 0o644))

		args, _ := json.Marshal(map[string]any{
			"file_path":   path,
			"old_string":  "foo",
			"new_string":  "qux",
			"replace_all": true,
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "qux bar qux baz", string(data))
	})

	t.Run("errors when old_string not found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		require.NoError(t, os.WriteFile(path, []byte("hello world"), 0o644))

		args, _ := json.Marshal(map[string]any{
			"file_path":  path,
			"old_string": "notfound",
			"new_string": "replacement",
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "not found")
	})

	t.Run("errors on missing file", func(t *testing.T) {
		t.Parallel()
		args, _ := json.Marshal(map[string]any{
			"file_path":  "/nonexistent/file.txt",
			"old_string": "a",
			"new_string": "b",
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("errors on missing file_path", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"old_string": "a", "new_string": "b"}`)
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("errors on invalid JSON", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{invalid`)
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("handles multi-line replacement", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		require.NoError(t, os.WriteFile(path, []byte("func old() {\n\treturn 1\n}\n"), 0o644))

		args, _ := json.Marshal(map[string]any{
			"file_path":  path,
			"old_string": "func old() {\n\treturn 1\n}",
			"new_string": "func new() {\n\treturn 2\n}",
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "func new() {\n\treturn 2\n}\n", string(data))
	})

	t.Run("errors on empty old_string", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		require.NoError(t, os.WriteFile(path, []byte("hello world"), 0o644))

		args, _ := json.Marshal(map[string]any{
			"file_path":  path,
			"old_string": "",
			"new_string": "replacement",
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "old_string must not be empty")
	})

	t.Run("errors on empty old_string with replace_all", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		require.NoError(t, os.WriteFile(path, []byte("hello world"), 0o644))

		args, _ := json.Marshal(map[string]any{
			"file_path":   path,
			"old_string":  "",
			"new_string":  "X",
			"replace_all": true,
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		// File should be unchanged.
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(data))
	})

	t.Run("preserves file permissions", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.sh")
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/bash\necho old\n"), 0o755))

		args, _ := json.Marshal(map[string]any{
			"file_path":  path,
			"old_string": "echo old",
			"new_string": "echo new",
		})
		result, err := fs.ExecuteEdit(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})
}
