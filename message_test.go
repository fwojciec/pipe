package pipe_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
)

func TestUserMessage_ImplementsMessage(t *testing.T) {
	t.Parallel()
	var msg pipe.Message = pipe.UserMessage{
		Content:   []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}},
		Timestamp: time.Now(),
	}
	assert.NotNil(t, msg)
}

func TestAssistantMessage_ImplementsMessage(t *testing.T) {
	t.Parallel()
	var msg pipe.Message = pipe.AssistantMessage{
		Content:       []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}},
		StopReason:    pipe.StopEndTurn,
		RawStopReason: "end_turn",
		Usage:         pipe.Usage{InputTokens: 10, OutputTokens: 5},
		Timestamp:     time.Now(),
	}
	assert.NotNil(t, msg)
}

func TestToolResultMessage_ImplementsMessage(t *testing.T) {
	t.Parallel()
	var msg pipe.Message = pipe.ToolResultMessage{
		ToolCallID: "tc_1",
		ToolName:   "read",
		Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "file contents"}},
		IsError:    false,
		Timestamp:  time.Now(),
	}
	assert.NotNil(t, msg)
}

func TestMessageTypeSwitch_Exhaustive(t *testing.T) {
	t.Parallel()
	messages := []pipe.Message{
		pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}}},
		pipe.AssistantMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}},
		pipe.ToolResultMessage{ToolCallID: "tc_1", ToolName: "read"},
	}
	for _, msg := range messages {
		switch msg.(type) {
		case pipe.UserMessage:
		case pipe.AssistantMessage:
		case pipe.ToolResultMessage:
		default:
			t.Fatalf("unexpected message type: %T", msg)
		}
	}
}

func TestMessage_Role(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  pipe.Message
		want pipe.Role
	}{
		{"UserMessage", pipe.UserMessage{}, pipe.RoleUser},
		{"AssistantMessage", pipe.AssistantMessage{}, pipe.RoleAssistant},
		{"ToolResultMessage", pipe.ToolResultMessage{}, pipe.RoleToolResult},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.msg.Role())
		})
	}
}

func TestContentBlock_TextBlock(t *testing.T) {
	t.Parallel()
	var block pipe.ContentBlock = pipe.TextBlock{Text: "hello"}
	assert.NotNil(t, block)
}

func TestContentBlock_ThinkingBlock(t *testing.T) {
	t.Parallel()
	var block pipe.ContentBlock = pipe.ThinkingBlock{Thinking: "reasoning..."}
	assert.NotNil(t, block)
}

func TestContentBlock_ImageBlock(t *testing.T) {
	t.Parallel()
	var block pipe.ContentBlock = pipe.ImageBlock{
		Data:     []byte{0x89, 0x50, 0x4E, 0x47},
		MimeType: "image/png",
	}
	assert.NotNil(t, block)
}

func TestContentBlock_ToolCallBlock(t *testing.T) {
	t.Parallel()
	var block pipe.ContentBlock = pipe.ToolCallBlock{
		ID:        "tc_1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path": "main.go"}`),
	}
	assert.NotNil(t, block)
}

func TestContentBlockTypeSwitch_Exhaustive(t *testing.T) {
	t.Parallel()
	blocks := []pipe.ContentBlock{
		pipe.TextBlock{Text: "hello"},
		pipe.ThinkingBlock{Thinking: "reasoning"},
		pipe.ImageBlock{Data: []byte{0x89}, MimeType: "image/png"},
		pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{}`)},
	}
	for _, block := range blocks {
		switch block.(type) {
		case pipe.TextBlock:
		case pipe.ThinkingBlock:
		case pipe.ImageBlock:
		case pipe.ToolCallBlock:
		default:
			t.Fatalf("unexpected content block type: %T", block)
		}
	}
}
