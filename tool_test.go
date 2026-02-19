package pipe_test

import (
	"encoding/json"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/assert"
)

func TestTool_Fields(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type": "object", "properties": {"path": {"type": "string"}}}`)
	tool := pipe.Tool{
		Name:        "read",
		Description: "Read a file",
		Parameters:  schema,
	}
	assert.Equal(t, "read", tool.Name)
	assert.Equal(t, "Read a file", tool.Description)
	assert.JSONEq(t, `{"type": "object", "properties": {"path": {"type": "string"}}}`, string(tool.Parameters))
}

func TestToolResult_Fields(t *testing.T) {
	t.Parallel()
	result := pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: "file contents"}},
		IsError: false,
	}
	assert.Len(t, result.Content, 1)
	assert.False(t, result.IsError)
}

func TestToolResult_Error(t *testing.T) {
	t.Parallel()
	result := pipe.ToolResult{
		Content: []pipe.ContentBlock{pipe.TextBlock{Text: "file not found"}},
		IsError: true,
	}
	assert.True(t, result.IsError)
}
