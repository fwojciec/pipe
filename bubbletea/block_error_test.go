package bubbletea_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestErrorBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders error prefix and message", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewErrorBlock(errors.New("something broke"), styles)
		view := block.View(80)
		assert.Contains(t, view, "Error")
		assert.Contains(t, view, "something broke")
	})

	t.Run("pads view to full width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewErrorBlock(errors.New("fail"), styles)
		view := block.View(40)
		var checked int
		for _, line := range strings.Split(view, "\n") {
			if line == "" {
				continue
			}
			checked++
			assert.Equal(t, 40, lipgloss.Width(line))
		}
		assert.Greater(t, checked, 0, "expected at least one non-empty line")
	})

	t.Run("has 1-space left padding", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewErrorBlock(errors.New("fail"), styles)
		view := block.View(80)
		firstLine := strings.SplitN(view, "\n", 2)[0]
		stripped := ansi.Strip(firstLine)
		assert.True(t, strings.HasPrefix(stripped, " "), "expected leading space, got: %q", stripped)
	})
}
