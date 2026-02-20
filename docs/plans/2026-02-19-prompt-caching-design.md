# Prompt Caching Design

## Context

Prompt caching dramatically reduces cost and latency for agentic coding sessions.
Research into Claude Code, pi, and opencode reveals a consensus pattern: cache static
content (system prompt, tools) with explicit breakpoints, let the provider handle
dynamic content (messages) automatically.

Our Anthropic provider currently has zero caching support: no `cache_control` markers,
no cache usage tracking, system prompt sent as a plain string.

## Design Principles

1. **Caching mechanism is provider-private** — breakpoint injection, `cache_control`
   markers, TTL configuration all live in `anthropic/`. The domain has no concept of
   "cache breakpoints" or "cache control".

2. **Cache usage reporting is domain-level** — `pipe.Usage` gets generic cache fields
   that every provider normalizes to. Follows pi's proven pattern where Anthropic,
   OpenAI, and Google all map to the same two fields.

3. **Providers always cache when possible** — no opt-in/opt-out at the domain level.
   Caching is a transparent optimization, not a feature toggle. The Anthropic API
   currently ignores `cache_control` on unsupported models — it does not reject the
   request. If this changes (or a future provider rejects cache markers), the
   provider implementation handles fallback (strip markers, retry), not the domain.
   For Anthropic specifically: if `invalid_request_error` mentions cache params,
   strip all `cache_control` fields and retry once. Not implemented initially (the
   API doesn't reject today), but the design accommodates it in `anthropic/client.go`
   without domain changes. TODO: add a skipped test placeholder
   (`t.Skip("cache fallback not yet needed")`) to track this resilience gap.

## Domain Changes

### `pipe.Usage` — Add two generic cache fields

```go
// Usage tracks token consumption.
//
// Invariant across all providers:
//   InputTokens      = non-cached input tokens
//   CacheReadTokens  = tokens served from cache (cache hit)
//   CacheWriteTokens = tokens written to cache (cache creation)
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
```

Provider normalization:
| Provider  | InputTokens                                  | CacheReadTokens                        | CacheWriteTokens                       |
|-----------|----------------------------------------------|----------------------------------------|----------------------------------------|
| Anthropic | `input_tokens` (already excludes cache)      | `cache_read_input_tokens`              | `cache_creation_input_tokens`          |
| OpenAI    | `input_tokens - cached_tokens`               | `input_tokens_details.cached_tokens`   | `0` (not exposed)                      |
| Google    | `promptTokenCount - cachedContentTokenCount` | `cachedContentTokenCount`              | `0` (not exposed)                      |

### Session persistence

`json/usageDTO` gains `cache_read_tokens` and `cache_write_tokens` fields with
`omitempty` for backward compatibility with existing session files.

## Anthropic Provider Changes

### Caching Strategy: Hybrid (Automatic + Explicit Breakpoints)

Anthropic supports two caching modes. We combine them:

1. **Top-level `cache_control`** on the request body — automatic caching for the
   conversation message window (uses 1 of 4 breakpoint slots)
2. **System prompt** (last block) — explicit breakpoint for stable content
3. **Last tool** — explicit breakpoint for stable tool definitions

This gives 3 breakpoints (of 4 max). Automatic caching handles the sliding window on
messages, so no explicit message markers are needed.

### Cache TTL Configuration

`WithCacheTTL(ttl string)` client option on `anthropic.Client`:
- Default: `""` (omit TTL field, API defaults to 5 minutes)
- Valid value: `"1h"` (useful for long agent runs, 2x base input cost)
- Invalid values: validation error returned from `buildRequestBody` (without
  `anthropic:` prefix — the caller `Stream()` wraps with the prefix)

### System Prompt as Content Blocks

`apiRequest.System` changes from `string` to `[]apiContentBlock` to support
`cache_control` on the system prompt block. A `convertSystem(prompt string)` helper
produces the block array.

### Nullable Cache Fields in SSE

Anthropic's API schema marks cache usage fields as nullable. JSON `null` into Go `int`
fails unmarshal. Both SSE usage types use `*int` for cache fields:

```go
// message_start usage — cache fields nullable per API schema
type sseUsage struct {
    InputTokens              int  `json:"input_tokens"`
    OutputTokens             int  `json:"output_tokens"`
    CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
    CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

// message_delta usage — all cache fields nullable, may be absent entirely
type sseDeltaUsage struct {
    OutputTokens             int  `json:"output_tokens"`
    InputTokens              *int `json:"input_tokens,omitempty"`
    CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
    CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}
```

`handleMessageStart` and `handleMessageDelta` both nil-guard before assigning to
`pipe.Usage` (which uses plain `int`).

### New API Types

```go
type apiCacheControl struct {
    Type string `json:"type"`          // always "ephemeral"
    TTL  string `json:"ttl,omitempty"` // "" (default 5m) or "1h"
}
```

Added as optional fields on `apiRequest` (top-level), `apiContentBlock`, and `apiTool`.

## Test Plan

### Client tests (`anthropic/client_test.go`)
- Update `TestClient_RequestFormat`: system assertion changes from string to
  array-of-blocks with `cache_control`
- `TestClient_CacheMarkers`: verify top-level, system, and last-tool breakpoints;
  verify first tool NOT marked; edge cases (no system, no tools, empty messages)
- `TestClient_CacheTTL`: verify TTL propagation, default omission, invalid TTL
  returns error via `Client.Stream()` (not `buildRequestBody` — external test package).
  Invalid TTL subtest must also assert the HTTP handler was NOT invoked (tracks via
  boolean flag) — ensures validation happens before request construction/sending

### Stream tests (`anthropic/stream_test.go`)
- `TestStream_CacheUsage`: message_start with cache fields, assert Usage populated
- `TestStream_CacheUsageCumulative`: message_start + message_delta both with cache
  fields, assert final cumulative values
- `TestStream_CacheUsageDeltaAbsent`: message_start with cache fields, message_delta
  WITHOUT cache fields, assert message_start values NOT overwritten
- `TestStream_CacheUsageDeltaNull`: message_delta with explicit `null` cache fields,
  assert message_start values NOT overwritten (distinct from absent — tests `*int`
  nil-guard when JSON key is present but value is null)
- `TestStream_CacheUsageNull`: message_start with `null` cache fields, assert no
  unmarshal error, values remain zero
- `TestStream_DeltaInputTokens`: message_start with `input_tokens: 100`, then
  message_delta with `input_tokens: 100` (present) — verify InputTokens updated;
  then variant with input_tokens absent/null in delta — verify not overwritten.
  Protects the total-input invariant from regressions

### JSON tests (`json/json_test.go`)
- Cache usage round-trip: non-zero cache fields survive marshal/unmarshal
- Backward compat: old JSON without cache fields deserializes to zero values

## File Summary

| File | Change |
|------|--------|
| `usage.go` | +2 fields (`CacheReadTokens`, `CacheWriteTokens`), document invariant |
| `json/json.go` | Update `usageDTO` with cache fields |
| `json/json_test.go` | Cache round-trip + backward compat tests |
| `anthropic/anthropic.go` | `apiCacheControl`, `sseDeltaUsage`, `*int` cache fields on `sseUsage`, `CacheControl` on request/block/tool types |
| `anthropic/client.go` | `WithCacheTTL`, `convertSystem`, `injectCacheMarkers`, update `buildRequestBody` |
| `anthropic/stream.go` | Nil-guarded cache usage parsing in both `handleMessageStart` and `handleMessageDelta` |
| `anthropic/client_test.go` | Fix system assertion, add cache marker + TTL tests |
| `anthropic/stream_test.go` | Add cache usage, cumulative, absent, delta-null, and start-null tests |
