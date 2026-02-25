package bubbletea

import (
	"strconv"

	"github.com/charmbracelet/lipgloss"
	"github.com/fwojciec/pipe"
)

// Styles maps a Theme to lipgloss styles for TUI rendering.
type Styles struct {
	UserMsg      lipgloss.Style
	Thinking     lipgloss.Style
	ToolCall     lipgloss.Style
	Error        lipgloss.Style
	Success      lipgloss.Style
	Muted        lipgloss.Style
	Accent       lipgloss.Style
	UserBg       lipgloss.Style
	ToolCallBg   lipgloss.Style
	ToolResultBg lipgloss.Style
	ErrorBg      lipgloss.Style
}

// NewStyles creates Styles from a Theme.
func NewStyles(t pipe.Theme) Styles {
	return Styles{
		UserMsg:      lipgloss.NewStyle().Foreground(ansiColor(t.UserMsg)).Bold(true),
		Thinking:     lipgloss.NewStyle().Foreground(ansiColor(t.Thinking)).Faint(true),
		ToolCall:     lipgloss.NewStyle().Foreground(ansiColor(t.ToolCall)),
		Error:        lipgloss.NewStyle().Foreground(ansiColor(t.Error)),
		Success:      lipgloss.NewStyle().Foreground(ansiColor(t.Success)),
		Muted:        lipgloss.NewStyle().Foreground(ansiColor(t.Muted)).Faint(true),
		Accent:       lipgloss.NewStyle().Foreground(ansiColor(t.Accent)).Bold(true),
		UserBg:       lipgloss.NewStyle().Background(ansiColor(t.UserBg)).PaddingLeft(1),
		ToolCallBg:   lipgloss.NewStyle().Background(ansiColor(t.ToolCallBg)).PaddingLeft(1),
		ToolResultBg: lipgloss.NewStyle().Background(ansiColor(t.ToolResultBg)).PaddingLeft(1),
		ErrorBg:      lipgloss.NewStyle().Background(ansiColor(t.ErrorBg)).PaddingLeft(1),
	}
}

func ansiColor(index int) lipgloss.TerminalColor {
	if index < 0 {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(strconv.Itoa(index))
}
