package backend

import (
	"context"
	"testing"
	"time"
)

func TestMemoryTokenBucketAllowDeny(t *testing.T) {
	mb := NewMemoryBackend(MemoryConfig{
		Algorithm: 0, // TokenBucket
		Limit:     2,
		Burst:     2,
		Window:    time.Minute,
	})
	defer mb.Close()

	ctx := context.Background()

	for i := 0; i < 2; i++ {
		res, err := mb.GetAndUpdate(ctx, "k", 1)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("call %d should be allowed", i+1)
		}
		if res.Remaining != 1-i {
			t.Errorf("call %d: expected remaining %d, got %d", i+1, 1-i, res.Remaining)
		}
	}

	res, err := mb.GetAndUpdate(ctx, "k", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Error("3rd request should be denied")
	}
	if res.RetryAfter <= 0 {
		t.Errorf("denied request should carry RetryAfter, got %v", res.RetryAfter)
	}
}

func TestMemorySlidingWindowAllowDeny(t *testing.T) {
	mb := NewMemoryBackend(MemoryConfig{
		Algorithm: 1, // SlidingWindow
		Limit:     2,
		Window:    time.Minute,
	})
	defer mb.Close()

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		res, err := mb.GetAndUpdate(ctx, "k", 1)
		if err != nil || !res.Allowed {
			t.Fatalf("request %d should be allowed (err=%v)", i+1, err)
		}
	}
	res, _ := mb.GetAndUpdate(ctx, "k", 1)
	if res.Allowed {
		t.Error("3rd request should be denied")
	}
}

func TestMemoryKeysAreIndependent(t *testing.T) {
	mb := NewMemoryBackend(MemoryConfig{Algorithm: 0, Limit: 1, Burst: 1, Window: time.Minute})
	defer mb.Close()

	ctx := context.Background()
	if res, _ := mb.GetAndUpdate(ctx, "alice", 1); !res.Allowed {
		t.Fatal("alice first should pass")
	}
	if res, _ := mb.GetAndUpdate(ctx, "bob", 1); !res.Allowed {
		t.Fatal("bob first should pass")
	}
	if res, _ := mb.GetAndUpdate(ctx, "alice", 1); res.Allowed {
		t.Fatal("alice second should be denied")
	}
}

func TestMemoryContextCancelled(t *testing.T) {
	mb := NewMemoryBackend(MemoryConfig{Algorithm: 0, Limit: 1, Window: time.Minute})
	defer mb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := mb.GetAndUpdate(ctx, "k", 1); err == nil {
		t.Fatal("expected context error")
	}
}

func TestMemoryEvictionDropsExpiredOnInsert(t *testing.T) {
	mb := NewMemoryBackend(MemoryConfig{
		Algorithm: 0,
		Limit:     1,
		Burst:     1,
		Window:    10 * time.Millisecond, // expiry = 20ms
		MaxKeys:   2,
	})
	defer mb.Close()

	ctx := context.Background()
	mb.GetAndUpdate(ctx, "k1", 1)
	mb.GetAndUpdate(ctx, "k2", 1)

	time.Sleep(40 * time.Millisecond) // both entries now expired

	mb.GetAndUpdate(ctx, "k3", 1) // insert path evicts expired k1,k2 first

	if got := mb.entries.Len(); got != 1 {
		t.Errorf("expected 1 entry after expired-eviction, got %d", got)
	}
}

func TestMemoryEvictionOverflowIsBounded(t *testing.T) {
	mb := NewMemoryBackend(MemoryConfig{
		Algorithm: 0,
		Limit:     100,
		Burst:     100,
		Window:    time.Hour, // nothing expires
		MaxKeys:   2,
	})
	defer mb.Close()

	ctx := context.Background()
	for _, k := range []string{"a", "b", "c"} {
		mb.GetAndUpdate(ctx, k, 1)
	}
	if got := mb.entries.Len(); got != 3 {
		t.Fatalf("expected transient overshoot of 3 entries, got %d", got)
	}

	// Overflow eviction (expired-first pass found nothing) must bring the
	// count back within budget regardless of which keys it picks.
	mb.enforceMaxKeys()

	if got := mb.entries.Len(); got != 2 {
		t.Errorf("enforceMaxKeys should bound entries to MaxKeys=2, got %d", got)
	}
}

func TestMemorySweeperRemovesExpired(t *testing.T) {
	mb := NewMemoryBackend(MemoryConfig{
		Algorithm:     0,
		Limit:         1,
		Burst:         1,
		Window:        20 * time.Millisecond, // expiry = 40ms
		SweepInterval: 25 * time.Millisecond,
	})
	defer mb.Close()

	ctx := context.Background()
	for _, k := range []string{"a", "b", "c"} {
		mb.GetAndUpdate(ctx, k, 1)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mb.entries.Len() == 0 {
			return // sweeper did its job
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("sweeper never removed entries; len=%d", mb.entries.Len())
}

func TestMemoryCloseIdempotent(t *testing.T) {
	mb := NewMemoryBackend(MemoryConfig{Algorithm: 0, Limit: 1, Window: time.Minute})

	if err := mb.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := mb.Close(); err != nil { // double close must not panic
		t.Fatalf("second Close: %v", err)
	}
}
