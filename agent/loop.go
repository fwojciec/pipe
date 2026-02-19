// Package agent orchestrates the conversation loop between a Provider and a ToolExecutor.
package agent

import (
	"context"
	"io"
	"time"

	"github.com/fwojciec/pipe"
)

// Loop orchestrates the conversation between a Provider and a ToolExecutor.
type Loop struct {
	provider pipe.Provider
	executor pipe.ToolExecutor
}

// New creates a new Loop with the given provider and tool executor.
func New(provider pipe.Provider, executor pipe.ToolExecutor) *Loop {
	return &Loop{provider: provider, executor: executor}
}

// RunOption configures a single Run invocation.
type RunOption func(*runConfig)

type runConfig struct {
	onEvent func(pipe.Event)
	model   string
}

// WithEventHandler sets a callback that receives each streaming event during
// the run. If nil or not set, events are silently discarded.
func WithEventHandler(h func(pipe.Event)) RunOption {
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
func (l *Loop) Run(ctx context.Context, session *pipe.Session, tools []pipe.Tool, opts ...RunOption) error {
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
func (l *Loop) turn(ctx context.Context, session *pipe.Session, tools []pipe.Tool, cfg *runConfig) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	req := pipe.Request{
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
	var toolCalls []pipe.ToolCallBlock
	for _, block := range msg.Content {
		if tc, ok := block.(pipe.ToolCallBlock); ok {
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
			result = &pipe.ToolResult{
				Content: []pipe.ContentBlock{pipe.TextBlock{Text: execErr.Error()}},
				IsError: true,
			}
		}

		trm := pipe.ToolResultMessage{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    result.Content,
			IsError:    result.IsError,
			Timestamp:  time.Now(),
		}
		session.Messages = append(session.Messages, trm)
	}
	session.UpdatedAt = time.Now()

	return true, nil
}
