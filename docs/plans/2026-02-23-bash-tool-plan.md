# Bash Tool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rewrite the bash tool with tail truncation, filesystem offloading, output sanitization, separate stderr, and auto-backgrounding on timeout.

**Architecture:** Stateless execution (fresh `bash -c` per command). Output pipeline: rolling buffer → sanitize → tail truncate → offload to file if large. Auto-background on timeout instead of killing. Background process registry for check/kill.

**Tech Stack:** Go stdlib `os/exec`, `charmbracelet/x/ansi` for ANSI stripping, `stretchr/testify` for assertions.

**Design doc:** `docs/plans/2026-02-23-bash-tool-design.md`

---

### Task 1: Output Sanitization

**Files:**
- Create: `exec/sanitize.go`
- Test: `exec/sanitize_test.go`

**Step 1: Write failing tests**

```go
// exec/sanitize_test.go
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

	t.Run("removes lone CR", func(t *testing.T) {
		t.Parallel()
		// Lone \r (not followed by \n) is a carriage return overwrite — remove it
		assert.Equal(t, "progress done", pipeexec.Sanitize("progress 50%\rprogress done"))
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./exec/ -run TestSanitize -v`
Expected: compilation error — `Sanitize` not defined

**Step 3: Implement sanitization**

```go
// exec/sanitize.go
package exec

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Sanitize strips ANSI escape codes and control characters from command output.
// It preserves tabs and newlines but removes all other bytes <= 0x1F.
// CRLF sequences are normalized to LF. Lone CR (carriage return overwrites) are
// removed.
func Sanitize(s string) string {
	// Strip ANSI escape sequences (CSI, OSC, etc.) using parser-based stripper.
	s = ansi.Strip(s)

	// Normalize CRLF → LF before filtering, so \r in \r\n isn't dropped.
	s = strings.ReplaceAll(s, "\r\n", "\n")

	// Filter control characters, keeping only \t (0x09) and \n (0x0A).
	// Lone \r is removed (it's a carriage-return overwrite, not useful for LLM).
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || r > 0x1F {
			b.WriteRune(r)
		}
	}
	return b.String()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./exec/ -run TestSanitize -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add exec/sanitize.go exec/sanitize_test.go
git commit -m "feat(exec): add output sanitization

Strip ANSI escape codes and control characters from bash output
before sending to LLM. Uses charmbracelet/x/ansi parser."
```

---

### Task 2: Tail Truncation

**Files:**
- Create: `exec/truncate.go`
- Test: `exec/truncate_test.go`

**Step 1: Write failing tests**

```go
// exec/truncate_test.go
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
		var lines []string
		for i := range 100 {
			lines = append(lines, fmt.Sprintf("line %d", i))
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./exec/ -run TestTruncateTail -v`
Expected: compilation error — `TruncateTail`, `TruncateResult`, `DefaultMaxLines`, `DefaultMaxBytes` not defined

**Step 3: Implement tail truncation**

```go
// exec/truncate.go
package exec

import "strings"

const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024 // 50KB
)

// TruncateResult describes the outcome of tail truncation.
type TruncateResult struct {
	Content         string
	Truncated       bool
	TruncatedBy     string // "lines" or "bytes"
	TotalLines      int
	TotalBytes      int
	OutputLines     int
	OutputBytes     int
	LastLinePartial bool
}

// TruncateTail keeps the last maxLines lines or maxBytes bytes of input,
// whichever limit is hit first. It works backwards from the end, collecting
// complete lines. If a single line exceeds maxBytes, it takes the tail of that
// line (setting LastLinePartial).
func TruncateTail(s string, maxLines, maxBytes int) TruncateResult {
	if s == "" {
		return TruncateResult{}
	}

	hasTrailingNewline := strings.HasSuffix(s, "\n")
	lines := splitLines(s)
	totalLines := len(lines)
	totalBytes := len(s)

	// Check if within limits.
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncateResult{
			Content:     s,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
		}
	}

	// Work backwards collecting lines. Budget bytes carefully:
	// the final output is lines joined by \n, optionally with a trailing \n.
	// Reserve 1 byte for trailing newline if original had one.
	byteBudget := maxBytes
	if hasTrailingNewline {
		byteBudget-- // reserve for trailing \n in reconstructed output
	}

	var collected []string
	outputBytes := 0
	truncatedBy := ""

	for i := len(lines) - 1; i >= 0 && len(collected) < maxLines; i-- {
		lineBytes := len(lines[i])
		if len(collected) > 0 {
			lineBytes++ // account for the \n separator between lines
		}
		if outputBytes+lineBytes > byteBudget {
			truncatedBy = "bytes"
			// Edge case: no lines collected yet and this single line exceeds maxBytes.
			if len(collected) == 0 {
				tail := lines[i]
				if len(tail) > maxBytes {
					tail = tail[len(tail)-maxBytes:]
				}
				return TruncateResult{
					Content:         tail,
					Truncated:       true,
					TruncatedBy:     "bytes",
					TotalLines:      totalLines,
					TotalBytes:      totalBytes,
					OutputLines:     1,
					OutputBytes:     len(tail),
					LastLinePartial: true,
				}
			}
			break
		}
		collected = append(collected, lines[i])
		outputBytes += lineBytes
	}

	if truncatedBy == "" {
		truncatedBy = "lines"
	}

	// Reverse collected lines (they were added back-to-front).
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}

	// Reconstruct with original trailing newline if present.
	content := strings.Join(collected, "\n")
	if hasTrailingNewline {
		content += "\n"
	}

	return TruncateResult{
		Content:     content,
		Truncated:   true,
		TruncatedBy: truncatedBy,
		TotalLines:  totalLines,
		TotalBytes:  totalBytes,
		OutputLines: len(collected),
		OutputBytes: len(content),
	}
}

// splitLines splits s into lines, treating the final line as a line even
// without a trailing newline. A trailing newline does NOT produce an empty
// final element.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./exec/ -run TestTruncateTail -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add exec/truncate.go exec/truncate_test.go
git commit -m "feat(exec): add tail truncation for bash output

Keep last 2000 lines or 50KB (whichever hits first). Byte budget
accounts for trailing newline. Handles edge cases: single oversized
line, empty input, no trailing newline."
```

