package goldmark_test

import (
	"strings"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/goldmark"
	"github.com/stretchr/testify/assert"
)

func TestRender(t *testing.T) {
	t.Parallel()

	theme := pipe.DefaultTheme()

	t.Run("empty input returns empty string", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("", 80, theme)
		assert.Equal(t, "", result)
	})

	t.Run("plain paragraph", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("hello world", 80, theme)
		assert.Contains(t, result, "hello world")
	})

	t.Run("heading renders content", func(t *testing.T) {
		t.Parallel()
		heading := goldmark.Render("# Title", 80, theme)
		assert.Contains(t, heading, "Title")
	})

	t.Run("bold text", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("**bold**", 80, theme)
		assert.Contains(t, result, "bold")
	})

	t.Run("italic text", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("*italic*", 80, theme)
		assert.Contains(t, result, "italic")
	})

	t.Run("inline code", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("`code`", 80, theme)
		assert.Contains(t, result, "code")
	})

	t.Run("fenced code block preserves content without reflow", func(t *testing.T) {
		t.Parallel()
		src := "```go\nfmt.Println(\"hello world\")\n```"
		result := goldmark.Render(src, 20, theme)
		assert.Contains(t, result, `fmt.Println("hello world")`)
	})

	t.Run("fenced code block shows language label", func(t *testing.T) {
		t.Parallel()
		src := "```python\nprint('hi')\n```"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "python")
		assert.Contains(t, result, "print('hi')")
	})

	t.Run("bullet list", func(t *testing.T) {
		t.Parallel()
		src := "- one\n- two\n- three"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "one")
		assert.Contains(t, result, "two")
		assert.Contains(t, result, "three")
	})

	t.Run("ordered list", func(t *testing.T) {
		t.Parallel()
		src := "1. first\n2. second"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "first")
		assert.Contains(t, result, "second")
	})

	t.Run("link shows text and URL", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("[click](https://example.com)", 80, theme)
		assert.Contains(t, result, "click")
		assert.Contains(t, result, "example.com")
	})

	t.Run("paragraph wraps to width", func(t *testing.T) {
		t.Parallel()
		long := "word1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12"
		result := goldmark.Render(long, 30, theme)
		assert.Contains(t, result, "word1")
		assert.Contains(t, result, "word12")
		lines := strings.Split(result, "\n")
		assert.Greater(t, len(lines), 1)
	})

	t.Run("bold italic text", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("***bold italic***", 80, theme)
		assert.Contains(t, result, "bold italic")
	})

	t.Run("multiple paragraphs separated by blank lines", func(t *testing.T) {
		t.Parallel()
		src := "first paragraph\n\nsecond paragraph"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "first paragraph")
		assert.Contains(t, result, "second paragraph")
	})

	t.Run("heading levels", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("## Subtitle", 80, theme)
		assert.Contains(t, result, "Subtitle")
	})

	t.Run("nested list", func(t *testing.T) {
		t.Parallel()
		src := "- outer\n  - inner one\n  - inner two"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "outer")
		assert.Contains(t, result, "inner one")
		assert.Contains(t, result, "inner two")
	})

	t.Run("list item continuation lines are indented", func(t *testing.T) {
		t.Parallel()
		src := "- this is a very long list item that should wrap and have continuation lines properly indented"
		result := goldmark.Render(src, 30, theme)
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

	t.Run("fenced code block without language label", func(t *testing.T) {
		t.Parallel()
		src := "```\nsome code\n```"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "some code")
	})

	t.Run("indented code block", func(t *testing.T) {
		t.Parallel()
		src := "paragraph\n\n    indented code\n    more code"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "indented code")
		assert.Contains(t, result, "more code")
	})

	t.Run("thematic break", func(t *testing.T) {
		t.Parallel()
		src := "above\n\n---\n\nbelow"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "above")
		assert.Contains(t, result, "---")
		assert.Contains(t, result, "below")
	})

	t.Run("image renders alt text and URL", func(t *testing.T) {
		t.Parallel()
		src := "![alt text](https://example.com/img.png)"
		result := goldmark.Render(src, 80, theme)
		assert.Contains(t, result, "alt text")
		assert.Contains(t, result, "example.com/img.png")
	})

	t.Run("width zero defaults to 80", func(t *testing.T) {
		t.Parallel()
		result := goldmark.Render("hello world", 0, theme)
		assert.Contains(t, result, "hello world")
	})
}
