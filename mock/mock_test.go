package mock_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_Stream(t *testing.T) {
	t.Parallel()
	t.Run("delegates to StreamFn", func(t *testing.T) {
		t.Parallel()
		var s mock.Stream
		p := mock.Provider{
			StreamFn: func(ctx context.Context, req pipe.Request) (pipe.Stream, error) {
				return &s, nil
			},
		}
		got, err := p.Stream(context.Background(), pipe.Request{})
		require.NoError(t, err)
		assert.Equal(t, &s, got)
	})

	t.Run("returns error", func(t *testing.T) {
		t.Parallel()
		wantErr := errors.New("api error")
		p := mock.Provider{
			StreamFn: func(ctx context.Context, req pipe.Request) (pipe.Stream, error) {
				return nil, wantErr
			},
		}
		_, err := p.Stream(context.Background(), pipe.Request{})
		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("panics when StreamFn not set", func(t *testing.T) {
		t.Parallel()
		p := mock.Provider{}
		assert.Panics(t, func() {
			_, _ = p.Stream(context.Background(), pipe.Request{})
		})
	})
}

func TestStream_Next(t *testing.T) {
	t.Parallel()
	t.Run("delegates to NextFn", func(t *testing.T) {
		t.Parallel()
		want := pipe.EventTextDelta{Index: 0, Delta: "hello"}
		s := mock.Stream{
			NextFn: func() (pipe.Event, error) {
				return want, nil
			},
		}
		got, err := s.Next()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns EOF", func(t *testing.T) {
		t.Parallel()
		s := mock.Stream{
			NextFn: func() (pipe.Event, error) {
				return nil, io.EOF
			},
		}
		_, err := s.Next()
		assert.ErrorIs(t, err, io.EOF)
	})
}

func TestStream_State(t *testing.T) {
	t.Parallel()
	t.Run("delegates to StateFn", func(t *testing.T) {
		t.Parallel()
		s := mock.Stream{
			StateFn: func() pipe.StreamState {
				return pipe.StreamStateComplete
			},
		}
		assert.Equal(t, pipe.StreamStateComplete, s.State())
	})
}

func TestStream_Message(t *testing.T) {
	t.Parallel()
	t.Run("delegates to MessageFn", func(t *testing.T) {
		t.Parallel()
		want := pipe.AssistantMessage{
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "hello"}},
			StopReason: pipe.StopEndTurn,
		}
		s := mock.Stream{
			MessageFn: func() (pipe.AssistantMessage, error) {
				return want, nil
			},
		}
		got, err := s.Message()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestStream_Close(t *testing.T) {
	t.Parallel()
	t.Run("delegates to CloseFn", func(t *testing.T) {
		t.Parallel()
		called := false
		s := mock.Stream{
			CloseFn: func() error {
				called = true
				return nil
			},
		}
		err := s.Close()
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("returns error", func(t *testing.T) {
		t.Parallel()
		wantErr := errors.New("close error")
		s := mock.Stream{
			CloseFn: func() error {
				return wantErr
			},
		}
		err := s.Close()
		assert.ErrorIs(t, err, wantErr)
	})
}

func TestToolExecutor_Execute(t *testing.T) {
	t.Parallel()
	t.Run("delegates to ExecuteFn", func(t *testing.T) {
		t.Parallel()
		want := &pipe.ToolResult{
			Content: []pipe.ContentBlock{pipe.TextBlock{Text: "result"}},
		}
		e := mock.ToolExecutor{
			ExecuteFn: func(ctx context.Context, name string, args json.RawMessage) (*pipe.ToolResult, error) {
				assert.Equal(t, "read", name)
				assert.JSONEq(t, `{"path":"foo.go"}`, string(args))
				return want, nil
			},
		}
		got, err := e.Execute(context.Background(), "read", json.RawMessage(`{"path":"foo.go"}`))
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns error", func(t *testing.T) {
		t.Parallel()
		wantErr := errors.New("exec error")
		e := mock.ToolExecutor{
			ExecuteFn: func(ctx context.Context, name string, args json.RawMessage) (*pipe.ToolResult, error) {
				return nil, wantErr
			},
		}
		_, err := e.Execute(context.Background(), "read", nil)
		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("panics when ExecuteFn not set", func(t *testing.T) {
		t.Parallel()
		e := mock.ToolExecutor{}
		assert.Panics(t, func() {
			_, _ = e.Execute(context.Background(), "read", nil)
		})
	})
}
