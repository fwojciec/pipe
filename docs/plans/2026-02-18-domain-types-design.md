# Domain Types Design

## Context

Pipe is a minimal, Go-based agentic coding harness. It connects a human to an LLM
and a filesystem through composable tools. The name references Unix pipes and
Magritte's "Ceci n'est pas une pipe."

The project follows Ben Johnson's Standard Package Layout: domain types in the root
package, subpackages named after dependencies, main as composition root.

## Design Decisions

### Messages: Sealed Interface

Messages use a sealed interface (unexported marker method) with three concrete types:
`UserMessage`, `AssistantMessage`, `ToolResultMessage`. The sealed pattern prevents
external packages from adding message types. Type switches over messages should use
the `exhaustive` linter to catch missing cases (Go does not enforce this at compile
time).

`ToolResultMessage` is first-class in the conversation array (not nested inside
`UserMessage`), following pi-mono's approach.

### Content Blocks: Sealed Interface

Content blocks use the same sealed pattern: `TextBlock`, `ThinkingBlock`,
`ImageBlock`, `ToolCallBlock`. Tool calls are content blocks inside
`AssistantMessage`, matching how the Anthropic API returns them.

Note: the type system does not prevent invalid combinations (e.g. `ToolCallBlock`
inside `UserMessage`). Validation is enforced at construction/use time, not at the
type level.

### Streaming: Pull-Based Iterator

Inspired by Rob Pike's lexer talk ("Lexical Scanning in Go"). The `Stream` interface
uses a `Next() (Event, error)` pull-based pattern (like `bufio.Scanner`,
`json.Decoder`). This avoids channels, shared mutable state, and goroutine
coordination. The SSE parser drives one step at a time, returning when a semantic
event is ready. `io.EOF` signals completion.

Cancellation flows through the `context.Context` passed to `Provider.Stream()`,
which cancels the underlying HTTP request and unblocks `Next()`. This follows Go
stdlib conventions (`bufio.Scanner`, `sql.Rows`) where context is passed at
construction, not on every read call.

The `Stream` assembles the `AssistantMessage` internally as it processes deltas.
Consumers get real-time delta events for rendering; the final assembled message is
available via `Message()` after `io.EOF`.

Events are purely semantic (content deltas, tool call lifecycle). Transport and
protocol errors are reported through `Next()`'s error return, not through events.

### context.Context on All Interface Methods

All interface methods take `context.Context` as the first parameter, except for
`Stream.Next()` and `Stream.Close()` which inherit the context from
`Provider.Stream()`. Even local operations should be cancellable (ctrl+c during
agent loop). The interface doesn't know if the implementation is local or remote.

### Provider Interface

`Provider` is a strategy pattern interface with a single method:
`Stream(ctx, *Request) (Stream, error)`. Anthropic first, others added later.
No Api/Provider split (unlike pi-mono) - unnecessary complexity for our use case.

### Request Configuration

`Request` carries model selection and generation parameters explicitly. This ensures
reproducibility and allows switching models mid-conversation. The provider uses its
own defaults when fields are zero/nil.

### Tool Definition vs Execution

`Tool` is a plain struct (name, description, JSON Schema parameters) - the schema
sent to the LLM. `ToolExecutor` is the interface that runs tools. This separation
means the root package doesn't know how tools work, just what they look like.

Error semantics: `Execute` returns `error` for infrastructure failures (can't run
the tool at all). `ToolResult.IsError` indicates tool-reported domain failures
(tool ran but the operation failed - this gets sent back to the LLM).

### Stop Reasons

`StopReason` includes `StopUnknown` for forward compatibility. `AssistantMessage`
carries both the mapped `StopReason` and the raw provider string, so no information
is lost when providers add new stop reasons.

### Session

Minimal: ID, messages, system prompt, timestamps. Persistence lives in a subpackage.
Serialization requires a versioned, tagged wire format with type discriminators for
polymorphic interfaces (`Message`, `ContentBlock`). This will be defined when
implementing persistence.

## Types

### Root Package (pipe/)

