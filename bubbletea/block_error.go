package bubbletea

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ MessageBlock = (*ErrorBlock)(nil)

// ErrorBlock renders an error message.
type ErrorBlock struct {
	err    error
	styles Styles
}

// NewErrorBlock creates an ErrorBlock.
func NewErrorBlock(err error, styles Styles) *ErrorBlock {
	return &ErrorBlock{err: err, styles: styles}
}

func (b *ErrorBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *ErrorBlock) View(width int) string {
	content := b.styles.Error.Render(fmt.Sprintf("Error: %v", b.err))
	return lipgloss.NewStyle().Width(width).Render(content)
}
