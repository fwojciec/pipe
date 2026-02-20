// Package fs provides filesystem tools: read, write, edit, grep, and glob.
package fs

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
