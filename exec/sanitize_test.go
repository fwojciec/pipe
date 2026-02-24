package exec_test

import (
	"strings"
	"testing"

	pipeexec "github.com/fwojciec/pipe/exec"
	"github.com/stretchr/testify/assert"
)

func TestSanitize(t *testing.T) {
	t.Parallel()

	t.Run("passes plain text through unchanged", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "hello world", pipeexec.Sanitize("hello world"))
	})

	t.Run("strips ANSI color codes", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "hello", pipeexec.Sanitize("\x1b[31mhello\x1b[0m"))
	})

	t.Run("strips ANSI bold and underline", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "bold", pipeexec.Sanitize("\x1b[1mbold\x1b[22m"))
	})

	t.Run("preserves tabs and newlines", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "a\tb\nc", pipeexec.Sanitize("a\tb\nc"))
	})

	t.Run("removes control characters", func(t *testing.T) {
		t.Parallel()
		// \x01 (SOH), \x02 (STX), \x07 (BEL) should be removed
		assert.Equal(t, "abc", pipeexec.Sanitize("a\x01b\x02c\x07"))
	})

	t.Run("normalizes CRLF to LF", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "a\nb\n", pipeexec.Sanitize("a\r\nb\r\n"))
	})

	t.Run("resolves lone CR as terminal overwrite", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "progress done", pipeexec.Sanitize("progress 50%\rprogress done"))
	})

	t.Run("resolves multiple CRs on one line", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "done", pipeexec.Sanitize("10%\r50%\rdone"))
	})

	t.Run("CR overwrite preserves trailing chars when segment is shorter", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "xycdef", pipeexec.Sanitize("abcdef\rxy"))
	})

	t.Run("handles empty string", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", pipeexec.Sanitize(""))
	})

	t.Run("handles string with only escape codes", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", pipeexec.Sanitize("\x1b[31m\x1b[0m"))
	})

	t.Run("strips OSC sequences", func(t *testing.T) {
		t.Parallel()
		// OSC (Operating System Command) like terminal title setting
		assert.Equal(t, "text", pipeexec.Sanitize("\x1b]0;title\x07text"))
	})

	t.Run("handles mixed content", func(t *testing.T) {
		t.Parallel()
		input := "\x1b[32m✓\x1b[0m test passed\n\x1b[31m✗\x1b[0m test failed\n"
		expected := "✓ test passed\n✗ test failed\n"
		assert.Equal(t, expected, pipeexec.Sanitize(input))
	})

	t.Run("handles realistic compiler output", func(t *testing.T) {
		t.Parallel()
		input := "\x1b[1mmain.go:10:5:\x1b[0m \x1b[31merror:\x1b[0m undefined: foo\n"
		expected := "main.go:10:5: error: undefined: foo\n"
		assert.Equal(t, expected, pipeexec.Sanitize(input))
	})

	t.Run("handles large input efficiently", func(t *testing.T) {
		t.Parallel()
		line := "\x1b[32m" + strings.Repeat("x", 1000) + "\x1b[0m\n"
		input := strings.Repeat(line, 1000)
		result := pipeexec.Sanitize(input)
		assert.NotContains(t, result, "\x1b")
		assert.Contains(t, result, strings.Repeat("x", 1000))
	})
}
