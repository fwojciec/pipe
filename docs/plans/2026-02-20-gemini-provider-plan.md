# Gemini Provider Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Google Gemini 3.1 Pro as a second provider, with thinking signature support across both providers.

**Architecture:** SDK-wrapped provider in `gemini/` using `google.golang.org/genai`. Domain type `ThinkingBlock` gains `Signature []byte`. Anthropic provider updated to capture/replay signatures. Provider selection via `-provider` flag with env var auto-detect.

**Tech Stack:** Go 1.24, `google.golang.org/genai` v1.47.0 SDK, `testify` for assertions.

**Key SDK type facts** (verified against `google.golang.org/genai@v1.47.0/types.go`):
- `Part.ThoughtSignature` is `[]byte`, not `string`
- `FunctionCall.ID` exists (`string`) — prefer SDK ID when present, generate client-side as fallback
- `FunctionResponse.ID` exists (`string`) — pass `ToolCallID` through for correlation
- `FunctionResponse.Response` should use `"output"` key for output and `"error"` key for errors

**Cross-provider note:** Thinking signatures are provider-specific opaque blobs. Sessions are bound to a provider. Each provider replays signatures it stored; switching providers mid-session will produce invalid signatures (acceptable edge case).

---

### Task 1: Add Signature field to ThinkingBlock

**Files:**
- Modify: `message.go:70-71` (ThinkingBlock struct)

**Step 1: Write the failing test**

Add to `message_test.go`:

```go
func TestThinkingBlock_SignatureField(t *testing.T) {
	t.Parallel()
	sig := []byte("opaque-signature-data")
	tb := pipe.ThinkingBlock{Thinking: "step by step", Signature: sig}
	assert.Equal(t, "step by step", tb.Thinking)
	assert.Equal(t, sig, tb.Signature)
}

func TestThinkingBlock_SignatureNilByDefault(t *testing.T) {
	t.Parallel()
	tb := pipe.ThinkingBlock{Thinking: "no sig"}
	assert.Nil(t, tb.Signature)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run TestThinkingBlock_Signature -count=1`
Expected: FAIL — `tb.Signature undefined`

**Step 3: Write minimal implementation**

In `message.go`, change:

```go
type ThinkingBlock struct {
	Thinking string
}
```

to:

```go
// ThinkingBlock contains thinking/reasoning content.
// Signature holds an opaque provider-specific signature (e.g., Anthropic's
// thinking block signature, Gemini's thought signature) that must be preserved
// across turns for multi-turn reasoning. Nil when not applicable.
type ThinkingBlock struct {
	Thinking  string
	Signature []byte
}
```

**Step 4: Fix existing tests**

The existing `TestStream_Thinking` in `anthropic/stream_test.go:108` asserts:
```go
assert.Equal(t, pipe.ThinkingBlock{Thinking: "Let me think... step 2"}, msg.Content[0])
```

This will still pass because `Signature` is nil (zero value). Verify no breakage.

Similarly, `json/json_test.go:139` (`TestMarshalSession_AllContentBlockTypes`) creates `pipe.ThinkingBlock{Thinking: "let me think..."}` — also fine since Signature defaults to nil.

**Step 5: Run all tests to verify nothing breaks**

Run: `go test ./... -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add message.go message_test.go
git commit -m "Add Signature field to ThinkingBlock for provider reasoning state"
```

---

### Task 2: Add Signature to JSON serialization

**Files:**
- Modify: `json/content_block.go:12-21` (contentBlock struct)
- Modify: `json/content_block.go:39-40` (marshalContentBlock thinking case)
- Modify: `json/content_block.go:72-76` (unmarshalContentBlock thinking case)
- Test: `json/json_test.go`

**Step 1: Write the failing test**

Add to `json/json_test.go`:

```go
func TestMarshalSession_ThinkingSignatureRoundTrip(t *testing.T) {
	t.Parallel()
	sig := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	session := pipe.Session{
		ID:        "sig-test",
		CreatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		Messages: []pipe.Message{
			pipe.AssistantMessage{
				Content: []pipe.ContentBlock{
					pipe.ThinkingBlock{Thinking: "reasoning", Signature: sig},
				},
				StopReason: pipe.StopEndTurn,
				Timestamp:  time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	got, err := pipejson.UnmarshalSession(data)
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	am := got.Messages[0].(pipe.AssistantMessage)
	require.Len(t, am.Content, 1)
	tb := am.Content[0].(pipe.ThinkingBlock)
	assert.Equal(t, "reasoning", tb.Thinking)
	assert.Equal(t, sig, tb.Signature)
}

func TestUnmarshalSession_ThinkingSignatureBackwardCompat(t *testing.T) {
	t.Parallel()
	// Old JSON without signature field
	data := []byte(`{
		"version": 1,
		"id": "old-thinking",
		"created_at": "2026-02-18T12:00:00Z",
		"updated_at": "2026-02-18T12:00:00Z",
		"messages": [
			{
				"type": "assistant",
				"content": [{"type": "thinking", "thinking": "old reasoning"}],
				"stop_reason": "end_turn",
				"raw_stop_reason": "end_turn",
				"usage": {"input_tokens": 10, "output_tokens": 5},
				"timestamp": "2026-02-18T12:00:00Z"
			}
		]
	}`)

	got, err := pipejson.UnmarshalSession(data)
	require.NoError(t, err)

	am := got.Messages[0].(pipe.AssistantMessage)
	tb := am.Content[0].(pipe.ThinkingBlock)
	assert.Equal(t, "old reasoning", tb.Thinking)
	assert.Nil(t, tb.Signature)
}

func TestMarshalSession_ThinkingSignatureOmitsNil(t *testing.T) {
	t.Parallel()
	session := pipe.Session{
		ID:        "no-sig",
		CreatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
		Messages: []pipe.Message{
			pipe.AssistantMessage{
				Content:    []pipe.ContentBlock{pipe.ThinkingBlock{Thinking: "no signature"}},
				StopReason: pipe.StopEndTurn,
				Timestamp:  time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	data, err := pipejson.MarshalSession(session)
	require.NoError(t, err)

	// Parse raw JSON to verify "signature" key is absent
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	var msgs []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["messages"], &msgs))
	var content []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(msgs[0]["content"], &content))
	assert.NotContains(t, content[0], "signature")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./json/... -run TestMarshalSession_ThinkingSignature -count=1`
