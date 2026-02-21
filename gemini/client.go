package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fwojciec/pipe"
	"google.golang.org/genai"
)

// Interface compliance check.
var _ pipe.Provider = (*Client)(nil)

// Client implements [pipe.Provider] for the Google Gemini API.
type Client struct {
	client *genai.Client
	model  string
}

// Option configures a [Client].
type Option func(*Client)

// WithModel sets the model ID. Default is gemini-3.1-pro-preview.
func WithModel(model string) Option {
	return func(c *Client) { c.model = model }
}

// New creates a new Gemini [Client] with the given API key and options.
func New(ctx context.Context, apiKey string, opts ...Option) (*Client, error) {
	gc, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	c := &Client{
		client: gc,
		model:  defaultModel,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Stream sends a streaming request to the Gemini API and returns a
// [pipe.Stream] that emits semantic events.
func (c *Client) Stream(ctx context.Context, req pipe.Request) (pipe.Stream, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	contents := ConvertMessages(req.Messages)
	config := buildConfig(req)

	iter := c.client.Models.GenerateContentStream(ctx, model, contents, config)
	return newStream(ctx, iter, contents), nil
}

func buildConfig(req pipe.Request) *genai.GenerateContentConfig {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(maxTokens),
		Tools:           ConvertTools(req.Tools),
		ThinkingConfig: &genai.ThinkingConfig{
			IncludeThoughts: true,
		},
	}

	if req.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: req.SystemPrompt}},
		}
	}

	if req.Temperature != nil {
		temp := float32(*req.Temperature)
		config.Temperature = &temp
	}

	return config
}

// ConvertMessages converts pipe Messages to genai Contents.
// Exported for testing.
func ConvertMessages(msgs []pipe.Message) []*genai.Content {
	var result []*genai.Content
	for _, msg := range msgs {
		switch m := msg.(type) {
		case pipe.UserMessage:
			result = append(result, &genai.Content{
				Role:  "user",
				Parts: convertParts(m.Content),
			})
		case pipe.AssistantMessage:
			result = append(result, &genai.Content{
				Role:  "model",
				Parts: convertParts(m.Content),
			})
		case pipe.ToolResultMessage:
			text := extractText(m.Content)
			var responseMap map[string]any
			if m.IsError {
				responseMap = map[string]any{"error": text}
			} else {
				responseMap = map[string]any{"output": text}
			}
			result = append(result, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						ID:       m.ToolCallID,
						Name:     m.ToolName,
						Response: responseMap,
					},
				}},
			})
		}
	}
	return result
}

func convertParts(blocks []pipe.ContentBlock) []*genai.Part {
	var parts []*genai.Part
	for _, b := range blocks {
		switch bl := b.(type) {
		case pipe.TextBlock:
			parts = append(parts, &genai.Part{Text: bl.Text})
		case pipe.ThinkingBlock:
			p := &genai.Part{Text: bl.Thinking, Thought: true}
			if bl.Signature != nil {
				p.ThoughtSignature = bl.Signature
			}
			parts = append(parts, p)
		case pipe.ToolCallBlock:
			// Arguments is json.RawMessage — always valid JSON from domain types.
			var args map[string]any
			_ = json.Unmarshal(bl.Arguments, &args)
			parts = append(parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   bl.ID,
					Name: bl.Name,
					Args: args,
				},
			})
		case pipe.ImageBlock:
			parts = append(parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: bl.MimeType,
					Data:     bl.Data,
				},
			})
		}
	}
	return parts
}

// extractText returns the text of the first TextBlock, or empty string if none.
func extractText(blocks []pipe.ContentBlock) string {
	for _, b := range blocks {
		if tb, ok := b.(pipe.TextBlock); ok {
			return tb.Text
		}
	}
	return ""
}

// ConvertTools converts pipe Tools to genai Tools.
// Exported for testing.
func ConvertTools(tools []pipe.Tool) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]*genai.FunctionDeclaration, len(tools))
	for i, t := range tools {
		// Parameters is json.RawMessage — always valid JSON from domain types.
		var schema map[string]any
		_ = json.Unmarshal(t.Parameters, &schema)
		decls[i] = &genai.FunctionDeclaration{
			Name:                 t.Name,
			Description:          t.Description,
			ParametersJsonSchema: schema,
		}
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}
