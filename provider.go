package pipe

import "context"

// Provider is a strategy pattern interface for LLM providers.
//
// Stream accepts Request by value so that implementations cannot mutate the
// caller's data (e.g., by appending to Messages or Tools). Note that value
// passing copies slice headers but shares the underlying arrays; providers
// must not modify existing elements of the slices.
type Provider interface {
	Stream(ctx context.Context, req Request) (Stream, error)
}
