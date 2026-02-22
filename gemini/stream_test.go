package gemini_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// mockChunks returns a genai-style streaming iterator from pre-built chunks.
func mockChunks(chunks []*genai.GenerateContentResponse) func(func(*genai.GenerateContentResponse, error) bool) {
	return func(yield func(*genai.GenerateContentResponse, error) bool) {
		for _, c := range chunks {
			if !yield(c, nil) {
				return
			}
		}
	}
}

func collectStreamEvents(t *testing.T, s pipe.Stream) []pipe.Event {
	t.Helper()
	var events []pipe.Event
	for {
		evt, err := s.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		events = append(events, evt)
	}
	return events
}

func TestStream_TextDelta(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hello"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: " world"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 2)
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: "Hello"}, events[0])
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: " world"}, events[1])

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopEndTurn, msg.StopReason)
	require.Len(t, msg.Content, 1)
	assert.Equal(t, pipe.TextBlock{Text: "Hello world"}, msg.Content[0])
	assert.Equal(t, 10, msg.Usage.InputTokens)
	assert.Equal(t, 8, msg.Usage.OutputTokens)
}

func TestStream_ThinkingDelta(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "reasoning", Thought: true, ThoughtSignature: []byte("sig123")},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "Answer"},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 2)
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: "reasoning"}, events[0])
	assert.Equal(t, pipe.EventTextDelta{Index: 1, Delta: "Answer"}, events[1])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	tb := msg.Content[0].(pipe.ThinkingBlock)
	assert.Equal(t, "reasoning", tb.Thinking)
	assert.Equal(t, []byte("sig123"), tb.Signature)
	assert.Equal(t, pipe.TextBlock{Text: "Answer"}, msg.Content[1])
}

func TestStream_ToolCallComplete(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{ID: "sdk_id_1", Name: "read", Args: map[string]any{"path": "foo.go"}}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 2) // Begin + End, no Delta
	begin, ok := events[0].(pipe.EventToolCallBegin)
	require.True(t, ok)
	assert.Equal(t, "read", begin.Name)
	assert.Equal(t, "sdk_id_1", begin.ID)

	end, ok := events[1].(pipe.EventToolCallEnd)
	require.True(t, ok)
	assert.Equal(t, "read", end.Call.Name)
	assert.Equal(t, "sdk_id_1", end.Call.ID)
	assert.JSONEq(t, `{"path":"foo.go"}`, string(end.Call.Arguments))

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopToolUse, msg.StopReason)
}

func TestStream_ToolCallFallbackID(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{Name: "bash", Args: map[string]any{"cmd": "ls"}}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	begin := events[0].(pipe.EventToolCallBegin)
	assert.NotEmpty(t, begin.ID)
	assert.True(t, len(begin.ID) > 5, "generated ID should be non-trivial")
}

func TestStream_MultiPartChunk(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "reasoning", Thought: true, ThoughtSignature: []byte("sig")},
					{Text: "I'll check."},
					{FunctionCall: &genai.FunctionCall{ID: "tc_1", Name: "read", Args: map[string]any{"path": "a.go"}}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 15,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 4) // ThinkingDelta, TextDelta, ToolCallBegin, ToolCallEnd
	assert.IsType(t, pipe.EventThinkingDelta{}, events[0])
	assert.IsType(t, pipe.EventTextDelta{}, events[1])
	assert.IsType(t, pipe.EventToolCallBegin{}, events[2])
	assert.IsType(t, pipe.EventToolCallEnd{}, events[3])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 3)
	assert.IsType(t, pipe.ThinkingBlock{}, msg.Content[0])
	assert.IsType(t, pipe.TextBlock{}, msg.Content[1])
	assert.IsType(t, pipe.ToolCallBlock{}, msg.Content[2])
	assert.Equal(t, pipe.StopToolUse, msg.StopReason)
}

func TestStream_FunctionCallThoughtSignatureBackfillsThinking(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "reasoning", Thought: true},
					{
						FunctionCall: &genai.FunctionCall{
							ID:   "tc_1",
							Name: "read",
							Args: map[string]any{"path": "a.go"},
						},
						ThoughtSignature: []byte("sig-from-call"),
					},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 3)
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: "reasoning"}, events[0])
	assert.IsType(t, pipe.EventToolCallBegin{}, events[1])
	assert.IsType(t, pipe.EventToolCallEnd{}, events[2])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, pipe.ThinkingBlock{
		Thinking:  "reasoning",
		Signature: []byte("sig-from-call"),
	}, msg.Content[0])
	assert.Equal(t, pipe.ToolCallBlock{
		ID:        "tc_1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"a.go"}`),
	}, msg.Content[1])
	assert.Equal(t, pipe.StopToolUse, msg.StopReason)
}

