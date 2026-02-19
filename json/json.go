package json

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fwojciec/pipe"
)

// envelope is the v1 wire format for a persisted session.
type envelope struct {
	Version      int          `json:"version"`
	ID           string       `json:"id"`
	SystemPrompt string       `json:"system_prompt"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	Messages     []messageDTO `json:"messages"`
}

// messageDTO is the JSON representation of a Message with a type discriminator.
type messageDTO struct {
	Type          string         `json:"type"`
	Content       []contentBlock `json:"content"`
	Timestamp     time.Time      `json:"timestamp"`
	StopReason    *string        `json:"stop_reason,omitempty"`
	RawStopReason *string        `json:"raw_stop_reason,omitempty"`
	Usage         *usageDTO      `json:"usage,omitempty"`
	ToolCallID    *string        `json:"tool_call_id,omitempty"`
	ToolName      *string        `json:"tool_name,omitempty"`
	IsError       *bool          `json:"is_error,omitempty"`
}

// contentBlock is the JSON representation of a ContentBlock with a type discriminator.
type contentBlock struct {
	Type      string           `json:"type"`
	Text      *string          `json:"text,omitempty"`
	Thinking  *string          `json:"thinking,omitempty"`
	Data      *string          `json:"data,omitempty"`
	MimeType  *string          `json:"mime_type,omitempty"`
	ID        *string          `json:"id,omitempty"`
	Name      *string          `json:"name,omitempty"`
	Arguments *json.RawMessage `json:"arguments,omitempty"`
}

type usageDTO struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// MarshalSession serializes a Session to JSON in v1 envelope format.
func MarshalSession(s pipe.Session) ([]byte, error) {
	env := envelope{
		Version:      1,
		ID:           s.ID,
		SystemPrompt: s.SystemPrompt,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		Messages:     make([]messageDTO, len(s.Messages)),
	}
	for i, msg := range s.Messages {
		dto, err := marshalMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", i, err)
		}
		env.Messages[i] = dto
	}
	return json.MarshalIndent(env, "", "  ")
}

// UnmarshalSession deserializes a Session from JSON in v1 envelope format.
func UnmarshalSession(data []byte) (pipe.Session, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return pipe.Session{}, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.Version != 1 {
		return pipe.Session{}, fmt.Errorf("unsupported envelope version: %d", env.Version)
	}
	msgs := make([]pipe.Message, len(env.Messages))
	for i, dto := range env.Messages {
		msg, err := unmarshalMessage(dto)
		if err != nil {
			return pipe.Session{}, fmt.Errorf("message %d: %w", i, err)
		}
		msgs[i] = msg
	}
	return pipe.Session{
		ID:           env.ID,
		SystemPrompt: env.SystemPrompt,
		CreatedAt:    env.CreatedAt,
		UpdatedAt:    env.UpdatedAt,
		Messages:     msgs,
	}, nil
}

// Save writes a Session to a JSON file, creating parent directories as needed.
func Save(path string, s pipe.Session) error {
	data, err := MarshalSession(s)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// Load reads a Session from a JSON file.
func Load(path string) (pipe.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pipe.Session{}, fmt.Errorf("read file: %w", err)
	}
	return UnmarshalSession(data)
}

func marshalMessage(msg pipe.Message) (messageDTO, error) {
	switch m := msg.(type) {
	case pipe.UserMessage:
		blocks, err := marshalContentBlocks(m.Content)
		if err != nil {
			return messageDTO{}, err
		}
		return messageDTO{
			Type:      "user",
			Content:   blocks,
			Timestamp: m.Timestamp,
		}, nil
	case pipe.AssistantMessage:
		blocks, err := marshalContentBlocks(m.Content)
		if err != nil {
			return messageDTO{}, err
		}
		sr := string(m.StopReason)
		return messageDTO{
			Type:          "assistant",
			Content:       blocks,
			Timestamp:     m.Timestamp,
			StopReason:    &sr,
			RawStopReason: &m.RawStopReason,
			Usage:         &usageDTO{InputTokens: m.Usage.InputTokens, OutputTokens: m.Usage.OutputTokens},
		}, nil
	case pipe.ToolResultMessage:
		blocks, err := marshalContentBlocks(m.Content)
		if err != nil {
			return messageDTO{}, err
		}
		return messageDTO{
			Type:       "tool_result",
			Content:    blocks,
			Timestamp:  m.Timestamp,
			ToolCallID: &m.ToolCallID,
			ToolName:   &m.ToolName,
			IsError:    &m.IsError,
		}, nil
	default:
		return messageDTO{}, fmt.Errorf("unknown message type: %T", msg)
	}
}

func unmarshalMessage(dto messageDTO) (pipe.Message, error) {
	blocks, err := unmarshalContentBlocks(dto.Content)
	if err != nil {
		return nil, err
	}
	switch dto.Type {
	case "user":
		return pipe.UserMessage{
			Content:   blocks,
			Timestamp: dto.Timestamp,
		}, nil
	case "assistant":
		var sr pipe.StopReason
		if dto.StopReason != nil {
			sr = pipe.StopReason(*dto.StopReason)
		}
		var rawSR string
		if dto.RawStopReason != nil {
			rawSR = *dto.RawStopReason
		}
		var usage pipe.Usage
		if dto.Usage != nil {
			usage = pipe.Usage{InputTokens: dto.Usage.InputTokens, OutputTokens: dto.Usage.OutputTokens}
		}
		return pipe.AssistantMessage{
			Content:       blocks,
			StopReason:    sr,
			RawStopReason: rawSR,
			Usage:         usage,
			Timestamp:     dto.Timestamp,
		}, nil
	case "tool_result":
		var toolCallID, toolName string
		if dto.ToolCallID != nil {
			toolCallID = *dto.ToolCallID
		}
		if dto.ToolName != nil {
			toolName = *dto.ToolName
		}
		var isError bool
		if dto.IsError != nil {
			isError = *dto.IsError
		}
		return pipe.ToolResultMessage{
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Content:    blocks,
			IsError:    isError,
			Timestamp:  dto.Timestamp,
		}, nil
	default:
		return nil, fmt.Errorf("unknown message type: %q", dto.Type)
	}
}

func marshalContentBlocks(blocks []pipe.ContentBlock) ([]contentBlock, error) {
	result := make([]contentBlock, len(blocks))
	for i, b := range blocks {
		cb, err := marshalContentBlock(b)
		if err != nil {
			return nil, fmt.Errorf("content block %d: %w", i, err)
		}
		result[i] = cb
	}
	return result, nil
}

func marshalContentBlock(b pipe.ContentBlock) (contentBlock, error) {
	switch v := b.(type) {
	case pipe.TextBlock:
		return contentBlock{Type: "text", Text: &v.Text}, nil
	case pipe.ThinkingBlock:
		return contentBlock{Type: "thinking", Thinking: &v.Thinking}, nil
	case pipe.ImageBlock:
		encoded := base64.StdEncoding.EncodeToString(v.Data)
		return contentBlock{Type: "image", Data: &encoded, MimeType: &v.MimeType}, nil
	case pipe.ToolCallBlock:
		args := v.Arguments
		return contentBlock{Type: "tool_call", ID: &v.ID, Name: &v.Name, Arguments: &args}, nil
	default:
		return contentBlock{}, fmt.Errorf("unknown content block type: %T", b)
	}
}

func unmarshalContentBlocks(dtos []contentBlock) ([]pipe.ContentBlock, error) {
	result := make([]pipe.ContentBlock, len(dtos))
	for i, dto := range dtos {
		b, err := unmarshalContentBlock(dto)
		if err != nil {
			return nil, fmt.Errorf("content block %d: %w", i, err)
		}
		result[i] = b
	}
	return result, nil
}

func unmarshalContentBlock(dto contentBlock) (pipe.ContentBlock, error) {
	switch dto.Type {
	case "text":
		var text string
		if dto.Text != nil {
			text = *dto.Text
		}
		return pipe.TextBlock{Text: text}, nil
	case "thinking":
		var thinking string
		if dto.Thinking != nil {
			thinking = *dto.Thinking
		}
		return pipe.ThinkingBlock{Thinking: thinking}, nil
	case "image":
		var data []byte
		if dto.Data != nil {
			var err error
			data, err = base64.StdEncoding.DecodeString(*dto.Data)
			if err != nil {
				return nil, fmt.Errorf("decode image data: %w", err)
			}
		}
		var mimeType string
		if dto.MimeType != nil {
			mimeType = *dto.MimeType
		}
		return pipe.ImageBlock{Data: data, MimeType: mimeType}, nil
	case "tool_call":
		var id, name string
		if dto.ID != nil {
			id = *dto.ID
		}
		if dto.Name != nil {
			name = *dto.Name
		}
		var args json.RawMessage
		if dto.Arguments != nil {
			args = *dto.Arguments
		}
		return pipe.ToolCallBlock{ID: id, Name: name, Arguments: args}, nil
	default:
		return nil, fmt.Errorf("unknown content block type: %q", dto.Type)
	}
}
