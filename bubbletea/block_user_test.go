package bubbletea_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestUserMessageBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders text without prompt prefix", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewUserMessageBlock("hello world", styles)
		view := block.View(80)
		assert.NotContains(t, view, "> ")
		assert.Contains(t, view, "hello world")
	})

	t.Run("pads each line to full width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewUserMessageBlock("test", styles)
		view := block.View(40)
		for _, line := range strings.Split(view, "\n") {
			if line == "" {
				continue
			}
			assert.Equal(t, 40, lipgloss.Width(line))
		}
	})

	t.Run("has 1-space left padding", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewUserMessageBlock("hello", styles)
		view := block.View(80)
		assert.Contains(t, view, " hello")
	})

	t.Run("wraps long text to width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		longText := "short words that keep going and going beyond the viewport width easily"
		block := bt.NewUserMessageBlock(longText, styles)
		view := block.View(30)
		assert.Contains(t, view, "easily")
		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1)
	})
}
