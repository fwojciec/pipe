package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fwojciec/pipe"
)

type readArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"` // 1-based line number to start from
	Limit    int    `json:"limit"`  // number of lines to read
}

// ReadTool returns the tool definition for the read tool.
func ReadTool() pipe.Tool {
	return pipe.Tool{
		Name:        "read",
		Description: "Read the contents of a file, optionally with line offset and limit.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"file_path": {
					"type": "string",
					"description": "The path to the file to read"
				},
				"offset": {
					"type": "integer",
					"description": "Line number to start reading from (1-based)"
				},
				"limit": {
					"type": "integer",
					"description": "Maximum number of lines to read"
				}
			},
			"required": ["file_path"]
		}`),
	}
}

// ExecuteRead reads file contents and returns the result.
func ExecuteRead(_ context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a readArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	if a.FilePath == "" {
		return domainError("file_path is required"), nil
	}

	f, err := os.Open(a.FilePath)
	if err != nil {
		return domainError(fmt.Sprintf("failed to open file: %s", err)), nil
	}
	defer f.Close()

	var b strings.Builder
	scanner := bufio.NewScanner(f)
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++

		if a.Offset > 0 && lineNum < a.Offset {
			continue
		}

		if a.Limit > 0 && linesRead >= a.Limit {
			break
		}

		fmt.Fprintf(&b, "%d\t%s\n", lineNum, scanner.Text())
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return domainError(fmt.Sprintf("error reading file: %s", err)), nil
	}

	return textResult(b.String()), nil
}
