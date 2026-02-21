package bubbletea_test

import (
	"encoding/json"
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestToolCallBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("collapsed shows tool name", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", "tc-1", styles)
		view := block.View(80)
		assert.Contains(t, view, "▶")
		assert.Contains(t, view, "read")
	})

	t.Run("expanded shows arguments", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", "tc-1", styles)
		block.AppendArgs(`{"path": "/tmp/foo"}`)
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolCallBlock).View(80)
		assert.Contains(t, view, "▼")
		assert.Contains(t, view, "/tmp/foo")
	})

	t.Run("finalize with call applies arguments from EventToolCallEnd", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		// Simulate Gemini pattern: begin + end with no deltas.
		block := bt.NewToolCallBlock("bash", "tc-2", styles)
		block.FinalizeWithCall(pipe.ToolCallBlock{
			ID:        "tc-2",
			Name:      "bash",
			Arguments: json.RawMessage(`{"command":"ls"}`),
		})
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolCallBlock).View(80)
		assert.Contains(t, view, "ls")
	})

	t.Run("finalize does not overwrite streamed args", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("bash", "tc-3", styles)
		block.AppendArgs(`{"command":"echo hi"}`)
		block.FinalizeWithCall(pipe.ToolCallBlock{
			ID:        "tc-3",
			Name:      "bash",
			Arguments: json.RawMessage(`{"command":"ls"}`),
		})
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolCallBlock).View(80)
		assert.Contains(t, view, "echo hi")
		assert.NotContains(t, view, `"ls"`)
	})

	t.Run("toggle via ToggleMsg", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", "tc-1", styles)
		block.AppendArgs(`{"path":"x"}`)
		// Starts collapsed.
		assert.NotContains(t, block.View(80), "path")
		// First toggle: expand.
		updated, _ := block.Update(bt.ToggleMsg{})
		block = updated.(*bt.ToolCallBlock)
		assert.Contains(t, block.View(80), "path")
		// Second toggle: collapse again.
		updated, _ = block.Update(bt.ToggleMsg{})
		block = updated.(*bt.ToolCallBlock)
		assert.NotContains(t, block.View(80), "path")
	})

	t.Run("append accumulates argument text", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", "tc-1", styles)
		block.AppendArgs(`{"path":`)
		block.AppendArgs(`"/tmp/foo"}`)
		updated, _ := block.Update(bt.ToggleMsg{})
		view := updated.(*bt.ToolCallBlock).View(80)
		assert.Contains(t, view, "/tmp/foo")
	})

	t.Run("ID returns tool call ID", func(t *testing.T) {
		t.Parallel()
		styles := bt.NewStyles(pipe.DefaultTheme())
		block := bt.NewToolCallBlock("read", "tc-42", styles)
		assert.Equal(t, "tc-42", block.ID())
	})
}
