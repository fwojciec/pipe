package bubbletea_test

import (
	"strings"
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestToolResultBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders result content", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "file contents here", false, styles)
		view := block.View(80)
		assert.Contains(t, view, "file contents here")
	})

	t.Run("renders tool name", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "output text", false, styles)
		view := block.View(80)
		assert.Contains(t, view, "bash")
	})

	t.Run("error result contains content", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "command failed", true, styles)
		view := block.View(80)
		assert.Contains(t, view, "command failed")
	})

	t.Run("long result wraps to width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		long := "this is a very long result that should wrap properly within the viewport"
		block := bt.NewToolResultBlock("read", long, false, styles)
		view := block.View(30)
		assert.Contains(t, view, "viewport")
		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 2)
	})
}
