package goldmark

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/fwojciec/pipe"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type ansiRenderer struct {
	bold      lipgloss.Style
	italic    lipgloss.Style
	accent    lipgloss.Style
	muted     lipgloss.Style
	underline lipgloss.Style
}

func newRenderer(theme pipe.Theme) *ansiRenderer {
	return &ansiRenderer{
		bold:      lipgloss.NewStyle().Bold(true),
		italic:    lipgloss.NewStyle().Italic(true),
		accent:    lipgloss.NewStyle().Foreground(ansiColor(theme.Accent)).Bold(true),
		muted:     lipgloss.NewStyle().Foreground(ansiColor(theme.Muted)).Faint(true),
		underline: lipgloss.NewStyle().Underline(true),
	}
}

func ansiColor(index int) lipgloss.TerminalColor {
	if index < 0 {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(strconv.Itoa(index))
}

func (r *ansiRenderer) render(source []byte, width int) string {
	p := goldmark.DefaultParser()
	reader := text.NewReader(source)
	doc := p.Parse(reader)

	var buf bytes.Buffer
	r.walkBlock(doc, source, width, &buf)
	return strings.TrimRight(buf.String(), "\n")
}

func (r *ansiRenderer) walkBlock(node ast.Node, source []byte, width int, buf *bytes.Buffer) {
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		r.renderBlock(c, source, width, buf)
	}
}

func (r *ansiRenderer) renderBlock(node ast.Node, source []byte, width int, buf *bytes.Buffer) {
	switch n := node.(type) {
	case *ast.Paragraph:
		inline := r.collectInline(n, source)
		wrapped := lipgloss.NewStyle().Width(width).Render(inline)
		buf.WriteString(wrapped)
		buf.WriteString("\n")
		if n.NextSibling() != nil {
			buf.WriteString("\n")
		}

	case *ast.Heading:
		inline := r.collectInline(n, source)
		styled := r.accent.Render(inline)
		wrapped := lipgloss.NewStyle().Width(width).Render(styled)
		buf.WriteString(wrapped)
		buf.WriteString("\n")
		if n.NextSibling() != nil {
			buf.WriteString("\n")
		}

	case *ast.FencedCodeBlock:
		lang := string(n.Language(source))
		if lang != "" {
			buf.WriteString(r.muted.Render(lang))
			buf.WriteString("\n")
		}
		gutter := r.muted.Render("│") + " "
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			content := strings.TrimRight(string(line.Value(source)), "\n")
			buf.WriteString(gutter + content)
			buf.WriteString("\n")
		}
		if n.NextSibling() != nil {
			buf.WriteString("\n")
		}

	case *ast.CodeBlock:
		gutter := r.muted.Render("│") + " "
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			content := strings.TrimRight(string(line.Value(source)), "\n")
			buf.WriteString(gutter + content)
			buf.WriteString("\n")
		}
		if n.NextSibling() != nil {
			buf.WriteString("\n")
		}

	case *ast.List:
		r.renderList(n, source, width, buf, 0)
		if n.NextSibling() != nil {
			buf.WriteString("\n")
		}

	case *ast.ThematicBreak:
		buf.WriteString("---\n")
		if n.NextSibling() != nil {
			buf.WriteString("\n")
		}

	case *ast.HTMLBlock:
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			buf.Write(line.Value(source))
		}

	default:
		// Blockquotes and other unrecognized blocks: recurse into children.
		// Blockquotes are intentionally not styled — they are uncommon in
		// LLM output and out of scope for the initial renderer.
		r.walkBlock(node, source, width, buf)
	}
}

func (r *ansiRenderer) renderList(node *ast.List, source []byte, width int, buf *bytes.Buffer, depth int) {
	ordered := node.IsOrdered()
	start := node.Start
	itemNum := 0

	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		item, ok := c.(*ast.ListItem)
		if !ok {
			continue
		}
		indent := strings.Repeat("  ", depth)
		var marker string
		if ordered {
			itemNum++
			marker = fmt.Sprintf("%d. ", start+itemNum-1)
		} else {
			marker = "- "
		}

		// Collect item content.
		var itemBuf bytes.Buffer
		for ic := item.FirstChild(); ic != nil; ic = ic.NextSibling() {
			switch in := ic.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				inline := r.collectInline(in, source)
				itemBuf.WriteString(inline)
			case *ast.List:
				if itemBuf.Len() > 0 {
					r.writeListItem(buf, indent, marker, itemBuf.String(), width)
					itemBuf.Reset()
				}
				r.renderList(in, source, width, buf, depth+1)
				marker = strings.Repeat(" ", len(marker))
			default:
				r.renderBlock(ic, source, width, &itemBuf)
			}
		}

		if itemBuf.Len() > 0 {
			r.writeListItem(buf, indent, marker, itemBuf.String(), width)
		}
	}
}

// writeListItem writes a list item with proper continuation-line indentation.
func (r *ansiRenderer) writeListItem(buf *bytes.Buffer, indent, marker, content string, width int) {
	prefix := indent + marker
	itemWidth := width - len(prefix)
	if itemWidth < 10 {
		itemWidth = 10
	}
	wrapped := lipgloss.NewStyle().Width(itemWidth).Render(content)
	lines := strings.Split(wrapped, "\n")
	continuation := strings.Repeat(" ", len(prefix))
	for i, line := range lines {
		if i == 0 {
			buf.WriteString(prefix + line + "\n")
		} else {
			buf.WriteString(continuation + line + "\n")
		}
	}
}

// collectInline recursively collects styled inline text from a node's children.
func (r *ansiRenderer) collectInline(node ast.Node, source []byte) string {
	var buf bytes.Buffer
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		r.renderInline(c, source, &buf)
	}
	return buf.String()
}

func (r *ansiRenderer) renderInline(node ast.Node, source []byte, buf *bytes.Buffer) {
	switch n := node.(type) {
	case *ast.Text:
		buf.Write(n.Segment.Value(source))
		if n.SoftLineBreak() {
			buf.WriteByte(' ')
		}
		if n.HardLineBreak() {
			buf.WriteByte('\n')
		}

	case *ast.String:
		buf.Write(n.Value)

	case *ast.Emphasis:
		inner := r.collectInline(n, source)
		switch n.Level {
		case 1:
			buf.WriteString(r.italic.Render(inner))
		default:
			// Level 2 = bold. Goldmark represents ***bold italic*** as
			// nested Emphasis nodes, so level 3+ is not reachable.
			buf.WriteString(r.bold.Render(inner))
		}

	case *ast.CodeSpan:
		inner := r.collectInline(n, source)
		buf.WriteString(r.bold.Render(inner))

	case *ast.Link:
		inner := r.collectInline(n, source)
		url := string(n.Destination)
		buf.WriteString(r.underline.Render(inner))
		buf.WriteString(" ")
		buf.WriteString(r.muted.Render("(" + url + ")"))

	case *ast.AutoLink:
		url := string(n.URL(source))
		buf.WriteString(r.underline.Render(url))

	case *ast.Image:
		alt := r.collectInline(n, source)
		url := string(n.Destination)
		buf.WriteString(r.underline.Render(alt))
		buf.WriteString(" ")
		buf.WriteString(r.muted.Render("(" + url + ")"))

	case *ast.RawHTML:
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			buf.Write(seg.Value(source))
		}

	default:
		// Recurse for any unrecognized inline.
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			r.renderInline(c, source, buf)
		}
	}
}
