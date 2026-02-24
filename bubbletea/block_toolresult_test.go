package bubbletea_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestToolResultBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("success result starts collapsed with summary", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "file contents here", false, styles)
		view := block.View(80)
		assert.Contains(t, view, "read")
		assert.Contains(t, view, "✓")
		assert.Contains(t, view, "file contents here")
		// Collapsed: shows ▶ indicator (not ▼).
		assert.Contains(t, view, "▶")
		assert.NotContains(t, view, "▼")
	})

	t.Run("error result starts expanded", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "command failed", true, styles)
		view := block.View(80)
		assert.Contains(t, view, "bash")
		assert.Contains(t, view, "✗")
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "command failed")
	})

	t.Run("collapsed shows first-line preview truncated to 60 chars", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		long := "this is a very long first line that exceeds sixty characters and should be truncated properly"
		block := bt.NewToolResultBlock("read", long, false, styles)
		view := block.View(120)
		stripped := ansi.Strip(view)
		// Should contain truncated preview, not the full line.
		assert.NotContains(t, stripped, "truncated properly")
		assert.Contains(t, stripped, "…")
	})

	t.Run("collapsed preview uses only first line of multiline content", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "first line\nsecond line\nthird line", false, styles)
		view := block.View(80)
		stripped := ansi.Strip(view)
		assert.Contains(t, stripped, "first line")
		assert.NotContains(t, stripped, "second line")
	})

	t.Run("toggle expands collapsed success result", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "first line\nsecond line", false, styles)
		// Starts collapsed: no second line.
		assert.NotContains(t, ansi.Strip(block.View(80)), "second line")
		// Toggle to expand.
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolResultBlock).View(80)
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "second line")
	})

	t.Run("toggle does not collapse expanded error result", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "error details\nmore info", true, styles)
		// Starts expanded.
		assert.Contains(t, block.View(80), "more info")
		// Toggle should keep it expanded.
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolResultBlock).View(80)
		stripped := ansi.Strip(view)
		assert.Contains(t, stripped, "more info")
		assert.Contains(t, stripped, "▼")
	})

	t.Run("set collapsed does not collapse error result", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "error details\nmore info", true, styles)
		updated, _ := block.Update(bt.SetCollapsedMsg{Collapsed: true})
		view := updated.(*bt.ToolResultBlock).View(80)
		stripped := ansi.Strip(view)
		assert.Contains(t, stripped, "more info")
		assert.Contains(t, stripped, "▼")
	})

	t.Run("expanded shows header without preview and full content", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "line one\nline two", false, styles)
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolResultBlock).View(80)
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "read")
		assert.Contains(t, view, "line one")
		assert.Contains(t, view, "line two")
	})

	t.Run("empty content renders header only", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "", false, styles)
		view := block.View(80)
		assert.Contains(t, view, "read")
		assert.Contains(t, view, "✓")
	})

	t.Run("pads collapsed view to full width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "some content", false, styles)
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

	t.Run("pads expanded view to full width", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("bash", "error output", true, styles)
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
		block := bt.NewToolResultBlock("read", "content", false, styles)
		view := block.View(80)
		firstLine := strings.SplitN(view, "\n", 2)[0]
		stripped := ansi.Strip(firstLine)
		assert.True(t, strings.HasPrefix(stripped, " "), "expected leading space, got: %q", stripped)
	})

	t.Run("double toggle returns to original state", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolResultBlock("read", "first line\nsecond line", false, styles)
		original := block.View(80)
		updated, _ := block.Update(bt.ToggleMsg{})
		updated, _ = updated.Update(bt.ToggleMsg{})
		assert.Equal(t, original, updated.(*bt.ToolResultBlock).View(80))
	})
}
