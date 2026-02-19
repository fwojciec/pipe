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

// Execute dispatches a tool call by name. Unknown tool names return an IsError
// result so the model can self-correct.
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
	case "grep":
		return ExecuteGrep(ctx, args)
	case "glob":
		return ExecuteGlob(ctx, args)
	default:
		return &pipe.ToolResult{
			Content: []pipe.ContentBlock{pipe.TextBlock{Text: fmt.Sprintf("unknown tool: %s", name)}},
			IsError: true,
		}, nil
	}
}

// Tools returns the tool definitions for all built-in tools.
func (e *Executor) Tools() []pipe.Tool {
	return []pipe.Tool{
		BashTool(),
		ReadTool(),
		WriteTool(),
		EditTool(),
		GrepTool(),
		GlobTool(),
	}
}
