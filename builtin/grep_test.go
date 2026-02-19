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

func TestGrepTool(t *testing.T) {
	t.Parallel()

	t.Run("returns tool definition with correct schema", func(t *testing.T) {
		t.Parallel()
		tool := builtin.GrepTool()
		assert.Equal(t, "grep", tool.Name)
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
		_, hasGlob := props["glob"]
		assert.True(t, hasGlob)
	})

	t.Run("finds matching lines in a single file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "test.go")
		require.NoError(t, os.WriteFile(path, []byte("package main\n\nfunc hello() {}\nfunc world() {}\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "func", "path": path})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "test.go:3:func hello()")
		assert.Contains(t, text.Text, "test.go:4:func world()")
	})

	t.Run("searches directory recursively", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		require.NoError(t, os.MkdirAll(sub, 0o755))

		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("match here\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(sub, "b.txt"), []byte("match there\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "match", "path": dir})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "a.txt")
		assert.Contains(t, text.Text, "b.txt")
	})

	t.Run("supports regex patterns", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("foo123bar\nbaz456qux\nhello\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": `\d+`, "path": dir})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "foo123bar")
		assert.Contains(t, text.Text, "baz456qux")
		assert.NotContains(t, text.Text, "hello")
	})

	t.Run("filters files by glob pattern", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "code.go"), []byte("match\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("match\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "match", "path": dir, "glob": "*.go"})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "code.go")
		assert.NotContains(t, text.Text, "notes.txt")
	})

	t.Run("returns no matches message when nothing found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "zzzzz", "path": dir})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "no matches")
	})

	t.Run("returns domain error for invalid regex", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("text\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "[invalid", "path": dir})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns domain error for missing pattern", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{"path": "/tmp"}`)
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns domain error for invalid JSON", func(t *testing.T) {
		t.Parallel()
		args := json.RawMessage(`{invalid`)
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns domain error for nonexistent path", func(t *testing.T) {
		t.Parallel()
		args, _ := json.Marshal(map[string]any{"pattern": "test", "path": "/nonexistent/dir"})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("includes line numbers in output", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "beta", "path": dir})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, ":2:")
	})

	t.Run("skips binary files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "text.txt"), []byte("match\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "binary.bin"), []byte("match\x00\x01\x02"), 0o644))

		args, _ := json.Marshal(map[string]any{"pattern": "match", "path": dir})
		result, err := builtin.ExecuteGrep(context.Background(), args)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text, ok := result.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Contains(t, text.Text, "text.txt")
		assert.NotContains(t, text.Text, "binary.bin")
	})
}
