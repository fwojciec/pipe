package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
)

var _ MessageBlock = (*ToolCallBlock)(nil)

// ToolCallBlock renders a tool call with a collapsible toggle.
type ToolCallBlock struct {
	name      string
	id        string
	args      strings.Builder
	collapsed bool
	styles    Styles
}

// NewToolCallBlock creates a ToolCallBlock that starts collapsed.
func NewToolCallBlock(name, id string, styles Styles) *ToolCallBlock {
	return &ToolCallBlock{name: name, id: id, collapsed: true, styles: styles}
}

// ID returns the tool call ID for event correlation.
func (b *ToolCallBlock) ID() string { return b.id }

// AppendArgs adds a tool call argument delta.
func (b *ToolCallBlock) AppendArgs(text string) {
	b.args.WriteString(text)
}

// FinalizeWithCall applies the completed tool call, including arguments
// from EventToolCallEnd. This handles providers like Gemini that emit
// begin+end without intermediate deltas.
func (b *ToolCallBlock) FinalizeWithCall(call pipe.ToolCallBlock) {
	if b.args.Len() == 0 && len(call.Arguments) > 0 {
		b.args.Write(call.Arguments)
	}
}

func (b *ToolCallBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	if _, ok := msg.(ToggleMsg); ok {
		b.collapsed = !b.collapsed
	}
	return b, nil
}

func (b *ToolCallBlock) View(width int) string {
	indicator := "▶"
	if !b.collapsed {
		indicator = "▼"
	}
	header := b.styles.ToolCall.Render(indicator + " " + b.name)
	content := header
	if !b.collapsed && b.args.Len() > 0 {
		content = header + "\n" + b.styles.Muted.Render(b.args.String())
	}
	return b.styles.ToolCallBg.
		Width(width).
		Render(content)
}
