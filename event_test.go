package pipe_test

import (
	"encoding/json"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
)

func TestEventTextDelta_ImplementsEvent(t *testing.T) {
	t.Parallel()
	var e pipe.Event = pipe.EventTextDelta{Index: 0, Delta: "hello"}
	assert.NotNil(t, e)
}

func TestEventThinkingDelta_ImplementsEvent(t *testing.T) {
	t.Parallel()
	var e pipe.Event = pipe.EventThinkingDelta{Index: 0, Delta: "reasoning..."}
	assert.NotNil(t, e)
}

func TestEventToolCallBegin_ImplementsEvent(t *testing.T) {
	t.Parallel()
	var e pipe.Event = pipe.EventToolCallBegin{ID: "tc_1", Name: "read"}
	assert.NotNil(t, e)
}

func TestEventToolCallDelta_ImplementsEvent(t *testing.T) {
	t.Parallel()
	var e pipe.Event = pipe.EventToolCallDelta{ID: "tc_1", Delta: `{"path":"`}
	assert.NotNil(t, e)
}

func TestEventToolCallEnd_ImplementsEvent(t *testing.T) {
	t.Parallel()
	var e pipe.Event = pipe.EventToolCallEnd{
		Call: pipe.ToolCallBlock{
			ID:        "tc_1",
			Name:      "read",
			Arguments: json.RawMessage(`{"path": "main.go"}`),
		},
	}
	assert.NotNil(t, e)
}

func TestEventTypeSwitch_Exhaustive(t *testing.T) {
	t.Parallel()
	events := []pipe.Event{
		pipe.EventTextDelta{Index: 0, Delta: "hello"},
		pipe.EventThinkingDelta{Index: 0, Delta: "reasoning"},
		pipe.EventToolCallBegin{ID: "tc_1", Name: "read"},
		pipe.EventToolCallDelta{ID: "tc_1", Delta: `{"path":"`},
		pipe.EventToolCallEnd{Call: pipe.ToolCallBlock{ID: "tc_1", Name: "read"}},
	}
	assert.Len(t, events, 5, "update slice and switch when adding new Event types")
	for _, e := range events {
		switch e.(type) {
		case pipe.EventTextDelta:
		case pipe.EventThinkingDelta:
		case pipe.EventToolCallBegin:
		case pipe.EventToolCallDelta:
		case pipe.EventToolCallEnd:
		default:
			t.Fatalf("unexpected event type: %T", e)
		}
	}
}