Expected: FAIL — round-trip test loses signature because marshal/unmarshal don't handle it

**Step 3: Write minimal implementation**

In `json/content_block.go`, add `Signature` field to `contentBlock` struct:

```go
type contentBlock struct {
	Type      string           `json:"type"`
	Text      *string          `json:"text,omitempty"`
	Thinking  *string          `json:"thinking,omitempty"`
	Signature []byte           `json:"signature,omitempty"` // base64 auto by encoding/json
	Data      *string          `json:"data,omitempty"`
	MimeType  *string          `json:"mime_type,omitempty"`
	ID        *string          `json:"id,omitempty"`
	Name      *string          `json:"name,omitempty"`
	Arguments *json.RawMessage `json:"arguments,omitempty"`
}
```

In `marshalContentBlock`, update the thinking case:

```go
	case pipe.ThinkingBlock:
		return contentBlock{Type: "thinking", Thinking: &v.Thinking, Signature: v.Signature}, nil
```

In `unmarshalContentBlock`, update the thinking case:

```go
	case "thinking":
		var thinking string
		if dto.Thinking != nil {
			thinking = *dto.Thinking
		}
		return pipe.ThinkingBlock{Thinking: thinking, Signature: dto.Signature}, nil
```

**Step 4: Run tests to verify they pass**

Run: `go test ./json/... -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add json/content_block.go json/json_test.go
git commit -m "Add thinking signature to JSON session persistence"
```

---

### Task 3: Capture thinking signatures in Anthropic stream

**Files:**
- Modify: `anthropic/stream.go:26-33` (blockState struct — add signatureBuf)
- Modify: `anthropic/stream.go:268-270` (signature_delta case in handleContentBlockDelta)
- Modify: `anthropic/stream.go:300-301` (handleContentBlockStop, finalize thinking signature)
- Modify: `anthropic/stream.go:265-267` (thinking_delta case — include Signature on ThinkingBlock)
- Test: `anthropic/stream_test.go`

**Step 1: Write the failing test**

Add to `anthropic/stream_test.go`:

```go
func TestStream_ThinkingSignature(t *testing.T) {
	t.Parallel()
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":50,"output_tokens":1}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Step 1"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"EqQBsig1"}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"EqQBsig2"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Answer"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":1}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":10}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)

	tb, ok := msg.Content[0].(pipe.ThinkingBlock)
	require.True(t, ok)
	assert.Equal(t, "Step 1", tb.Thinking)
	assert.Equal(t, []byte("EqQBsig1EqQBsig2"), tb.Signature)
}

func TestStream_ThinkingSignatureAbsent(t *testing.T) {
	t.Parallel()
	// No signature_delta events — Signature should be nil.
	resp := sseResponse{events: []sseEvent{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":50,"output_tokens":1}}}`},
		{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`},
		{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reasoning"}}`},
		{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}}

	s := streamFromSSE(t, resp)
	collectEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 1)

	tb, ok := msg.Content[0].(pipe.ThinkingBlock)
	require.True(t, ok)
	assert.Equal(t, "reasoning", tb.Thinking)
	assert.Nil(t, tb.Signature)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./anthropic/... -run TestStream_ThinkingSignature -count=1`
Expected: FAIL — `TestStream_ThinkingSignature` fails because Signature is nil (signature_delta silently dropped)

**Step 3: Write minimal implementation**

In `anthropic/stream.go`, add `signatureBuf` to `blockState`:

```go
type blockState struct {
	blockType    string
	toolID       string
	toolName     string
	inputBuf     strings.Builder
	textBuf      strings.Builder
	thinkingBuf  strings.Builder
	signatureBuf strings.Builder
}
```

In `handleContentBlockDelta`, update the `signature_delta` case:

```go
	case "signature_delta":
		bs.signatureBuf.WriteString(evt.Delta.Signature)
		return nil, nil
```

In `handleContentBlockDelta`, update the `thinking_delta` case to include accumulated signature:

```go
	case "thinking_delta":
		bs.thinkingBuf.WriteString(evt.Delta.Thinking)
		var sig []byte
		if bs.signatureBuf.Len() > 0 {
			sig = []byte(bs.signatureBuf.String())
		}
		s.msg.Content[evt.Index] = pipe.ThinkingBlock{Thinking: bs.thinkingBuf.String(), Signature: sig}
		return pipe.EventThinkingDelta{Index: evt.Index, Delta: evt.Delta.Thinking}, nil
```

In `handleContentBlockStop`, add a case for `thinking` to finalize the signature:

```go
	switch bs.blockType {
	case "tool_use":
		// ... existing code ...
	case "thinking":
		var sig []byte
		if bs.signatureBuf.Len() > 0 {
			sig = []byte(bs.signatureBuf.String())
		}
		s.msg.Content[evt.Index] = pipe.ThinkingBlock{Thinking: bs.thinkingBuf.String(), Signature: sig}
		return nil, nil
	default:
		return nil, nil
	}
