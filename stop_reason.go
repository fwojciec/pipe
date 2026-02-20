package pipe

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
