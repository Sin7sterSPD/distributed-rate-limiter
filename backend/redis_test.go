package backend

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sin7sterSPD/distributed-rate-limiter/metrics"
	"github.com/alicebob/miniredis/v2"
)

func newTestRedisBackend(t *testing.T, mr *miniredis.Miniredis, algo, limit int, window time.Duration) *RedisBackend {
	t.Helper()

	rb, err := NewRedisBackend(RedisConfig{
		Algorithm:      algo,
		Limit:          limit,
		Burst:          limit,
		Window:         window,
		Addr:           mr.Addr(),
		Timeout:        500 * time.Millisecond,
		KeyPrefix:      "rl",
		MaxFailures:    3,
		BreakerTimeout: 200 * time.Millisecond,
		Metrics:        metrics.NewMetrics("app"),
	})
	if err != nil {
		t.Fatalf("NewRedisBackend: %v", err)
	}
	t.Cleanup(func() { _ = rb.Close() })
	return rb
}

func TestRedisTokenBucketAllowDeny(t *testing.T) {
	mr := miniredis.RunT(t)
	rb := newTestRedisBackend(t, mr, 0, 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		res, err := rb.GetAndUpdate(ctx, "user:1", 1)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("call %d should be allowed", i+1)
		}
		if res.Remaining != 2-i {
			t.Errorf("call %d: expected remaining %d, got %d", i+1, 2-i, res.Remaining)
		}
	}

	res, err := rb.GetAndUpdate(ctx, "user:1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Error("4th request should be denied")
	}
	if res.RetryAfter <= 0 {
		t.Errorf("denied request should carry RetryAfter, got %v", res.RetryAfter)
	}
}

func TestRedisTokenBucketStatePersistsAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	ctx := context.Background()

	rb1 := newTestRedisBackend(t, mr, 0, 5, time.Minute)
	res1, err := rb1.GetAndUpdate(ctx, "k", 2)
	if err != nil || !res1.Allowed {
		t.Fatalf("first call: res=%+v err=%v", res1, err)
	}

	// A second backend instance (e.g. another gateway) must see the same state.
	rb2 := newTestRedisBackend(t, mr, 0, 5, time.Minute)
	res2, err := rb2.GetAndUpdate(ctx, "k", 4)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Allowed {
		t.Errorf("expected denial with only 3 tokens left, got %+v", res2)
	}
}

func TestRedisSetsTTLOnKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	rb := newTestRedisBackend(t, mr, 0, 5, time.Minute)
	ctx := context.Background()

	window := time.Minute
	if _, err := rb.GetAndUpdate(ctx, "k", 1); err != nil {
		t.Fatal(err)
	}

	ttl := mr.TTL("rl:k")
	if ttl <= 0 || ttl > 2*window {
		t.Errorf("TTL %v outside (0, %v]", ttl, 2*window)
	}
}

func TestRedisSlidingWindowAllowDenyAndRoll(t *testing.T) {
	mr := miniredis.RunT(t)
	rb := newTestRedisBackend(t, mr, 1, 3, 200*time.Millisecond)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		res, err := rb.GetAndUpdate(ctx, "k", 1)
		if err != nil || !res.Allowed {
			t.Fatalf("request %d should be allowed (res=%+v err=%v)", i+1, res, err)
		}
	}
	res, err := rb.GetAndUpdate(ctx, "k", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Error("4th request should be denied")
	}

	// Roll into a new window and wait for weight decay (~55% in):
	// used = 3*(1-0.55) = 1.35, +1 => 2.35 <= 3 => allowed.
	time.Sleep(310 * time.Millisecond)

	res, err = rb.GetAndUpdate(ctx, "k", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Allowed {
		t.Errorf("request after window roll + decay should be allowed: %+v", res)
	}
}

func TestRedisCircuitBreakerOpensAndRecovers(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close() // dead endpoint

	rb, err := NewRedisBackend(RedisConfig{
		Algorithm:      0,
		Limit:          5,
		Burst:          5,
		Window:         time.Minute,
		Addr:           addr,
		Timeout:        500 * time.Millisecond,
		KeyPrefix:      "rl",
		MaxFailures:    2,
		BreakerTimeout: 150 * time.Millisecond,
		Metrics:        metrics.NewMetrics("app"),
	})
	if err != nil {
		t.Fatalf("constructor must not fail on unreachable redis: %v", err)
	}
	defer rb.Close()
	ctx := context.Background()

	// Two failures trip the breaker.
	for i := 0; i < 2; i++ {
		_, err := rb.GetAndUpdate(ctx, "k", 1)
		if err == nil {
			t.Fatal("expected error against dead redis")
		}
		if strings.Contains(err.Error(), "circuit breaker open") {
			t.Fatalf("failure %d should be a dial error, not breaker: %v", i+1, err)
		}
		if !errors.Is(err, ErrBackendUnavailable) {
			t.Fatalf("error should wrap ErrBackendUnavailable: %v", err)
		}
	}

	// Third call fails fast via the breaker.
	_, err = rb.GetAndUpdate(ctx, "k", 1)
	if err == nil || !strings.Contains(err.Error(), "circuit breaker open") {
		t.Fatalf("expected breaker-open error, got %v", err)
	}

	// After BreakerTimeout the probe is allowed through (and fails again,
	// but as a dial error — proving the breaker let it out).
	time.Sleep(250 * time.Millisecond)
	_, err = rb.GetAndUpdate(ctx, "k", 1)
	if err == nil {
		t.Fatal("probe against dead redis must fail")
	}
	if strings.Contains(err.Error(), "circuit breaker open") {
		t.Fatalf("half-open probe should reach the dial layer, got: %v", err)
	}
}

func TestRedisIndependentKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	rb := newTestRedisBackend(t, mr, 0, 1, time.Minute)
	ctx := context.Background()

	if res, err := rb.GetAndUpdate(ctx, "a", 1); err != nil || !res.Allowed {
		t.Fatalf("key a first: res=%+v err=%v", res, err)
	}
	if res, err := rb.GetAndUpdate(ctx, "b", 1); err != nil || !res.Allowed {
		t.Fatalf("key b first: res=%+v err=%v", res, err)
	}
	res, _ := rb.GetAndUpdate(ctx, "a", 1)
	if res.Allowed {
		t.Error("key a second should be denied")
	}
}
