package distributedratelimiter

import "time"

type Algorithm int

const (
	TokenBucket Algorithm = iota
	SlidingWind
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

}
