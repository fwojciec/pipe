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
		assert.Less(t, r.OutputBytes, 351)
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
		assert.Contains(t, r.Content, "line 4999")
		assert.Contains(t, r.Content, "line 3000")
		assert.NotContains(t, r.Content, "line 0000\n")
	})
}
```

Note: add `"fmt"` to imports.

**Step 2: Run tests to verify they fail**

Run: `go test ./exec/ -run TestTruncateTail -v`
Expected: compilation error — `TruncateTail`, `TruncateResult`, `DefaultMaxLines`, `DefaultMaxBytes` not defined

**Step 3: Implement tail truncation**

```go
// exec/truncate.go
package exec

const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024 // 50KB
)

// TruncateResult describes the outcome of tail truncation.
type TruncateResult struct {
	Content        string
	Truncated      bool
	TruncatedBy    string // "lines" or "bytes"
	TotalLines     int
	TotalBytes     int
	OutputLines    int
	OutputBytes    int
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

	lines := splitLines(s)
	totalLines := len(lines)
	totalBytes := len(s)

	// Check if within limits.
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncateResult{
			Content:    s,
			TotalLines: totalLines,
			TotalBytes: totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
		}
	}

	// Work backwards collecting lines.
	var collected []string
	outputBytes := 0
	truncatedBy := ""

	for i := len(lines) - 1; i >= 0 && len(collected) < maxLines; i-- {
		lineBytes := len(lines[i])
		if len(collected) > 0 {
			lineBytes++ // account for the \n separator
		}
		if outputBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			// Edge case: no lines collected yet and this single line exceeds maxBytes.
			if len(collected) == 0 {
				// Take the tail of this line.
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
	content := joinLines(collected, strings.HasSuffix(s, "\n"))

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

// joinLines joins lines with \n. If trailingNewline is true, appends a final \n.
func joinLines(lines []string, trailingNewline bool) string {
	s := strings.Join(lines, "\n")
	if trailingNewline {
		s += "\n"
	}
	return s
}
```

Note: add `"strings"` to imports.

**Step 4: Run tests to verify they pass**

Run: `go test ./exec/ -run TestTruncateTail -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add exec/truncate.go exec/truncate_test.go
git commit -m "feat(exec): add tail truncation for bash output

Keep last 2000 lines or 50KB (whichever hits first). Handles edge
cases: single oversized line, empty input, no trailing newline."
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
	"os"
	"sync"
)

// OutputCollector is an io.Writer that captures command output with:
//   - A rolling buffer (last maxBuf bytes) for in-memory access
//   - File offloading for full output when total exceeds threshold
//
// It is safe for concurrent use.
type OutputCollector struct {
	mu        sync.Mutex
	buf       []byte
	total     int64
	file      *os.File
	filePath  string
	threshold int64
	maxBuf    int
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
	c.buf = append(c.buf, p...)

	// File offloading.
	if c.file == nil && c.total > c.threshold {
		if f, err := os.CreateTemp("", "pipe-bash-*.log"); err == nil {
			c.file = f
			c.filePath = f.Name()
			c.file.Write(c.buf) // flush everything accumulated so far
		}
	} else if c.file != nil {
		c.file.Write(p)
	}

	// Trim rolling buffer.
	if len(c.buf) > c.maxBuf {
		c.buf = c.buf[len(c.buf)-c.maxBuf:]
	}

	return n, nil
}

// Bytes returns the current rolling buffer content.
func (c *OutputCollector) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf...)
}

// TotalBytes returns the total number of bytes written.
func (c *OutputCollector) TotalBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

