package gemini_test

import (
	"encoding/json"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/fwojciec/pipe/gemini"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertMessages_UserMessage(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.UserMessage{Content: []pipe.ContentBlock{pipe.TextBlock{Text: "Hello"}}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	assert.Equal(t, "user", got[0].Role)
	require.Len(t, got[0].Parts, 1)
	assert.Equal(t, "Hello", got[0].Parts[0].Text)
}

func TestConvertMessages_AssistantMessage(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.TextBlock{Text: "Let me help."},
		}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	assert.Equal(t, "model", got[0].Role)
	require.Len(t, got[0].Parts, 1)
	assert.Equal(t, "Let me help.", got[0].Parts[0].Text)
}

func TestConvertMessages_ThinkingWithSignature(t *testing.T) {
	t.Parallel()
	sig := []byte("thought-sig-data")
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "reasoning", Signature: sig},
			pipe.TextBlock{Text: "Answer"},
		}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 2)
	assert.Equal(t, "reasoning", got[0].Parts[0].Text)
	assert.True(t, got[0].Parts[0].Thought)
	assert.Equal(t, []byte("thought-sig-data"), got[0].Parts[0].ThoughtSignature)
	assert.Equal(t, "Answer", got[0].Parts[1].Text)
}

func TestConvertMessages_ToolCallAndResult(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "call_123", Name: "read", Arguments: json.RawMessage(`{"path":"foo.go"}`)},
		}},
		pipe.ToolResultMessage{
			ToolCallID: "call_123",
			ToolName:   "read",
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "file contents"}},
		},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 2)

	// Assistant with tool call — ID passed through.
	assert.Equal(t, "model", got[0].Role)
	require.Len(t, got[0].Parts, 1)
	require.NotNil(t, got[0].Parts[0].FunctionCall)
	assert.Equal(t, "call_123", got[0].Parts[0].FunctionCall.ID)
	assert.Equal(t, "read", got[0].Parts[0].FunctionCall.Name)
	assert.Equal(t, "foo.go", got[0].Parts[0].FunctionCall.Args["path"])

	// Tool result — ID correlates, output in "output" key.
	assert.Equal(t, "user", got[1].Role)
	require.Len(t, got[1].Parts, 1)
	require.NotNil(t, got[1].Parts[0].FunctionResponse)
	assert.Equal(t, "call_123", got[1].Parts[0].FunctionResponse.ID)
	assert.Equal(t, "read", got[1].Parts[0].FunctionResponse.Name)
	assert.Equal(t, "file contents", got[1].Parts[0].FunctionResponse.Response["output"])
}

func TestConvertMessages_ToolResultError(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "call_err", Name: "bash", Arguments: json.RawMessage(`{"cmd":"ls"}`)},
		}},
		pipe.ToolResultMessage{
			ToolCallID: "call_err",
			ToolName:   "bash",
			Content:    []pipe.ContentBlock{pipe.TextBlock{Text: "permission denied"}},
			IsError:    true,
		},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 2)

	// Error result — uses "error" key.
	resp := got[1].Parts[0].FunctionResponse
	assert.Equal(t, "call_err", resp.ID)
	assert.Equal(t, "permission denied", resp.Response["error"])
	assert.Nil(t, resp.Response["output"])
}

func TestConvertMessages_ImageBlock(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.UserMessage{Content: []pipe.ContentBlock{
			pipe.ImageBlock{Data: []byte("PNG"), MimeType: "image/png"},
		}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 1)
	require.NotNil(t, got[0].Parts[0].InlineData)
	assert.Equal(t, "image/png", got[0].Parts[0].InlineData.MIMEType)
	assert.Equal(t, []byte("PNG"), got[0].Parts[0].InlineData.Data)
}

func TestConvertTools(t *testing.T) {
	t.Parallel()
	tools := []pipe.Tool{
		{Name: "read", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)},
		{Name: "bash", Description: "Run a command", Parameters: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`)},
	}
	got := gemini.ConvertTools(tools)
	require.Len(t, got, 1) // single genai.Tool with multiple declarations
	require.Len(t, got[0].FunctionDeclarations, 2)
	assert.Equal(t, "read", got[0].FunctionDeclarations[0].Name)
	assert.Equal(t, "Read a file", got[0].FunctionDeclarations[0].Description)
	assert.Equal(t, "bash", got[0].FunctionDeclarations[1].Name)
}

func TestConvertTools_Empty(t *testing.T) {
	t.Parallel()
	got := gemini.ConvertTools(nil)
	assert.Nil(t, got)
}

func TestConvertMessages_ThinkingNoSignature(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "just thinking"},
			pipe.TextBlock{Text: "Answer"},
		}},
	}
	got := gemini.ConvertMessages(msgs)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 2)
	assert.True(t, got[0].Parts[0].Thought)
	assert.Nil(t, got[0].Parts[0].ThoughtSignature)
}
