package mock_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
