package exec

import "strings"

const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024 // 50KB
)

// TruncateResult describes the outcome of tail truncation.
type TruncateResult struct {
	Content         string
	Truncated       bool
	TruncatedBy     string // "lines" or "bytes"
	TotalLines      int
	TotalBytes      int
	OutputLines     int
	OutputBytes     int
	LastLinePartial bool
}

// TruncateTail keeps the last maxLines lines or maxBytes bytes of input,
// whichever limit is hit first. It works backwards from the end, collecting
// complete lines. If a single line exceeds maxBytes, it takes the tail of that
// line (setting LastLinePartial).
func TruncateTail(s string, maxLines, maxBytes int) TruncateResult {
	if s == "" {
		return TruncateResult{}
	}

	hasTrailingNewline := strings.HasSuffix(s, "\n")
	lines := splitLines(s)
	totalLines := len(lines)
	totalBytes := len(s)

	// Check if within limits.
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncateResult{
			Content:     s,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
		}
	}

	// Work backwards collecting lines. Budget bytes carefully:
	// the final output is lines joined by \n, optionally with a trailing \n.
	// Reserve 1 byte for trailing newline if original had one.
	byteBudget := maxBytes
	if hasTrailingNewline {
		byteBudget-- // reserve for trailing \n in reconstructed output
	}

	var collected []string
	outputBytes := 0
	truncatedBy := ""

	for i := len(lines) - 1; i >= 0 && len(collected) < maxLines; i-- {
		lineBytes := len(lines[i])
		if len(collected) > 0 {
			lineBytes++ // account for the \n separator between lines
		}
		if outputBytes+lineBytes > byteBudget {
			truncatedBy = "bytes"
			// Edge case: no lines collected yet and this single line exceeds maxBytes.
			// Use maxBytes (not byteBudget) because this path returns a partial
			// line without restoring the trailing newline.
			if len(collected) == 0 {
				tail := lines[i]
				if len(tail) > maxBytes {
					tail = tail[len(tail)-maxBytes:]
				}
				return TruncateResult{
					Content:         tail,
					Truncated:       true,
					TruncatedBy:     "bytes",
					TotalLines:      totalLines,
					TotalBytes:      totalBytes,
					OutputLines:     1,
					OutputBytes:     len(tail),
					LastLinePartial: true,
				}
			}
			break
		}
		collected = append(collected, lines[i])
		outputBytes += lineBytes
	}

	if truncatedBy == "" {
		truncatedBy = "lines"
	}

	// Reverse collected lines (they were added back-to-front).
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}

	// Reconstruct with original trailing newline if present.
	content := strings.Join(collected, "\n")
	if hasTrailingNewline {
		content += "\n"
	}

	return TruncateResult{
		Content:     content,
		Truncated:   true,
		TruncatedBy: truncatedBy,
		TotalLines:  totalLines,
		TotalBytes:  totalBytes,
		OutputLines: len(collected),
		OutputBytes: len(content),
	}
}

// splitLines splits s into lines, treating the final line as a line even
// without a trailing newline. A trailing newline does NOT produce an empty
// final element.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