func TestStream_InterleavedThinkTextThink(t *testing.T) {
	t.Parallel()
	// Interleaved think/text/think produces 3 blocks, not 2.
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "think1", Thought: true},
				}},
			}},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "text1"},
				}},
			}},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "think2", Thought: true},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 3)
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: "think1"}, events[0])
	assert.Equal(t, pipe.EventTextDelta{Index: 1, Delta: "text1"}, events[1])
	assert.Equal(t, pipe.EventThinkingDelta{Index: 2, Delta: "think2"}, events[2])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 3)
	assert.Equal(t, pipe.ThinkingBlock{Thinking: "think1"}, msg.Content[0])
	assert.Equal(t, pipe.TextBlock{Text: "text1"}, msg.Content[1])
	assert.Equal(t, pipe.ThinkingBlock{Thinking: "think2"}, msg.Content[2])
}

func TestStream_Usage(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:        210,
				CandidatesTokenCount:    5,
				CachedContentTokenCount: 200,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	collectStreamEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, 10, msg.Usage.InputTokens) // 210 - 200
	assert.Equal(t, 5, msg.Usage.OutputTokens)
	assert.Equal(t, 200, msg.Usage.CacheReadTokens)
	assert.Equal(t, 0, msg.Usage.CacheWriteTokens)
}

func TestStream_UsageClampsNegative(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:        5,
				CandidatesTokenCount:    3,
				CachedContentTokenCount: 100, // more cached than total
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	collectStreamEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, 0, msg.Usage.InputTokens) // clamped to zero
	assert.Equal(t, 100, msg.Usage.CacheReadTokens)
}

func TestStream_StopReasonMaxTokens(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "truncated"}}},
				FinishReason: genai.FinishReasonMaxTokens,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 100,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	collectStreamEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopLength, msg.StopReason)
	assert.Equal(t, string(genai.FinishReasonMaxTokens), msg.RawStopReason)
}

func TestStream_StopReasonDefaultEndTurn(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}},
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	collectStreamEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopEndTurn, msg.StopReason)
	assert.Equal(t, "end_turn", msg.RawStopReason)
}

func TestStream_ContextCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	emptyIter := func(yield func(*genai.GenerateContentResponse, error) bool) {}

	s := gemini.NewStreamFromIter(ctx, emptyIter)
	_, err := s.Next()
	assert.Error(t, err)

	msg, _ := s.Message()
	assert.Equal(t, pipe.StopAborted, msg.StopReason)
}

func TestStream_IteratorError(t *testing.T) {
	t.Parallel()
	errIter := func(yield func(*genai.GenerateContentResponse, error) bool) {
		yield(nil, assert.AnError)
	}

	s := gemini.NewStreamFromIter(context.Background(), errIter)
	_, err := s.Next()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gemini:")
	assert.Equal(t, pipe.StreamStateError, s.State())

	msg, _ := s.Message()
	assert.Equal(t, pipe.StopError, msg.StopReason)
}

func TestStream_State(t *testing.T) {
	t.Parallel()

	t.Run("new before first next", func(t *testing.T) {
		t.Parallel()
		chunks := []*genai.GenerateContentResponse{
			{
				Candidates: []*genai.Candidate{{
					Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
					FinishReason: genai.FinishReasonStop,
				}},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 1,
				},
			},
		}
		s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
		assert.Equal(t, pipe.StreamStateNew, s.State())
	})

	t.Run("streaming after first next", func(t *testing.T) {
		t.Parallel()
		chunks := []*genai.GenerateContentResponse{
			{
				Candidates: []*genai.Candidate{{
					Content: &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				}},
			},
			{
				Candidates: []*genai.Candidate{{
					Content:      &genai.Content{Parts: []*genai.Part{{Text: " there"}}},
					FinishReason: genai.FinishReasonStop,
				}},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 2,
				},
			},
		}
		s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
		_, err := s.Next()
		require.NoError(t, err)
		assert.Equal(t, pipe.StreamStateStreaming, s.State())
	})

	t.Run("complete after EOF", func(t *testing.T) {
		t.Parallel()
		chunks := []*genai.GenerateContentResponse{
			{
				Candidates: []*genai.Candidate{{
					Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
					FinishReason: genai.FinishReasonStop,
				}},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 1,
				},
			},
		}
		s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
		collectStreamEvents(t, s)
		assert.Equal(t, pipe.StreamStateComplete, s.State())
	})

	t.Run("closed after close mid-stream", func(t *testing.T) {
		t.Parallel()
		chunks := []*genai.GenerateContentResponse{
			{
				Candidates: []*genai.Candidate{{
					Content: &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				}},
			},
			{
				Candidates: []*genai.Candidate{{
					Content:      &genai.Content{Parts: []*genai.Part{{Text: " there"}}},
					FinishReason: genai.FinishReasonStop,
				}},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 2,
				},
			},
		}
		s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
		_, err := s.Next()
		require.NoError(t, err)
		require.NoError(t, s.Close())
		assert.Equal(t, pipe.StreamStateClosed, s.State())
	})
}

