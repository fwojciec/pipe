package bubbletea_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestThinkingBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("collapsed shows indicator and label", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("deep thoughts here")
		view := block.View(80)
		assert.Contains(t, view, "▶")
		assert.Contains(t, view, "Thinking")
		assert.NotContains(t, view, "deep thoughts here")
	})

	t.Run("expanded shows content", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("deep thoughts here")
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ThinkingBlock).View(80)
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "deep thoughts here")
	})

	t.Run("toggle via ToggleMsg", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("thoughts")
		// Starts collapsed.
		assert.NotContains(t, block.View(80), "thoughts")
		// ToggleMsg expands it.
		updated, _ := block.Update(bt.ToggleMsg{})
		block = updated.(*bt.ThinkingBlock)
		assert.Contains(t, block.View(80), "thoughts")
	})

	t.Run("expanded with empty content", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ThinkingBlock).View(80)
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "Thinking")
	})

	t.Run("wraps long content to width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("short words that keep going and going beyond the viewport width easily")
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ThinkingBlock).View(30)
		assert.Contains(t, view, "easily")
		// Content should wrap across multiple lines (excluding header).
		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 2)
	})

	t.Run("unrecognized message does not change state", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("thoughts")
		updated, _ := block.Update(tea.KeyMsg{})
		view := updated.(*bt.ThinkingBlock).View(80)
		assert.NotContains(t, view, "thoughts")
		assert.Contains(t, view, "▶")
	})

	t.Run("append accumulates text", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewThinkingBlock(styles)
		block.Append("hello ")
		block.Append("world")
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ThinkingBlock).View(80)
		assert.Contains(t, view, "hello world")
	})
}
