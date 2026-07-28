package algorithm

import (
	"math"
	"sync"
	"time"
)

type TokenBucketState struct {
	mu         sync.Mutex
	tokens     float64
	lastUpdate time.Time
	expiry     time.Time
}

type TokenBucketConfig struct {
	Capacity float64

	RefillRate float64
}

func (cfg *TokenBucketConfig) Evaluate(state *TokenBucketState, cost int, now time.Time, window time.Duration) Result {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.lastUpdate.IsZero() {
		state.tokens = cfg.Capacity
		state.lastUpdate = now
	}

	elapsed := now.Sub(state.lastUpdate).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	refilled := elapsed * cfg.RefillRate
	state.tokens = math.Min(cfg.Capacity, state.tokens+refilled)
	state.lastUpdate = now
	state.expiry = now.Add(window * 2)

	fCost := float64(cost)
	if state.tokens >= fCost {
		state.tokens -= fCost
		return Result{
			Allowed:   true,
			Remaining: int(math.Floor(state.tokens)),
		}
	}

	deficit := fCost - state.tokens
	waitSeconds := deficit / cfg.RefillRate
	return Result{
		Allowed:    false,
		Remaining:  0,
		RetryAfter: time.Duration(math.Ceil(waitSeconds)) * time.Second,
	}
}

func (s *TokenBucketState) IsExpired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.expiry.IsZero() && now.After(s.expiry)
}
