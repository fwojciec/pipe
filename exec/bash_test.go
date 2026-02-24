package exec_test

import (
	"encoding/json"
	"testing"

	"github.com/fwojciec/pipe"
	"github.com/stretchr/testify/require"
)

// mustJSON marshals v to json.RawMessage, failing the test on error.
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// resultText extracts the text from the first content block of a tool result.
func resultText(t *testing.T, r *pipe.ToolResult) string {
	t.Helper()
	require.NotEmpty(t, r.Content)
	text, ok := r.Content[0].(pipe.TextBlock)
	require.True(t, ok)
	return text.Text
}
