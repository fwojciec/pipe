package bubbletea

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ MessageBlock = (*ToolResultBlock)(nil)

// ToolResultBlock renders the result of a tool execution.
type ToolResultBlock struct {
	toolName string
	content  string
	isError  bool
	styles   Styles
}

// NewToolResultBlock creates a ToolResultBlock.
func NewToolResultBlock(toolName, content string, isError bool, styles Styles) *ToolResultBlock {
	return &ToolResultBlock{toolName: toolName, content: content, isError: isError, styles: styles}
}

func (b *ToolResultBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *ToolResultBlock) View(width int) string {
	contentStyle := b.styles.Muted
	if b.isError {
		contentStyle = b.styles.Error
	}
	header := b.styles.ToolCall.Render(b.toolName)
	rendered := contentStyle.Render(b.content)
	full := header + "\n" + rendered
	return lipgloss.NewStyle().Width(width).Render(full)
}
