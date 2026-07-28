package algorithm

import (
	"math"
	"sync"
	"time"
)

type SlidingWindowState struct {
	mu       sync.Mutex
	requests []time.Time // sorted ascending; oldest first
	expiry   time.Time
}

type SlidingWindowConfig struct {
	Limit  int
	Window time.Duration
}

func (cfg *SlidingWindowConfig) Evaluate(state *SlidingWindowState, cost int, now time.Time) Result {
	state.mu.Lock()
	defer state.mu.Unlock()

	windowStart := now.Add(-cfg.Window)

	valid := state.requests[:0]
	for _, t := range state.requests {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}
	state.requests = valid
	state.expiry = now.Add(cfg.Window * 2)

	current := len(state.requests)

	if current+cost <= cfg.Limit {

		for i := 0; i < cost; i++ {
			state.requests = append(state.requests, now)
		}
		return Result{
			Allowed:   true,
			Remaining: cfg.Limit - current - cost,
		}
	}

	var retryAfter time.Duration
	if len(state.requests) > 0 {
		oldestExpiry := state.requests[0].Add(cfg.Window)
		waitSecs := math.Ceil(oldestExpiry.Sub(now).Seconds())
		if waitSecs < 1 {
			waitSecs = 1
		}
		retryAfter = time.Duration(waitSecs) * time.Second
	} else {
		retryAfter = time.Second
	}

	return Result{
		Allowed:    false,
		Remaining:  0,
		RetryAfter: retryAfter,
	}
}

func (s *SlidingWindowState) IsExpired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.expiry.IsZero() && now.After(s.expiry)
}
