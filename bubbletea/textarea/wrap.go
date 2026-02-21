package textarea

import (
	"strings"
	"unicode"

	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// wrapCache caches wrap results. Invalidates all entries when width changes.
type wrapCache struct {
	entries map[string][][]rune
	width   int
}

func newWrapCache() *wrapCache {
	return &wrapCache{
		entries: make(map[string][][]rune),
	}
}

func (c *wrapCache) get(runes []rune, width int) ([][]rune, bool) {
	if width != c.width {
		return nil, false
	}
	v, ok := c.entries[string(runes)]
	return v, ok
}

func (c *wrapCache) set(runes []rune, width int, result [][]rune) {
	if width != c.width {
		c.entries = make(map[string][][]rune)
		c.width = width
	}
	c.entries[string(runes)] = result
}

func (c *wrapCache) invalidate() {
	c.entries = make(map[string][][]rune)
	c.width = 0
}

// wrap performs word wrapping on runes to fit within the given width.
func wrap(runes []rune, width int) [][]rune {
	if width <= 0 {
		return [][]rune{runes}
	}

	var (
		lines  = [][]rune{{}}
		word   []rune
		row    int
		spaces int
	)

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}

		if spaces > 0 {
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], repeatSpaces(spaces)...)
				spaces = 0
				word = nil
			} else {
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], repeatSpaces(spaces)...)
				spaces = 0
				word = nil
			}
		} else if len(word) > 0 {
			lastCharLen := rw.RuneWidth(word[len(word)-1])
			if uniseg.StringWidth(string(word))+lastCharLen > width {
				if len(lines[row]) > 0 {
					row++
					lines = append(lines, []rune{})
				}
				lines[row] = append(lines[row], word...)
				word = nil
			}
		}
	}

	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		lines = append(lines, []rune{})
		lines[row+1] = append(lines[row+1], word...)
		spaces++
		lines[row+1] = append(lines[row+1], repeatSpaces(spaces)...)
	} else {
		lines[row] = append(lines[row], word...)
		spaces++
		lines[row] = append(lines[row], repeatSpaces(spaces)...)
	}

	return lines
}

func repeatSpaces(n int) []rune {
	return []rune(strings.Repeat(" ", n))
}
