package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

var _ MessageBlock = (*ToolResultBlock)(nil)

const maxPreviewLen = 60

// ToolResultBlock renders a tool result with a collapsible toggle.
// Success results start collapsed; error results start expanded.
type ToolResultBlock struct {
	toolName  string
	content   string
	isError   bool
	collapsed bool
	styles    Styles
}

// NewToolResultBlock creates a ToolResultBlock.
func NewToolResultBlock(toolName, content string, isError bool, styles Styles) *ToolResultBlock {
	return &ToolResultBlock{
		toolName:  toolName,
		content:   content,
		isError:   isError,
		collapsed: !isError,
		styles:    styles,
	}
}

func (b *ToolResultBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	if _, ok := msg.(ToggleMsg); ok {
		b.collapsed = !b.collapsed
	}
	return b, nil
}

func (b *ToolResultBlock) View(width int) string {
	statusIcon := "✓"
	if b.isError {
		statusIcon = "✗"
	}

	if b.collapsed {
		return b.viewCollapsed(width, statusIcon)
	}
	return b.viewExpanded(width, statusIcon)
}

func (b *ToolResultBlock) viewCollapsed(width int, statusIcon string) string {
	header := b.styles.ToolCall.Render(b.toolName + " " + statusIcon)
	if b.content != "" {
		preview := firstLine(b.content)
		runes := []rune(preview)
		if len(runes) > maxPreviewLen {
			preview = string(runes[:maxPreviewLen]) + "…"
		}
		if b.isError {
			header += "  " + b.styles.Error.Render(preview)
		} else {
			header += "  " + preview
		}
	}
	return b.styles.ToolResultBg.
		Width(width).
		Render(header)
}

func (b *ToolResultBlock) viewExpanded(width int, statusIcon string) string {
	header := b.styles.ToolCall.Render("▼ " + b.toolName + " " + statusIcon)
	content := header
	if b.content != "" {
		rendered := b.content
		if b.isError {
			rendered = b.styles.Error.Render(b.content)
		}
		content = header + "\n" + rendered
	}
	return b.styles.ToolResultBg.
		Width(width).
		Render(content)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
