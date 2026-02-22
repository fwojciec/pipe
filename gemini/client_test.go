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
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
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
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
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
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 2)
	assert.Equal(t, "reasoning", got[0].Parts[0].Text)
	assert.True(t, got[0].Parts[0].Thought)
	assert.Equal(t, []byte("thought-sig-data"), got[0].Parts[0].ThoughtSignature)
	assert.Equal(t, "Answer", got[0].Parts[1].Text)
}

func TestConvertMessages_ToolCallWithoutSignatureNoInference(t *testing.T) {
	t.Parallel()
	sig := []byte("thought-sig-data")
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "reasoning", Signature: sig},
			pipe.ToolCallBlock{ID: "call_1", Name: "bash", Arguments: json.RawMessage(`{"cmd":"ls"}`)},
		}},
	}
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 2)
	// Thinking part has signature.
	assert.Equal(t, sig, got[0].Parts[0].ThoughtSignature)
	// Tool call without its own signature gets nil — no inference from thinking.
	require.NotNil(t, got[0].Parts[1].FunctionCall)
	assert.Nil(t, got[0].Parts[1].ThoughtSignature)
}

func TestConvertMessages_ToolCallWithoutThinking(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "call_1", Name: "bash", Arguments: json.RawMessage(`{"cmd":"ls"}`)},
		}},
	}
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 1)
	require.NotNil(t, got[0].Parts[0].FunctionCall)
	assert.Nil(t, got[0].Parts[0].ThoughtSignature)
}

func TestConvertMessages_MultipleToolCallsEachCarryOwnSignature(t *testing.T) {
	t.Parallel()
	sig1 := []byte("sig-1")
	sig2 := []byte("sig-2")
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "reasoning", Signature: []byte("think-sig")},
			pipe.ToolCallBlock{ID: "call_1", Name: "bash", Arguments: json.RawMessage(`{"cmd":"ls"}`), Signature: sig1},
			pipe.ToolCallBlock{ID: "call_2", Name: "read", Arguments: json.RawMessage(`{"path":"foo.go"}`), Signature: sig2},
		}},
	}
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 3)
	assert.Equal(t, sig1, got[0].Parts[1].ThoughtSignature)
	assert.Equal(t, sig2, got[0].Parts[2].ThoughtSignature)
}

func TestConvertMessages_ToolCallSignaturePreferredOverThinking(t *testing.T) {
	t.Parallel()
	thinkSig := []byte("thinking-sig")
	callSig := []byte("call-sig")
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "reasoning", Signature: thinkSig},
			pipe.ToolCallBlock{ID: "call_1", Name: "bash", Arguments: json.RawMessage(`{"cmd":"ls"}`), Signature: callSig},
		}},
	}
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 2)
	// Call-level signature wins over thinking-level.
	assert.Equal(t, callSig, got[0].Parts[1].ThoughtSignature)
}

func TestConvertMessages_ToolCallSignatureWithoutThinking(t *testing.T) {
	t.Parallel()
	callSig := []byte("orphan-sig")
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "call_1", Name: "bash", Arguments: json.RawMessage(`{"cmd":"ls"}`), Signature: callSig},
		}},
	}
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 1)
	assert.Equal(t, callSig, got[0].Parts[0].ThoughtSignature)
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
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
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
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
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
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
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
	got, err := gemini.ConvertTools(tools)
	require.NoError(t, err)
	require.Len(t, got, 1) // single genai.Tool with multiple declarations
	require.Len(t, got[0].FunctionDeclarations, 2)
	assert.Equal(t, "read", got[0].FunctionDeclarations[0].Name)
	assert.Equal(t, "Read a file", got[0].FunctionDeclarations[0].Description)
	assert.Equal(t, "bash", got[0].FunctionDeclarations[1].Name)
}

func TestConvertTools_Empty(t *testing.T) {
	t.Parallel()
	got, err := gemini.ConvertTools(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestConvertMessages_InvalidToolCallJSON(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ToolCallBlock{ID: "call_bad", Name: "read", Arguments: json.RawMessage(`{bad}`)},
		}},
	}
	got, err := gemini.ConvertMessages(msgs)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "assistant message")
	assert.Contains(t, err.Error(), "invalid tool call arguments JSON")
}

func TestConvertTools_InvalidParametersJSON(t *testing.T) {
	t.Parallel()
	tools := []pipe.Tool{
		{Name: "broken", Description: "Bad tool", Parameters: json.RawMessage(`not json`)},
	}
	got, err := gemini.ConvertTools(tools)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), `"broken"`)
	assert.Contains(t, err.Error(), "invalid tool parameters JSON")
}

func TestConvertMessages_ToolResultMultipleTextBlocks(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.ToolResultMessage{
			ToolCallID: "call_multi",
			ToolName:   "bash",
			Content: []pipe.ContentBlock{
				pipe.TextBlock{Text: "line one"},
				pipe.TextBlock{Text: "line two"},
				pipe.TextBlock{Text: "line three"},
			},
		},
	}
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	resp := got[0].Parts[0].FunctionResponse
	assert.Equal(t, "line one\nline two\nline three", resp.Response["output"])
}

func TestConvertMessages_ThinkingNoSignature(t *testing.T) {
	t.Parallel()
	msgs := []pipe.Message{
		pipe.AssistantMessage{Content: []pipe.ContentBlock{
			pipe.ThinkingBlock{Thinking: "just thinking"},
			pipe.TextBlock{Text: "Answer"},
		}},
	}
	got, err := gemini.ConvertMessages(msgs)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Parts, 2)
	assert.True(t, got[0].Parts[0].Thought)
	assert.Nil(t, got[0].Parts[0].ThoughtSignature)
}