---

### Task 3: Output Collector (Rolling Buffer + File Offloading)

**Files:**
- Create: `exec/collector.go`
- Test: `exec/collector_test.go`

**Step 1: Write failing tests**

```go
// exec/collector_test.go
package exec_test

import (
	"os"
	"strings"
	"testing"

	pipeexec "github.com/fwojciec/pipe/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputCollector(t *testing.T) {
	t.Parallel()

	t.Run("collects small output in memory", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(1024, 2048)
		c.Write([]byte("hello\n"))
		c.Write([]byte("world\n"))

		assert.Equal(t, "hello\nworld\n", string(c.Bytes()))
		assert.Equal(t, int64(12), c.TotalBytes())
		assert.Empty(t, c.FilePath())
	})

	t.Run("tracks total line count across trims", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(100, 200)
		// Write many lines, enough to trigger rolling buffer trim.
		for range 50 {
			c.Write([]byte("a line of text here\n")) // 20 bytes per line
		}
		// Total = 1000 bytes, buffer trimmed to 200, but total lines should be 50.
		assert.Equal(t, int64(1000), c.TotalBytes())
		assert.Equal(t, 50, c.TotalLines())
		assert.LessOrEqual(t, len(c.Bytes()), 200)
	})

	t.Run("rolling buffer keeps last maxBuf bytes", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(100, 200) // threshold 100, maxBuf 200
		// Write 300 bytes — rolling buffer should keep last 200
		c.Write([]byte(strings.Repeat("a", 150)))
		c.Write([]byte(strings.Repeat("b", 150)))

		buf := c.Bytes()
		assert.LessOrEqual(t, len(buf), 200)
		// Should end with b's
		assert.True(t, strings.HasSuffix(string(buf), strings.Repeat("b", 150)))
	})

	t.Run("offloads to file when threshold exceeded", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(100, 200)
		c.Write([]byte(strings.Repeat("x", 50)))
		assert.Empty(t, c.FilePath(), "should not offload yet")

		c.Write([]byte(strings.Repeat("y", 60))) // total 110 > threshold
		require.NotEmpty(t, c.FilePath(), "should offload after threshold")
		assert.NoError(t, c.Err(), "offload should succeed")

		// Verify file contains full output
		data, err := os.ReadFile(c.FilePath())
		require.NoError(t, err)
		assert.Equal(t, 110, len(data))
		assert.True(t, strings.HasPrefix(string(data), strings.Repeat("x", 50)))
		c.Close()
		os.Remove(c.FilePath())
	})

	t.Run("file receives all subsequent writes", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(50, 200)
		c.Write([]byte(strings.Repeat("a", 60))) // triggers offload
		c.Write([]byte(strings.Repeat("b", 60))) // should go to file too

		data, err := os.ReadFile(c.FilePath())
		require.NoError(t, err)
		assert.Equal(t, 120, len(data))
		c.Close()
		os.Remove(c.FilePath())
	})

	t.Run("close flushes and closes file", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(50, 200)
		c.Write([]byte(strings.Repeat("x", 100)))
		path := c.FilePath()
		require.NotEmpty(t, path)

		c.Close()
		// File should still exist (caller responsible for cleanup)
		_, err := os.Stat(path)
		assert.NoError(t, err)
		os.Remove(path)
	})

	t.Run("is safe for concurrent writes", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(1024, 2048)
		done := make(chan struct{})
		for range 10 {
			go func() {
				for range 100 {
					c.Write([]byte("data\n"))
				}
				done <- struct{}{}
			}()
		}
		for range 10 {
			<-done
		}
		assert.Equal(t, int64(5000), c.TotalBytes()) // 10 * 100 * 5
		c.Close()
		if c.FilePath() != "" {
			os.Remove(c.FilePath())
		}
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./exec/ -run TestOutputCollector -v`
Expected: compilation error — `NewOutputCollector` not defined

