// Package gemini implements [pipe.Provider] for the Google Gemini API.
//
// It wraps the google.golang.org/genai SDK, translating between pipe's
// domain types and the Gemini API types. Streaming uses the SDK's iter.Seq2
// iterator, wrapped into the pull-based [pipe.Stream] interface.
package gemini

const (
	defaultModel     = "gemini-3.1-pro-preview"
	defaultMaxTokens = 65536
)
