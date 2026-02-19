package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fwojciec/pipe"
)

type editArgs struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// EditTool returns the tool definition for the edit tool.
func EditTool() pipe.Tool {
	return pipe.Tool{
		Name:        "edit",
		Description: "Replace a string in a file. Fails if old_string is not unique unless replace_all is true.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"file_path": {
					"type": "string",
					"description": "The path to the file to edit"
				},
				"old_string": {
					"type": "string",
					"description": "The exact string to find and replace"
				},
				"new_string": {
					"type": "string",
					"description": "The replacement string"
				},
				"replace_all": {
					"type": "boolean",
					"description": "Replace all occurrences instead of requiring a unique match"
				}
			},
			"required": ["file_path", "old_string", "new_string"]
		}`),
	}
}

// ExecuteEdit performs a string replacement in a file and returns the result.
func ExecuteEdit(_ context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a editArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	if a.FilePath == "" {
		return domainError("file_path is required"), nil
	}

	info, err := os.Stat(a.FilePath)
	if err != nil {
		return domainError(fmt.Sprintf("failed to stat file: %s", err)), nil
	}

	data, err := os.ReadFile(a.FilePath)
	if err != nil {
		return domainError(fmt.Sprintf("failed to read file: %s", err)), nil
	}

	content := string(data)
	count := strings.Count(content, a.OldString)

	if count == 0 {
		return domainError(fmt.Sprintf("old_string not found in %s", a.FilePath)), nil
	}

	if count > 1 && !a.ReplaceAll {
		return domainError(fmt.Sprintf("old_string found %d times in %s; use replace_all to replace all occurrences", count, a.FilePath)), nil
	}

	var newContent string
	if a.ReplaceAll {
		newContent = strings.ReplaceAll(content, a.OldString, a.NewString)
	} else {
		newContent = strings.Replace(content, a.OldString, a.NewString, 1)
	}

	if err := os.WriteFile(a.FilePath, []byte(newContent), info.Mode().Perm()); err != nil {
		return domainError(fmt.Sprintf("failed to write file: %s", err)), nil
	}

	replacements := count
	if !a.ReplaceAll {
		replacements = 1
	}

	return textResult(fmt.Sprintf("replaced %d occurrence(s) in %s", replacements, a.FilePath)), nil
}
