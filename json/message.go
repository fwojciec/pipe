package json

import (
	"fmt"
	"time"

	"github.com/fwojciec/pipe"
)

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
			Usage:         &usageDTO{InputTokens: m.Usage.InputTokens, OutputTokens: m.Usage.OutputTokens, CacheReadTokens: m.Usage.CacheReadTokens, CacheWriteTokens: m.Usage.CacheWriteTokens},
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
			usage = pipe.Usage{InputTokens: dto.Usage.InputTokens, OutputTokens: dto.Usage.OutputTokens, CacheReadTokens: dto.Usage.CacheReadTokens, CacheWriteTokens: dto.Usage.CacheWriteTokens}
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