```go
package pipe

type Role string
const (
    RoleUser       Role = "user"
    RoleAssistant  Role = "assistant"
    RoleToolResult Role = "tool_result"
)

type StopReason string
const (
    StopEndTurn StopReason = "end_turn"
    StopLength  StopReason = "length"
    StopToolUse StopReason = "tool_use"
    StopError   StopReason = "error"
    StopAborted StopReason = "aborted"
    StopUnknown StopReason = "unknown"
)

// Messages (sealed)
type Message interface { role() Role }

type UserMessage struct {
    Content   []ContentBlock
    Timestamp time.Time
}

type AssistantMessage struct {
    Content       []ContentBlock
    StopReason    StopReason
    RawStopReason string
    Usage         Usage
    Timestamp     time.Time
}

type ToolResultMessage struct {
    ToolCallID string
    ToolName   string
    Content    []ContentBlock
    IsError    bool
    Timestamp  time.Time
}

// Content Blocks (sealed)
type ContentBlock interface { contentBlock() }

type TextBlock struct{ Text string }
type ThinkingBlock struct{ Thinking string }
type ImageBlock struct{ Data []byte; MimeType string }
type ToolCallBlock struct{ ID string; Name string; Arguments json.RawMessage }

// Events (sealed)
//
// Events are purely semantic. Transport/protocol errors come from Next()'s
// error return, not from events.
type Event interface { event() }

type EventTextDelta struct{ Delta string }
type EventThinkingDelta struct{ Delta string }
type EventToolCallBegin struct{ ID string; Name string }
type EventToolCallDelta struct{ ID string; Delta string }
type EventToolCallEnd struct{ Call ToolCallBlock }

// Streaming
//
// Stream uses a pull-based iterator pattern. Cancellation flows through the
// context passed to Provider.Stream().
//
// Message() returns the assembled AssistantMessage at any point:
// - After io.EOF: complete message, nil error.
// - After non-EOF error: partial message (what was assembled before failure),
//   nil error. StopReason will be StopError. Partial output is recoverable
//   for debugging/retry UX.
// - Before Next() is ever called: zero-value message, non-nil error.
type Stream interface {
    Next() (Event, error)
    Message() (AssistantMessage, error)
    Close() error
}

// Provider
type Provider interface {
    Stream(ctx context.Context, req *Request) (Stream, error)
}

type Request struct {
    Model        string       // model ID, provider-specific; empty = provider default
    SystemPrompt string
    Messages     []Message
    Tools        []Tool
    MaxTokens    int          // 0 = provider default
    Temperature  *float64     // nil = provider default
}

// Validate checks universal constraints on Request.
// Provider implementations may apply additional provider-specific validation.
func (r *Request) Validate() error

// Tools
//
// Tool is the schema sent to the LLM. ToolExecutor runs tools.
// Execute returns error for infrastructure failures.
// ToolResult.IsError indicates tool-reported domain failures sent back to the LLM.
type Tool struct {
    Name        string
    Description string
    Parameters  json.RawMessage
}

type ToolExecutor interface {
    Execute(ctx context.Context, name string, args json.RawMessage) (*ToolResult, error)
}

type ToolResult struct {
    Content []ContentBlock
    IsError bool
}

// Usage
type Usage struct {
    InputTokens  int
    OutputTokens int
}

// Session
type Session struct {
    ID           string
    Messages     []Message
    SystemPrompt string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

## Package Layout

```
pipe/
├── message.go              # Message, ContentBlock (sealed interfaces + impls)
├── event.go                # Event (sealed interface + impls)
├── provider.go             # Provider, Stream, Request
├── tool.go                 # Tool, ToolExecutor, ToolResult
├── session.go              # Session
├── usage.go                # Usage, StopReason
├── json/                   # Session persistence (dep: encoding/json)
│   └── session.go          # Save/Load with v1 envelope format
├── anthropic/              # Provider impl (dep: net/http for SSE)
│   ├── client.go
│   └── stream.go
├── builtin/                # ToolExecutor impl (capability: built-in tools)
│   ├── bash.go
│   ├── read.go
│   ├── write.go
│   ├── edit.go
│   ├── grep.go
│   └── glob.go
├── agent/                  # Agent loop (capability: orchestration)
│   └── loop.go
├── bubbletea/              # TUI frontend (dep: charmbracelet/bubbletea)
│   └── app.go
├── mock/                   # Testing (capability: mocks)
│   ├── provider.go
│   └── tool.go
└── cmd/pipe/
    └── main.go             # Composition root
```

## Persistence Schema (v1)

Session files use JSON with type discriminators on polymorphic types. Version field
at the envelope level for forward compatibility.

```json
{
  "version": 1,
  "id": "session-id",
  "system_prompt": "You are...",
  "created_at": "2026-02-18T12:00:00Z",
  "updated_at": "2026-02-18T12:05:00Z",
  "messages": [
    {
      "type": "user",
      "content": [
        {"type": "text", "text": "Fix the login bug"}
      ],
      "timestamp": "2026-02-18T12:00:00Z"
    },
    {
      "type": "assistant",
      "content": [
        {"type": "text", "text": "I'll look at the auth module."},
        {"type": "tool_call", "id": "tc_1", "name": "read", "arguments": {"path": "auth.go"}}
      ],
      "stop_reason": "tool_use",
      "raw_stop_reason": "tool_use",
      "usage": {"input_tokens": 150, "output_tokens": 42},
      "timestamp": "2026-02-18T12:00:01Z"
    },
    {
      "type": "tool_result",
      "tool_call_id": "tc_1",
      "tool_name": "read",
      "content": [
        {"type": "text", "text": "package auth\n..."}
      ],
      "is_error": false,
      "timestamp": "2026-02-18T12:00:02Z"
    }
  ]
}
```

Persistence implementation lives in a `json/` subpackage (per Ben Johnson: named
after the dependency `encoding/json`). The root package defines the domain types;
the `json/` package provides `Save()`/`Load()` with format conversion.

## Validation

- **`Request.Validate()`**: Universal constraints in root package. Temperature must
  be in [0, 2] if set. MaxTokens must be > 0 if set. Provider adds its own
  validation on top (e.g. model ID must be recognized).
- **Content block combinations**: Validated at construction/use time via a
  `ValidateMessage()` function in root, not at the type level.

## Review Notes

Items deferred to implementation:

- **Content block validation**: Enforce valid combinations (e.g. no ToolCallBlock
  in UserMessage) at construction/use time, not at the type level.
- **ImageBlock sizing**: Define max sizes and/or external blob references when
  implementing persistence.
- **Stream conformance tests**: cancellation, EOF, partial reads, close-unblocks-read.
- **Serialization round-trip tests**: across versioned session schema.
- **Tool-call reconstruction tests**: including interleaved deltas.
