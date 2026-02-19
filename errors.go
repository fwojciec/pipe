package pipe

import "errors"

// Sentinel errors for common failure modes.
var (
	// ErrValidation indicates a request or message failed validation.
	ErrValidation = errors.New("validation error")

	// ErrStreamNotReady indicates Message() was called before Next().
	ErrStreamNotReady = errors.New("stream not ready: call Next() first")

	// ErrStreamClosed indicates an operation on a closed stream.
	ErrStreamClosed = errors.New("stream closed")

	// ErrToolNotFound indicates the requested tool does not exist.
	ErrToolNotFound = errors.New("tool not found")
)
