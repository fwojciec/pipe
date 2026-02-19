package pipe

import "fmt"

// Validate checks universal constraints on Request.
// Provider implementations may apply additional provider-specific validation.
func (r Request) Validate() error {
	if r.Temperature != nil {
		if *r.Temperature < 0 || *r.Temperature > 2 {
			return fmt.Errorf("temperature must be in [0, 2], got %g: %w", *r.Temperature, ErrValidation)
		}
	}
	if r.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be non-negative, got %d: %w", r.MaxTokens, ErrValidation)
	}
	return nil
}

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
