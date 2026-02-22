package pipe

// Theme defines semantic color mappings using ANSI color indices (0-15).
// The user's terminal theme determines the actual RGB values, so the app
// automatically matches any color scheme.
type Theme struct {
	UserMsg      int // User message accent
	Thinking     int // Thinking block text
	ToolCall     int // Tool call header
	Error        int // Error messages
	Success      int // Success indicators
	Muted        int // Status bar, placeholders
	CodeBg       int // Code block background
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
		CodeBg:       0,
		Accent:       5,
		UserBg:       4,
		ToolCallBg:   3,
		ToolResultBg: 8,
		ErrorBg:      1,
	}
}
