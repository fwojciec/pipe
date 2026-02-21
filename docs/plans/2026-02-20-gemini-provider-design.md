# Gemini Provider Design

## Context

Adding Google Gemini 3.1 Pro as a second provider alongside Anthropic. The codebase
follows Ben Johnson's Standard Package Layout: domain types in root, one subdirectory
per external dependency. The new provider lives in `gemini/` and implements
`pipe.Provider`.

Research into opencode and pi-mono revealed:
- Both use the official Go SDK (`google.golang.org/genai`)
- Both rely on implicit caching (no explicit cache resource management)
- Tool call IDs must be generated client-side (Gemini doesn't return them)
- Tool calls arrive complete in chunks, not incrementally streamed
- Thought signatures (Gemini 3) must round-trip across turns

## Design Decisions

1. **Official Go SDK** (`google.golang.org/genai`) — handles auth, types, SSE parsing.
   The SDK's `iter.Seq2` streaming maps naturally to pull-based `stream.Next()`.

2. **Implicit caching only** — Gemini automatically caches repeated prefixes. We read
   `CachedContentTokenCount` from response metadata. No explicit cache resource
   management. Explicit caching can be added later as an optimization.

3. **Thinking with signatures** — Full support for Gemini 3's thought parts and opaque
   `thoughtSignature` blobs. Signatures stored in `ThinkingBlock.Signature` (domain
   type), enabling round-trip across turns and session persistence.

4. **Provider selection via flag + env var** — `-provider` flag with auto-detection
   fallback from `ANTHROPIC_API_KEY` / `GEMINI_API_KEY` env vars.

5. **Anthropic signature support** — Update Anthropic provider to also capture and
   replay `ThinkingBlock.Signature`, since Claude has the same mechanism.

## Domain Changes

### `ThinkingBlock` gains Signature field

```go
type ThinkingBlock struct {
    Thinking  string
    Signature []byte // opaque provider signature; nil if not applicable
}
```

- `[]byte` — both Gemini and Claude return opaque binary-ish data
- `nil` for providers/models that don't use signatures
- Three major providers need this (Gemini, Claude, OpenAI) — not YAGNI
- No validation changes needed

### Session persistence

`json/` package's thinking block DTO gains:

```go
Signature []byte `json:"signature,omitempty"` // base64 by Go's json package
```

Go's `encoding/json` handles `[]byte` ↔ base64 automatically. `omitempty` preserves
backward compatibility with old session files.

## Gemini Provider (`gemini/`)

### `gemini/client.go`

```go
type Client struct {
    client *genai.Client
    model  string
}
```

**Constructor**: `New(ctx, apiKey, ...Option)` — creates `genai.Client` with
`BackendGeminiAPI`. Default model: `gemini-3.1-pro-preview`.

**`Stream(ctx, req)` method**:
1. Convert `pipe.Request` messages → `[]*genai.Content`
2. Build `genai.GenerateContentConfig` with system instruction, tools, thinking config
3. Call `client.Models.GenerateContentStream(ctx, model, contents, config)`
4. Return `&stream{...}` wrapping the SDK iterator

**Message conversion**:

| pipe type | genai equivalent |
|---|---|
| `UserMessage` | `Content{Role: "user"}` with text/image Parts |
| `AssistantMessage` | `Content{Role: "model"}` with text/thinking/functionCall Parts |
| `ToolResultMessage` | `Content{Role: "user"}` with `FunctionResponse` Part |

- `ThinkingBlock.Signature` → `Part.ThoughtSignature` (round-trip)
- Tool name for `FunctionResponse` resolved by scanning prior messages for matching
  `ToolCallBlock.ID`

**Tool conversion**: All `pipe.Tool` entries → single `genai.Tool` with
`FunctionDeclaration` entries. Schema passed via `ParametersJsonSchema` field.

**Tool call IDs**: Generated client-side as `"call_" + uuid`. Mapping tracked
internally for `ToolResultMessage` correlation.

### `gemini/stream.go`

Wraps SDK's `iter.Seq2` into `pipe.Stream`:

```go
type stream struct {
    ctx     context.Context
    next    func() (*genai.GenerateContentResponse, error, bool) // pull-converted
    state   pipe.StreamState
    msg     pipe.AssistantMessage
    pending []pipe.Event  // buffered events from a single chunk
    err     error
}
```

Each SDK chunk can contain multiple Parts. `Next()` buffers all events from a chunk,
drains the buffer one at a time, then pulls the next chunk.

**Part → Event mapping**:
- `Part.Text != ""` && `Part.Thought == true` → `EventThinkingDelta`
- `Part.Text != ""` && `!Part.Thought` → `EventTextDelta`
- `Part.FunctionCall != nil` → `EventToolCallBegin` + `EventToolCallEnd` (complete)

**Usage** (from last chunk with `UsageMetadata`):
```go
cached := int(meta.CachedContentTokenCount)
msg.Usage = pipe.Usage{
    InputTokens:     int(meta.PromptTokenCount) - cached,
    OutputTokens:    int(meta.CandidatesTokenCount),
    CacheReadTokens: cached,
}
```

**Stop reason mapping**:
- `FinishReasonStop` → `StopEndTurn`
- `FinishReasonMaxTokens` → `StopLength`
- Any function calls present → `StopToolUse` (override)
- Safety/recitation/etc → `StopError`
- Context cancelled → `StopAborted`

## Anthropic Provider Updates

### Capture signatures (`stream.go`)

`signature_delta` events during `content_block_delta` handling: accumulate in
`blockState`, finalize on `content_block_stop` → `ThinkingBlock.Signature`.

### Replay signatures (`client.go`)

`ThinkingBlock` with non-nil `Signature` serialized as:
```json
{"type": "thinking", "thinking": "...", "signature": "EqQB..."}
```

Thinking blocks without signatures (old sessions) sent without the field.

## Provider Selection (`cmd/pipe/`)

New `-provider` flag with auto-detection fallback:

```
pipe -provider=gemini -model=gemini-3.1-pro-preview
pipe -provider=anthropic
pipe                          # auto-detect from env vars
```

**Resolution order**:
1. Explicit `-provider` flag → use that provider, require its API key
2. No flag → check env vars: `ANTHROPIC_API_KEY` → anthropic; `GEMINI_API_KEY` → gemini
3. Both set, no flag → error: "multiple API keys found, use -provider flag"
4. Neither set → error with usage message

`-api-key` flag overrides the env var for whichever provider is selected.

## Test Plan

### `gemini/client_test.go`
- `TestClient_ConvertMessages`: all message types map correctly, signature round-trips
- `TestClient_ConvertTools`: pipe.Tool → FunctionDeclaration with correct schema
- `TestClient_ToolCallIDGeneration`: unique IDs, ToolResultMessage name resolution
- `TestClient_SystemInstruction`: system prompt → GenerateContentConfig

### `gemini/stream_test.go`
- `TestStream_TextDelta`: text parts → EventTextDelta, accumulated in msg
- `TestStream_ThinkingDelta`: thought parts → EventThinkingDelta, signature preserved
- `TestStream_ToolCallComplete`: FunctionCall → Begin + End with no delta
- `TestStream_Usage`: CachedContentTokenCount subtracted from PromptTokenCount
- `TestStream_StopReason`: each FinishReason maps correctly; tool call override
- `TestStream_MultiPartChunk`: text + function call in one chunk
- `TestStream_ContextCancelled`: cancelled context → StopAborted

### `anthropic/stream_test.go` (additions)
- `TestStream_ThinkingSignature`: signature_delta accumulates, ThinkingBlock populated
- `TestStream_ThinkingSignatureAbsent`: no delta → nil Signature

### `anthropic/client_test.go` (additions)
- `TestClient_ThinkingSignatureReplay`: Signature present → JSON includes it
- `TestClient_ThinkingNoSignature`: no Signature → no field in JSON

### `json/json_test.go` (additions)
- `TestThinkingSignatureRoundTrip`: non-nil signature survives marshal/unmarshal
- `TestThinkingSignatureBackwardCompat`: old JSON → nil Signature

### `cmd/pipe/` (additions)
- `TestProviderResolution`: flag precedence, auto-detect, both-set error, neither-set

## File Summary

| File | Change |
|------|--------|
| `message.go` | Add `Signature []byte` to `ThinkingBlock` |
| `gemini/client.go` | New — Client, options, conversion, Stream method |
| `gemini/stream.go` | New — Wrap SDK iter into pipe.Stream |
| `gemini/client_test.go` | New — conversion, tool IDs, system instruction |
| `gemini/stream_test.go` | New — text, thinking, tool call, usage, stop reason |
| `anthropic/stream.go` | Capture signature_delta → ThinkingBlock.Signature |
| `anthropic/client.go` | Replay ThinkingBlock.Signature in request JSON |
| `anthropic/stream_test.go` | Signature capture tests |
| `anthropic/client_test.go` | Signature replay tests |
| `json/json.go` | Add signature field to thinkingBlockDTO |
| `json/json_test.go` | Signature round-trip + backward compat |
| `cmd/pipe/main.go` | -provider flag, env var auto-detect, provider wiring |
| `go.mod` | Add google.golang.org/genai dependency |
