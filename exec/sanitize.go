package exec

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Sanitize strips ANSI escape codes and control characters from command output.
// It preserves tabs and newlines but removes all other bytes <= 0x1F.
// CRLF sequences are normalized to LF. Lone CR simulates terminal carriage
// return behavior: text after \r overwrites from the beginning of the line.
func Sanitize(s string) string {
	// Strip ANSI escape sequences (CSI, OSC, etc.) using parser-based stripper.
	s = ansi.Strip(s)

	// Normalize CRLF → LF before filtering, so \r in \r\n isn't dropped.
	s = strings.ReplaceAll(s, "\r\n", "\n")

	// Filter control characters, keeping only \t (0x09), \n (0x0A), and \r (0x0D).
	// We keep \r temporarily to resolve carriage return overwrites below.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' || r > 0x1F {
			b.WriteRune(r)
		}
	}
	s = b.String()

	// Resolve lone \r (carriage return overwrites) within each line.
	// In terminal behavior, \r moves cursor to column 0 and subsequent text
	// overwrites from there. We simulate this per-line.
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.ContainsRune(line, '\r') {
			lines[i] = resolveCarriageReturns(line)
		}
	}
	return strings.Join(lines, "\n")
}

// resolveCarriageReturns simulates terminal CR behavior within a single line.
// Each \r resets the write position to 0; subsequent characters overwrite.
func resolveCarriageReturns(line string) string {
	segments := strings.Split(line, "\r")
	buf := []rune(segments[0])
	for _, seg := range segments[1:] {
		runes := []rune(seg)
		for j, r := range runes {
			if j < len(buf) {
				buf[j] = r
			} else {
				buf = append(buf, r)
			}
		}
		// If the new segment is longer than old buf, buf already grew.
		// If shorter, trailing chars from previous content remain (terminal behavior).
	}
	return string(buf)
}
