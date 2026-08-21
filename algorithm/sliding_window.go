package algorithm

import (
	"math"
	"sync"
	"time"
)

// SlidingWindowState holds the two fixed-window counters used by the
// sliding-window counter algorithm (weighted estimate of previous + current).
type SlidingWindowState struct {
	mu sync.Mutex

	prevCount int64
	currCount int64

	// windowStart is the beginning of the current fixed window.
	windowStart time.Time
	expiry      time.Time
}

type SlidingWindowConfig struct {
	Limit  int
	Window time.Duration
}

func (cfg *SlidingWindowConfig) Evaluate(state *SlidingWindowState, cost int, now time.Time) Result {
	state.mu.Lock()
	defer state.mu.Unlock()

	cfg.roll(state, now)
	state.expiry = now.Add(cfg.Window * 2)

	fLimit := float64(cfg.Limit)
	elapsed := now.Sub(state.windowStart).Seconds()
	weight := 1 - elapsed/cfg.Window.Seconds()
	if weight < 0 {
		weight = 0
	}
	used := float64(state.currCount) + float64(state.prevCount)*weight

	if used+float64(cost) <= fLimit {
		state.currCount += int64(cost)
		return Result{
			Allowed:   true,
			Remaining: cfg.Limit - int(math.Ceil(used)) - cost,
		}
	}

	// Estimate when enough of the previous window ages out for this request to
	// pass. Conservative: report at least 1 second, at most one window.
	retryAfter := cfg.Window
	if state.prevCount > 0 {
		need := used + float64(cost) - fLimit // excess to shed
		shedPerSec := float64(state.prevCount) / cfg.Window.Seconds()
		if shedPerSec > 0 {
			retryAfter = time.Duration(math.Ceil(need/shedPerSec)) * time.Second
		}
	}
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	if retryAfter > cfg.Window {
		retryAfter = cfg.Window
	}

	return Result{
		Allowed:    false,
		Remaining:  0,
		RetryAfter: retryAfter,
	}
}

// roll advances the fixed-window counters if `now` has crossed into a new
// window. Must be called with state.mu held.
func (cfg *SlidingWindowConfig) roll(state *SlidingWindowState, now time.Time) {
	if state.windowStart.IsZero() {
		state.windowStart = now.Truncate(cfg.Window)
		return
	}

	elapsedWindows := int64(now.Sub(state.windowStart) / cfg.Window)
	if elapsedWindows <= 0 {
		return
	}
	if elapsedWindows == 1 {
		state.prevCount = state.currCount
	} else {
		// More than a full window has passed: both counters are stale.
		state.prevCount = 0
	}
	state.currCount = 0
	state.windowStart = state.windowStart.Add(time.Duration(elapsedWindows) * cfg.Window)
}

func (s *SlidingWindowState) IsExpired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.expiry.IsZero() && now.After(s.expiry)
}
