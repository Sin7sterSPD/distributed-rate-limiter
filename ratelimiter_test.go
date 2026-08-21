package distributedratelimiter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sin7sterSPD/distributed-rate-limiter/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// deadRedisAddr is a reserved port that refuses connections immediately.
const deadRedisAddr = "127.0.0.1:1"

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"zero limit", func(c *Config) { c.Limit = 0 }},
		{"negative limit", func(c *Config) { c.Limit = -1 }},
		{"zero window", func(c *Config) { c.Window = 0 }},
		{"redis without addr", func(c *Config) { c.Backend = "redis" }},
		{"unknown backend", func(c *Config) { c.Backend = "etcd" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{Limit: 10, Window: time.Minute}
			tc.mut(&cfg)
			if err := cfg.validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("expected ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestConfigDefaultsApplied(t *testing.T) {
	cfg := Config{Limit: 10, Window: time.Minute}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}

	if cfg.Backend != "memory" {
		t.Errorf("Backend default: got %q", cfg.Backend)
	}
	if cfg.KeyPrefix != "ratelimit" {
		t.Errorf("KeyPrefix default: got %q", cfg.KeyPrefix)
	}
	if cfg.Burst != 10 {
		t.Errorf("Burst should default to Limit, got %d", cfg.Burst)
	}
	if cfg.RedisTimeout != 100*time.Millisecond {
		t.Errorf("RedisTimeout default: got %v", cfg.RedisTimeout)
	}
	if cfg.MaxMemoryKeys != 100_000 {
		t.Errorf("MaxMemoryKeys default: got %d", cfg.MaxMemoryKeys)
	}
	if cfg.MemorySweepInterval != time.Minute {
		t.Errorf("MemorySweepInterval default: got %v", cfg.MemorySweepInterval)
	}
	if cfg.MetricsNamespace != "app" {
		t.Errorf("MetricsNamespace default: got %q", cfg.MetricsNamespace)
	}
}

func TestMemoryLimiterAllowDeny(t *testing.T) {
	lim, err := New(Config{Algorithm: TokenBucket, Limit: 2, Window: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer lim.Close()

	ctx := context.Background()
	if res, err := lim.Allow(ctx, "k"); err != nil || !res.Allowed {
		t.Fatalf("first: res=%+v err=%v", res, err)
	}
	res, err := lim.Allow(ctx, "k")
	if err != nil || !res.Allowed {
		t.Fatalf("second: res=%+v err=%v", res, err)
	}
	if res.Remaining != 0 {
		t.Errorf("expected remaining 0, got %d", res.Remaining)
	}
	if res, _ := lim.Allow(ctx, "k"); res.Allowed {
		t.Error("third request should be denied")
	}
}

func TestAllowNCost(t *testing.T) {
	lim, _ := New(Config{Algorithm: TokenBucket, Limit: 5, Window: time.Minute})
	defer lim.Close()

	ctx := context.Background()
	res, err := lim.AllowN(ctx, "k", 3)
	if err != nil || !res.Allowed {
		t.Fatalf("AllowN(3): res=%+v err=%v", res, err)
	}
	if res.Remaining != 2 {
		t.Errorf("expected remaining 2, got %d", res.Remaining)
	}
	if res, _ := lim.AllowN(ctx, "k", 3); res.Allowed {
		t.Error("AllowN(3) with 2 left should be denied")
	}
}

func TestFallbackAllowWhenRedisDown(t *testing.T) {
	lim, err := New(Config{
		Algorithm: TokenBucket,
		Limit:     1,
		Window:    time.Minute,
		Backend:   "redis",
		RedisAddr: deadRedisAddr,
		Fallback:  FallbackAllow,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lim.Close()

	for i := 0; i < 3; i++ {
		res, err := lim.Allow(context.Background(), "k")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("FallbackAllow must allow when redis is down (call %d)", i)
		}
	}
}

func TestFallbackDenyWhenRedisDown(t *testing.T) {
	lim, err := New(Config{
		Algorithm: TokenBucket,
		Limit:     1,
		Window:    time.Minute,
		Backend:   "redis",
		RedisAddr: deadRedisAddr,
		Fallback:  FallbackDeny,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lim.Close()

	res, err := lim.Allow(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Fatal("FallbackDeny must deny when redis is down")
	}
	// Raw RetryAfter is 1s; exposed value carries ±10% jitter.
	if res.RetryAfter < 900*time.Millisecond || res.RetryAfter > 1100*time.Millisecond {
		t.Errorf("RetryAfter %v outside jittered [900ms, 1100ms]", res.RetryAfter)
	}
}

func TestFallbackMemoryEnforcesLimits(t *testing.T) {
	lim, err := New(Config{
		Algorithm: TokenBucket,
		Limit:     1,
		Window:    time.Minute,
		Backend:   "redis",
		RedisAddr: deadRedisAddr,
		Fallback:  FallbackMemory,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lim.Close()

	ctx := context.Background()
	if res, err := lim.Allow(ctx, "k"); err != nil || !res.Allowed {
		t.Fatalf("first via memory fallback: res=%+v err=%v", res, err)
	}
	if res, err := lim.Allow(ctx, "k"); err != nil || res.Allowed {
		t.Fatalf("second must be denied by memory fallback: res=%+v err=%v", res, err)
	}
}

func TestBlockedCacheShortCircuitsRejections(t *testing.T) {
	lim, err := New(Config{Algorithm: TokenBucket, Limit: 1, Burst: 1, Window: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer lim.Close()

	ctx := context.Background()
	if res, _ := lim.Allow(ctx, "bot"); !res.Allowed {
		t.Fatal("first request should pass")
	}
	if res, _ := lim.Allow(ctx, "bot"); res.Allowed {
		t.Fatal("second request should be rejected and cached")
	}

	before := testutil.ToFloat64(metrics.NewMetrics("app").BlockedCacheHits)
	res, err := lim.Allow(ctx, "bot") // served from blocked cache
	if err != nil {
		t.Fatal(err)
	}
	after := testutil.ToFloat64(metrics.NewMetrics("app").BlockedCacheHits)

	if res.Allowed {
		t.Fatal("cached rejection must stay rejected")
	}
	if after-before != 1 {
		t.Fatalf("expected exactly 1 blocked-cache hit, delta=%f", after-before)
	}

	// After cache TTL (<=1s) and token refill (~1s), traffic passes again.
	time.Sleep(1200 * time.Millisecond)
	if res, _ := lim.Allow(ctx, "bot"); !res.Allowed {
		t.Fatal("request after cache expiry + refill should pass")
	}
}

func TestExposedRetryAfterCarriesJitter(t *testing.T) {
	lim, _ := New(Config{Algorithm: TokenBucket, Limit: 1, Burst: 1, Window: time.Minute})
	defer lim.Close()

	ctx := context.Background()
	lim.Allow(ctx, "k")

	// Raw retryAfter = deficit/refill = 60s; exposed must be within ±10%.
	res, err := lim.Allow(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Fatal("expected rejection")
	}
	if res.RetryAfter < 54*time.Second || res.RetryAfter > 66*time.Second {
		t.Errorf("RetryAfter %v outside jittered [54s, 66s]", res.RetryAfter)
	}
}

func TestResultCarriesMetadata(t *testing.T) {
	lim, _ := New(Config{Algorithm: SlidingWindow, Limit: 7, Window: time.Minute, KeyPrefix: "pfx"})
	defer lim.Close()

	res, err := lim.Allow(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	if res.Limit != 7 {
		t.Errorf("Limit metadata: got %d", res.Limit)
	}
	if res.Window != time.Minute {
		t.Errorf("Window metadata: got %v", res.Window)
	}
	if res.Algorithm != SlidingWindow {
		t.Errorf("Algorithm metadata: got %v", res.Algorithm)
	}
}

func TestCloseIsIdempotentForMemoryMode(t *testing.T) {
	lim, _ := New(Config{Algorithm: TokenBucket, Limit: 1, Window: time.Minute})

	if err := lim.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := lim.Close(); err != nil { // must not double-close stopCh
		t.Fatalf("second Close: %v", err)
	}
}
