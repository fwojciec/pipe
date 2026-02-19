package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fwojciec/pipe"
)

type writeArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// WriteTool returns the tool definition for the write tool.
func WriteTool() pipe.Tool {
	return pipe.Tool{
		Name:        "write",
		Description: "Write content to a file, creating it if it doesn't exist or overwriting if it does.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"file_path": {
					"type": "string",
					"description": "The path to the file to write"
				},
				"content": {
					"type": "string",
					"description": "The content to write to the file"
				}
			},
			"required": ["file_path", "content"]
		}`),
	}
}

// ExecuteWrite writes content to a file and returns the result.
func ExecuteWrite(_ context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a writeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	if a.FilePath == "" {
		return domainError("file_path is required"), nil
	}

	if err := os.MkdirAll(filepath.Dir(a.FilePath), 0o755); err != nil {
		return domainError(fmt.Sprintf("failed to create directories: %s", err)), nil
	}

	perm := os.FileMode(0o644)
	if info, err := os.Stat(a.FilePath); err == nil {
		perm = info.Mode().Perm()
	}

	if err := os.WriteFile(a.FilePath, []byte(a.Content), perm); err != nil {
		return domainError(fmt.Sprintf("failed to write file: %s", err)), nil
	}

	return textResult(fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.FilePath)), nil
}
