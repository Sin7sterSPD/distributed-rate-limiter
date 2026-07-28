package distributedratelimiter

import (
	"fmt"
	"time"
)

type Algorithm int

const (
	TokenBucket Algorithm = iota
	SlidingWindow
)

type FallbackMode int

const (
	FallbackMemory FallbackMode = iota

	FallbackAllow

	FallbackDeny
)

type Config struct {
	Algorithm Algorithm
	Limit     int
	Window    time.Duration
	Burst     int

	Backend string

	RedisAddr string

	RedisPassword string

	// RedisDB is the Redis logical database index. Default: 0.
	RedisDB int

	// RedisTimeout is the per-operation context deadline for Redis calls.
	// Default: 100ms. Tune based on your network latency p99.
	RedisTimeout time.Duration
	Fallback     FallbackMode

	KeyPrefix string

	MaxMemoryKeys int

	// MemorySweepInterval is how often the memory backend sweeper runs
	// to clean up expired keys. Default: 60s.
	MemorySweepInterval time.Duration
}

func (c *Config) validate() error {
	if c.Limit <= 0 {
		return fmt.Errorf("%w: Limit must be > 0", ErrInvalidConfig)
	}
	if c.Window <= 0 {
		return fmt.Errorf("%w: Window must be > 0", ErrInvalidConfig)
	}
	if c.Backend == "" {
		c.Backend = "memory"
	}
	if c.Backend == "redis" && c.RedisAddr == "" {
		return fmt.Errorf("%w: RedisAddr required when Backend is 'redis'", ErrInvalidConfig)
	}
	if c.Backend != "memory" && c.Backend != "redis" {
		return fmt.Errorf("%w: Backend must be 'memory' or 'redis'", ErrInvalidConfig)
	}
	if c.KeyPrefix == "" {
		c.KeyPrefix = "ratelimit"
	}
	if c.Burst == 0 {
		c.Burst = c.Limit
	}
	if c.RedisTimeout == 0 {
		c.RedisTimeout = 100 * time.Millisecond
	}
	if c.MaxMemoryKeys == 0 {
		c.MaxMemoryKeys = 100_000
	}
	if c.MemorySweepInterval == 0 {
		c.MemorySweepInterval = 60 * time.Second
	}
	return nil
}