// FilePath returns the temp file path, or empty if output was not offloaded.
func (c *OutputCollector) FilePath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.filePath
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
file when threshold exceeded."
```

---

### Task 4: Rewrite Bash Execution

Rewrite `ExecuteBash` with: separate stdout/stderr, output pipeline (sanitize →
truncate → offload notice), new result format. No background execution yet.

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
		assert.Contains(t, props, "check_pid")
		assert.Contains(t, props, "kill_pid")
	})
}

func TestExecuteBash(t *testing.T) {
	t.Parallel()

	t.Run("executes simple command", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
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
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo out && echo err >&2",
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.Contains(t, text, "stdout:\nout\n")
		assert.Contains(t, text, "stderr:\nerr\n")
	})

	t.Run("strips ANSI codes from output", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
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
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo fail && exit 42",
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
		text := resultText(t, result)
		assert.Contains(t, text, "exit code: 42")
		assert.Contains(t, text, "fail")
	})

	t.Run("truncates large stdout and offloads to file", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		// Generate output larger than 50KB
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": fmt.Sprintf("seq 1 %d", pipeexec.DefaultMaxLines+1000),
		}))
		require.NoError(t, err)
		require.False(t, result.IsError)
		text := resultText(t, result)

		// Should contain truncation notice
		assert.Contains(t, text, "Showing last")
		assert.Contains(t, text, "Full output:")

		// Should contain last lines but not first
		assert.Contains(t, text, fmt.Sprintf("%d", pipeexec.DefaultMaxLines+1000))
		assert.NotContains(t, text, "\n1\n")

		// Temp file should exist and contain full output
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "Full output:") {
				path := strings.TrimSpace(strings.Split(line, "Full output:")[1])
				path = strings.TrimSuffix(path, "]")
				path = strings.TrimSpace(path)
				_, err := os.Stat(path)
				assert.NoError(t, err, "temp file should exist")
				os.Remove(path)
				break
			}
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(ctx, mustJSON(t, map[string]any{
			"command": "sleep 10",
		}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("respects timeout argument", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("sleep command differs on Windows")
		}
		e := pipeexec.NewBashExecutor()
		start := time.Now()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "sleep 10",
			"timeout": 200,
		}))
		elapsed := time.Since(start)
		require.NoError(t, err)
		// Should be backgrounded or error, not a 10s wait
		assert.Less(t, elapsed, 5*time.Second)
		// Result should indicate backgrounding
		text := resultText(t, result)
		assert.True(t, strings.Contains(text, "backgrounded") || result.IsError)
	})

	t.Run("returns error for missing command", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{}))
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error for invalid JSON args", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), json.RawMessage(`{invalid`))
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("omits empty stderr section", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
			"command": "echo hello",
		}))
		require.NoError(t, err)
		text := resultText(t, result)
		assert.NotContains(t, text, "stderr:")
	})

	t.Run("omits empty stdout section", func(t *testing.T) {
		t.Parallel()
		e := pipeexec.NewBashExecutor()
		result, err := e.Execute(context.Background(), mustJSON(t, map[string]any{
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
Expected: compilation error — `NewBashExecutor` not defined

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
	osexec "os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/fwojciec/pipe"
)

type bashArgs struct {
	Command  string `json:"command"`
	Timeout  int    `json:"timeout"`   // milliseconds
	CheckPID int    `json:"check_pid"`
	KillPID  int    `json:"kill_pid"`
}

// BashTool returns the tool definition for the bash tool.
func BashTool() pipe.Tool {
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

// BashExecutor executes bash commands with output truncation, sanitization,
// file offloading, and background process management.
type BashExecutor struct {
	bg *BackgroundRegistry
}

// NewBashExecutor creates a BashExecutor with a fresh background registry.
func NewBashExecutor() *BashExecutor {
	return &BashExecutor{bg: NewBackgroundRegistry()}
}

// Execute runs a bash command or manages a background process.
func (e *BashExecutor) Execute(ctx context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	switch {
	case a.CheckPID > 0:
		return e.checkPID(a.CheckPID)
	case a.KillPID > 0:
		return e.killPID(a.KillPID)
	case a.Command != "":
		return e.runCommand(ctx, a)
	default:
		return domainError("one of command, check_pid, or kill_pid is required"), nil
	}
}

const (
	rollingBufSize = 2 * DefaultMaxBytes // 100KB rolling buffer
)

func (e *BashExecutor) runCommand(ctx context.Context, a bashArgs) (*pipe.ToolResult, error) {
	timeout := 120 * time.Second
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout) * time.Millisecond
	}

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
	go func() { copyAndClose(stdoutC, stdoutPipe); close(stdoutDone) }()
	go func() { copyAndClose(stderrC, stderrPipe); close(stderrDone) }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case waitErr := <-waitCh:
		// Process completed.
		<-stdoutDone
		<-stderrDone
		stdoutC.Close()
		stderrC.Close()
		return e.formatResult(waitErr, stdoutC, stderrC), nil

	case <-timer.C:
		// Timeout: auto-background.
		pid := cmd.Process.Pid
		bg := &BackgroundProcess{
			cmd:     cmd,
			stdout:  stdoutC,
			stderr:  stderrC,
			waitCh:  waitCh,
			stdoutDone: stdoutDone,
			stderrDone: stderrDone,
		}
		go bg.watch()
		e.bg.Register(pid, bg)
		return e.formatBackgroundResult(pid, timeout, stdoutC, stderrC), nil

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

func (e *BashExecutor) formatResult(waitErr error, stdout, stderr *OutputCollector) *pipe.ToolResult {
	stdoutStr := processOutput(stdout)
	stderrStr := processOutput(stderr)

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

	var b strings.Builder
	if stdoutStr != "" {
		fmt.Fprintf(&b, "stdout:\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "stderr:\n%s\n", stderrStr)
	}
	fmt.Fprintf(&b, "exit code: %d", exitCode)

	// Append truncation notices.
	appendOffloadNotice(&b, "stdout", stdout)
	appendOffloadNotice(&b, "stderr", stderr)

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: isError,
	}
}

func (e *BashExecutor) formatBackgroundResult(pid int, timeout time.Duration, stdout, stderr *OutputCollector) *pipe.ToolResult {
	stdoutStr := processOutput(stdout)
	stderrStr := processOutput(stderr)

	var b strings.Builder
	fmt.Fprintf(&b, "[Command backgrounded after %s timeout (pid %d).\n", timeout, pid)
	if stdoutStr != "" {
		fmt.Fprintf(&b, "\nstdout (partial):\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(&b, "\nstderr (partial):\n%s\n", stderrStr)
	}
	appendOffloadNotice(&b, "stdout", stdout)
	appendOffloadNotice(&b, "stderr", stderr)
	b.WriteString("\nUse check_pid or kill_pid to manage.]")

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: b.String()}},
		IsError: false,
	}
}

// processOutput sanitizes and truncates collector output.
func processOutput(c *OutputCollector) string {
	raw := string(c.Bytes())
	clean := Sanitize(raw)
	tr := TruncateTail(clean, DefaultMaxLines, DefaultMaxBytes)
	return tr.Content
}

func appendOffloadNotice(b *strings.Builder, name string, c *OutputCollector) {
	raw := string(c.Bytes())
	clean := Sanitize(raw)
	tr := TruncateTail(clean, DefaultMaxLines, DefaultMaxBytes)

	if !tr.Truncated && c.FilePath() == "" {
		return
	}

	if c.FilePath() != "" {
		fmt.Fprintf(b, "\n[%s: showing last %d of %d lines. Full output: %s]",
			name, tr.OutputLines, tr.TotalLines, c.FilePath())
	} else if tr.Truncated {
		fmt.Fprintf(b, "\n[%s: showing last %d of %d lines]",
			name, tr.OutputLines, tr.TotalLines)
	}
}

func copyAndClose(dst *OutputCollector, src interface{ Read([]byte) (int, error) }) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// checkPID and killPID are placeholder stubs — implemented in Task 5.
func (e *BashExecutor) checkPID(pid int) (*pipe.ToolResult, error) {
	return e.bg.Check(pid)
}

func (e *BashExecutor) killPID(pid int) (*pipe.ToolResult, error) {
	return e.bg.Kill(pid)
}
```

Note: The `appendOffloadNotice` function re-processes output, which is redundant.
Refactor: have `formatResult` call a helper that returns both the processed string
and truncation metadata. This optimization can be done when tests pass.

**Step 4: Run tests to verify they pass**

Run: `go test ./exec/ -run TestBashTool -v && go test ./exec/ -run TestExecuteBash -v`
Expected: BashTool tests PASS. ExecuteBash tests may need `BackgroundRegistry` stub (Task 5). If so, create a minimal stub first.

**Step 5: Create minimal BackgroundRegistry stub to unblock tests**

```go
// exec/background.go
package exec

import (
	"fmt"
	osexec "os/exec"
	"sync"
	"syscall"

	"github.com/fwojciec/pipe"
)

// BackgroundProcess tracks a process that was auto-backgrounded.
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
	waitErr  error
}

func (bp *BackgroundProcess) watch() {
	err := <-bp.waitCh
	<-bp.stdoutDone
	<-bp.stderrDone
	bp.stdout.Close()
	bp.stderr.Close()

	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.done = true
	bp.waitErr = err
	if exitErr, ok := err.(*osexec.ExitError); ok {
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

// Check returns the current status and new output of a background process.
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

	stdoutStr := processOutput(bp.stdout)
	stderrStr := processOutput(bp.stderr)

	var b fmt.Stringer
	var sb = new(stringBuilder)

	if done {
		fmt.Fprintf(sb, "[Process %d exited with code %d.\n", pid, exitCode)
	} else {
		fmt.Fprintf(sb, "[Process %d still running.\n", pid)
	}
	if stdoutStr != "" {
		fmt.Fprintf(sb, "\nstdout:\n%s\n", stdoutStr)
	}
	if stderrStr != "" {
		fmt.Fprintf(sb, "\nstderr:\n%s\n", stderrStr)
	}
	appendOffloadNotice(sb.inner(), "stdout", bp.stdout)
	appendOffloadNotice(sb.inner(), "stderr", bp.stderr)
	sb.WriteString("]")

	_ = b
	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: sb.String()}},
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
		// Wait for watch goroutine to finish.
		for {
			bp.mu.Lock()
			if bp.done {
				bp.mu.Unlock()
				break
			}
			bp.mu.Unlock()
		}
	}

	// Remove from registry.
	r.mu.Lock()
	delete(r.processes, pid)
	r.mu.Unlock()

	return r.Check(pid)
}
```

Wait — the Kill method calls Check after deleting from registry, which would fail.
The actual implementation should be cleaner. This is a sketch — the real
implementation in Task 5 will be clean. For now, use inline formatting in Kill.

**Step 6: Run full test suite**

Run: `go test ./exec/ -v`
Expected: all PASS (sanitize, truncate, collector, bash tool, bash execute)

**Step 7: Run `make validate`**

Run: `make validate`
Expected: PASS

**Step 8: Commit**

```bash
git add exec/bash.go exec/bash_test.go exec/background.go
git commit -m "feat(exec): rewrite bash tool with output pipeline

Separate stdout/stderr, sanitize ANSI codes, tail truncation with
file offloading, auto-backgrounding on timeout."
```

---

### Task 5: Background Execution

Complete the background process management: auto-backgrounding on timeout,
check_pid, kill_pid.

**Files:**
- Modify: `exec/background.go` (complete implementation from stub)
- Create: `exec/background_test.go`

**Step 1: Write failing tests**

```go
// exec/background_test.go
package exec_test

import (
	"context"
	"encoding/json"
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

		// Extract pid from result.
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

		// Start a command that will be backgrounded.
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
			"timeout": 1, // 1ms — will almost certainly background
		}))
		require.NoError(t, err)
		text := resultText(t, result)

		// It might complete before timeout or get backgrounded.
		if !strings.Contains(text, "backgrounded") {
			t.Skip("command completed before timeout")
		}

		pid := extractPID(t, text)
		// Wait a moment for the process to finish.
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
	// Look for "pid NNNN" pattern.
	var pid int
	for _, line := range strings.Split(text, " ") {
		if _, err := fmt.Sscanf(line, "%d)", &pid); err == nil && pid > 0 {
			return pid
		}
		if _, err := fmt.Sscanf(line, "%d).", &pid); err == nil && pid > 0 {
			return pid
		}
	}
	// Try alternate format: "(pid NNNN)"
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
Expected: failures — background logic not yet properly implemented

**Step 3: Complete background.go implementation**

Clean up `exec/background.go` with proper Check and Kill methods. Ensure
`watch()` goroutine properly handles process completion. Use `strings.Builder`
consistently (not the `stringBuilder` sketch from Task 4).

Key implementation details:
- `Check`: read current collector state, format result based on `done` flag
- `Kill`: send SIGKILL to process group, wait for `done`, format result, remove
  from registry
- `watch()`: wait on `waitCh`, then `stdoutDone`/`stderrDone`, set `done = true`

**Step 4: Run tests**

Run: `go test ./exec/ -run TestBackgroundExecution -v`
Expected: all PASS

**Step 5: Run full suite + validate**

Run: `go test ./exec/ -v && make validate`
Expected: all PASS

**Step 6: Commit**

```bash
git add exec/background.go exec/background_test.go
git commit -m "feat(exec): add background process management

Auto-background commands on timeout. check_pid and kill_pid for
monitoring and terminating background processes."
```

---

### Task 6: Wire Up Dispatcher

Update `cmd/pipe/tools.go` to use `BashExecutor` instead of the standalone
`ExecuteBash` function.

**Files:**
- Modify: `cmd/pipe/tools.go`
- Modify: `cmd/pipe/tools_test.go` (if needed)
- Modify: `cmd/pipe/main.go` (if executor needs wiring)

**Step 1: Update executor struct**

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
	// ... rest unchanged
	}
}
```

**Step 2: Update main.go** to use `newExecutor()` instead of `&executor{}`.

**Step 3: Update tools_test.go** to use `newExecutor()`.

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
process support."
```

---

### Task 7: Clean Up and Delete Old Code

**Step 1:** Remove the old standalone `ExecuteBash` function if it still exists
(it should have been replaced in Task 4). Verify no other callers remain.

**Step 2:** Run full test suite.

Run: `go test ./... -v && make validate`
Expected: all PASS

**Step 3: Commit**

```bash
git add -A
git commit -m "chore: remove unused standalone ExecuteBash"
```
