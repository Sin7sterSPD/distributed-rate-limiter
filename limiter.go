package distributedratelimiter

import (
	"context"
	"net/http"
	"time"
)

type Limiter interface {
	Allow(ctx context.Context, key string) (*Result, error)

	AllowN(ctx context.Context, key string, n int) (*Result, error)

	Close() error
}


type Result struct {

	Allowed bool


	Remaining int


	RetryAfter time.Duration

	Limit int

	Window time.Duration

	Algorithm Algorithm
}

type KeyFunc func(r *http.Request) string