```

**Step 4: Update existing TestStream_Thinking**

The existing `TestStream_Thinking` at `anthropic/stream_test.go:108` asserts:
```go
assert.Equal(t, pipe.ThinkingBlock{Thinking: "Let me think... step 2"}, msg.Content[0])
```

This test has a `signature_delta` event (`"sig123"`), so now `msg.Content[0]` will be
`ThinkingBlock{..., Signature: []byte("sig123")}`. **This WILL break.** Update it:

```go
assert.Equal(t, pipe.ThinkingBlock{Thinking: "Let me think... step 2", Signature: []byte("sig123")}, msg.Content[0])
```

**Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add anthropic/stream.go anthropic/stream_test.go
git commit -m "Capture thinking signatures in Anthropic stream"
```

---

### Task 4: Replay thinking signatures in Anthropic client

**Files:**
- Modify: `anthropic/anthropic.go:50-52` (apiContentBlock — add Signature field)
- Modify: `anthropic/client.go:191-192` (convertContentBlocks thinking case)
- Test: `anthropic/client_test.go`

**Step 1: Write the failing test**

Add to `anthropic/client_test.go`:

```go
func TestClient_ThinkingSignatureReplay(t *testing.T) {
	t.Parallel()

	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":0}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	client := anthropic.New("test-key", anthropic.WithBaseURL(srv.URL))
	s, err := client.Stream(context.Background(), pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
			pipe.AssistantMessage{Content: []pipe.ContentBlock{
				pipe.ThinkingBlock{Thinking: "reasoning here", Signature: []byte("EqQBsig123")},
				pipe.TextBlock{Text: "The answer is 42."},
			}},
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Thanks"}}},
		},
	})
	require.NoError(t, err)
	defer s.Close()

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(captured, &body))

	msgs := body["messages"].([]interface{})
	require.Len(t, msgs, 3)
	assistantMsg := msgs[1].(map[string]interface{})
	content := assistantMsg["content"].([]interface{})
	require.Len(t, content, 2)

	thinkingBlock := content[0].(map[string]interface{})
	assert.Equal(t, "thinking", thinkingBlock["type"])
	assert.Equal(t, "reasoning here", thinkingBlock["thinking"])
	assert.Equal(t, "EqQBsig123", thinkingBlock["signature"])
}

func TestClient_ThinkingNoSignature(t *testing.T) {
	t.Parallel()

	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":0}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	client := anthropic.New("test-key", anthropic.WithBaseURL(srv.URL))
	s, err := client.Stream(context.Background(), pipe.Request{
		Messages: []pipe.Message{
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hi"}}},
			pipe.AssistantMessage{Content: []pipe.ContentBlock{
				pipe.ThinkingBlock{Thinking: "no sig"},
				pipe.TextBlock{Text: "Answer."},
			}},
			pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Thanks"}}},
		},
	})
	require.NoError(t, err)
	defer s.Close()

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(captured, &body))

	msgs := body["messages"].([]interface{})
	assistantMsg := msgs[1].(map[string]interface{})
	content := assistantMsg["content"].([]interface{})
	thinkingBlock := content[0].(map[string]interface{})
	assert.Equal(t, "thinking", thinkingBlock["type"])
	assert.Nil(t, thinkingBlock["signature"], "signature should be absent when nil")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./anthropic/... -run TestClient_Thinking -count=1`
Expected: FAIL — `TestClient_ThinkingSignatureReplay` fails because signature not included in request JSON

**Step 3: Write minimal implementation**

In `anthropic/anthropic.go`, add `Signature` to `apiContentBlock`:

```go
type apiContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// ... rest unchanged
}
```

In `anthropic/client.go`, update the thinking case in `convertContentBlocks`:

```go
		case pipe.ThinkingBlock:
			cb := apiContentBlock{Type: "thinking", Thinking: bl.Thinking}
			if bl.Signature != nil {
				cb.Signature = string(bl.Signature)
			}
			result = append(result, cb)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./anthropic/... -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add anthropic/anthropic.go anthropic/client.go anthropic/client_test.go
git commit -m "Replay thinking signatures in Anthropic API requests"
```

---

### Task 5: Add google.golang.org/genai dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

Run: `go get google.golang.org/genai`

**Step 2: Verify it's in go.mod**

Run: `grep genai go.mod`
Expected: `google.golang.org/genai v*`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Add google.golang.org/genai SDK dependency"
```

---

### Task 6: Gemini client — message conversion

**Files:**
- Create: `gemini/gemini.go` (package doc, constants)
- Create: `gemini/client.go` (Client struct, options, convertMessages, convertTools)
- Create: `gemini/client_test.go` (conversion tests)

**Step 1: Write the failing tests**

Create `gemini/client_test.go`:

```go
package gemini_test

import (
	"encoding/json"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestConvertMessages_UserMessage(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hello"}}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	assert.Equal(t, "user", got[0].Role)
	require.Len(t, got[0].Parts, 1)
	assert.Equal(t, "Hello", got[0].Parts[0].Text)
}

func TestConvertMessages_AssistantMessage(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.TextBlock{Text: "Let me help."},
		}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	assert.Equal(t, "model", got[0].Role)
	require.Len(t, got[0].Parts, 1)
	assert.Equal(t, "Let me help.", got[0].Parts[0].Text)
}

func TestConvertMessages_ThinkingWithSignature(t *testing.T) {
	t.Parallel()
	sig := []byte("thought-sig-data")
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "reasoning", Signature: sig},
			pipe.TextBlock{Text: "Answer"},
		}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 2)
	assert.Equal(t, "reasoning", got[0].Parts[0].Text)
	assert.True(t, got[0].Parts[0].Thought)
	assert.Equal(t, []byte("thought-sig-data"), got[0].Parts[0].ThoughtSignature) // ThoughtSignature is []byte
	assert.Equal(t, "Answer", got[0].Parts[1].Text)
}

