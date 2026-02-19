package mock

import (
	"context"
	"encoding/json"

	"github.com/fwojciec/pipe"
)

// Interface compliance check.
var _ pipe.ToolExecutor = (*ToolExecutor)(nil)

// ToolExecutor is a test double for pipe.ToolExecutor.
// Set ExecuteFn before calling Execute.
type ToolExecutor struct {
	ExecuteFn func(ctx context.Context, name string, args json.RawMessage) (*pipe.ToolResult, error)
}

// Execute delegates to ExecuteFn.
func (e *ToolExecutor) Execute(ctx context.Context, name string, args json.RawMessage) (*pipe.ToolResult, error) {
	return e.ExecuteFn(ctx, name, args)
}
