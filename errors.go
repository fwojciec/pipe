package pipe

import "errors"

// Sentinel errors for common failure modes.
var (
	// ErrValidation indicates a request or message failed validation.
	ErrValidation = errors.New("validation error")
)
