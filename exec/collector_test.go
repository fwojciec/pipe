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

		// Verify Write returns (len(p), nil) per io.Writer contract.
		n, err := c.Write([]byte("hello\n"))
		assert.Equal(t, 6, n)
		assert.NoError(t, err)

		n, err = c.Write([]byte("world\n"))
		assert.Equal(t, 6, n)
		assert.NoError(t, err)

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
		assert.Equal(t, 50, c.TotalNewlines())
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
		t.Cleanup(func() {
			c.Close()
			if p := c.FilePath(); p != "" {
				os.Remove(p)
			}
		})
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
	})

	t.Run("file receives all subsequent writes", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(50, 200)
		t.Cleanup(func() {
			c.Close()
			if p := c.FilePath(); p != "" {
				os.Remove(p)
			}
		})
		c.Write([]byte(strings.Repeat("a", 60))) // triggers offload
		c.Write([]byte(strings.Repeat("b", 60))) // should go to file too

		data, err := os.ReadFile(c.FilePath())
		require.NoError(t, err)
		assert.Equal(t, 120, len(data))
	})

	t.Run("close closes file", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(50, 200)
		c.Write([]byte(strings.Repeat("x", 100)))
		path := c.FilePath()
		require.NotEmpty(t, path)
		t.Cleanup(func() {
			if p := c.FilePath(); p != "" {
				os.Remove(p)
			}
		})

		err := c.Close()
		assert.NoError(t, err)
		// File should still exist (caller responsible for cleanup)
		_, err = os.Stat(path)
		assert.NoError(t, err)
	})

	t.Run("is safe for concurrent writes", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(1024, 2048)
		t.Cleanup(func() {
			c.Close()
			if p := c.FilePath(); p != "" {
				os.Remove(p)
			}
		})
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
		assert.Equal(t, 1000, c.TotalNewlines())     // 10 * 100
	})

	t.Run("write after close is a no-op", func(t *testing.T) {
		t.Parallel()
		c := pipeexec.NewOutputCollector(50, 200)
		c.Write([]byte(strings.Repeat("x", 60))) // triggers offload
		path := c.FilePath()
		require.NotEmpty(t, path)
		t.Cleanup(func() { os.Remove(path) })

		c.Close()
		c.Write([]byte("after close"))

		// TotalBytes should not change after close
		assert.Equal(t, int64(60), c.TotalBytes())
		// FilePath should still point to original file
		assert.Equal(t, path, c.FilePath())
	})

	t.Run("maxBuf is clamped to threshold", func(t *testing.T) {
		t.Parallel()
		// If maxBuf < threshold, constructor clamps it up
		c := pipeexec.NewOutputCollector(500, 50)
		// Write enough to trigger offload — file should contain everything
		c.Write([]byte(strings.Repeat("a", 300)))
		c.Write([]byte(strings.Repeat("b", 300))) // total 600 > threshold 500
		t.Cleanup(func() {
			c.Close()
			if p := c.FilePath(); p != "" {
				os.Remove(p)
			}
		})

		require.NotEmpty(t, c.FilePath())
		data, err := os.ReadFile(c.FilePath())
		require.NoError(t, err)
		assert.Equal(t, 600, len(data))
		assert.True(t, strings.HasPrefix(string(data), strings.Repeat("a", 300)))
	})
}
