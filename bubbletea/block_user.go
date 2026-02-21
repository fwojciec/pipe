package bubbletea

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ MessageBlock = (*UserMessageBlock)(nil)

// UserMessageBlock renders a user message with a "> " prefix.
type UserMessageBlock struct {
	text   string
	styles Styles
}

// NewUserMessageBlock creates a UserMessageBlock.
func NewUserMessageBlock(text string, styles Styles) *UserMessageBlock {
	return &UserMessageBlock{text: text, styles: styles}
}

func (b *UserMessageBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *UserMessageBlock) View(width int) string {
	content := b.styles.UserMsg.Render("> ") + b.text
	return lipgloss.NewStyle().Width(width).Render(content)
}
