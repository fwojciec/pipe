// Package builtin provides the built-in tools for the pipe agent.
package builtin

import "github.com/fwojciec/pipe"

func domainError(msg string) *pipe.ToolResult {
	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: msg}},
		IsError: true,
	}
}

func textResult(text string) *pipe.ToolResult {
	return &pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: text}},
		IsError: false,
	}
}
