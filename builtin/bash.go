package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/fwojciec/pipe"
)

type bashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // milliseconds
}

// BashTool returns the tool definition for the bash tool.
func BashTool() pipe.Tool {
	return pipe.Tool{
		Name:        "bash",
		Description: "Execute a bash command and return the output.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "The bash command to execute"
				},
				"timeout": {
					"type": "integer",
					"description": "Timeout in milliseconds (default: 120000)"
				}
			},
			"required": ["command"]
		}`),
	}
}

// ExecuteBash executes a bash command and returns the result.
func ExecuteBash(ctx context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	if a.Command == "" {
		return domainError("command is required"), nil
	}

	timeout := 120 * time.Second
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", a.Command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()

	if ctx.Err() != nil {
		return domainError(fmt.Sprintf("command timed out or cancelled: %s\n%s", ctx.Err(), output)), nil
	}

	if err != nil {
		return &pipe.ToolResult{
			Content: []pipe.ContentBlock{pipe.TextBlock{Text: output + err.Error()}},
			IsError: true,
		}, nil
	}

	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: output}},
		IsError: false,
	}, nil
}
