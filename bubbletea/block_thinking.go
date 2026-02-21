package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ MessageBlock = (*ThinkingBlock)(nil)

// ThinkingBlock renders LLM thinking content with a collapsible toggle.
type ThinkingBlock struct {
	content   strings.Builder
	collapsed bool
	styles    Styles
}

// NewThinkingBlock creates a ThinkingBlock that starts collapsed.
func NewThinkingBlock(styles Styles) *ThinkingBlock {
	return &ThinkingBlock{collapsed: true, styles: styles}
}

// Append adds a thinking text delta.
func (b *ThinkingBlock) Append(text string) {
	b.content.WriteString(text)
}

func (b *ThinkingBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	if _, ok := msg.(ToggleMsg); ok {
		b.collapsed = !b.collapsed
	}
	return b, nil
}

func (b *ThinkingBlock) View(width int) string {
	wrap := lipgloss.NewStyle().Width(width)

	indicator := "▶"
	if !b.collapsed {
		indicator = "▼"
	}
	header := b.styles.Thinking.Render(wrap.Render(indicator + " Thinking"))
	if b.collapsed {
		return header
	}
	content := b.styles.Thinking.Render(wrap.Render(b.content.String()))
	return header + "\n" + content
}
