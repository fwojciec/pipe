package anthropic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/fwojciec/pipe"
)

// Interface compliance check.
var _ pipe.Provider = (*Client)(nil)

// Client implements [pipe.Provider] for the Anthropic Messages API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Option configures a [Client].
type Option func(*Client)

// WithBaseURL sets the API base URL. Useful for testing with httptest.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// New creates a new Anthropic [Client] with the given API key and options.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Stream sends a streaming request to the Anthropic Messages API and returns
// a [pipe.Stream] that emits semantic events.
func (c *Client) Stream(ctx context.Context, req pipe.Request) (pipe.Stream, error) {
	body, err := c.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+messagesPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", c.apiKey)
	httpReq.Header.Set("Anthropic-Version", apiVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, parseHTTPError(resp)
	}

	return newStream(ctx, resp.Body), nil
}

func (c *Client) buildRequestBody(req pipe.Request) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	apiReq := apiRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Stream:      true,
		System:      convertSystem(req.SystemPrompt),
		Messages:    convertMessages(req.Messages),
		Tools:       convertTools(req.Tools),
		Temperature: req.Temperature,
	}
	injectCacheMarkers(&apiReq)

	return json.Marshal(apiReq)
}

// convertSystem converts a system prompt string to an array of content blocks
// suitable for the Anthropic API. Returns nil when the prompt is empty.
func convertSystem(prompt string) []apiContentBlock {
	if prompt == "" {
		return nil
	}
	return []apiContentBlock{{Type: "text", Text: prompt}}
}

// injectCacheMarkers sets cache_control breakpoints on the request:
//  1. Top-level: automatic caching for the conversation message window.
//  2. System prompt last block: stable content breakpoint.
//  3. Last tool: stable tool definitions breakpoint.
func injectCacheMarkers(req *apiRequest) {
	// cc is shared across all breakpoints; safe because it is read-only after assignment.
	cc := &apiCacheControl{Type: "ephemeral"}

	// Top-level cache_control for automatic message-window caching.
	req.CacheControl = cc

	// System prompt last block.
	if len(req.System) > 0 {
		req.System[len(req.System)-1].CacheControl = cc
	}

	// Last tool.
	if len(req.Tools) > 0 {
		req.Tools[len(req.Tools)-1].CacheControl = cc
	}
}

func convertMessages(msgs []pipe.Message) []apiMessage {
	var result []apiMessage
	for _, msg := range msgs {
		switch m := msg.(type) {
		case pipe.UserMessage:
			result = append(result, apiMessage{
				Role:    "user",
				Content: convertContentBlocks(m.Content),
			})
		case pipe.AssistantMessage:
			result = append(result, apiMessage{
				Role:    "assistant",
				Content: convertContentBlocks(m.Content),
			})
		case pipe.ToolResultMessage:
			block := apiContentBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   convertContentBlocks(m.Content),
				IsError:   m.IsError,
			}
			// Merge consecutive tool results into the same user message.
			if n := len(result); n > 0 && result[n-1].Role == "user" && isToolResultMessage(result[n-1]) {
				result[n-1].Content = append(result[n-1].Content, block)
			} else {
				result = append(result, apiMessage{
					Role:    "user",
					Content: []apiContentBlock{block},
				})
			}
		}
	}
	return result
}

func isToolResultMessage(msg apiMessage) bool {
	return len(msg.Content) > 0 && msg.Content[0].Type == "tool_result"
}

func convertContentBlocks(blocks []pipe.ContentBlock) []apiContentBlock {
	result := make([]apiContentBlock, 0, len(blocks))
	for _, b := range blocks {
		switch bl := b.(type) {
		case pipe.TextBlock:
			result = append(result, apiContentBlock{Type: "text", Text: bl.Text})
		case pipe.ThinkingBlock:
			result = append(result, apiContentBlock{Type: "thinking", Thinking: bl.Thinking})
		case pipe.ToolCallBlock:
			result = append(result, apiContentBlock{Type: "tool_use", ID: bl.ID, Name: bl.Name, Input: bl.Arguments})
		case pipe.ImageBlock:
			result = append(result, apiContentBlock{
				Type: "image",
				Source: &apiImageSource{
					Type:      "base64",
					MediaType: bl.MimeType,
					Data:      base64.StdEncoding.EncodeToString(bl.Data),
				},
			})
		}
	}
	return result
}

func convertTools(tools []pipe.Tool) []apiTool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]apiTool, len(tools))
	for i, t := range tools {
		result[i] = apiTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		}
	}
	return result
}

func parseHTTPError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("anthropic: HTTP %d (failed to read body: %w)", resp.StatusCode, err)
	}
	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return fmt.Errorf("anthropic: %s: %s", apiErr.Error.Type, apiErr.Error.Message)
}
