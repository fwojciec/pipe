package mock_test

import (
	"context"
	"errors"
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
