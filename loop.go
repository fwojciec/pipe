package pipe

import (
	"context"
	"io"
	"strings"
	"time"
)

// Loop orchestrates the conversation between a Provider and a ToolExecutor.
type Loop struct {
	provider Provider
	executor ToolExecutor
}

// NewLoop creates a new Loop with the given provider and tool executor.
func NewLoop(provider Provider, executor ToolExecutor) *Loop {
	return &Loop{provider: provider, executor: executor}
}

// RunOption configures a single Run invocation.
type RunOption func(*runConfig)

type runConfig struct {
	onEvent func(Event)
	model   string
}

// WithEventHandler sets a callback that receives each streaming event during
// the run. If nil or not set, events are silently discarded.
func WithEventHandler(h func(Event)) RunOption {
	return func(c *runConfig) {
		c.onEvent = h
	}
}

// WithModel sets the model ID for provider requests during this run.
// Empty string means the provider uses its default model.
func WithModel(model string) RunOption {
	return func(c *runConfig) {
		c.model = model
	}
}

// Run executes the agent loop. It sends the session's messages to the provider,
// streams the response, executes any tool calls, and repeats until the assistant
// stops requesting tools. It appends all messages to session.Messages.
func (l *Loop) Run(ctx context.Context, session *Session, tools []Tool, opts ...RunOption) error {
	var cfg runConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	for {
		cont, err := l.turn(ctx, session, tools, &cfg)
		if err != nil {
			return err
		}
		if !cont {
			return nil
		}
	}
}

// turn executes a single turn of the conversation loop. It returns true if the
// loop should continue (tool calls were made), false if it should stop.
func (l *Loop) turn(ctx context.Context, session *Session, tools []Tool, cfg *runConfig) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	req := Request{
		Model:        cfg.model,
		SystemPrompt: session.SystemPrompt,
		Messages:     session.Messages,
		Tools:        tools,
	}

	stream, err := l.provider.Stream(ctx, req)
	if err != nil {
		return false, err
	}
	defer stream.Close()

	// Drain the stream, forwarding events to handler if set.
	var streamErr error
	for {
		evt, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			streamErr = err
			break
		}
		if cfg.onEvent != nil {
			cfg.onEvent(evt)
		}
	}

	// Get the assembled message (partial or complete).
	msg, msgErr := stream.Message()
	if msgErr != nil {
		if streamErr != nil {
			return false, streamErr
		}
		return false, msgErr
	}

	session.Messages = append(session.Messages, msg)
	session.UpdatedAt = time.Now()

	if streamErr != nil {
		return false, streamErr
	}

	// Extract tool calls from the response.
	var toolCalls []ToolCallBlock
	for _, block := range msg.Content {
		if tc, ok := block.(ToolCallBlock); ok {
			toolCalls = append(toolCalls, tc)
		}
	}

	if len(toolCalls) == 0 {
		return false, nil
	}

	// Execute each tool call and append results to the session.
	for _, tc := range toolCalls {
		result, execErr := l.executor.Execute(ctx, tc.Name, tc.Arguments)
		if execErr != nil {
			result = &ToolResult{
				Content: []ContentBlock{TextBlock{Text: execErr.Error()}},
				IsError: true,
			}
		}

		trm := ToolResultMessage{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    result.Content,
			IsError:    result.IsError,
			Timestamp:  time.Now(),
		}
		session.Messages = append(session.Messages, trm)

		if cfg.onEvent != nil {
			// Only text content is surfaced in the event; other block
			// types (e.g. ImageBlock) are silently dropped by design.
			// If no text blocks exist, the event is skipped entirely.
			var sb strings.Builder
			for _, b := range result.Content {
				if tb, ok := b.(TextBlock); ok {
					if sb.Len() > 0 {
						sb.WriteByte('\n')
					}
					sb.WriteString(tb.Text)
				}
			}
			if sb.Len() > 0 {
				cfg.onEvent(EventToolResult{
					ID:       tc.ID,
					ToolName: tc.Name,
					Content:  sb.String(),
					IsError:  result.IsError,
				})
			}
		}
	}
	session.UpdatedAt = time.Now()

	return true, nil
}
