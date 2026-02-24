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
// MaxBuf must be >= threshold so the rolling buffer contains all data when
// the file offload triggers. This guarantees the file receives the complete
// output from the start.
//
// It is safe for concurrent use. Write after Close is a no-op.
type OutputCollector struct {
	mu            sync.Mutex
	buf           []byte
	total         int64
	totalNewlines int
	file          *os.File
	filePath      string
	err           error // first I/O error encountered during offloading
	closed        bool
	threshold     int64
	maxBuf        int
}

// NewOutputCollector creates a collector. Threshold is the byte count at which
// output is offloaded to a temp file. MaxBuf is the rolling buffer size and
// must be >= threshold to ensure no data is lost before offloading begins.
// If maxBuf < threshold, maxBuf is set to threshold.
func NewOutputCollector(threshold int64, maxBuf int) *OutputCollector {
	if int64(maxBuf) < threshold {
		maxBuf = int(threshold)
	}
	return &OutputCollector{
		threshold: threshold,
		maxBuf:    maxBuf,
	}
}

// Write implements io.Writer. Writes after Close are no-ops.
func (c *OutputCollector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return len(p), nil
	}

	n := len(p)
	c.total += int64(n)

	// Count newlines for total line tracking.
	c.totalNewlines += bytes.Count(p, []byte{'\n'})

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

	// Trim rolling buffer (copy to release old backing array).
	if len(c.buf) > c.maxBuf {
		trimmed := make([]byte, c.maxBuf)
		copy(trimmed, c.buf[len(c.buf)-c.maxBuf:])
		c.buf = trimmed
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

// TotalNewlines returns the total number of newlines seen (not just what's in buffer).
func (c *OutputCollector) TotalNewlines() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalNewlines
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

// Close closes the temp file if one was created. Subsequent writes are no-ops.
func (c *OutputCollector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.file != nil {
		err := c.file.Close()
		c.file = nil
		return err
	}
	return nil
}