**Step 3: Implement output collector**

```go
// exec/collector.go
package exec

import (
	"bytes"
	"os"
	"sync"
)

// OutputCollector is an io.Writer that captures command output with:
//   - A rolling buffer (last maxBuf bytes) for in-memory access
//   - File offloading for full output when total exceeds threshold
//   - Total byte and line counts (accurate even after rolling buffer trims)
//
// It is safe for concurrent use.
type OutputCollector struct {
	mu         sync.Mutex
	buf        []byte
	total      int64
	totalLines int
	file       *os.File
	filePath   string
	err        error // first I/O error encountered during offloading
	threshold  int64
	maxBuf     int
}

// NewOutputCollector creates a collector. Threshold is the byte count at which
// output is offloaded to a temp file. MaxBuf is the rolling buffer size
// (must be >= threshold).
func NewOutputCollector(threshold int64, maxBuf int) *OutputCollector {
	return &OutputCollector{
		threshold: threshold,
		maxBuf:    maxBuf,
	}
}

// Write implements io.Writer.
func (c *OutputCollector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := len(p)
	c.total += int64(n)

	// Count newlines for total line tracking. This counts terminated lines only;
	// processOutput adjusts for an unterminated final line.
	c.totalLines += bytes.Count(p, []byte{'\n'})

	c.buf = append(c.buf, p...)

	// File offloading: flush entire buffer to file when threshold first crossed.
	if c.file == nil && c.err == nil && c.total > c.threshold {
		f, err := os.CreateTemp("", "pipe-bash-*.log")
		if err != nil {
			c.err = err
		} else {
			c.file = f
			c.filePath = f.Name()
			if _, err := c.file.Write(c.buf); err != nil {
				c.err = err
			}
		}
	} else if c.file != nil && c.err == nil {
		if _, err := c.file.Write(p); err != nil {
			c.err = err
		}
	}

	// Trim rolling buffer.
	if len(c.buf) > c.maxBuf {
		c.buf = c.buf[len(c.buf)-c.maxBuf:]
	}

	return n, nil
}

// Bytes returns a copy of the current rolling buffer content.
func (c *OutputCollector) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf...)
}

// TotalBytes returns the total number of bytes written (not just what's in buffer).
func (c *OutputCollector) TotalBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

// TotalLines returns the total number of newlines seen (not just what's in buffer).
func (c *OutputCollector) TotalLines() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalLines
}

// FilePath returns the temp file path, or empty if output was not offloaded.
func (c *OutputCollector) FilePath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.filePath
}

// Err returns the first I/O error encountered during file offloading, or nil.
func (c *OutputCollector) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close flushes and closes the temp file if one was created.
func (c *OutputCollector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.file != nil {
		c.file.Close()
		c.file = nil
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./exec/ -run TestOutputCollector -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add exec/collector.go exec/collector_test.go
git commit -m "feat(exec): add output collector with rolling buffer and file offloading

Bounds memory during command execution. Offloads full output to temp
file when threshold exceeded. Tracks accurate total line/byte counts
across rolling buffer trims. Surfaces I/O errors via Err()."
```

---

### Task 4: Rewrite Bash Execution (No Background Yet)

Rewrite `ExecuteBash` with: separate stdout/stderr, output pipeline (sanitize →
truncate → offload notice), new result format. On timeout, **kill the process**
(like current behavior). Background execution is added in Task 5.

**Files:**
- Modify: `exec/bash.go` (full rewrite)
- Modify: `exec/bash_test.go` (rewrite to match new behavior)

**Step 1: Write failing tests for new behavior**

Replace `exec/bash_test.go` entirely:

