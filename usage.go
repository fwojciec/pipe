package pipe

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
