package mock_test

import (
	"errors"
	"io"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	t.Run("panics when NextFn not set", func(t *testing.T) {
		t.Parallel()
		s := mock.Stream{}
		assert.Panics(t, func() {
			_, _ = s.Next()
		})
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

	t.Run("returns StreamStateNew when StateFn not set", func(t *testing.T) {
		t.Parallel()
		s := mock.Stream{}
		assert.Equal(t, pipe.StreamStateNew, s.State())
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

	t.Run("panics when MessageFn not set", func(t *testing.T) {
		t.Parallel()
		s := mock.Stream{}
		assert.Panics(t, func() {
			_, _ = s.Message()
		})
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

	t.Run("returns nil when CloseFn not set", func(t *testing.T) {
		t.Parallel()
		s := mock.Stream{}
		assert.NoError(t, s.Close())
	})
}
