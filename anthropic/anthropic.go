// Package anthropic implements [pipe.Provider] for the Anthropic Messages API.
//
// It connects to the Anthropic Messages API via SSE and emits semantic events
// through the pull-based [pipe.Stream] interface. The SSE parser drives one
// step at a time using a state-machine approach inspired by Rob Pike's lexer
// talk.
package anthropic

import "encoding/json"

const (
	defaultBaseURL   = "https://api.anthropic.com"
	defaultModel     = "claude-sonnet-4-20250514"
	defaultMaxTokens = 8192
	apiVersion       = "2023-06-01"
	messagesPath     = "/v1/messages"
)

// apiCacheControl specifies a cache breakpoint for prompt caching.
type apiCacheControl struct {
	Type string `json:"type"`          // always "ephemeral"
	TTL  string `json:"ttl,omitempty"` // "" (default 5m) or "1h"
}

// apiRequest is the JSON body sent to the Anthropic Messages API.
type apiRequest struct {
	Model        string            `json:"model"`
	MaxTokens    int               `json:"max_tokens"`
	Stream       bool              `json:"stream"`
	System       []apiContentBlock `json:"system,omitempty"`
	Messages     []apiMessage      `json:"messages"`
	Tools        []apiTool         `json:"tools,omitempty"`
	Temperature  *float64          `json:"temperature,omitempty"`
	CacheControl *apiCacheControl  `json:"cache_control,omitempty"`
}

type apiMessage struct {
	Role    string            `json:"role"`
	Content []apiContentBlock `json:"content"`
}

// apiContentBlock represents a content block in the API request.
// Different fields are populated depending on Type.
type apiContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// thinking
	Thinking string `json:"thinking,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string            `json:"tool_use_id,omitempty"`
	Content   []apiContentBlock `json:"content,omitempty"`
	IsError   bool              `json:"is_error,omitempty"`

	// image
	Source *apiImageSource `json:"source,omitempty"`

	// cache control
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
}

type apiImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type apiTool struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	InputSchema  json.RawMessage  `json:"input_schema"`
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
}

// SSE response types.

type sseMessageStart struct {
	Type    string            `json:"type"`
	Message sseMessagePayload `json:"message"`
}

type sseMessagePayload struct {
	ID           string   `json:"id"`
	Model        string   `json:"model"`
	StopReason   *string  `json:"stop_reason"`
	StopSequence *string  `json:"stop_sequence"`
	Usage        sseUsage `json:"usage"`
}

// sseUsage is used in message_start events.
// Cache fields are nullable per the Anthropic API schema.
type sseUsage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

// sseDeltaUsage is used in message_delta events.
// All fields except OutputTokens may be absent or null.
type sseDeltaUsage struct {
	OutputTokens             int  `json:"output_tokens"`
	InputTokens              *int `json:"input_tokens,omitempty"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}

type sseContentBlockStart struct {
	Type         string          `json:"type"`
	Index        int             `json:"index"`
	ContentBlock sseContentBlock `json:"content_block"`
}

type sseContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
}

type sseContentBlockDelta struct {
	Type  string   `json:"type"`
	Index int      `json:"index"`
	Delta sseDelta `json:"delta"`
}

type sseDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

type sseContentBlockStop struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type sseMessageDelta struct {
	Type  string             `json:"type"`
	Delta sseMessageDeltaVal `json:"delta"`
	Usage sseDeltaUsage      `json:"usage"`
}

type sseMessageDeltaVal struct {
	StopReason   *string `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

type sseError struct {
	Type  string         `json:"type"`
	Error sseErrorDetail `json:"error"`
}

type sseErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// apiErrorResponse is the JSON body returned on non-200 HTTP responses.
type apiErrorResponse struct {
	Type  string         `json:"type"`
	Error sseErrorDetail `json:"error"`
}
