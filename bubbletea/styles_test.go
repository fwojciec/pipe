package bubbletea_test

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewStyles(t *testing.T) {
	t.Parallel()

	theme := pipe.DefaultTheme()
	styles := bt.NewStyles(theme)

	assert.Equal(t, lipgloss.Color("4"), styles.UserMsg.GetForeground())
	assert.True(t, styles.UserMsg.GetBold())

	assert.Equal(t, lipgloss.Color("8"), styles.Thinking.GetForeground())
	assert.True(t, styles.Thinking.GetFaint())

	assert.Equal(t, lipgloss.Color("3"), styles.ToolCall.GetForeground())

	assert.Equal(t, lipgloss.Color("1"), styles.Error.GetForeground())

	assert.Equal(t, lipgloss.Color("2"), styles.Success.GetForeground())

	assert.Equal(t, lipgloss.Color("8"), styles.Muted.GetForeground())
	assert.True(t, styles.Muted.GetFaint())

	assert.Equal(t, lipgloss.Color("5"), styles.Accent.GetForeground())
	assert.True(t, styles.Accent.GetBold())

	assert.Equal(t, lipgloss.Color("0"), styles.CodeBg.GetBackground())

	assert.Equal(t, lipgloss.Color("4"), styles.UserBg.GetBackground())
	assert.Equal(t, lipgloss.Color("3"), styles.ToolCallBg.GetBackground())
	assert.Equal(t, lipgloss.Color("8"), styles.ToolResultBg.GetBackground())
	assert.Equal(t, lipgloss.Color("1"), styles.ErrorBg.GetBackground())
}

func TestNewStylesNegativeIndexYieldsNoColor(t *testing.T) {
	t.Parallel()

	theme := pipe.Theme{UserMsg: -1}
	styles := bt.NewStyles(theme)

	assert.Equal(t, lipgloss.NoColor{}, styles.UserMsg.GetForeground())
}
