package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/agent"
	"github.com/fwojciec/pipe/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// completedStream returns a mock stream that immediately signals completion
// and returns the given AssistantMessage.
func completedStream(msg pipe.AssistantMessage) *mock.Stream {
	return &mock.Stream{
		NextFn: func() (pipe.Event, error) {
			return nil, io.EOF
		},
		MessageFn: func() (pipe.AssistantMessage, error) {
			return msg, nil
		},
	}
}

func TestLoop_Run(t *testing.T) {
	t.Parallel()

	t.Run("text response ends turn", func(t *testing.T) {
		t.Parallel()

		msg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}},
			StopReason: pipe.StopEndTurn,
		}

		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				return completedStream(msg), nil
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				t.Fatal("executor should not be called")
				return nil, nil
			},
		}

		session := &pipe.Session{SystemPrompt: "you are helpful"}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		require.NoError(t, err)

		require.Len(t, session.Messages, 1)
		am, ok := session.Messages[0].(pipe.AssistantMessage)
		require.True(t, ok)
		assert.Equal(t, pipe.StopEndTurn, am.StopReason)
	})

	t.Run("stop reason length ends loop", func(t *testing.T) {
		t.Parallel()

		msg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "truncated resp"}},
			StopReason: pipe.StopLength,
		}

		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				return completedStream(msg), nil
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				t.Fatal("executor should not be called")
				return nil, nil
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		require.NoError(t, err)

		require.Len(t, session.Messages, 1)
		am, ok := session.Messages[0].(pipe.AssistantMessage)
		require.True(t, ok)
		assert.Equal(t, pipe.StopLength, am.StopReason)
	})

	t.Run("single tool call", func(t *testing.T) {
		t.Parallel()

		toolArgs := json.RawMessage(`{"command":"echo hi"}`)
		toolCallMsg := pipe.AssistantMessage{
			Content: []pipe.ContentBlock{
				pipe.ToolCallBlock{ID: "tc_1", Name: "bash", Arguments: toolArgs},
			},
			StopReason: pipe.StopToolUse,
		}
		textMsg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "done"}},
			StopReason: pipe.StopEndTurn,
		}

		turn := 0
		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				turn++
				if turn == 1 {
					return completedStream(toolCallMsg), nil
				}
				return completedStream(textMsg), nil
			},
		}

		var executedName string
		var executedArgs json.RawMessage
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, name string, args json.RawMessage) (*pipe.ToolResult, error) {
				executedName = name
				executedArgs = args
				return &pipe.ToolResult{
					Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi\n"}},
				}, nil
			},
		}

		session := &pipe.Session{SystemPrompt: "test"}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		require.NoError(t, err)

		require.Len(t, session.Messages, 3)

		// First: assistant with tool call
		am1, ok := session.Messages[0].(pipe.AssistantMessage)
		require.True(t, ok)
		assert.Equal(t, pipe.StopToolUse, am1.StopReason)

		// Second: tool result
		trm, ok := session.Messages[1].(pipe.ToolResultMessage)
		require.True(t, ok)
		assert.Equal(t, "tc_1", trm.ToolCallID)
		assert.Equal(t, "bash", trm.ToolName)
		assert.False(t, trm.IsError)

		// Third: assistant with text
		am2, ok := session.Messages[2].(pipe.AssistantMessage)
		require.True(t, ok)
		assert.Equal(t, pipe.StopEndTurn, am2.StopReason)

		// Verify executor was called correctly
		assert.Equal(t, "bash", executedName)
		assert.JSONEq(t, `{"command":"echo hi"}`, string(executedArgs))
	})

	t.Run("multiple tool calls in single response", func(t *testing.T) {
		t.Parallel()

		toolCallMsg := pipe.AssistantMessage{
			Content: []pipe.ContentBlock{
				pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{"file_path":"/tmp/a"}`)},
				pipe.TextBlock{Text: "I'll read both files"},
				pipe.ToolCallBlock{ID: "tc_2", Name: "read", Arguments: json.RawMessage(`{"file_path":"/tmp/b"}`)},
			},
			StopReason: pipe.StopToolUse,
		}
		textMsg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "both files read"}},
			StopReason: pipe.StopEndTurn,
		}

		turn := 0
		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				turn++
				if turn == 1 {
					return completedStream(toolCallMsg), nil
				}
				return completedStream(textMsg), nil
			},
		}

		var executedNames []string
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, name string, _ json.RawMessage) (*pipe.ToolResult, error) {
				executedNames = append(executedNames, name)
				return &pipe.ToolResult{
					Content: []pipe.ContentBlock{pipe.TextBlock{Text: "content"}},
				}, nil
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		require.NoError(t, err)

		// assistant (2 tool calls) + tool result 1 + tool result 2 + assistant (text)
		require.Len(t, session.Messages, 4)

		trm1, ok := session.Messages[1].(pipe.ToolResultMessage)
		require.True(t, ok)
		assert.Equal(t, "tc_1", trm1.ToolCallID)

		trm2, ok := session.Messages[2].(pipe.ToolResultMessage)
		require.True(t, ok)
		assert.Equal(t, "tc_2", trm2.ToolCallID)

		assert.Equal(t, []string{"read", "read"}, executedNames)
	})

	t.Run("tool infrastructure error becomes error result", func(t *testing.T) {
		t.Parallel()

		toolCallMsg := pipe.AssistantMessage{
			Content: []pipe.ContentBlock{
				pipe.ToolCallBlock{ID: "tc_1", Name: "bash", Arguments: json.RawMessage(`{}`)},
			},
			StopReason: pipe.StopToolUse,
		}
		textMsg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "I see the error"}},
			StopReason: pipe.StopEndTurn,
		}

		turn := 0
		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				turn++
				if turn == 1 {
					return completedStream(toolCallMsg), nil
				}
				return completedStream(textMsg), nil
			},
		}

		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return nil, errors.New("process not found")
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		require.NoError(t, err)

		require.Len(t, session.Messages, 3)

		trm, ok := session.Messages[1].(pipe.ToolResultMessage)
		require.True(t, ok)
		assert.True(t, trm.IsError)
		assert.Equal(t, "tc_1", trm.ToolCallID)
		require.Len(t, trm.Content, 1)
		tb, ok := trm.Content[0].(pipe.TextBlock)
		require.True(t, ok)
		assert.Equal(t, "process not found", tb.Text)
	})

	t.Run("tool domain error fed back to LLM", func(t *testing.T) {
		t.Parallel()

		toolCallMsg := pipe.AssistantMessage{
			Content: []pipe.ContentBlock{
				pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{}`)},
			},
			StopReason: pipe.StopToolUse,
		}
		textMsg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "file not found, let me try another"}},
			StopReason: pipe.StopEndTurn,
		}

		turn := 0
		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				turn++
				if turn == 1 {
					return completedStream(toolCallMsg), nil
				}
				return completedStream(textMsg), nil
			},
		}

		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return &pipe.ToolResult{
					Content: []pipe.ContentBlock{pipe.TextBlock{Text: "file not found: /tmp/missing"}},
					IsError: true,
				}, nil
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		require.NoError(t, err)

		require.Len(t, session.Messages, 3)

		trm, ok := session.Messages[1].(pipe.ToolResultMessage)
		require.True(t, ok)
		assert.True(t, trm.IsError)
	})

	t.Run("stream error preserves partial message", func(t *testing.T) {
		t.Parallel()

		streamErr := errors.New("connection reset")
		partialMsg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "partial"}},
			StopReason: pipe.StopError,
		}

		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				return &mock.Stream{
					NextFn: func() (pipe.Event, error) {
						return nil, streamErr
					},
					MessageFn: func() (pipe.AssistantMessage, error) {
						return partialMsg, nil
					},
				}, nil
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return nil, nil
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		assert.ErrorIs(t, err, streamErr)

		require.Len(t, session.Messages, 1)
		am, ok := session.Messages[0].(pipe.AssistantMessage)
		require.True(t, ok)
		assert.Equal(t, pipe.StopError, am.StopReason)
	})

	t.Run("provider stream error", func(t *testing.T) {
		t.Parallel()

		providerErr := errors.New("API rate limited")
		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				return nil, providerErr
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return nil, nil
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		assert.ErrorIs(t, err, providerErr)
		assert.Empty(t, session.Messages)
	})

	t.Run("context cancellation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		provider := &mock.Provider{
			StreamFn: func(ctx context.Context, _ pipe.Request) (pipe.Stream, error) {
				return nil, ctx.Err()
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return nil, nil
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(ctx, session, nil)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("request includes system prompt and tools", func(t *testing.T) {
		t.Parallel()

		var capturedReq pipe.Request
		provider := &mock.Provider{
			StreamFn: func(_ context.Context, req pipe.Request) (pipe.Stream, error) {
				capturedReq = req
				msg := pipe.AssistantMessage{
					Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "ok"}},
					StopReason: pipe.StopEndTurn,
				}
				return completedStream(msg), nil
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return nil, nil
			},
		}

		tools := []pipe.Tool{
			{Name: "bash", Description: "run commands"},
		}
		session := &pipe.Session{
			SystemPrompt: "be helpful",
			Messages: []pipe.Message{
				pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "hi"}}},
			},
		}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, tools)
		require.NoError(t, err)

		assert.Equal(t, "be helpful", capturedReq.SystemPrompt)
		require.Len(t, capturedReq.Tools, 1)
		assert.Equal(t, "bash", capturedReq.Tools[0].Name)
		require.Len(t, capturedReq.Messages, 1)
	})

	t.Run("event handler receives stream events", func(t *testing.T) {
		t.Parallel()

		events := []pipe.Event{
			pipe.EventTextDelta{Index: 0, Delta: "hel"},
			pipe.EventTextDelta{Index: 0, Delta: "lo"},
		}

		msg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}},
			StopReason: pipe.StopEndTurn,
		}

		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				idx := 0
				return &mock.Stream{
					NextFn: func() (pipe.Event, error) {
						if idx >= len(events) {
							return nil, io.EOF
						}
						e := events[idx]
						idx++
						return e, nil
					},
					MessageFn: func() (pipe.AssistantMessage, error) {
						return msg, nil
					},
				}, nil
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return nil, nil
			},
		}

		var received []pipe.Event
		handler := func(e pipe.Event) {
			received = append(received, e)
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil, agent.WithEventHandler(handler))
		require.NoError(t, err)

		assert.Equal(t, events, received)
	})

	t.Run("nil event handler is safe without option", func(t *testing.T) {
		t.Parallel()

		msg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}},
			StopReason: pipe.StopEndTurn,
		}

		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				return completedStream(msg), nil
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return nil, nil
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		require.NoError(t, err)
		require.Len(t, session.Messages, 1)
	})

	t.Run("nil event handler is safe with explicit nil", func(t *testing.T) {
		t.Parallel()

		msg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}},
			StopReason: pipe.StopEndTurn,
		}

		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				return completedStream(msg), nil
			},
		}
		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return nil, nil
			},
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil, agent.WithEventHandler(nil))
		require.NoError(t, err)
		require.Len(t, session.Messages, 1)
	})

	t.Run("event handler receives events across multi-turn run", func(t *testing.T) {
		t.Parallel()

		turn1Events := []pipe.Event{
			pipe.EventTextDelta{Index: 0, Delta: "calling tool"},
		}
		turn2Events := []pipe.Event{
			pipe.EventTextDelta{Index: 0, Delta: "done"},
		}

		toolCallMsg := pipe.AssistantMessage{
			Content: []pipe.ContentBlock{
				pipe.ToolCallBlock{ID: "tc_1", Name: "bash", Arguments: json.RawMessage(`{}`)},
			},
			StopReason: pipe.StopToolUse,
		}
		textMsg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "done"}},
			StopReason: pipe.StopEndTurn,
		}

		var turn atomic.Int32
		provider := &mock.Provider{
			StreamFn: func(_ context.Context, _ pipe.Request) (pipe.Stream, error) {
				n := turn.Add(1)
				if n == 1 {
					idx := 0
					return &mock.Stream{
						NextFn: func() (pipe.Event, error) {
							if idx >= len(turn1Events) {
								return nil, io.EOF
							}
							e := turn1Events[idx]
							idx++
							return e, nil
						},
						MessageFn: func() (pipe.AssistantMessage, error) {
							return toolCallMsg, nil
						},
					}, nil
				}
				idx := 0
				return &mock.Stream{
					NextFn: func() (pipe.Event, error) {
						if idx >= len(turn2Events) {
							return nil, io.EOF
						}
						e := turn2Events[idx]
						idx++
						return e, nil
					},
					MessageFn: func() (pipe.AssistantMessage, error) {
						return textMsg, nil
					},
				}, nil
			},
		}

		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return &pipe.ToolResult{
					Content: []pipe.ContentBlock{pipe.TextBlock{Text: "output"}},
				}, nil
			},
		}

		var received []pipe.Event
		handler := func(e pipe.Event) {
			received = append(received, e)
		}

		session := &pipe.Session{}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil, agent.WithEventHandler(handler))
		require.NoError(t, err)

		allExpected := slices.Concat(turn1Events, turn2Events)
		assert.Equal(t, allExpected, received)
	})

	t.Run("tool results included in subsequent request", func(t *testing.T) {
		t.Parallel()

		toolCallMsg := pipe.AssistantMessage{
			Content: []pipe.ContentBlock{
				pipe.ToolCallBlock{ID: "tc_1", Name: "bash", Arguments: json.RawMessage(`{}`)},
			},
			StopReason: pipe.StopToolUse,
		}
		textMsg := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "done"}},
			StopReason: pipe.StopEndTurn,
		}

		var requests []pipe.Request
		turn := 0
		provider := &mock.Provider{
			StreamFn: func(_ context.Context, req pipe.Request) (pipe.Stream, error) {
				requests = append(requests, req)
				turn++
				if turn == 1 {
					return completedStream(toolCallMsg), nil
				}
				return completedStream(textMsg), nil
			},
		}

		executor := &mock.ToolExecutor{
			ExecuteFn: func(_ context.Context, _ string, _ json.RawMessage) (*pipe.ToolResult, error) {
				return &pipe.ToolResult{
					Content: []pipe.ContentBlock{pipe.TextBlock{Text: "output"}},
				}, nil
			},
		}

		session := &pipe.Session{
			Messages: []pipe.Message{
				pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "run it"}}},
			},
		}
		loop := agent.New(provider, executor)

		err := loop.Run(context.Background(), session, nil)
		require.NoError(t, err)

		require.Len(t, requests, 2)
		// First request: 1 message (user)
		assert.Len(t, requests[0].Messages, 1)
		// Second request: 3 messages (user + assistant + tool result)
		assert.Len(t, requests[1].Messages, 3)
	})
}
