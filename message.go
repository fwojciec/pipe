package pipe

import (
	"encoding/json"
	"fmt"
	"time"
)

// Message is a sealed interface representing a conversation message.
// The unexported marker method prevents external implementations.
// Role() returns the message's role without requiring a type switch.
type Message interface {
	isMessage()
	Role() Role
}

// UserMessage represents a message from the user.
type UserMessage struct {
	Content   []ContentBlock
	Timestamp time.Time
}

func (UserMessage) isMessage() {}

// Role returns RoleUser.
func (UserMessage) Role() Role { return RoleUser }

// AssistantMessage represents a message from the assistant.
type AssistantMessage struct {
	Content       []ContentBlock
	StopReason    StopReason
	RawStopReason string
	Usage         Usage
	Timestamp     time.Time
}

func (AssistantMessage) isMessage() {}

// Role returns RoleAssistant.
func (AssistantMessage) Role() Role { return RoleAssistant }

// ToolResultMessage represents the result of a tool execution.
type ToolResultMessage struct {
	ToolCallID string
	ToolName   string
	Content    []ContentBlock
	IsError    bool
	Timestamp  time.Time
}

func (ToolResultMessage) isMessage() {}

// Role returns RoleToolResult.
func (ToolResultMessage) Role() Role { return RoleToolResult }

// ContentBlock is a sealed interface representing a block of content.
// The unexported marker method prevents external implementations.
type ContentBlock interface {
	contentBlock()
}

// TextBlock contains text content.
type TextBlock struct {
	Text string
}

func (TextBlock) contentBlock() {}

// ThinkingBlock contains thinking/reasoning content.
type ThinkingBlock struct {
	Thinking  string
	Signature []byte
}

func (ThinkingBlock) contentBlock() {}

// ImageBlock contains image data.
type ImageBlock struct {
	Data     []byte
	MimeType string
}

func (ImageBlock) contentBlock() {}

// ToolCallBlock represents a tool call from the assistant.
type ToolCallBlock struct {
	ID        string
	Name      string
	Arguments json.RawMessage
	Signature []byte
}

func (ToolCallBlock) contentBlock() {}

// Interface compliance checks.
var (
	_ Message = UserMessage{}
	_ Message = AssistantMessage{}
	_ Message = ToolResultMessage{}

	_ ContentBlock = TextBlock{}
	_ ContentBlock = ThinkingBlock{}
	_ ContentBlock = ImageBlock{}
	_ ContentBlock = ToolCallBlock{}
)

// ValidateMessage checks that a message's content blocks are valid for its role.
func ValidateMessage(msg Message) error {
	switch m := msg.(type) {
	case UserMessage:
		return validateBlocks(m.Content, m.Role(), allowText|allowImage)
	case AssistantMessage:
		return validateBlocks(m.Content, m.Role(), allowText|allowThinking|allowToolCall)
	case ToolResultMessage:
		return validateBlocks(m.Content, m.Role(), allowText|allowImage)
	default:
		return fmt.Errorf("unknown message type %T: %w", msg, ErrValidation)
	}
}

type blockAllow uint8

const (
	allowText blockAllow = 1 << iota
	allowThinking
	allowImage
	allowToolCall
)

func validateBlocks(blocks []ContentBlock, role Role, allowed blockAllow) error {
	for _, b := range blocks {
		switch b.(type) {
		case TextBlock:
			if allowed&allowText == 0 {
				return fmt.Errorf("TextBlock not allowed in %s message: %w", role, ErrValidation)
			}
		case ThinkingBlock:
			if allowed&allowThinking == 0 {
				return fmt.Errorf("ThinkingBlock not allowed in %s message: %w", role, ErrValidation)
			}
		case ImageBlock:
			if allowed&allowImage == 0 {
				return fmt.Errorf("ImageBlock not allowed in %s message: %w", role, ErrValidation)
			}
		case ToolCallBlock:
			if allowed&allowToolCall == 0 {
				return fmt.Errorf("ToolCallBlock not allowed in %s message: %w", role, ErrValidation)
			}
		default:
			return fmt.Errorf("unknown content block type %T in %s message: %w", b, role, ErrValidation)
		}
	}
	return nil
}
