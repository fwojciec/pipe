package markdown_test

import (
	"strings"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/markdown"
	"github.com/stretchr/testify/assert"
)

func TestRender(t *testing.T) {
	t.Parallel()

	theme := pipe.DefaultTheme()

	t.Run("empty input returns empty string", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("", 80, theme)
		assert.Equal(t, "", result)
	})

	t.Run("plain paragraph", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("hello world", 80, theme)
		assert.Contains(t, result, "hello world")
	})

	t.Run("heading renders with styling", func(t *testing.T) {
		t.Parallel()
		heading := markdown.Render("# Title", 80, theme)
		paragraph := markdown.Render("Title", 80, theme)
		assert.Contains(t, heading, "Title")
		assert.NotEqual(t, heading, paragraph)
	})

	t.Run("bold text", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("**bold**", 80, theme)
		assert.Contains(t, result, "bold")
	})

	t.Run("italic text", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("*italic*", 80, theme)
		assert.Contains(t, result, "italic")
	})

	t.Run("inline code", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("`code`", 80, theme)
		assert.Contains(t, result, "code")
	})

	t.Run("fenced code block preserves content without reflow", func(t *testing.T) {
		t.Parallel()
		src := "```go\nfmt.Println(\"hello world\")\n```"
		result := markdown.Render(src, 20, theme)
		assert.Contains(t, result, `fmt.Println("hello world")`)
	})

	t.Run("fenced code block shows language label", func(t *testing.T) {
		t.Parallel()
		src := "```python\nprint('hi')\n```"
		result := markdown.Render(src, 80, theme)
		assert.Contains(t, result, "python")
		assert.Contains(t, result, "print('hi')")
	})

	t.Run("bullet list", func(t *testing.T) {
		t.Parallel()
		src := "- one\n- two\n- three"
		result := markdown.Render(src, 80, theme)
		assert.Contains(t, result, "one")
		assert.Contains(t, result, "two")
		assert.Contains(t, result, "three")
	})

	t.Run("ordered list", func(t *testing.T) {
		t.Parallel()
		src := "1. first\n2. second"
		result := markdown.Render(src, 80, theme)
		assert.Contains(t, result, "first")
		assert.Contains(t, result, "second")
	})

	t.Run("link shows text and URL", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("[click](https://example.com)", 80, theme)
		assert.Contains(t, result, "click")
		assert.Contains(t, result, "example.com")
	})

	t.Run("paragraph wraps to width", func(t *testing.T) {
		t.Parallel()
		long := "word1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12"
		result := markdown.Render(long, 30, theme)
		assert.Contains(t, result, "word1")
		assert.Contains(t, result, "word12")
		lines := strings.Split(result, "\n")
		assert.Greater(t, len(lines), 1)
	})

	t.Run("bold italic text", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("***bold italic***", 80, theme)
		assert.Contains(t, result, "bold italic")
	})

	t.Run("multiple paragraphs separated by blank lines", func(t *testing.T) {
		t.Parallel()
		src := "first paragraph\n\nsecond paragraph"
		result := markdown.Render(src, 80, theme)
		assert.Contains(t, result, "first paragraph")
		assert.Contains(t, result, "second paragraph")
	})

	t.Run("heading levels", func(t *testing.T) {
		t.Parallel()
		result := markdown.Render("## Subtitle", 80, theme)
		assert.Contains(t, result, "Subtitle")
	})

	t.Run("nested list", func(t *testing.T) {
		t.Parallel()
		src := "- outer\n  - inner one\n  - inner two"
		result := markdown.Render(src, 80, theme)
		assert.Contains(t, result, "outer")
		assert.Contains(t, result, "inner one")
		assert.Contains(t, result, "inner two")
	})

	t.Run("list item continuation lines are indented", func(t *testing.T) {
		t.Parallel()
		src := "- this is a very long list item that should wrap and have continuation lines properly indented"
		result := markdown.Render(src, 30, theme)
		lines := strings.Split(result, "\n")
		// First line starts with "- ".
		assert.True(t, strings.HasPrefix(lines[0], "- "))
		// Continuation lines should be indented with spaces (not start at column 0).
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) != "" {
				assert.True(t, strings.HasPrefix(line, "  "), "continuation line should be indented: %q", line)
			}
		}
	})
}
