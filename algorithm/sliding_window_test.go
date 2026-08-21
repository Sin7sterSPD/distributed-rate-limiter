package algorithm

import (
	"sync"
	"testing"
	"time"
)

func TestSlidingWindowFirstUseStartsEmpty(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 5, Window: time.Minute}
	state := &SlidingWindowState{}

	res := cfg.Evaluate(state, 1, base)

	if !res.Allowed {
		t.Fatalf("expected allowed: %+v", res)
	}
	if res.Remaining != 4 {
		t.Errorf("expected remaining 4, got %d", res.Remaining)
	}
	if !state.windowStart.Equal(base.Truncate(time.Minute)) {
		t.Errorf("expected windowStart %v, got %v", base.Truncate(time.Minute), state.windowStart)
	}
}

func TestSlidingWindowAllowsUpToLimit(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 3, Window: time.Minute}
	state := &SlidingWindowState{}

	for i := 0; i < 3; i++ {
		res := cfg.Evaluate(state, 1, base)
		if !res.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	res := cfg.Evaluate(state, 1, base)
	if res.Allowed {
		t.Fatal("4th request should be rejected")
	}
}

func TestSlidingWindowExactBoundaryAllowed(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 5, Window: time.Minute}
	state := &SlidingWindowState{}

	cfg.Evaluate(state, 2, base)

	// used=2, cost=3 => exactly at limit; <= means allowed.
	res := cfg.Evaluate(state, 3, base)
	if !res.Allowed {
		t.Fatalf("exact-boundary request should be allowed: %+v", res)
	}
}

func TestSlidingWindowWeightedPreviousWindow(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 10, Window: time.Minute}
	state := &SlidingWindowState{}

	// Fill current window with 8 requests.
	for i := 0; i < 8; i++ {
		cfg.Evaluate(state, 1, base)
	}

	// Roll into the next window: prev=8, curr=0, start=10:01.
	next := base.Add(time.Minute)
	at15s := next.Add(15 * time.Second) // 25% into the window

	// weight = 1 - 15/60 = 0.75 => used = 0 + 8*0.75 = 6.
	// cost 4 => used+cost = 10 <= limit => allowed.
	res := cfg.Evaluate(state, 4, at15s)
	if !res.Allowed {
		t.Fatalf("weighted estimate (6)+4 should be allowed: %+v", res)
	}

	// Now curr=4: used = 4 + 6 = 10. Any further cost must be rejected.
	res = cfg.Evaluate(state, 1, at15s)
	if res.Allowed {
		t.Fatal("weighted estimate (10)+1 should be rejected")
	}
	if res.RetryAfter < time.Second || res.RetryAfter > time.Minute {
		t.Errorf("retryAfter %v outside [1s, window]", res.RetryAfter)
	}
}

func TestSlidingWindowRollAfterOneWindow(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 2, Window: time.Minute}
	state := &SlidingWindowState{}

	cfg.Evaluate(state, 2, base) // curr=2

	// One full window later: prev=2, curr=0. At the exact boundary the
	// previous window still fully overlaps the sliding window (weight=1),
	// so a request is still rejected; once ~31s in, weight decays enough.
	next := base.Add(time.Minute)
	if res := cfg.Evaluate(state, 1, next); res.Allowed {
		t.Fatal("at exact boundary prev fully overlaps: should be rejected")
	}

	res := cfg.Evaluate(state, 1, next.Add(31*time.Second))
	if !res.Allowed {
		t.Fatalf("weight decay should allow request: %+v", res)
	}

	state.mu.Lock()
	prev, curr := state.prevCount, state.currCount
	state.mu.Unlock()
	if prev != 2 || curr != 1 {
		t.Errorf("after roll expected prev=2 curr=1, got prev=%d curr=%d", prev, curr)
	}
}

func TestSlidingWindowRollAfterMultipleWindowsResetsPrev(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 2, Window: time.Minute}
	state := &SlidingWindowState{}

	cfg.Evaluate(state, 2, base)

	// Two full windows later: both counters stale => prev=0.
	muchLater := base.Add(2 * time.Minute)
	res := cfg.Evaluate(state, 2, muchLater)
	if !res.Allowed {
		t.Fatalf("stale window should allow fresh usage: %+v", res)
	}

	state.mu.Lock()
	prev := state.prevCount
	state.mu.Unlock()
	if prev != 0 {
		t.Errorf("prev should reset to 0 after >=2 windows, got %d", prev)
	}
}

func TestSlidingWindowRetryAfterClamped(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 1, Window: 200 * time.Millisecond}
	state := &SlidingWindowState{}

	cfg.Evaluate(state, 1, base)

	res := cfg.Evaluate(state, 1, base.Add(50*time.Millisecond))
	if res.Allowed {
		t.Fatal("expected rejection")
	}
	// Clamps: at least 1s... but window is 200ms, so capped to window.
	if res.RetryAfter != 200*time.Millisecond {
		t.Errorf("retryAfter should be clamped to window (200ms), got %v", res.RetryAfter)
	}
}

func TestSlidingWindowRetryAfterClampedToSmallWindow(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 5, Window: 200 * time.Millisecond}
	state := &SlidingWindowState{}

	// prev=0 path: retryAfter starts at Window (200ms), the 1s floor raises
	// it, then the window cap lowers it back => exactly Window.
	cfg.Evaluate(state, 4, base)

	res := cfg.Evaluate(state, 2, base.Add(50*time.Millisecond))
	if res.Allowed {
		t.Fatal("expected rejection")
	}
	if res.RetryAfter != 200*time.Millisecond {
		t.Errorf("retryAfter should be clamped to window (200ms), got %v", res.RetryAfter)
	}
}

func TestSlidingWindowExpirySetForSweeper(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 5, Window: time.Minute}
	state := &SlidingWindowState{}

	cfg.Evaluate(state, 1, base)

	if state.IsExpired(base.Add(time.Minute)) {
		t.Error("state must not be expired before 2x window")
	}
	if !state.IsExpired(base.Add(2*time.Minute + time.Second)) {
		t.Error("state must be expired after 2x window")
	}
}

func TestSlidingWindowConcurrentEvaluateIsRaceFree(t *testing.T) {
	cfg := &SlidingWindowConfig{Limit: 1000, Window: time.Minute}
	state := &SlidingWindowState{}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg.Evaluate(state, 1, base)
		}()
	}
	wg.Wait()

	state.mu.Lock()
	curr := state.currCount
	state.mu.Unlock()
	if curr != 50 {
		t.Errorf("expected exactly 50 counted requests, got %d", curr)
	}
}
