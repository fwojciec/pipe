package builtin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fwojciec/pipe"
)

type grepArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Glob    string `json:"glob"`
}

// GrepTool returns the tool definition for the grep tool.
func GrepTool() pipe.Tool {
	return pipe.Tool{
		Name:        "grep",
		Description: "Search file contents with a regular expression. Returns matching lines with file:line:content context.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {
					"type": "string",
					"description": "Regular expression pattern to search for"
				},
				"path": {
					"type": "string",
					"description": "File or directory to search in"
				},
				"glob": {
					"type": "string",
					"description": "Glob pattern to filter files (e.g. *.go)"
				}
			},
			"required": ["pattern", "path"]
		}`),
	}
}

// ExecuteGrep searches file contents and returns matching lines.
func ExecuteGrep(_ context.Context, args json.RawMessage) (*pipe.ToolResult, error) {
	var a grepArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return domainError(fmt.Sprintf("invalid arguments: %s", err)), nil
	}

	if a.Pattern == "" {
		return domainError("pattern is required"), nil
	}

	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return domainError(fmt.Sprintf("invalid regex pattern: %s", err)), nil
	}

	info, err := os.Stat(a.Path)
	if err != nil {
		return domainError(fmt.Sprintf("failed to access path: %s", err)), nil
	}

	var b strings.Builder

	if !info.IsDir() {
		grepFile(&b, a.Path, filepath.Dir(a.Path), re)
	} else {
		err = filepath.WalkDir(a.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if a.Glob != "" {
				rel, relErr := filepath.Rel(a.Path, path)
				if relErr != nil {
					return nil
				}
				matched, matchErr := doublestar.Match(a.Glob, filepath.ToSlash(rel))
				if matchErr != nil || !matched {
					return nil
				}
			}
			grepFile(&b, path, a.Path, re)
			return nil
		})
		if err != nil {
			return domainError(fmt.Sprintf("error walking directory: %s", err)), nil
		}
	}

	if b.Len() == 0 {
		return textResult("no matches found"), nil
	}

	return textResult(b.String()), nil
}

func grepFile(b *strings.Builder, path string, basePath string, re *regexp.Regexp) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	// Read first 512 bytes to detect binary files.
	header := make([]byte, 512)
	n, _ := f.Read(header)
	if n == 0 {
		return
	}
	if bytes.ContainsRune(header[:n], 0) {
		return
	}

	// Seek back to beginning.
	if _, err := f.Seek(0, 0); err != nil {
		return
	}

	relPath, err := filepath.Rel(basePath, path)
	if err != nil {
		relPath = path
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			fmt.Fprintf(b, "%s:%d:%s\n", relPath, lineNum, line)
		}
	}
	// scanner.Err() intentionally unchecked — partial results are acceptable
	// for grep, matching standard grep behavior on oversized lines.
}
