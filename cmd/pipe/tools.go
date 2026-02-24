package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fwojciec/pipe"
	pipeexec "github.com/fwojciec/pipe/exec"
	"github.com/fwojciec/pipe/fs"
)

// Compile-time interface check.
var _ pipe.ToolExecutor = (*executor)(nil)

// executor dispatches tool calls to the appropriate built-in tool implementation.
type executor struct {
	bash *pipeexec.BashExecutor
}

// Execute dispatches a tool call by name. Unknown tool names return an IsError
// result so the model can self-correct.
func (e *executor) Execute(ctx context.Context, name string, args json.RawMessage) (*pipe.ToolResult, error) {
	switch name {
	case "bash":
		return e.bash.Execute(ctx, args)
	case "read":
		return fs.ExecuteRead(ctx, args)
	case "write":
		return fs.ExecuteWrite(ctx, args)
	case "edit":
		return fs.ExecuteEdit(ctx, args)
	case "grep":
		return fs.ExecuteGrep(ctx, args)
	case "glob":
		return fs.ExecuteGlob(ctx, args)
	default:
		return &pipe.ToolResult{
			Content: []pipe.ContentBlock{pipe.TextBlock{Text: fmt.Sprintf("unknown tool: %s", name)}},
			IsError: true,
		}, nil
	}
}

// tools returns the tool definitions for all built-in tools.
func tools() []pipe.Tool {
	return []pipe.Tool{
		pipeexec.BashExecutorTool(),
		fs.ReadTool(),
		fs.WriteTool(),
		fs.EditTool(),
		fs.GrepTool(),
		fs.GlobTool(),
	}
}
