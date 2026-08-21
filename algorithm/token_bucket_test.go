package algorithm

import (
	"math"
	"sync"
	"testing"
	"time"
)

var base = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

func TestTokenBucketFirstUseStartsFull(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 10, RefillRate: 1}
	state := &TokenBucketState{}

	res := cfg.Evaluate(state, 4, base, time.Minute)

	if !res.Allowed {
		t.Fatalf("expected allowed, got rejected: %+v", res)
	}
	if res.Remaining != 6 {
		t.Errorf("expected remaining 6, got %d", res.Remaining)
	}
}

func TestTokenBucketRefillsOverTime(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 10, RefillRate: 2} // 2 tokens/sec
	state := &TokenBucketState{}

	if res := cfg.Evaluate(state, 10, base, time.Minute); !res.Allowed {
		t.Fatalf("expected first request allowed: %+v", res)
	}

	// After 1s, 2 tokens refilled. Consuming 1 leaves 1.
	res := cfg.Evaluate(state, 1, base.Add(time.Second), time.Minute)
	if !res.Allowed {
		t.Fatalf("expected allowed after refill: %+v", res)
	}
	if res.Remaining != 1 {
		t.Errorf("expected remaining 1, got %d", res.Remaining)
	}
}

func TestTokenBucketRefillCapsAtCapacity(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 5, RefillRate: 100}
	state := &TokenBucketState{}

	if res := cfg.Evaluate(state, 5, base, time.Minute); !res.Allowed {
		t.Fatalf("expected first request allowed: %+v", res)
	}

	// Far more than capacity would refill; must clamp to 5.
	res := cfg.Evaluate(state, 3, base.Add(time.Hour), time.Minute)
	if !res.Allowed {
		t.Fatalf("expected allowed: %+v", res)
	}
	if res.Remaining != 2 {
		t.Errorf("expected remaining clamped to 2, got %d", res.Remaining)
	}
}

func TestTokenBucketRejectsWhenEmpty(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 10, RefillRate: 1}
	state := &TokenBucketState{}

	if res := cfg.Evaluate(state, 10, base, time.Minute); !res.Allowed {
		t.Fatalf("expected first request allowed: %+v", res)
	}

	res := cfg.Evaluate(state, 1, base, time.Minute)
	if res.Allowed {
		t.Fatal("expected rejection on empty bucket")
	}
	if res.Remaining != 0 {
		t.Errorf("expected remaining 0, got %d", res.Remaining)
	}
}

func TestTokenBucketRetryAfterMath(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 10, RefillRate: 2} // 2 tokens/sec
	state := &TokenBucketState{}

	cfg.Evaluate(state, 9, base, time.Minute) // 1 token left

	// Cost 4 with 1 token: deficit 3 at 2/s => ceil(1.5) = 2s.
	res := cfg.Evaluate(state, 4, base, time.Minute)
	if res.Allowed {
		t.Fatal("expected rejection")
	}
	if res.RetryAfter != 2*time.Second {
		t.Errorf("expected retryAfter 2s, got %v", res.RetryAfter)
	}
}

func TestTokenBucketNegativeElapsedClamped(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 10, RefillRate: 1}
	state := &TokenBucketState{}

	cfg.Evaluate(state, 10, base, time.Minute)

	// Clock going backwards must not grant tokens (or panic).
	res := cfg.Evaluate(state, 1, base.Add(-time.Hour), time.Minute)
	if res.Allowed {
		t.Fatal("expected rejection with negative elapsed time")
	}
}

func TestTokenBucketCostAboveCapacityNeverAllowed(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 5, RefillRate: 1}
	state := &TokenBucketState{}

	for i := 0; i < 10; i++ {
		res := cfg.Evaluate(state, 6, base.Add(time.Duration(i)*time.Second), time.Minute)
		if res.Allowed {
			t.Fatalf("cost above capacity must never be allowed (iter %d)", i)
		}
	}
}

func TestTokenBucketExpirySetForSweeper(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 10, RefillRate: 1}
	state := &TokenBucketState{}
	window := time.Minute

	cfg.Evaluate(state, 1, base, window)

	if state.IsExpired(base.Add(window)) {
		t.Error("state must not be expired before 2x window")
	}
	if !state.IsExpired(base.Add(2*window + time.Second)) {
		t.Error("state must be expired after 2x window")
	}
}

func TestTokenBucketConcurrentEvaluateIsRaceFree(t *testing.T) {
	cfg := &TokenBucketConfig{Capacity: 1000, RefillRate: 1000}
	state := &TokenBucketState{}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg.Evaluate(state, 1, base.Add(time.Duration(i)*time.Millisecond), time.Minute)
		}()
	}
	wg.Wait()

	state.mu.Lock()
	tokens := state.tokens
	state.mu.Unlock()
	if math.Floor(tokens) > 1000 || tokens < 0 {
		t.Errorf("tokens out of bounds after concurrent use: %f", tokens)
	}
}