func TestStream_MessageBeforeNext(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 1,
			},
		},
	}
	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	_, err := s.Message()
	assert.Error(t, err)
}

func TestStream_CloseAbortsMessage(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
			}},
		},
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: " there"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 2,
			},
		},
	}
	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))

	_, err := s.Next()
	require.NoError(t, err)
	require.NoError(t, s.Close())

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopAborted, msg.StopReason)
}

func TestStream_ClosePreservesTerminalState(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 1,
			},
		},
	}
	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	collectStreamEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopEndTurn, msg.StopReason)

	require.NoError(t, s.Close())
	msg, err = s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopEndTurn, msg.StopReason)
}

func TestStream_NextAfterClose(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 1,
			},
		},
	}
	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	require.NoError(t, s.Close())

	_, err := s.Next()
	assert.ErrorIs(t, err, gemini.ErrStreamClosed)
}

func TestStream_ConsecutiveThinkingAccumulates(t *testing.T) {
	t.Parallel()
	// Two consecutive thinking chunks accumulate into one block.
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "step 1", Thought: true},
				}},
			}},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: " step 2", Thought: true, ThoughtSignature: []byte("sig")},
				}},
			}},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "Answer"},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 3)
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: "step 1"}, events[0])
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: " step 2"}, events[1])
	assert.Equal(t, pipe.EventTextDelta{Index: 1, Delta: "Answer"}, events[2])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, pipe.ThinkingBlock{Thinking: "step 1 step 2", Signature: []byte("sig")}, msg.Content[0])
	assert.Equal(t, pipe.TextBlock{Text: "Answer"}, msg.Content[1])
}

func TestStream_MultipleToolCalls(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{ID: "tc_1", Name: "read", Args: map[string]any{"path": "a.go"}}},
					{FunctionCall: &genai.FunctionCall{ID: "tc_2", Name: "read", Args: map[string]any{"path": "b.go"}}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 20,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 4) // Begin+End for each
	assert.IsType(t, pipe.EventToolCallBegin{}, events[0])
	assert.IsType(t, pipe.EventToolCallEnd{}, events[1])
	assert.IsType(t, pipe.EventToolCallBegin{}, events[2])
	assert.IsType(t, pipe.EventToolCallEnd{}, events[3])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, pipe.ToolCallBlock{ID: "tc_1", Name: "read", Arguments: json.RawMessage(`{"path":"a.go"}`)}, msg.Content[0])
	assert.Equal(t, pipe.ToolCallBlock{ID: "tc_2", Name: "read", Arguments: json.RawMessage(`{"path":"b.go"}`)}, msg.Content[1])
	assert.Equal(t, pipe.StopToolUse, msg.StopReason)
}

func TestStream_TrailingSignatureOnly(t *testing.T) {
	t.Parallel()
	// Signature-only part (empty text) should update block signature without emitting delta.
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "reasoning", Thought: true},
				}},
			}},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Thought: true, ThoughtSignature: []byte("trailing-sig")},
				}},
			}},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "Answer"},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	// Only 2 events: thinking delta + text delta. No delta for signature-only part.
	require.Len(t, events, 2)
	assert.Equal(t, pipe.EventThinkingDelta{Index: 0, Delta: "reasoning"}, events[0])
	assert.Equal(t, pipe.EventTextDelta{Index: 1, Delta: "Answer"}, events[1])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, pipe.ThinkingBlock{Thinking: "reasoning", Signature: []byte("trailing-sig")}, msg.Content[0])
	assert.Equal(t, pipe.TextBlock{Text: "Answer"}, msg.Content[1])
}