func TestConvertMessages_ToolCallAndResult(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "call_123", Name: "read", Arguments: json.RawMessage(`{"path":"foo.go"}`)},
		}},
		pipe.ToolResultMessage{
			ToolCallID: "call_123",
			ToolName:   "read",
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "file contents"}},
		},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 2)

	// Assistant with tool call — ID passed through
	assert.Equal(t, "model", got[0].Role)
	require.Len(t, got[0].Parts, 1)
	require.NotNil(t, got[0].Parts[0].FunctionCall)
	assert.Equal(t, "call_123", got[0].Parts[0].FunctionCall.ID)
	assert.Equal(t, "read", got[0].Parts[0].FunctionCall.Name)
	assert.Equal(t, "foo.go", got[0].Parts[0].FunctionCall.Args["path"])

	// Tool result — ID correlates, output in "output" key
	assert.Equal(t, "user", got[1].Role)
	require.Len(t, got[1].Parts, 1)
	require.NotNil(t, got[1].Parts[0].FunctionResponse)
	assert.Equal(t, "call_123", got[1].Parts[0].FunctionResponse.ID)
	assert.Equal(t, "read", got[1].Parts[0].FunctionResponse.Name)
	assert.Equal(t, "file contents", got[1].Parts[0].FunctionResponse.Response["output"])
}

func TestConvertMessages_ToolResultError(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "call_err", Name: "bash", Arguments: json.RawMessage(`{"cmd":"rm -rf /"}`)},
		}},
		pipe.ToolResultMessage{
			ToolCallID: "call_err",
			ToolName:   "bash",
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "permission denied"}},
			IsError:    true,
		},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 2)

	// Error result — uses "error" key
	resp := got[1].Parts[0].FunctionResponse
	assert.Equal(t, "call_err", resp.ID)
	assert.Equal(t, "permission denied", resp.Response["error"])
	assert.Nil(t, resp.Response["output"])
}

func TestConvertMessages_ImageBlock(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.UserMessage{Content: []pipe.ContentBlock{
			pipe.ImageBlock{Data: []byte("PNG"), MimeType: "image/png"},
		}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 1)
	require.NotNil(t, got[0].Parts[0].InlineData)
	assert.Equal(t, "image/png", got[0].Parts[0].InlineData.MIMEType)
	assert.Equal(t, []byte("PNG"), got[0].Parts[0].InlineData.Data)
}

