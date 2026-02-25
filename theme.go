package pipe

// Theme defines semantic color mappings using ANSI color indices.
// Foreground colors use indices 0-15 so the user's terminal theme determines
// the actual RGB values. Background colors use ANSI 256 indices (e.g. 234-236
// for subtle dark grays, 52 for dark red) to avoid colliding with foreground colors.
type Theme struct {
	UserMsg      int // User message accent
	Thinking     int // Thinking block text
	ToolCall     int // Tool call header
	Error        int // Error messages
	Success      int // Success indicators
	Muted        int // Status bar, placeholders
	Accent       int // Headings, links
	UserBg       int // User message block background
	ToolCallBg   int // Tool call block background
	ToolResultBg int // Tool result block background
	ErrorBg      int // Error block background
}

// DefaultTheme returns the default ANSI color mapping.
func DefaultTheme() Theme {
	return Theme{
		UserMsg:      4,
		Thinking:     8,
		ToolCall:     3,
		Error:        1,
		Success:      2,
		Muted:        8,
		Accent:       5,
		UserBg:       234,
		ToolCallBg:   235,
		ToolResultBg: 236,
		ErrorBg:      52,
	}
}
