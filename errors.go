package distributedratelimiter

import "errors"

var (
	ErrLimitExceeded = errors.New("rate limit exceeded")

	// use this when redis is unrechable orr backiend is not configured
	ErrBackendUnavailable = errors.New("backend unavailable")

	// ErrInvalidConfig is returned when the provided Config fails validation.
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrContextCancelled is returned when the request context is cancelled
	// before the backend responds.
	ErrContextCancelled = errors.New("context cancelled before backend response")
)
