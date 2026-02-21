package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/markdown"
)

var _ MessageBlock = (*AssistantTextBlock)(nil)

// AssistantTextBlock renders streamed LLM text with markdown formatting.
// Finalized paragraphs (separated by double newline) are rendered once and
// cached; only the trailing unfinalized text is re-rendered on each delta.
type AssistantTextBlock struct {
	content strings.Builder
	theme   pipe.Theme

	// finalizedRaw is the stable prefix ending at the last double newline.
	// It's rendered once per width and cached in finalizedByWidth.
	finalizedRaw     string
	finalizedByWidth map[int]string
}

// NewAssistantTextBlock creates a new block for streaming assistant text.
func NewAssistantTextBlock(theme pipe.Theme, styles Styles) *AssistantTextBlock {
	return &AssistantTextBlock{
		theme:            theme,
		finalizedByWidth: make(map[int]string),
	}
}

// Append adds a text delta from the LLM stream.
func (b *AssistantTextBlock) Append(text string) {
	b.content.WriteString(text)
	b.promoteFinalized()
}

func (b *AssistantTextBlock) Update(msg tea.Msg) (MessageBlock, tea.Cmd) {
	return b, nil
}

func (b *AssistantTextBlock) View(width int) string {
	finalizedRendered := b.renderFinalized(width)
	trailing := b.trailingRaw()
	if hasUnclosedFence(trailing) {
		// Close fence only for rendering so partial streams display safely.
		trailing += "\n```"
	}
	// Empty trailing text (content ends exactly at "\n\n") should not be
	// passed to the renderer — some renderers return whitespace for empty
	// input, which would append spurious blank lines after finalized content.
	if trailing == "" {
		return finalizedRendered
	}
	trailingRendered := markdown.Render(trailing, width, b.theme)
	// Whitespace-only trailing input (e.g. " ") may render to whitespace;
	// treat it the same as empty to avoid spurious blank lines.
	if strings.TrimSpace(trailingRendered) == "" {
		return finalizedRendered
	}
	switch finalizedRendered {
	case "":
		return trailingRendered
	default:
		// Trim trailing/leading whitespace from independently-rendered
		// fragments to avoid a visible seam (extra blank lines) at the
		// finalization boundary. The paragraph break is reconstructed
		// with a single "\n\n" to match full-document render output.
		return strings.TrimRight(finalizedRendered, "\n") + "\n\n" + strings.TrimLeft(trailingRendered, "\n")
	}
}

// promoteFinalized scans for the last "\n\n" boundary that doesn't fall inside
// an unclosed fenced code block. Splitting inside a fence would produce a
// finalized fragment with an unclosed opening fence and a trailing fragment
// starting mid-code-block, causing transient rendering glitches.
func (b *AssistantTextBlock) promoteFinalized() {
	raw := b.content.String()
	// Walk backwards through all "\n\n" positions to find the last one
	// where the prefix has all fences closed.
	for end := len(raw); ; {
		idx := strings.LastIndex(raw[:end], "\n\n")
		if idx <= 0 {
			return
		}
		candidate := raw[:idx]
		if !hasUnclosedFence(candidate) {
			if candidate != b.finalizedRaw {
				b.finalizedRaw = candidate
				// Width-sensitive cache must be invalidated when finalized text grows.
				clear(b.finalizedByWidth)
			}
			return
		}
		end = idx
	}
}

func (b *AssistantTextBlock) renderFinalized(width int) string {
	if width <= 0 || b.finalizedRaw == "" {
		return ""
	}
	if cached, ok := b.finalizedByWidth[width]; ok {
		return cached
	}
	rendered := markdown.Render(b.finalizedRaw, width, b.theme)
	b.finalizedByWidth[width] = rendered
	return rendered
}

func (b *AssistantTextBlock) trailingRaw() string {
	raw := b.content.String()
	if b.finalizedRaw == "" {
		return raw
	}
	prefix := b.finalizedRaw + "\n\n"
	return strings.TrimPrefix(raw, prefix)
}

// hasUnclosedFence detects whether s contains an unclosed fenced code block
// by checking for an odd number of "```" occurrences. This uses a simple
// substring count which does not distinguish triple backticks inside inline
// code spans (e.g., `foo ``` bar`). In practice LLM streaming output rarely
// contains literal triple backticks in inline code, so this is acceptable.
func hasUnclosedFence(s string) bool {
	return strings.Count(s, "```")%2 == 1
}
