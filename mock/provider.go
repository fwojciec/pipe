package mock

import (
	"context"

	"github.com/fwojciec/pipe"
)

// Interface compliance check.
var _ pipe.Provider = (*Provider)(nil)

// Provider is a test double for pipe.Provider.
// Set StreamFn before calling Stream.
type Provider struct {
	StreamFn func(ctx context.Context, req pipe.Request) (pipe.Stream, error)
}

// Stream delegates to StreamFn.
func (p *Provider) Stream(ctx context.Context, req pipe.Request) (pipe.Stream, error) {
	return p.StreamFn(ctx, req)
}
