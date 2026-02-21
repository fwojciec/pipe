package bubbletea

import tea "github.com/charmbracelet/bubbletea"

// MessageBlock is a renderable element in the conversation.
// Unlike tea.Model, View takes a width parameter so the root model
// controls layout and blocks are testable in isolation.
type MessageBlock interface {
	Update(tea.Msg) (MessageBlock, tea.Cmd)
	View(width int) string
}

// ToggleMsg tells a collapsible block to toggle its collapsed state.
// Sent by the root model when the user presses the toggle key on a focused block.
type ToggleMsg struct{}