```go
// exec/bash_test.go
package exec_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fwojciec/pipe"
	pipeexec "github.com/fwojciec/pipe/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBashTool(t *testing.T) {
	t.Parallel()

	t.Run("returns tool definition with correct schema", func(t *testing.T) {
		t.Parallel()
		tool := pipeexec.BashTool()
		assert.Equal(t, "bash", tool.Name)
		assert.NotEmpty(t, tool.Description)
		assert.Contains(t, tool.Description, "truncat")

		var schema map[string]any
		err := json.Unmarshal(tool.Parameters, &schema)
		require.NoError(t, err)

		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "command")
		assert.Contains(t, props, "timeout")
	})
}

func TestExecuteBash(t *testing.T) {
	t.Parallel()

	t.Run("executes simple command", func(t *testing.T) {
		t.Parallel()
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": "echo hello",
		}))
		require.NoError(t, err)
		require.False(t, result.IsError)
		text := resultText(t, result)
		assert.Contains(t, text, "stdout:\nhello\n")
		assert.Contains(t, text, "exit code: 0")
	})

	t.Run("separates stdout and stderr", func(t *testing.T) {
		t.Parallel()
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": "echo out && echo err >&2",
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.Contains(t, text, "stdout:\nout\n")
		assert.Contains(t, text, "stderr:\nerr\n")
	})

	t.Run("strips ANSI codes from output", func(t *testing.T) {
		t.Parallel()
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": `printf '\033[31mred\033[0m\n'`,
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.Contains(t, text, "red")
		assert.NotContains(t, text, "\033")
		assert.NotContains(t, text, "\x1b")
	})

	t.Run("reports non-zero exit code", func(t *testing.T) {
		t.Parallel()
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": "echo fail && exit 42",
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		text := resultText(t, result)
		assert.Contains(t, text, "exit code: 42")
		assert.Contains(t, text, "fail")
	})

	t.Run("truncates large stdout by line count", func(t *testing.T) {
		t.Parallel()
		// Generate more lines than DefaultMaxLines but small total bytes.
		// Each line is ~7 bytes ("NNNNN\n"), so 3000 lines ≈ 21KB < 50KB threshold.
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": fmt.Sprintf("seq 1 %d", pipeexec.DefaultMaxLines+1000),
		}))
		require.NoError(t, err)
		require.False(t, result.IsError)
		text := resultText(t, result)

		// Should contain truncation notice (line-truncated, no file offload)
		assert.Contains(t, text, "Showing last")
		assert.NotContains(t, text, "Full output:", "no file offload for line-only truncation")

		// Should contain last lines but not first
		assert.Contains(t, text, fmt.Sprintf("%d", pipeexec.DefaultMaxLines+1000))
	})

	t.Run("offloads to file when output exceeds byte threshold", func(t *testing.T) {
		t.Parallel()
		// Generate output larger than DefaultMaxBytes (50KB).
		// printf repeats a 100-byte line 1000 times = 100KB > 50KB threshold.
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": `for i in $(seq 1 1000); do printf '%099d\n' $i; done`,
		}))
		require.NoError(t, err)
		require.False(t, result.IsError)
		text := resultText(t, result)

		assert.Contains(t, text, "Showing last")
		assert.Contains(t, text, "Full output:")

		// Temp file should exist
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "Full output:") {
				path := strings.TrimSpace(strings.Split(line, "Full output:")[1])
				path = strings.TrimSuffix(path, "]")
				path = strings.TrimSpace(path)
				_, statErr := os.Stat(path)
				assert.NoError(t, statErr, "temp file should exist")
				os.Remove(path)
				break
			}
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := pipeexec.ExecuteBash(ctx, mustJSON(t, map[string]any{
			"command": "sleep 10",
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("kills process on timeout", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("sleep command differs on Windows")
		}
		start := time.Now()
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": "sleep 10",
			"timeout": 200,
		}))
		elapsed := time.Since(start)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Less(t, elapsed, 5*time.Second)
		text := resultText(t, result)
		assert.Contains(t, text, "timed out")
	})

	t.Run("returns error for missing command", func(t *testing.T) {
		t.Parallel()
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error for invalid JSON args", func(t *testing.T) {
		t.Parallel()
		result, err := pipeexec.ExecuteBash(context.Background(), json.RawMessage(`{invalid`))
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("omits empty stderr section", func(t *testing.T) {
		t.Parallel()
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": "echo hello",
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.NotContains(t, text, "stderr:")
	})

	t.Run("omits empty stdout section", func(t *testing.T) {
		t.Parallel()
		result, err := pipeexec.ExecuteBash(context.Background(), mustJSON(t, map[string]any{
			"command": "echo err >&2",
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.NotContains(t, text, "stdout:")
		assert.Contains(t, text, "stderr:\nerr\n")
	})
}

// mustJSON marshals v to json.RawMessage, failing the test on error.
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// resultText extracts the text from the first content block of a tool result.
func resultText(t *testing.T, r *pipe.ToolResult) string {
	t.Helper()
	require.NotEmpty(t, r.Content)
	text, ok := r.Content[0].(pipe.TextBlock)
	require.True(t, ok)
	return text.Text
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./exec/ -run TestExecuteBash -v`
Expected: compilation failure or test failures — new result format not implemented

**Step 3: Implement rewritten bash execution**

Rewrite `exec/bash.go`:

```go
// exec/bash.go
package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	osexec "os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/fwojciec/pipe"
)

type bashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // milliseconds
}

// BashTool returns the tool definition for the bash tool.
func BashTool() pipe.Tool {
	return pipe.Tool{
		Name: "bash",
		Description: fmt.Sprintf(
			"Execute a bash command. Output truncated to last %d lines or %dKB; "+
				"if truncated, full output saved to temp file readable with the read tool.",
			DefaultMaxLines, DefaultMaxBytes/1024,
		),
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "The bash command to execute"
				},
				"timeout": {
					"type": "integer",
					"description": "Timeout in milliseconds (default: 120000)"
				}
			},
			"required": ["command"]
		}`),
	}
}

