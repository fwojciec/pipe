package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fwojciec/pipe"
)

// Compile-time interface check.
var _ pipe.ToolExecutor = (*Executor)(nil)

// Executor dispatches tool calls to the appropriate built-in tool implementation.
type Executor struct{}

// NewExecutor creates a new Executor.
func NewExecutor() *Executor {
	return &Executor{}
}

// Execute dispatches a tool call by name. Returns an infrastructure error for
// unknown tool names.
func (e *Executor) Execute(ctx context.Context, name string, args json.RawMessage) (*pipe.ToolResult, error) {
	switch name {
	case "bash":
		return ExecuteBash(ctx, args)
	case "read":
		return ExecuteRead(ctx, args)
	case "write":
		return ExecuteWrite(ctx, args)
	case "edit":
		return ExecuteEdit(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// Tools returns the tool definitions for all built-in tools.
func (e *Executor) Tools() []pipe.Tool {
	return []pipe.Tool{
		BashTool(),
		ReadTool(),
		WriteTool(),
		EditTool(),
	}
}
