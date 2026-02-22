package bubbletea_test

import (
	"strings"
	"testing"

	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestBlockSeparator(t *testing.T) {
	t.Parallel()

	styles := bt.NewStyles(pipe.DefaultTheme())

	toolCall := bt.NewToolCallBlock("read", "tc-1", styles)
	toolResult := bt.NewToolResultBlock("read", "content", false, styles)
	text := bt.NewAssistantTextBlock(pipe.DefaultTheme())
	user := bt.NewUserMessageBlock("hi", styles)
	errBlock := bt.NewErrorBlock(assert.AnError, styles)

	t.Run("tool call then tool call", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n", bt.BlockSeparator(toolCall, toolCall))
	})

	t.Run("tool call then tool result", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n", bt.BlockSeparator(toolCall, toolResult))
	})

	t.Run("tool result then tool result", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n", bt.BlockSeparator(toolResult, toolResult))
	})

	t.Run("tool result then tool call", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n", bt.BlockSeparator(toolResult, toolCall))
	})

	t.Run("text then tool call", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(text, toolCall))
	})

	t.Run("tool result then text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(toolResult, text))
	})

	t.Run("text then text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(text, text))
	})

	t.Run("user then tool call", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(user, toolCall))
	})

	t.Run("user then text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(user, text))
	})

	t.Run("tool call then text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(toolCall, text))
	})

	t.Run("error then tool call", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(errBlock, toolCall))
	})

	t.Run("tool result then error", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "\n\n", bt.BlockSeparator(toolResult, errBlock))
	})
}

func TestModel_BlockSpacing(t *testing.T) {
	t.Parallel()

	t.Run("tool-only sequence has no blank lines", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "read"}}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "read", Content: "file data", IsError: false}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-2", Name: "bash"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-2", Name: "bash"}}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolResult{ToolName: "bash", Content: "output", IsError: false}})

		content := bt.RenderContent(m)
		assert.NotEmpty(t, content, "expected non-empty rendered content")
		// Check separators between blocks only (not within block views).
		lines := strings.Split(content, "\n")
		for i := 0; i+1 < len(lines); i++ {
			if lines[i] == "" && lines[i+1] == "" {
				t.Errorf("found consecutive blank lines at line %d in:\n%s", i, content)
				break
			}
		}
	})

	t.Run("text then tool has blank line", func(t *testing.T) {
		t.Parallel()
		m := initModel(t, nopAgent)
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventTextDelta{Index: 0, Delta: "hello"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallBegin{ID: "tc-1", Name: "read"}})
		m = updateModel(t, m, bt.StreamEventMsg{Event: pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc-1", Name: "read"}}})

		content := bt.RenderContent(m)
		// Find the text block output and tool block output — they should be separated by "\n\n".
		assert.True(t, strings.Contains(content, "\n\n"), "expected blank line between text and tool block, got:\n%s", content)
	})
}
