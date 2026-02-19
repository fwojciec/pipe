package pipe

import "time"

// Session represents a conversation session.
type Session struct {
	ID           string
	Messages     []Message
	SystemPrompt string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
