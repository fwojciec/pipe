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
`UserMessage`, `AssistantMessage`, `ToolResultMessage`. The sealed pattern means only
the root package can define message types, giving exhaustive type switches and
compiler-enforced completeness.

`ToolResultMessage` is first-class in the conversation array (not nested inside
`UserMessage`), following pi-mono's approach.

### Content Blocks: Sealed Interface

Content blocks use the same sealed pattern: `TextBlock`, `ThinkingBlock`,
`ImageBlock`, `ToolCallBlock`. Tool calls are content blocks inside
`AssistantMessage`, matching how the Anthropic API returns them.

### Streaming: Pull-Based Iterator

Inspired by Rob Pike's lexer talk ("Lexical Scanning in Go"). The `Stream` interface
uses a `Next() (Event, error)` pull-based pattern (like `bufio.Scanner`,
`json.Decoder`). This avoids channels, shared mutable state, and goroutine
coordination. The SSE parser drives one step at a time, returning when a semantic
event is ready. `io.EOF` signals completion.

The `Stream` assembles the `AssistantMessage` internally as it processes deltas.
Consumers get real-time delta events for rendering; the final assembled message is
available via `Message()` after `io.EOF`.

Events are sealed: `EventTextDelta`, `EventThinkingDelta`, `EventToolCallBegin`,
`EventToolCallDelta`, `EventToolCallEnd`, `EventError`.

### context.Context on All Interface Methods

All interface methods take `context.Context` as the first parameter. Even local
operations should be cancellable (ctrl+c during agent loop). The interface doesn't
know if the implementation is local or remote.

### Provider Interface

`Provider` is a strategy pattern interface with a single method:
`Stream(ctx, *Request) (Stream, error)`. Anthropic first, others added later.
No Api/Provider split (unlike pi-mono) - unnecessary complexity for our use case.

### Tool Definition vs Execution

`Tool` is a plain struct (name, description, JSON Schema parameters) - the schema
sent to the LLM. `ToolExecutor` is the interface that runs tools. This separation
means the root package doesn't know how tools work, just what they look like.

### Session

Minimal: ID, messages, system prompt, timestamps. Persistence lives in a subpackage.

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
)

// Messages (sealed)
type Message interface { role() Role }

type UserMessage struct {
    Content   []ContentBlock
    Timestamp time.Time
}

type AssistantMessage struct {
    Content    []ContentBlock
    StopReason StopReason
    Usage      Usage
    Timestamp  time.Time
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
type Event interface { event() }

type EventTextDelta struct{ Delta string }
type EventThinkingDelta struct{ Delta string }
type EventToolCallBegin struct{ ID string; Name string }
type EventToolCallDelta struct{ Delta string }
type EventToolCallEnd struct{ Call ToolCallBlock }
type EventError struct{ Err error }

// Streaming
type Stream interface {
    Next() (Event, error)
    Message() AssistantMessage
    Close() error
}

// Provider
type Provider interface {
    Stream(ctx context.Context, req *Request) (Stream, error)
}

type Request struct {
    SystemPrompt string
    Messages     []Message
    Tools        []Tool
}

// Tools
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
├── anthropic/              # Provider impl
│   ├── client.go
│   └── stream.go
├── builtin/                # ToolExecutor impl
│   ├── bash.go
│   ├── read.go
│   ├── write.go
│   ├── edit.go
│   ├── grep.go
│   └── glob.go
├── agent/                  # Agent loop
│   └── loop.go
├── bubbletea/              # TUI frontend
│   └── app.go
├── mock/                   # Testing
│   ├── provider.go
│   └── tool.go
└── cmd/pipe/
    └── main.go             # Composition root
```
