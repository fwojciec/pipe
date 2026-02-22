# Preserve Gemini function-call thought_signature

Issue: #52

## Problem

Gemini can attach `thought_signature` to `FunctionCall` parts without a
preceding `ThinkingBlock`. Current code only backfills signatures to thinking
blocks, so when no thinking block exists, the signature is dropped. On replay,
Gemini rejects with `INVALID_ARGUMENT`.

## Design

Four changes:

### 1. Domain model (`message.go`)

Add `Signature []byte` to `ToolCallBlock`. Opaque provider metadata, same
pattern as `ThinkingBlock.Signature`.

### 2. Stream ingestion (`gemini/stream.go`)

In `processPart` for `FunctionCall`, always store `part.ThoughtSignature` on
`ToolCallBlock.Signature`. Keep existing `backfillThinkingSignature` so
thinking blocks also get their signatures for egress.

### 3. Egress conversion (`gemini/client.go`)

In `convertParts`, use `ToolCallBlock.Signature` directly on `FunctionCall`
parts. Remove the `lastSig` heuristic that inferred signatures from preceding
thinking blocks. Each content block is now self-contained — no cross-block
inference.

### 4. JSON persistence (`json/content_block.go`)

Add `signature` field to `tool_call` blocks with base64 encoding (same pattern
as thinking blocks). Old sessions without this field load with nil signature.

## Test changes

- `TestStream_FunctionCallThoughtSignatureNoPrecedingThinkingBlock`: Verify
  signature IS preserved on `ToolCallBlock`.
- `TestStream_FunctionCallThoughtSignatureBackfillsThinking`: Still works, AND
  preserves signature on `ToolCallBlock`.
- New: egress uses call-level signature directly, no inference from thinking.
- New: JSON round-trip for tool call with signature.
- All existing tests remain green.
