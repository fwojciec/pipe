package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

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

	contents, err := ConvertMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	config, err := buildConfig(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}

	iter := c.client.Models.GenerateContentStream(ctx, model, contents, config)
	return newStream(ctx, iter), nil
}

func buildConfig(req pipe.Request) (*genai.GenerateContentConfig, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	if maxTokens > math.MaxInt32 {
		maxTokens = math.MaxInt32
	}

	tools, err := ConvertTools(req.Tools)
	if err != nil {
		return nil, err
	}

	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(maxTokens), //nolint:gosec // clamped above
		Tools:           tools,
		// ThinkingConfig is set unconditionally; models that don't support
		// thinking will reject the request. Callers should use a
		// thinking-capable model (e.g. gemini-3.1-pro-preview).
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

	return config, nil
}

// ConvertMessages converts pipe Messages to genai Contents.
// Exported for testing.
func ConvertMessages(msgs []pipe.Message) ([]*genai.Content, error) {
	var result []*genai.Content
	for _, msg := range msgs {
		switch m := msg.(type) {
		case pipe.UserMessage:
			parts, err := convertParts(m.Content)
			if err != nil {
				return nil, fmt.Errorf("user message: %w", err)
			}
			result = append(result, &genai.Content{
				Role:  "user",
				Parts: parts,
			})
		case pipe.AssistantMessage:
			parts, err := convertParts(m.Content)
			if err != nil {
				return nil, fmt.Errorf("assistant message: %w", err)
			}
			result = append(result, &genai.Content{
				Role:  "model",
				Parts: parts,
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
		default:
			return nil, fmt.Errorf("unsupported message type: %T", msg)
		}
	}
	return result, nil
}

func convertParts(blocks []pipe.ContentBlock) ([]*genai.Part, error) {
	var parts []*genai.Part
	// Gemini requires ThoughtSignature on FunctionCall parts that follow
	// thinking. Track the last signature so tool calls can include it.
	// lastSig intentionally persists across non-thinking blocks (Text, Image)
	// because Gemini's thinking always logically precedes the tool calls it
	// produces, regardless of any intervening content parts.
	var lastSig []byte
	for _, b := range blocks {
		switch bl := b.(type) {
		case pipe.TextBlock:
			parts = append(parts, &genai.Part{Text: bl.Text})
		case pipe.ThinkingBlock:
			p := &genai.Part{Text: bl.Thinking, Thought: true}
			if bl.Signature != nil {
				p.ThoughtSignature = bl.Signature
				lastSig = bl.Signature
			} else {
				lastSig = nil
			}
			parts = append(parts, p)
		case pipe.ToolCallBlock:
			var args map[string]any
			if err := json.Unmarshal(bl.Arguments, &args); err != nil {
				return nil, fmt.Errorf("invalid tool call arguments JSON: %w", err)
			}
			p := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   bl.ID,
					Name: bl.Name,
					Args: args,
				},
			}
			if lastSig != nil {
				p.ThoughtSignature = lastSig
			}
			parts = append(parts, p)
		case pipe.ImageBlock:
			parts = append(parts, &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: bl.MimeType,
					Data:     bl.Data,
				},
			})
		default:
			return nil, fmt.Errorf("unsupported content block type: %T", b)
		}
	}
	return parts, nil
}

// extractText returns the concatenated text of all TextBlocks, separated by
// newlines. Returns empty string if no TextBlocks are present.
func extractText(blocks []pipe.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if tb, ok := b.(pipe.TextBlock); ok {
			parts = append(parts, tb.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// ConvertTools converts pipe Tools to genai Tools.
// Exported for testing.
func ConvertTools(tools []pipe.Tool) ([]*genai.Tool, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	decls := make([]*genai.FunctionDeclaration, len(tools))
	for i, t := range tools {
		var schema map[string]any
		if err := json.Unmarshal(t.Parameters, &schema); err != nil {
			return nil, fmt.Errorf("invalid tool parameters JSON for %q: %w", t.Name, err)
		}
		decls[i] = &genai.FunctionDeclaration{
			Name:                 t.Name,
			Description:          t.Description,
			ParametersJsonSchema: schema,
		}
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}, nil
}