const rollingBufSize = 2 * DefaultMaxBytes // 100KB rolling buffer

// ExecuteBash executes a bash command and returns the result with separate
// stdout/stderr, output sanitization, tail truncation, and file offloading.
func ExecuteBash(ctx context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	if a.Command == "" {
		return domainError("command is required"), nil
	}

	timeout := 120 * time.Second
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := osexec.CommandContext(ctx, "bash", "-c", a.Command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return domainError(fmt.Sprintf("failed to create stdout pipe: %s", err)), nil
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return domainError(fmt.Sprintf("failed to create stderr pipe: %s", err)), nil
	}

	if err := cmd.Start(); err != nil {
		return domainError(fmt.Sprintf("failed to start command: %s", err)), nil
	}

	stdoutC := NewOutputCollector(int64(DefaultMaxBytes), rollingBufSize)
	stderrC := NewOutputCollector(int64(DefaultMaxBytes), rollingBufSize)

	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() { io.Copy(stdoutC, stdoutPipe); close(stdoutDone) }()
	go func() { io.Copy(stderrC, stderrPipe); close(stderrDone) }()

	waitErr := cmd.Wait()
	<-stdoutDone
	<-stderrDone
	stdoutC.Close()
	stderrC.Close()

	// Determine exit code.
	exitCode := 0
	isError := false
	if waitErr != nil {
		var exitErr *osexec.ExitError
		isRealExit := errors.As(waitErr, &exitErr) && exitErr.ExitCode() >= 0
		if !isRealExit && ctx.Err() != nil {
			return formatTimeoutResult(ctx.Err(), stdoutC, stderrC), nil
		}
		isError = true
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return formatResult(exitCode, isError, stdoutC, stderrC), nil
}

// processOutput sanitizes and truncates collector output. Returns the processed
// string and truncation metadata.
func processOutput(c *OutputCollector) (string, TruncateResult) {
	raw := string(c.Bytes())
	clean := Sanitize(raw)
	tr := TruncateTail(clean, DefaultMaxLines, DefaultMaxBytes)
	// Override total lines with the collector's accurate count (rolling buffer
	// may have dropped early data). TotalLines() counts \n characters; add 1
	// for an unterminated final line.
	total := c.TotalLines()
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		total++
	}
	tr.TotalLines = total
	return tr.Content, tr
}

func formatResult(exitCode int, isError bool, stdout, stderr *OutputCollector) *pipe.ToolResult {
	stdoutStr, stdoutTR := processOutput(stdout)
	stderrStr, stderrTR := processOutput(stderr)

	var b strings.Builder
	if stdoutStr != "" {
		fmt.Fprintf(&b, "stdout:\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "stderr:\n%s\n", stderrStr)
	}
	fmt.Fprintf(&b, "exit code: %d", exitCode)

	appendOffloadNotice(&b, "stdout", stdoutTR, stdout)
	appendOffloadNotice(&b, "stderr", stderrTR, stderr)

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: isError,
	}
}

func formatTimeoutResult(ctxErr error, stdout, stderr *OutputCollector) *pipe.ToolResult {
	stdoutStr, _ := processOutput(stdout)
	stderrStr, _ := processOutput(stderr)

	var b strings.Builder
	fmt.Fprintf(&b, "command timed out: %s\n", ctxErr)
	if stdoutStr != "" {
		fmt.Fprintf(&b, "\nstdout (partial):\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "\nstderr (partial):\n%s\n", stderrStr)
	}

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: true,
	}
}

