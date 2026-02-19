package pipe

// Event is a sealed interface representing a streaming event.
// Events are purely semantic. Transport/protocol errors come from
// Next()'s error return, not from events.
// The unexported marker method prevents external implementations.
type Event interface {
	event()
}

// EventTextDelta represents a text content delta.
type EventTextDelta struct {
	Delta string
}

func (EventTextDelta) event() {}

// EventThinkingDelta represents a thinking content delta.
type EventThinkingDelta struct {
	Delta string
}

func (EventThinkingDelta) event() {}

// EventToolCallBegin signals the start of a tool call.
type EventToolCallBegin struct {
	ID   string
	Name string
}

func (EventToolCallBegin) event() {}

// EventToolCallDelta represents an argument delta for a tool call.
type EventToolCallDelta struct {
	ID    string
	Delta string
}

func (EventToolCallDelta) event() {}

// EventToolCallEnd signals the completion of a tool call with the assembled block.
type EventToolCallEnd struct {
	Call ToolCallBlock
}

func (EventToolCallEnd) event() {}

// Interface compliance checks.
var (
	_ Event = EventTextDelta{}
	_ Event = EventThinkingDelta{}
	_ Event = EventToolCallBegin{}
	_ Event = EventToolCallDelta{}
	_ Event = EventToolCallEnd{}
)
