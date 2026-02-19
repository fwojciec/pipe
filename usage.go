package pipe

// Role represents the role of a message sender.
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "tool_result"
)

// StopReason indicates why the assistant stopped generating.
type StopReason string

const (
	StopEndTurn StopReason = "end_turn"
	StopLength  StopReason = "length"
	StopToolUse StopReason = "tool_use"
	StopError   StopReason = "error"
	StopAborted StopReason = "aborted"
	StopUnknown StopReason = "unknown"
)

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int
	OutputTokens int
}
