package pipe

import (
	"context"
	"encoding/json"
)

// Tool is the schema sent to the LLM describing a tool's capabilities.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolExecutor runs tools. Execute returns error for infrastructure failures.
// ToolResult.IsError indicates tool-reported domain failures sent back to the LLM.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args json.RawMessage) (*ToolResult, error)
}

// ToolResult represents the outcome of a tool execution.
type ToolResult struct {
	Content []ContentBlock
	IsError bool
}
