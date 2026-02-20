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
//
// Invariant across all providers:
//
//	InputTokens      = non-cached input tokens
//	CacheReadTokens  = tokens served from cache (cache hit)
//	CacheWriteTokens = tokens written to cache (cache creation)
//
// Total input tokens = InputTokens + CacheReadTokens + CacheWriteTokens.
// Each category has a different cost rate. Providers normalize their
// API-specific fields to this invariant (e.g., OpenAI subtracts
// cached_tokens from input_tokens to produce InputTokens).
// Providers must clamp to zero: max(0, derived) when subtracting to
// guard against inconsistent upstream data.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}
