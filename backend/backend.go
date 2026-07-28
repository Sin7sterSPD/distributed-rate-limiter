package backend

import (
	"context"
	"time"
)

type BackendResult struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

type Backend interface {
	GetAndUpdate(ctx context.Context, key string, cost int) (*BackendResult, error)

	Close() error
}
