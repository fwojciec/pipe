package bubbletea_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
	bt "github.com/fwojciec/pipe/bubbletea"
	"github.com/fwojciec/pipe/markdown"
	"github.com/stretchr/testify/assert"
)

func TestAssistantTextBlock_View(t *testing.T) {
	t.Parallel()

	t.Run("renders markdown", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("hello **world**")
		view := block.View(80)
		assert.Contains(t, view, "hello")
		assert.Contains(t, view, "world")
	})

	t.Run("append accumulates deltas", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("hello ")
		block.Append("world")
		view := block.View(80)
		assert.Contains(t, view, "hello world")
	})

	t.Run("wraps paragraphs to width", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("short words that keep going and going beyond thirty columns easily")
		view := block.View(30)
		assert.Contains(t, view, "easily")
	})

	t.Run("finalized paragraph stays while trailing text streams", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("first paragraph\n\n")
		block.Append("trailing")
		view := block.View(80)
		assert.Contains(t, view, "first paragraph")
		assert.Contains(t, view, "trailing")
	})

	t.Run("width change re-renders cached finalized content", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("word1 word2 word3 word4 word5 word6\n\ntail")
		narrow := block.View(20)
		wide := block.View(80)
		assert.NotEqual(t, strings.Count(narrow, "\n"), strings.Count(wide, "\n"))
	})

	t.Run("content ending at paragraph boundary has no spurious whitespace", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("complete paragraph\n\n")
		view := block.View(80)
		// The finalized content should render cleanly with no extra blank
		// lines from an empty trailing fragment.
		assert.Contains(t, view, "complete paragraph")
		// promoteFinalized strips the "\n\n" delimiter, so finalizedRaw is
		// "complete paragraph" (without trailing newlines). We compare
		// against the same input to match what renderFinalized produces.
		// TrimRight on both sides normalises renderer-added trailing
		// newlines which are not semantically significant.
		trimmed := strings.TrimRight(view, "\n")
		assert.Equal(t, trimmed, strings.TrimRight(
			markdown.Render("complete paragraph", 80, theme), "\n",
		))
	})

	t.Run("unclosed fenced code block renders safely", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("```go\nfmt.Println(\"x\")")
		view := block.View(80)
		assert.Contains(t, view, "fmt.Println")
	})

	t.Run("blank line inside code fence does not split finalization", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("text\n\n```go\nfunc() {\n\ncode")
		view := block.View(80)
		// The code block content should render as code, not prose.
		assert.Contains(t, view, "code")
		assert.Contains(t, view, "text")
	})

	t.Run("update returns self with no command", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("hello")
		updated, cmd := block.Update(tea.KeyMsg{})
		assert.Equal(t, block, updated)
		assert.Nil(t, cmd)
	})

	t.Run("empty content renders empty string", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		view := block.View(80)
		assert.Empty(t, view)
	})

	t.Run("zero width renders gracefully", func(t *testing.T) {
		t.Parallel()
		theme := pipe.DefaultTheme()
		styles := bt.NewStyles(theme)
		block := bt.NewAssistantTextBlock(theme, styles)
		block.Append("hello world")
		view := block.View(0)
		assert.NotPanics(t, func() { block.View(0) })
		_ = view
	})
}