func appendOffloadNotice(b *strings.Builder, name string, tr TruncateResult, c *OutputCollector) {
	filePath := c.FilePath()
	offloadErr := c.Err()

	if !tr.Truncated && filePath == "" {
		return
	}
	if filePath != "" && offloadErr == nil {
		fmt.Fprintf(b, "\n[%s: Showing last %d of %d lines. Full output: %s]",
			name, tr.OutputLines, tr.TotalLines, filePath)
	} else if filePath != "" && offloadErr != nil {
		fmt.Fprintf(b, "\n[%s: Showing last %d of %d lines. Full output file may be incomplete: %s (%s)]",
			name, tr.OutputLines, tr.TotalLines, filePath, offloadErr)
	} else if tr.Truncated {
		fmt.Fprintf(b, "\n[%s: Showing last %d of %d lines]",
			name, tr.OutputLines, tr.TotalLines)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./exec/ -v`
Expected: all PASS (sanitize, truncate, collector, bash tool, bash execute)

**Step 5: Update dispatcher in `cmd/pipe/tools.go`**

The signature changed — `ExecuteBash` stays a standalone function with the same
`(ctx, args)` signature, so the dispatcher needs no changes. Verify:

Run: `go test ./cmd/pipe/ -v`
Expected: PASS

**Step 6: Run `make validate`**

Run: `make validate`
Expected: PASS

**Step 7: Commit**

```bash
git add exec/bash.go exec/bash_test.go
git commit -m "feat(exec): rewrite bash tool with output pipeline

Separate stdout/stderr, sanitize ANSI codes, tail truncation with
file offloading. Accurate total line counts from collector. Kill
on timeout (background execution added in next task)."
```

---

### Task 5: Background Execution

Add auto-backgrounding on timeout, `check_pid`/`kill_pid` management. This
changes `ExecuteBash` from a standalone function to a `BashExecutor` struct that
holds a `BackgroundRegistry`.

**Files:**
- Create: `exec/background.go`
- Create: `exec/background_test.go`
- Modify: `exec/bash.go` (add BashExecutor, update tool schema)
- Modify: `exec/bash_test.go` (add background tests)
- Modify: `cmd/pipe/tools.go` (wire BashExecutor)
- Modify: `cmd/pipe/main.go` (if executor needs wiring)

**Step 1: Write failing tests for background behavior**

Add to `exec/bash_test.go` (or create `exec/background_test.go`):

```go
// exec/background_test.go
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

		// Start a fast command with very short timeout so it backgrounds.
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo done",
			"timeout": 1,
		}))
		require.NoError(t, err)
		text := resultText(t, result)

		if !strings.Contains(text, "backgrounded") {
			t.Skip("command completed before timeout")
		}

		pid := extractPID(t, text)
		// Wait for the process to finish.
		time.Sleep(500 * time.Millisecond)

		result, err = e.Execute(context.Background(), mustJSON(t, map[string]any{
			"check_pid": pid,
		}))
		require.NoError(t, err)
		text = resultText(t, result)
		assert.Contains(t, text, "exited")
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./exec/ -run TestBackgroundExecution -v`
Expected: compilation error — `NewBashExecutor` not defined

**Step 3: Implement background.go**

```go
// exec/background.go
package exec

import (
	"fmt"
	osexec "os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fwojciec/pipe"
)

// BackgroundProcess tracks a process that was auto-backgrounded on timeout.
type BackgroundProcess struct {
	cmd        *osexec.Cmd
	stdout     *OutputCollector
	stderr     *OutputCollector
	waitCh     <-chan error
	stdoutDone <-chan struct{}
	stderrDone <-chan struct{}

	mu       sync.Mutex
	done     bool
	exitCode int
}

// watch waits for the background process to complete and records its exit code.
// Run as a goroutine.
func (bp *BackgroundProcess) watch() {
	waitErr := <-bp.waitCh
	<-bp.stdoutDone
	<-bp.stderrDone
	bp.stdout.Close()
	bp.stderr.Close()

	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.done = true
	if exitErr, ok := waitErr.(*osexec.ExitError); ok {
		bp.exitCode = exitErr.ExitCode()
	}
}

// BackgroundRegistry tracks auto-backgrounded processes.
type BackgroundRegistry struct {
	mu        sync.Mutex
	processes map[int]*BackgroundProcess
}

// NewBackgroundRegistry creates an empty registry.
func NewBackgroundRegistry() *BackgroundRegistry {
	return &BackgroundRegistry{processes: make(map[int]*BackgroundProcess)}
}

// Register adds a background process.
func (r *BackgroundRegistry) Register(pid int, bp *BackgroundProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processes[pid] = bp
}

// Check returns the current status and output of a background process.
func (r *BackgroundRegistry) Check(pid int) (*pipe.ToolResult, error) {
	r.mu.Lock()
	bp, ok := r.processes[pid]
	r.mu.Unlock()

	if !ok {
		return domainError(fmt.Sprintf("no background process with pid %d", pid)), nil
	}

	bp.mu.Lock()
	done := bp.done
	exitCode := bp.exitCode
	bp.mu.Unlock()

	stdoutStr, stdoutTR := processOutput(bp.stdout)
	stderrStr, stderrTR := processOutput(bp.stderr)

	var b strings.Builder
	if done {
		fmt.Fprintf(&b, "[Process %d exited with code %d.\n", pid, exitCode)
	} else {
		fmt.Fprintf(&b, "[Process %d still running.\n", pid)
	}
	if stdoutStr != "" {
		fmt.Fprintf(&b, "\nstdout:\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "\nstderr:\n%s\n", stderrStr)
	}
	appendOffloadNotice(&b, "stdout", stdoutTR, bp.stdout)
	appendOffloadNotice(&b, "stderr", stderrTR, bp.stderr)
	b.WriteString("]")

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: done && exitCode != 0,
	}, nil
}

