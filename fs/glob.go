package fs

import (
	"context"
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fwojciec/pipe"
)

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

// GlobTool returns the tool definition for the glob tool.
func GlobTool() pipe.Tool {
	return pipe.Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Supports ** for recursive matching.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {
					"type": "string",
					"description": "Glob pattern to match files (e.g. **/*.go)"
				},
				"path": {
					"type": "string",
					"description": "Base directory to search from"
				}
			},
			"required": ["pattern", "path"]
		}`),
	}
}

// ExecuteGlob finds files matching a glob pattern and returns their paths.
func ExecuteGlob(_ context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a globArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	if a.Pattern == "" {
		return domainError("pattern is required"), nil
	}

	if !doublestar.ValidatePattern(a.Pattern) {
		return domainError(fmt.Sprintf("invalid glob pattern: %s", a.Pattern)), nil
	}

	info, err := os.Stat(a.Path)
	if err != nil {
		return domainError(fmt.Sprintf("failed to access path: %s", err)), nil
	}
	if !info.IsDir() {
		return domainError("path must be a directory"), nil
	}

	fsys := os.DirFS(a.Path)
	var matches []string

	err = doublestar.GlobWalk(fsys, a.Pattern, func(path string, d iofs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		matches = append(matches, filepath.FromSlash(path))
		return nil
	})
	if err != nil {
		return domainError(fmt.Sprintf("error matching pattern: %s", err)), nil
	}

	if len(matches) == 0 {
		return textResult("no matches found"), nil
	}

	return textResult(strings.Join(matches, "\n")), nil
}