func TestConvertTools(t *testing.T) {
	t.Parallel()
	tools := []pipe.Tool{
		{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)},
		{Name: "bash", Description: "Run a command", Parameters: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`)},
	}
	got := gemini.ConvertTools(tools)
	require.Len(t, got, 1) // single genai.Tool with multiple declarations
	require.Len(t, got[0].FunctionDeclarations, 2)
	assert.Equal(t, "read", got[0].FunctionDeclarations[0].Name)
	assert.Equal(t, "Read a file", got[0].FunctionDeclarations[0].Description)
	assert.Equal(t, "bash", got[0].FunctionDeclarations[1].Name)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./gemini/... -count=1`
Expected: FAIL — package doesn't exist

**Step 3: Write minimal implementation**

Create `gemini/gemini.go`:

```go
// Package gemini implements [pipe.Provider] for the Google Gemini API.
//
// It wraps the google.golang.org/genai SDK, translating between pipe's
// domain types and the Gemini API types. Streaming uses the SDK's iter.Seq2
// iterator, wrapped into the pull-based [pipe.Stream] interface.
package gemini

const (
	defaultModel     = "gemini-3.1-pro-preview"
	defaultMaxTokens = 65536
)
```

Create `gemini/client.go`:

```go
package gemini

import (
	"context"
	"encoding/json"

	"github.com/fwojciec/pipe"
	"google.golang.org/genai"
)

// Interface compliance check.
var _ pipe.Provider = (*Client)(nil)

// Client implements [pipe.Provider] for the Google Gemini API.
type Client struct {
	client *genai.Client
	model  string
}

// Option configures a [Client].
type Option func(*Client)

// WithModel sets the model ID. Default is gemini-3.1-pro-preview.
func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

// New creates a new Gemini [Client] with the given API key and options.
func New(ctx context.Context, apiKey string, opts ...Option) (*Client, error) {
	gc, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	c := &Client{
		client: gc,
		model:  defaultModel,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Stream sends a streaming request to the Gemini API and returns a
// [pipe.Stream] that emits semantic events.
func (c *Client) Stream(ctx context.Context, req pipe.Request) (pipe.Stream, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	contents := ConvertMessages(req.Messages)
	config := buildConfig(req)

	iter := c.client.Models.GenerateContentStream(ctx, model, contents, config)
	return newStream(ctx, iter, contents), nil
}

func buildConfig(req pipe.Request) *genai.GenerateContentConfig {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(maxTokens),
		Tools:           ConvertTools(req.Tools),
		ThinkingConfig: &genai.ThinkingConfig{
			IncludeThoughts: true,
		},
	}

	if req.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		}
	}

	if req.Temperature != nil {
		temp := float32(*req.Temperature)
		config.Temperature = &temp
	}

	return config
}

// ConvertMessages converts pipe Messages to genai Contents.
// Exported for testing.
func ConvertMessages(msgs []pipe.Message) []*genai.Content {
	var result []*genai.Content
	for _, msg := range msgs {
		switch m := msg.(type) {
		case pipe.UserMessage:
			result = append(result, &genai.Content{
				Role:  "user",
				Parts: convertParts(m.Content),
			})
		case pipe.AssistantMessage:
			result = append(result, &genai.Content{
				Role:  "model",
				Parts: convertParts(m.Content),
			})
		case pipe.ToolResultMessage:
			// Build function response per SDK convention:
			// Use "output" key for success, "error" key for errors.
			text := extractText(m.Content)
			var responseMap map[string]any
			if m.IsError {
				responseMap = map[string]any{"error": text}
			} else if err := json.Unmarshal([]byte(text), &responseMap); err != nil {
				responseMap = map[string]any{"output": text}
			}
			result = append(result, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						ID:       m.ToolCallID, // correlate with FunctionCall.ID
						Name:     m.ToolName,
						Response: responseMap,
					},
				}},
			})
		}
	}
	return result
}

func convertParts(blocks []pipe.ContentBlock) []*genai.Part {
	var parts []*genai.Part
	for _, b := range blocks {
		switch bl := b.(type) {
		case pipe.TextBlock:
			parts = append(parts, &genai.Part{Text: bl.Text})
		case pipe.ThinkingBlock:
			p := &genai.Part{Text: bl.Thinking, Thought: true}
			if bl.Signature != nil {
				p.ThoughtSignature = bl.Signature // both are []byte
			}
			parts = append(parts, p)
		case pipe.ToolCallBlock:
			var args map[string]any
			_ = json.Unmarshal(bl.Arguments, &args)
			parts = append(parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   bl.ID, // pass through for correlation
					Name: bl.Name,
					Args: args,
				},
			})
		case pipe.ImageBlock:
			parts = append(parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: bl.MimeType,
					Data:     bl.Data,
				},
			})
		}
	}
	return parts
}

// extractText concatenates all TextBlocks in a slice of ContentBlocks.
func extractText(blocks []pipe.ContentBlock) string {
	for _, b := range blocks {
		if tb, ok := b.(pipe.TextBlock); ok {
			return tb.Text
		}
	}
	return ""
}

// ConvertTools converts pipe Tools to genai Tools.
// Exported for testing.
func ConvertTools(tools []pipe.Tool) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]*genai.FunctionDeclaration, len(tools))
	for i, t := range tools {
		var schema map[string]any
		_ = json.Unmarshal(t.Parameters, &schema)
		decls[i] = &genai.FunctionDeclaration{
			Name:                 t.Name,
			Description:          t.Description,
			ParametersJsonSchema: schema,
		}
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./gemini/... -run TestConvert -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gemini/gemini.go gemini/client.go gemini/client_test.go
git commit -m "Add Gemini client with message and tool conversion"
```

---

### Task 7: Gemini stream implementation

**Files:**
- Create: `gemini/stream.go`
- Create: `gemini/stream_test.go`

This is the most complex task. The stream wraps the SDK's `iter.Seq2` into `pipe.Stream`.

**Step 1: Write the failing tests**

Create `gemini/stream_test.go`:

```go
package gemini_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// mockChunks returns a genai-style streaming iterator from pre-built chunks.
func mockChunks(chunks []*genai.GenerateContentResponse) func(func(*genai.GenerateContentResponse, error) bool) {
	return func(yield func(*genai.GenerateContentResponse, error) bool) {
		for _, c := range chunks {
			if !yield(c, nil) {
				return
			}
		}
	}
}

func collectStreamEvents(t *testing.T, s pipe.Stream) []pipe.Event {
	t.Helper()
	var events []pipe.Event
	for {
		evt, err := s.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		events = append(events, evt)
	}
	return events
}

func TestStream_TextDelta(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hello"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: " world"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks), nil)
	events := collectStreamEvents(t, s)

	require.Len(t, events, 2)
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: "Hello"}, events[0])
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: " world"}, events[1])

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopEndTurn, msg.StopReason)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, pipe.TextBlock{Text: "Hello world"}, msg.Content[0])
	assert.Equal(t, 10, msg.Usage.InputTokens)
	assert.Equal(t, 8, msg.Usage.OutputTokens)
}

func TestStream_ThinkingDelta(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "reasoning", Thought: true, ThoughtSignature: []byte("sig123")}, // ThoughtSignature is []byte
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "Answer", Thought: false},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks), nil)
	events := collectStreamEvents(t, s)

	require.Len(t, events, 2)
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: "reasoning"}, events[0])
	assert.Equal(t, pipe.EventTextDelta{Index: 1, Delta: "Answer"}, events[1])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	tb := msg.Content[0].(pipe.ThinkingBlock)
	assert.Equal(t, "reasoning", tb.Thinking)
	assert.Equal(t, []byte("sig123"), tb.Signature)
	assert.Equal(t, pipe.TextBlock{Text: "Answer"}, msg.Content[1])
}

func TestStream_ToolCallComplete(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{ID: "sdk_id_1", Name: "read", Args: map[string]any{"path": "foo.go"}}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks), nil)
	events := collectStreamEvents(t, s)

	require.Len(t, events, 2) // Begin + End, no Delta
	begin, ok := events[0].(pipe.EventToolCallBegin)
	require.True(t, ok)
	assert.Equal(t, "read", begin.Name)
	assert.Equal(t, "sdk_id_1", begin.ID) // prefer SDK ID when present

	end, ok := events[1].(pipe.EventToolCallEnd)
	require.True(t, ok)
	assert.Equal(t, "read", end.Call.Name)
	assert.Equal(t, "sdk_id_1", end.Call.ID)
	assert.JSONEq(t, `{"path":"foo.go"}`, string(end.Call.Arguments))

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopToolUse, msg.StopReason)
}

func TestStream_ToolCallFallbackID(t *testing.T) {
	t.Parallel()
	// FunctionCall without ID — should generate client-side ID.
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{Name: "bash", Args: map[string]any{"cmd": "ls"}}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks), nil)
	events := collectStreamEvents(t, s)

	begin := events[0].(pipe.EventToolCallBegin)
	assert.NotEmpty(t, begin.ID)
	assert.True(t, len(begin.ID) > 5, "generated ID should be non-trivial")
}

func TestStream_MultiPartChunk(t *testing.T) {
	t.Parallel()
	// Single chunk with thinking + text + tool call — tests append-based ordering.
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "reasoning", Thought: true, ThoughtSignature: []byte("sig")},
					{Text: "I'll check."},
					{FunctionCall: &genai.FunctionCall{ID: "tc_1", Name: "read", Args: map[string]any{"path": "a.go"}}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 15,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks), nil)
	events := collectStreamEvents(t, s)

	require.Len(t, events, 4) // ThinkingDelta, TextDelta, ToolCallBegin, ToolCallEnd
	assert.IsType(t, pipe.EventThinkingDelta{}, events[0])
	assert.IsType(t, pipe.EventTextDelta{}, events[1])
	assert.IsType(t, pipe.EventToolCallBegin{}, events[2])
	assert.IsType(t, pipe.EventToolCallEnd{}, events[3])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 3) // thinking[0], text[1], tool_call[2] — append order preserved
	assert.IsType(t, pipe.ThinkingBlock{}, msg.Content[0])
	assert.IsType(t, pipe.TextBlock{}, msg.Content[1])
	assert.IsType(t, pipe.ToolCallBlock{}, msg.Content[2])
	assert.Equal(t, pipe.StopToolUse, msg.StopReason)
}

func TestStream_Usage(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:        210,
				CandidatesTokenCount:    5,
				CachedContentTokenCount: 200,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks), nil)
	collectStreamEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, 10, msg.Usage.InputTokens)      // 210 - 200
	assert.Equal(t, 5, msg.Usage.OutputTokens)
	assert.Equal(t, 200, msg.Usage.CacheReadTokens)
	assert.Equal(t, 0, msg.Usage.CacheWriteTokens)
}

func TestStream_StopReasonMaxTokens(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "truncated"}}},
				FinishReason: genai.FinishReasonMaxTokens,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 100,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks), nil)
	collectStreamEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopLength, msg.StopReason)
}

func TestStream_ContextCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Empty iterator that yields nothing — stream should detect cancelled ctx.
	emptyIter := func(yield func(*genai.GenerateContentResponse, error) bool) {}

	s := gemini.NewStreamFromIter(ctx, emptyIter, nil)
	_, err := s.Next()
	assert.Error(t, err)

	msg, _ := s.Message()
	assert.Equal(t, pipe.StopAborted, msg.StopReason)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./gemini/... -run TestStream -count=1`
Expected: FAIL — `NewStreamFromIter` doesn't exist

**Step 3: Write minimal implementation**

Create `gemini/stream.go`:

```go
package gemini

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"iter"

	"github.com/fwojciec/pipe"
	"google.golang.org/genai"
)

// stream implements [pipe.Stream] by wrapping the genai SDK's streaming iterator.
type stream struct {
	ctx     context.Context
	pull    func() (*genai.GenerateContentResponse, error, bool)
	stop    func()
	state   pipe.StreamState
	msg     pipe.AssistantMessage
	pending []pipe.Event
	err     error

	// Block tracking uses per-block state keyed by content index.
	// Each new part type from the SDK gets its own content block (append-based,
	// not type-scanned). Consecutive same-type parts accumulate into the
	// current block; a different type always starts a new block.
	blocks      []*blockState
	hasToolCall bool
}

// blockState tracks accumulation for a single content block.
type blockState struct {
	blockType string // "thinking", "text", "tool_call"
	textBuf   string
	signature []byte // for thinking blocks (ThoughtSignature is []byte)
}

// Interface compliance check.
var _ pipe.Stream = (*stream)(nil)

func newStream(ctx context.Context, iterFn iter.Seq2[*genai.GenerateContentResponse, error], contents []*genai.Content) *stream {
	return NewStreamFromIter(ctx, iterFn, contents)
}

// NewStreamFromIter creates a stream from an iter.Seq2 for testing.
func NewStreamFromIter(ctx context.Context, iterFn iter.Seq2[*genai.GenerateContentResponse, error], contents []*genai.Content) *stream {
	next, stop := iter.Pull2(iterFn)
	return &stream{
		ctx:   ctx,
		pull:  next,
		stop:  stop,
		state: pipe.StreamStateNew,
	}
}

func (s *stream) Next() (pipe.Event, error) {
	switch s.state {
	case pipe.StreamStateComplete:
		return nil, io.EOF
	case pipe.StreamStateError:
		return nil, s.err
	case pipe.StreamStateClosed:
		return nil, fmt.Errorf("gemini: stream closed")
	}

	// Drain pending events first.
	if len(s.pending) > 0 {
		evt := s.pending[0]
		s.pending = s.pending[1:]
		return evt, nil
	}

	// Check context before pulling.
	if s.ctx.Err() != nil {
		s.terminate(s.ctx.Err())
		return nil, s.err
	}

	// Pull next chunk from SDK iterator.
	resp, err, ok := s.pull()
	if !ok {
		// Iterator exhausted.
		s.finalize()
		return nil, io.EOF
	}
	if err != nil {
		s.terminate(err)
		return nil, s.err
	}

	s.state = pipe.StreamStateStreaming
	s.processChunk(resp)

	// Return first pending event.
	if len(s.pending) > 0 {
		evt := s.pending[0]
		s.pending = s.pending[1:]
		return evt, nil
	}

	// No events from this chunk — recurse to get next.
	return s.Next()
}

func (s *stream) State() pipe.StreamState {
	return s.state
}

func (s *stream) Message() (pipe.AssistantMessage, error) {
	if s.state == pipe.StreamStateNew {
		return pipe.AssistantMessage{}, fmt.Errorf("gemini: no data received yet")
	}
	return s.msg, nil
}

func (s *stream) Close() error {
	if s.state != pipe.StreamStateComplete && s.state != pipe.StreamStateError {
		s.state = pipe.StreamStateClosed
		s.msg.StopReason = pipe.StopAborted
		s.msg.RawStopReason = "aborted"
	}
	s.stop()
	return nil
}

func (s *stream) terminate(err error) {
	s.state = pipe.StreamStateError
	s.err = fmt.Errorf("gemini: %w", err)
	if s.ctx.Err() != nil {
		s.msg.StopReason = pipe.StopAborted
		s.msg.RawStopReason = "aborted"
	} else {
		s.msg.StopReason = pipe.StopError
		s.msg.RawStopReason = "error"
	}
}

func (s *stream) finalize() {
	s.state = pipe.StreamStateComplete
	if s.hasToolCall {
		s.msg.StopReason = pipe.StopToolUse
		s.msg.RawStopReason = "tool_use"
	}
}

func (s *stream) processChunk(resp *genai.GenerateContentResponse) {
	if resp.UsageMetadata != nil {
		cached := int(resp.UsageMetadata.CachedContentTokenCount)
		input := int(resp.UsageMetadata.PromptTokenCount) - cached
		if input < 0 {
			input = 0
		}
		s.msg.Usage = pipe.Usage{
			InputTokens:     input,
			OutputTokens:    int(resp.UsageMetadata.CandidatesTokenCount),
			CacheReadTokens: cached,
		}
	}

	if len(resp.Candidates) == 0 {
		return
	}
	candidate := resp.Candidates[0]

	if candidate.FinishReason != "" {
		s.msg.RawStopReason = string(candidate.FinishReason)
		s.msg.StopReason = mapFinishReason(candidate.FinishReason)
	}

	if candidate.Content == nil {
		return
	}

	for _, part := range candidate.Content.Parts {
		s.processPart(part)
	}
}

func (s *stream) processPart(part *genai.Part) {
	switch {
	case part.FunctionCall != nil:
		s.hasToolCall = true
		args, _ := json.Marshal(part.FunctionCall.Args)
		// Prefer SDK-provided ID; generate client-side as fallback.
		id := part.FunctionCall.ID
		if id == "" {
			id = generateToolCallID()
		}
		call := pipe.ToolCallBlock{
			ID:        id,
			Name:      part.FunctionCall.Name,
			Arguments: json.RawMessage(args),
		}
		s.msg.Content = append(s.msg.Content, call)
		s.blocks = append(s.blocks, &blockState{blockType: "tool_call"})
		s.pending = append(s.pending,
			pipe.EventToolCallBegin{ID: id, Name: part.FunctionCall.Name},
			pipe.EventToolCallEnd{Call: call},
		)

	case part.Thought && part.Text != "":
		// Append-based: accumulate into current thinking block if the last
		// block is thinking; otherwise start a new one.
		idx := s.currentBlockIndex("thinking")
		bs := s.blocks[idx]
		bs.textBuf += part.Text
		if len(part.ThoughtSignature) > 0 {
			bs.signature = append(bs.signature, part.ThoughtSignature...)
		}
		var sig []byte
		if len(bs.signature) > 0 {
			sig = bs.signature
		}
		s.msg.Content[idx] = pipe.ThinkingBlock{Thinking: bs.textBuf, Signature: sig}
		s.pending = append(s.pending, pipe.EventThinkingDelta{Index: idx, Delta: part.Text})

	case part.Text != "":
		idx := s.currentBlockIndex("text")
		bs := s.blocks[idx]
		bs.textBuf += part.Text
		s.msg.Content[idx] = pipe.TextBlock{Text: bs.textBuf}
		s.pending = append(s.pending, pipe.EventTextDelta{Index: idx, Delta: part.Text})
	}
}

// currentBlockIndex returns the index of the current block if it matches the
// given type. If the last block is a different type (or no blocks exist), a new
// block is appended. This preserves ordering for interleaved content.
func (s *stream) currentBlockIndex(blockType string) int {
	if n := len(s.blocks); n > 0 && s.blocks[n-1].blockType == blockType {
		return n - 1
	}
	// Start a new block.
	idx := len(s.blocks)
	s.blocks = append(s.blocks, &blockState{blockType: blockType})
	// Append placeholder content block.
	switch blockType {
	case "thinking":
		s.msg.Content = append(s.msg.Content, pipe.ThinkingBlock{})
	case "text":
		s.msg.Content = append(s.msg.Content, pipe.TextBlock{})
	}
	return idx
}

func mapFinishReason(reason genai.FinishReason) pipe.StopReason {
	switch reason {
	case genai.FinishReasonStop:
		return pipe.StopEndTurn
	case genai.FinishReasonMaxTokens:
		return pipe.StopLength
	case genai.FinishReasonSafety, genai.FinishReasonRecitation,
		genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII, genai.FinishReasonMalformedFunctionCall:
		return pipe.StopError
	default:
		return pipe.StopUnknown
	}
}

// generateToolCallID generates a unique fallback ID for tool calls
// when the SDK doesn't provide one.
func generateToolCallID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "call_" + hex.EncodeToString(b)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./gemini/... -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gemini/stream.go gemini/stream_test.go
git commit -m "Add Gemini stream wrapping SDK iterator into pipe.Stream"
```

---

### Task 8: Provider selection in cmd/pipe

**Files:**
- Modify: `cmd/pipe/main.go`

**Step 1: Create provider.go with resolveProvider**

Extract provider resolution into a testable function in `cmd/pipe/provider.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/anthropic"
	"github.com/fwojciec/pipe/gemini"
)

func resolveProvider(ctx context.Context, providerFlag, apiKeyFlag string) (pipe.Provider, error) {
	provider := providerFlag

	// Auto-detect from env vars if no flag.
	if provider == "" {
		hasAnthropic := os.Getenv("ANTHROPIC_API_KEY") != ""
		hasGemini := os.Getenv("GEMINI_API_KEY") != ""
		switch {
		case hasAnthropic && hasGemini:
			return nil, fmt.Errorf("multiple API keys found (ANTHROPIC_API_KEY, GEMINI_API_KEY): use -provider flag to select")
		case hasAnthropic:
			provider = "anthropic"
		case hasGemini:
			provider = "gemini"
		default:
			return nil, fmt.Errorf("no API key found: set ANTHROPIC_API_KEY or GEMINI_API_KEY (or use -provider and -api-key flags)")
		}
	}

	// Resolve API key.
	key := apiKeyFlag
	switch provider {
	case "anthropic":
		if key == "" {
			key = os.Getenv("ANTHROPIC_API_KEY")
		}
		if key == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY not set (use -api-key flag or environment variable)")
		}
		return anthropic.New(key), nil
	case "gemini":
		if key == "" {
			key = os.Getenv("GEMINI_API_KEY")
		}
		if key == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY not set (use -api-key flag or environment variable)")
		}
		client, err := gemini.New(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("gemini: %w", err)
		}
		return client, nil
	default:
		return nil, fmt.Errorf("unknown provider %q: must be \"anthropic\" or \"gemini\"", provider)
	}
}
```

**Step 2: Write tests for resolveProvider**

Create `cmd/pipe/provider_test.go`:

```go
package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProvider_ExplicitAnthropic(t *testing.T) {
	t.Parallel()
	p, err := resolveProvider(context.Background(), "anthropic", "sk-test")
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestResolveProvider_UnknownProvider(t *testing.T) {
	t.Parallel()
	_, err := resolveProvider(context.Background(), "openai", "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestResolveProvider_NoKeysNoFlag(t *testing.T) {
	t.Parallel()
	// Unset env vars for this test.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	_, err := resolveProvider(context.Background(), "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no API key found")
}

func TestResolveProvider_BothKeysNoFlag(t *testing.T) {
	t.Parallel()
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	t.Setenv("GEMINI_API_KEY", "gk-gem")
	_, err := resolveProvider(context.Background(), "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple API keys")
}

func TestResolveProvider_AutoDetectAnthropic(t *testing.T) {
	t.Parallel()
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	t.Setenv("GEMINI_API_KEY", "")
	p, err := resolveProvider(context.Background(), "", "")
	require.NoError(t, err)
	assert.NotNil(t, p)
}
```

Note: these tests use internal package access (`package main`) which is allowed for `cmd/` wiring tests.

**Step 3: Update main.go**

In `cmd/pipe/main.go`, the `run()` function changes:

```go
func run() error {
	var (
		model        = flag.String("model", "", "Model ID (provider-specific)")
		sessionPath  = flag.String("session", "", "Path to session file to resume")
		promptPath   = flag.String("system-prompt", defaultPromptPath, "Path to system prompt file")
		providerFlag = flag.String("provider", "", "Provider: anthropic, gemini (auto-detected from env vars if omitted)")
		apiKey       = flag.String("api-key", "", "API key (overrides provider's env var)")
	)
	flag.Parse()

	// Handle OS signals for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Resolve provider.
	provider, err := resolveProvider(ctx, *providerFlag, *apiKey)
	if err != nil {
		return err
	}

	// Load or create session.
	session, err := loadOrCreateSession(*sessionPath, *promptPath)
	if err != nil {
		return err
	}

	// ... rest unchanged from "Create tool executor" onward ...
```

**Step 4: Run tests and build**

Run: `go test ./cmd/pipe/... -count=1 && go build ./cmd/pipe/`
Expected: PASS + SUCCESS

**Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add cmd/pipe/main.go cmd/pipe/provider.go cmd/pipe/provider_test.go
git commit -m "Add provider selection with -provider flag and env var auto-detect"
```

---

### Task 9: Run make validate

**Step 1: Run the quality gate**

Run: `make validate`
Expected: PASS — linting, tests, build all succeed

**Step 2: Fix any issues found**

If linting fails (e.g., `gochecknoglobals` on the `cryptoRead` variable, or unused imports), fix them.

**Step 3: Commit fixes if any**

```bash
git add -A
git commit -m "Fix lint issues from validate"
```

---

### Task 10: Final integration smoke test

**Step 1: Verify the binary builds**

Run: `go build -o /dev/null ./cmd/pipe/`
Expected: SUCCESS

**Step 2: Verify interface compliance**

The `var _ pipe.Provider = (*Client)(nil)` checks in both `anthropic/client.go` and `gemini/client.go` ensure compile-time interface compliance.

**Step 3: Verify all tests pass**

Run: `go test ./... -count=1 -race`
Expected: PASS with no race conditions

**Step 4: Final commit if needed**

No commit needed if everything passed.
