package pipe

import "fmt"

// Request carries model selection and generation parameters.
// The provider uses its own defaults when fields are zero/nil.
type Request struct {
	Model        string // model ID, provider-specific; empty = provider default
	SystemPrompt string
	Messages     []Message
	Tools        []Tool
	MaxTokens    int      // 0 = provider default
	Temperature  *float64 // nil = provider default
}

// Validate checks universal constraints on Request.
// Provider implementations may apply additional provider-specific validation.
func (r Request) Validate() error {
	if r.Temperature != nil {
		if *r.Temperature < 0 || *r.Temperature > 2 {
			return fmt.Errorf("temperature must be in [0, 2], got %g: %w", *r.Temperature, ErrValidation)
		}
	}
	if r.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be non-negative, got %d: %w", r.MaxTokens, ErrValidation)
	}
	return nil
}
