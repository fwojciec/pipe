package json

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/fwojciec/pipe"
)

// contentBlock is the JSON representation of a ContentBlock with a type discriminator.
type contentBlock struct {
	Type      string           `json:"type"`
	Text      *string          `json:"text,omitempty"`
	Thinking  *string          `json:"thinking,omitempty"`
	Signature *string          `json:"signature,omitempty"`
	Data      *string          `json:"data,omitempty"`
	MimeType  *string          `json:"mime_type,omitempty"`
	ID        *string          `json:"id,omitempty"`
	Name      *string          `json:"name,omitempty"`
	Arguments *json.RawMessage `json:"arguments,omitempty"`
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
		cb := contentBlock{Type: "thinking", Thinking: &v.Thinking}
		if len(v.Signature) > 0 {
			encoded := base64.StdEncoding.EncodeToString(v.Signature)
			cb.Signature = &encoded
		}
		return cb, nil
	case pipe.ImageBlock:
		encoded := base64.StdEncoding.EncodeToString(v.Data)
		return contentBlock{Type: "image", Data: &encoded, MimeType: &v.MimeType}, nil
	case pipe.ToolCallBlock:
		args := v.Arguments
		cb := contentBlock{Type: "tool_call", ID: &v.ID, Name: &v.Name, Arguments: &args}
		if len(v.Signature) > 0 {
			encoded := base64.StdEncoding.EncodeToString(v.Signature)
			cb.Signature = &encoded
		}
		return cb, nil
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
		var sig []byte
		if dto.Signature != nil && *dto.Signature != "" {
			var err error
			sig, err = base64.StdEncoding.DecodeString(*dto.Signature)
			if err != nil {
				return nil, fmt.Errorf("decode thinking signature: %w", err)
			}
		}
		return pipe.ThinkingBlock{Thinking: thinking, Signature: sig}, nil
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
		var sig []byte
		if dto.Signature != nil && *dto.Signature != "" {
			var err error
			sig, err = base64.StdEncoding.DecodeString(*dto.Signature)
			if err != nil {
				return nil, fmt.Errorf("decode tool call signature: %w", err)
			}
		}
		return pipe.ToolCallBlock{ID: id, Name: name, Arguments: args, Signature: sig}, nil
	default:
		return nil, fmt.Errorf("unknown content block type: %q", dto.Type)
	}
}
