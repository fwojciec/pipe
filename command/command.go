// Package command provides the bash command execution tool.
package command

import "github.com/fwojciec/pipe"

func domainError(msg string) *pipe.ToolResult {
	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: msg}},
		IsError: true,
	}
}
