package exec_test

import (
	"fmt"
	"strings"
	"testing"

	pipeexec "github.com/fwojciec/pipe/exec"
	"github.com/stretchr/testify/assert"
)

func TestTruncateTail(t *testing.T) {
	t.Parallel()

	t.Run("returns short input unchanged", func(t *testing.T) {
		t.Parallel()
		r := pipeexec.TruncateTail("hello\nworld\n", 100, 1024)
		assert.Equal(t, "hello\nworld\n", r.Content)
		assert.False(t, r.Truncated)
		assert.Equal(t, 2, r.TotalLines)
	})

	t.Run("truncates by line count", func(t *testing.T) {
		t.Parallel()
		lines := make([]string, 100)
		for i := range 100 {
			lines[i] = fmt.Sprintf("line %d", i)
		}
		input := strings.Join(lines, "\n") + "\n"

		r := pipeexec.TruncateTail(input, 10, 1024*1024)
		assert.True(t, r.Truncated)
		assert.Equal(t, "lines", r.TruncatedBy)
		assert.Equal(t, 100, r.TotalLines)
		assert.Equal(t, 10, r.OutputLines)
		// Should contain the LAST 10 lines
		assert.Contains(t, r.Content, "line 90")
		assert.Contains(t, r.Content, "line 99")
		assert.NotContains(t, r.Content, "line 0\n")
	})

	t.Run("truncates by byte count", func(t *testing.T) {
		t.Parallel()
		lines := make([]string, 10)
		for i := range 10 {
			lines[i] = strings.Repeat("x", 100) // 100 bytes per line
		}
		input := strings.Join(lines, "\n") + "\n"

		r := pipeexec.TruncateTail(input, 1000, 350) // 350 bytes < 10 lines of 101 bytes
		assert.True(t, r.Truncated)
		assert.Equal(t, "bytes", r.TruncatedBy)
		assert.Equal(t, 10, r.TotalLines)
		assert.LessOrEqual(t, len(r.Content), 350)
	})

	t.Run("handles empty input", func(t *testing.T) {
		t.Parallel()
		r := pipeexec.TruncateTail("", 100, 1024)
		assert.Equal(t, "", r.Content)
		assert.False(t, r.Truncated)
		assert.Equal(t, 0, r.TotalLines)
	})

	t.Run("handles input without trailing newline", func(t *testing.T) {
		t.Parallel()
		r := pipeexec.TruncateTail("hello\nworld", 100, 1024)
		assert.Equal(t, "hello\nworld", r.Content)
		assert.False(t, r.Truncated)
		assert.Equal(t, 2, r.TotalLines)
	})

	t.Run("single line exceeding byte limit takes tail", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("x", 1000)
		r := pipeexec.TruncateTail(long, 100, 200)
		assert.True(t, r.Truncated)
		assert.True(t, r.LastLinePartial)
		assert.Equal(t, 200, len(r.Content))
		assert.Equal(t, long[800:], r.Content) // last 200 bytes
	})

	t.Run("single line with trailing newline exceeding byte limit", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("x", 1000) + "\n"
		r := pipeexec.TruncateTail(long, 100, 200)
		assert.True(t, r.Truncated)
		assert.True(t, r.LastLinePartial)
		assert.LessOrEqual(t, len(r.Content), 200)
	})

	t.Run("preserves complete lines at boundary", func(t *testing.T) {
		t.Parallel()
		// 3 lines, limit of 2
		r := pipeexec.TruncateTail("a\nb\nc\n", 2, 1024)
		assert.True(t, r.Truncated)
		assert.Equal(t, "b\nc\n", r.Content)
		assert.Equal(t, 2, r.OutputLines)
	})

	t.Run("output never exceeds maxBytes including trailing newline", func(t *testing.T) {
		t.Parallel()
		// Lines of exactly 10 bytes each. With \n separator = 11 bytes per line.
		// maxBytes = 25 should fit at most 2 lines (10 + 1 + 10 + 1 = 22, or 10 + 1 + 10 = 21 without trailing).
		lines := make([]string, 10)
		for i := range 10 {
			lines[i] = fmt.Sprintf("line_%04d!", i) // exactly 10 bytes
		}
		input := strings.Join(lines, "\n") + "\n"
		r := pipeexec.TruncateTail(input, 1000, 25)
		assert.True(t, r.Truncated)
		assert.LessOrEqual(t, len(r.Content), 25, "content must not exceed maxBytes")
	})

	t.Run("with default limits handles large output", func(t *testing.T) {
		t.Parallel()
		// 5000 lines, each 20 bytes
		var b strings.Builder
		for i := range 5000 {
			fmt.Fprintf(&b, "line %04d padding\n", i)
		}
		input := b.String()

		r := pipeexec.TruncateTail(input, pipeexec.DefaultMaxLines, pipeexec.DefaultMaxBytes)
		assert.True(t, r.Truncated)
		assert.Equal(t, 5000, r.TotalLines)
		assert.Equal(t, pipeexec.DefaultMaxLines, r.OutputLines)
		assert.LessOrEqual(t, len(r.Content), pipeexec.DefaultMaxBytes)
		assert.Contains(t, r.Content, "line 4999")
		assert.Contains(t, r.Content, "line 3000")
		assert.NotContains(t, r.Content, "line 0000\n")
	})
}
