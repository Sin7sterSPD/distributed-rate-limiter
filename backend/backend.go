package backend

import (
	"context"
	"errors"
	"time"
)

var ErrBackendUnavailable = errors.New("backend unavailable")

type BackendResult struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

type Backend interface {
	GetAndUpdate(ctx context.Context, key string, cost int) (*BackendResult, error)

	Close() error
}