func TestStream_EmptyThoughtPart(t *testing.T) {
	t.Parallel()
	// An empty thought part (no text, no signature) should allocate a thinking
	// block but emit no delta event.
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Thought: true}, // empty thought part
				}},
			}},
		},
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{Text: "Answer"},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	// Only text delta — no thinking delta for the empty thought part.
	require.Len(t, events, 1)
	assert.Equal(t, pipe.EventTextDelta{Index: 1, Delta: "Answer"}, events[0])

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 2)
	assert.Equal(t, pipe.ThinkingBlock{}, msg.Content[0])
	assert.Equal(t, pipe.TextBlock{Text: "Answer"}, msg.Content[1])
}

func TestStream_FinalizePreservesNonDefaultStopReason(t *testing.T) {
	t.Parallel()
	// When a safety filter sets StopError and a tool call is also present,
	// finalize should preserve StopError rather than overwriting to StopToolUse.
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{ID: "tc_1", Name: "read", Args: map[string]any{"path": "a.go"}}},
				}},
				FinishReason: genai.FinishReasonSafety,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	collectStreamEvents(t, s)

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopError, msg.StopReason)
	assert.Equal(t, string(genai.FinishReasonSafety), msg.RawStopReason)
}

func TestStream_NilChunkSkipped(t *testing.T) {
	t.Parallel()
	// A nil chunk sandwiched between valid chunks should be silently skipped.
	iter := func(yield func(*genai.GenerateContentResponse, error) bool) {
		if !yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{{Text: "before"}}},
			}},
		}, nil) {
			return
		}
		if !yield(nil, nil) { // nil chunk
			return
		}
		if !yield(&genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: " after"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		}, nil) {
			return
		}
	}

	s := gemini.NewStreamFromIter(context.Background(), iter)
	events := collectStreamEvents(t, s)

	require.Len(t, events, 2)
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: "before"}, events[0])
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: " after"}, events[1])

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.TextBlock{Text: "before after"}, msg.Content[0])
}

func TestStream_EmptyChunkSkipped(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{}, // empty chunk — no candidates
		{
			Candidates: []*genai.Candidate{{
				Content:      &genai.Content{Parts: []*genai.Part{{Text: "Hi"}}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 1,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 1)
	assert.Equal(t, pipe.EventTextDelta{Index: 0, Delta: "Hi"}, events[0])
}

func TestStream_ToolCallNilArgs(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{ID: "tc_nil", Name: "noop", Args: nil}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	events := collectStreamEvents(t, s)

	require.Len(t, events, 2)
	end, ok := events[1].(pipe.EventToolCallEnd)
	require.True(t, ok)
	assert.Equal(t, json.RawMessage("{}"), end.Call.Arguments)

	msg, err := s.Message()
	require.NoError(t, err)
	require.Len(t, msg.Content, 1)
	call := msg.Content[0].(pipe.ToolCallBlock)
	assert.Equal(t, json.RawMessage("{}"), call.Arguments)
}

func TestStream_PromptBlocked(t *testing.T) {
	t.Parallel()
	// When a prompt is blocked for safety, PromptFeedback is set with zero
	// candidates. The stream should surface this as an error, not a normal
	// empty turn.
	chunks := []*genai.GenerateContentResponse{
		{
			PromptFeedback: &genai.GenerateContentResponsePromptFeedback{
				BlockReason: genai.BlockedReasonSafety,
			},
			// No Candidates — blocked prompt.
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	_, err := s.Next()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt blocked")

	assert.Equal(t, pipe.StreamStateError, s.State())
	msg, _ := s.Message()
	assert.Equal(t, pipe.StopError, msg.StopReason)
	assert.Equal(t, "SAFETY", msg.RawStopReason)
}

func TestStream_ProcessPartMarshalError(t *testing.T) {
	t.Parallel()
	chunks := []*genai.GenerateContentResponse{
		{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{ID: "tc_bad", Name: "read", Args: map[string]any{"val": math.NaN()}}},
				}},
				FinishReason: genai.FinishReasonStop,
			}},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		},
	}

	s := gemini.NewStreamFromIter(context.Background(), mockChunks(chunks))
	_, err := s.Next()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gemini:")
	assert.Contains(t, err.Error(), "invalid tool call arguments")
	assert.Equal(t, pipe.StreamStateError, s.State())

	msg, err := s.Message()
	require.NoError(t, err)
	assert.Equal(t, pipe.StopError, msg.StopReason)
}