// Kill terminates a background process and returns its final output.
func (r *BackgroundRegistry) Kill(pid int) (*pipe.ToolResult, error) {
	r.mu.Lock()
	bp, ok := r.processes[pid]
	r.mu.Unlock()

	if !ok {
		return domainError(fmt.Sprintf("no background process with pid %d", pid)), nil
	}

	bp.mu.Lock()
	done := bp.done
	bp.mu.Unlock()

	if !done {
		_ = syscall.Kill(-bp.cmd.Process.Pid, syscall.SIGKILL)
		// Wait for watch goroutine to finish (poll with short sleep).
		for {
			bp.mu.Lock()
			if bp.done {
				bp.mu.Unlock()
				break
			}
			bp.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Build result before removing from registry.
	stdoutStr, stdoutTR := processOutput(bp.stdout)
	stderrStr, stderrTR := processOutput(bp.stderr)

	var b strings.Builder
	fmt.Fprintf(&b, "[Process %d killed.\n", pid)
	if stdoutStr != "" {
		fmt.Fprintf(&b, "\nstdout:\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "\nstderr:\n%s\n", stderrStr)
	}
	appendOffloadNotice(&b, "stdout", stdoutTR, bp.stdout)
	appendOffloadNotice(&b, "stderr", stderrTR, bp.stderr)
	b.WriteString("]")

	// Remove from registry.
	r.mu.Lock()
	delete(r.processes, pid)
	r.mu.Unlock()

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: false,
	}, nil
}
```

**Step 4: Add BashExecutor to bash.go and update tool schema**

Add to `exec/bash.go`:

```go
// bashExecutorArgs extends bashArgs with background management parameters.
type bashExecutorArgs struct {
	Command  string `json:"command"`
	Timeout  int    `json:"timeout"`
	CheckPID int    `json:"check_pid"`
	KillPID  int    `json:"kill_pid"`
}

// BashExecutorTool returns the tool definition with background parameters.
func BashExecutorTool() pipe.Tool {
	return pipe.Tool{
		Name: "bash",
		Description: fmt.Sprintf(
			"Execute a bash command. Output truncated to last %d lines or %dKB; "+
				"if truncated, full output saved to temp file readable with the read tool. "+
				"Commands exceeding timeout are auto-backgrounded.",
			DefaultMaxLines, DefaultMaxBytes/1024,
		),
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "The bash command to execute"
				},
				"timeout": {
					"type": "integer",
					"description": "Timeout in milliseconds before auto-backgrounding (default: 120000)"
				},
				"check_pid": {
					"type": "integer",
					"description": "Check on a backgrounded process and return new output"
				},
				"kill_pid": {
					"type": "integer",
					"description": "Kill a backgrounded process and return final output"
				}
			}
		}`),
	}
}

// BashExecutor executes bash commands with background process management.
type BashExecutor struct {
	bg *BackgroundRegistry
}

// NewBashExecutor creates a BashExecutor with a fresh background registry.
func NewBashExecutor() *BashExecutor {
	return &BashExecutor{bg: NewBackgroundRegistry()}
}

// Execute runs a bash command or manages a background process.
func (e *BashExecutor) Execute(ctx context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a bashExecutorArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	switch {
	case a.CheckPID > 0:
		return e.bg.Check(a.CheckPID)
	case a.KillPID > 0:
		return e.bg.Kill(a.KillPID)
	case a.Command != "":
		return e.runCommand(ctx, a)
	default:
		return domainError("one of command, check_pid, or kill_pid is required"), nil
	}
}

func (e *BashExecutor) runCommand(ctx context.Context, a bashExecutorArgs) (*pipe.ToolResult, error) {
	timeout := 120 * time.Second
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout) * time.Millisecond
	}

	// Use exec.Command (not CommandContext) so timeout doesn't auto-kill —
	// we want to auto-background instead.
	cmd := osexec.Command("bash", "-c", a.Command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return domainError(fmt.Sprintf("failed to create stdout pipe: %s", err)), nil
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return domainError(fmt.Sprintf("failed to create stderr pipe: %s", err)), nil
	}

	if err := cmd.Start(); err != nil {
		return domainError(fmt.Sprintf("failed to start command: %s", err)), nil
	}

	stdoutC := NewOutputCollector(int64(DefaultMaxBytes), rollingBufSize)
	stderrC := NewOutputCollector(int64(DefaultMaxBytes), rollingBufSize)

	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	go func() { io.Copy(stdoutC, stdoutPipe); close(stdoutDone) }()
	go func() { io.Copy(stderrC, stderrPipe); close(stderrDone) }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case waitErr := <-waitCh:
		// Process completed before timeout.
		<-stdoutDone
		<-stderrDone
		stdoutC.Close()
		stderrC.Close()
		return e.formatCompletedResult(waitErr, stdoutC, stderrC), nil

	case <-timer.C:
		// Timeout: auto-background.
		pid := cmd.Process.Pid
		bg := &BackgroundProcess{
			cmd:        cmd,
			stdout:     stdoutC,
			stderr:     stderrC,
			waitCh:     waitCh,
			stdoutDone: stdoutDone,
			stderrDone: stderrDone,
		}
		go bg.watch()
		e.bg.Register(pid, bg)

		stdoutStr, _ := processOutput(stdoutC)
		stderrStr, _ := processOutput(stderrC)

		var b strings.Builder
		fmt.Fprintf(&b, "[Command backgrounded after %s timeout (pid %d).\n", timeout, pid)
		if stdoutStr != "" {
			fmt.Fprintf(&b, "\nstdout (partial):\n%s\n", stdoutStr)
		}
		if stderrStr != "" {
			fmt.Fprintf(&b, "\nstderr (partial):\n%s\n", stderrStr)
		}
		b.WriteString("\nUse check_pid or kill_pid to manage.]")

		return &pipe.ToolResult{
			Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
			IsError: false,
		}, nil

	case <-ctx.Done():
		// External cancellation: kill.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-waitCh
		<-stdoutDone
		<-stderrDone
		stdoutC.Close()
		stderrC.Close()
		return domainError(fmt.Sprintf("command cancelled: %s", ctx.Err())), nil
	}
}

func (e *BashExecutor) formatCompletedResult(waitErr error, stdout, stderr *OutputCollector) *pipe.ToolResult {
	exitCode := 0
	isError := false
	if waitErr != nil {
		isError = true
		var exitErr *osexec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return formatResult(exitCode, isError, stdout, stderr)
}
```

Note: `io` import needed for `io.Copy`. Add to import block.

**Step 5: Run tests**

Run: `go test ./exec/ -v`
Expected: all PASS

**Step 6: Commit**

```bash
git add exec/background.go exec/background_test.go exec/bash.go exec/bash_test.go
git commit -m "feat(exec): add background process management

Auto-background commands on timeout instead of killing. check_pid
and kill_pid for monitoring and terminating background processes.
BashExecutor struct holds BackgroundRegistry for session-scoped
process tracking."
```

---

### Task 6: Wire Up Dispatcher

Update `cmd/pipe/tools.go` to use `BashExecutor` and `BashExecutorTool`.

**Files:**
- Modify: `cmd/pipe/tools.go`
- Modify: `cmd/pipe/main.go`
- Modify: `cmd/pipe/tools_test.go`

**Step 1: Update executor struct in `cmd/pipe/tools.go`**

```go
// cmd/pipe/tools.go
type executor struct {
	bash *pipeexec.BashExecutor
}

func newExecutor() *executor {
	return &executor{
		bash: pipeexec.NewBashExecutor(),
	}
}

func (e *executor) Execute(ctx context.Context, name string, args json.RawMessage) (*pipe.ToolResult, error) {
	switch name {
	case "bash":
		return e.bash.Execute(ctx, args)
	case "read":
		return fs.ExecuteRead(ctx, args)
	// ... rest unchanged
	}
}

func tools() []pipe.Tool {
	return []pipe.Tool{
		pipeexec.BashExecutorTool(), // was BashTool()
		fs.ReadTool(),
		// ... rest unchanged
	}
}
```

**Step 2: Update `cmd/pipe/main.go`** to use `newExecutor()` instead of `&executor{}`.

**Step 3: Update `cmd/pipe/tools_test.go`**

Replace all `&executor{}` with `newExecutor()`. The `executor` struct now has a
`bash *pipeexec.BashExecutor` field that must be initialized — bare `&executor{}`
leaves `bash` nil, causing a panic on bash dispatch.

```go
// In every test case, change:
//   exec := &executor{}
// to:
//   exec := newExecutor()
```

There are 8 occurrences across the test file (lines 20, 37, 53, 70, 91, 107,
120, 132 in the current tools_test.go).

**Step 4: Run tests**

Run: `go test ./cmd/pipe/ -v && go test ./... -v`
Expected: all PASS

**Step 5: Run `make validate`**

Run: `make validate`
Expected: PASS

**Step 6: Commit**

```bash
git add cmd/pipe/tools.go cmd/pipe/tools_test.go cmd/pipe/main.go
git commit -m "wire BashExecutor into tool dispatcher

Replace standalone ExecuteBash with BashExecutor for background
process support. Use BashExecutorTool for updated schema."
```

---

### Task 7: Clean Up

**Step 1:** Verify the standalone `ExecuteBash` is still used (it may be called
directly in non-background contexts). If only `BashExecutor.Execute` is used,
consider whether to keep `ExecuteBash` as a convenience or remove it. If keeping
it, ensure it and `BashExecutor` share the same `formatResult` / `processOutput`
helpers (they should already from Task 4).

**Step 2:** Run full test suite.

Run: `go test ./... -v && make validate`
Expected: all PASS

**Step 3: Commit if any changes**

```bash
git add -A
git commit -m "chore: clean up bash tool after rewrite"
```
