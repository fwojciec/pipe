// Package goldmark renders markdown text to ANSI-styled terminal output
// using goldmark for parsing and lipgloss for styling.
package goldmark

import "github.com/fwojciec/pipe"

// Render parses markdown source and returns ANSI-styled terminal output.
// Paragraphs and list items are word-wrapped to width. Code blocks are
// rendered at full width without reflow.
func Render(source string, width int, theme pipe.Theme) string {
	if source == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	r := newRenderer(theme)
	return r.render([]byte(source), width)
}
