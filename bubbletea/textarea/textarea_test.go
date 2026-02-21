package textarea_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe/bubbletea/textarea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findHeightMsg searches a batched command tree for an InputHeightMsg.
func findHeightMsg(t *testing.T, cmd tea.Cmd) (textarea.InputHeightMsg, bool) {
	t.Helper()
	if cmd == nil {
		return textarea.InputHeightMsg{}, false
	}
	msg := cmd()
	if h, ok := msg.(textarea.InputHeightMsg); ok {
		return h, true
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if h, found := findHeightMsg(t, c); found {
				return h, true
			}
		}
	}
	return textarea.InputHeightMsg{}, false
}

func newFocused(t *testing.T) textarea.Model {
	t.Helper()
	ta := textarea.New()
	ta.SetWidth(80)
	ta.Focus()
	return ta
}

func typeString(t *testing.T, ta textarea.Model, s string) textarea.Model {
	t.Helper()
	for _, r := range s {
		ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return ta
}

func TestNew(t *testing.T) {
	t.Parallel()
	ta := textarea.New()
	assert.Equal(t, "", ta.Value())
}

func TestSetValueAndValue(t *testing.T) {
	t.Parallel()
	ta := textarea.New()
	ta.SetWidth(80)
	ta.SetValue("hello\nworld")
	assert.Equal(t, "hello\nworld", ta.Value())
}

func TestTypingInsertsCharacters(t *testing.T) {
	t.Parallel()
	ta := newFocused(t)
	ta = typeString(t, ta, "hello")
	assert.Equal(t, "hello", ta.Value())
}

func TestBackspaceDeletesCharacter(t *testing.T) {
	t.Parallel()
	ta := newFocused(t)
	ta = typeString(t, ta, "hello")
	ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "hell", ta.Value())
}

func TestEnterInsertsNewline(t *testing.T) {
	t.Parallel()

	t.Run("when CheckInputComplete is nil", func(t *testing.T) {
		t.Parallel()
		ta := newFocused(t)
		ta.MaxHeight = 10
		ta = typeString(t, ta, "hello")
		ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyEnter})
		ta = typeString(t, ta, "world")
		assert.Equal(t, "hello\nworld", ta.Value())
	})

	t.Run("when CheckInputComplete returns false", func(t *testing.T) {
		t.Parallel()
		ta := newFocused(t)
		ta.MaxHeight = 10
		ta.CheckInputComplete = func(string) bool { return false }
		ta = typeString(t, ta, "hello")
		ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyEnter})
		ta = typeString(t, ta, "world")
		assert.Equal(t, "hello\nworld", ta.Value())
	})
}

func TestEnterDoesNotInsertNewlineWhenCheckInputCompleteReturnsTrue(t *testing.T) {
	t.Parallel()
	ta := newFocused(t)
	ta.CheckInputComplete = func(string) bool { return true }
	ta = typeString(t, ta, "hello")
	ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, "hello", ta.Value())
	assert.Equal(t, 1, ta.LineCount())
}

func TestCtrlJInsertsNewline(t *testing.T) {
	t.Parallel()

	t.Run("without CheckInputComplete", func(t *testing.T) {
		t.Parallel()
		ta := newFocused(t)
		ta.MaxHeight = 10
		ta = typeString(t, ta, "hello")
		ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
		ta = typeString(t, ta, "world")
		assert.Equal(t, "hello\nworld", ta.Value())
	})

	t.Run("even when CheckInputComplete returns true", func(t *testing.T) {
		t.Parallel()
		ta := newFocused(t)
		ta.MaxHeight = 10
		ta.CheckInputComplete = func(string) bool { return true }
		ta = typeString(t, ta, "hello")
		ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
		ta = typeString(t, ta, "world")
		assert.Equal(t, "hello\nworld", ta.Value())
	})
}

func TestAutoGrow(t *testing.T) {
	t.Parallel()

	t.Run("emits InputHeightMsg when content grows", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.SetHeight(1)
		ta.MaxHeight = 3
		ta.Focus()

		ta = typeString(t, ta, "line1")
		ta, cmd := ta.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})

		require.NotNil(t, cmd)
		heightMsg, found := findHeightMsg(t, cmd)
		require.True(t, found)
		assert.Equal(t, 2, heightMsg.Height)
	})

	t.Run("does not exceed MaxHeight", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.SetHeight(1)
		ta.MaxHeight = 3
		ta.Focus()

		for i := 0; i < 5; i++ {
			ta = typeString(t, ta, "line")
			ta, _ = ta.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
		}

		assert.Equal(t, 3, ta.Height())
	})
}

func TestSetWidthInvalidatesCache(t *testing.T) {
	t.Parallel()
	ta := textarea.New()
	ta.SetWidth(80)
	ta.SetValue("a long line that should wrap at different widths")
	view80 := ta.View()

	ta.SetWidth(20)
	view20 := ta.View()

	require.NotEqual(t, view80, view20)
}

func TestFocusAndBlur(t *testing.T) {
	t.Parallel()

	t.Run("starts blurred", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		assert.False(t, ta.Focused())
	})

	t.Run("focus enables input", func(t *testing.T) {
		t.Parallel()
		ta := textarea.New()
		ta.SetWidth(80)
		ta.Focus()
		assert.True(t, ta.Focused())
		ta = typeString(t, ta, "hello")
		assert.Equal(t, "hello", ta.Value())
	})

	t.Run("blur disables input", func(t *testing.T) {
		t.Parallel()
		ta := newFocused(t)
		ta.Blur()
		assert.False(t, ta.Focused())
		ta = typeString(t, ta, "hello")
		assert.Equal(t, "", ta.Value())
	})
}
