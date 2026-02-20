// Package exec provides the bash command execution tool.
package exec

import "github.com/fwojciec/pipe"

func domainError(msg string) *pipe.ToolResult {
	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: msg}},
		IsError: true,
	}
}
