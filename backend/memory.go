package backend

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Sin7sterSPD/distributed-rate-limiter/algorithm"
	"github.com/Sin7sterSPD/distributed-rate-limiter/internal/shardmap"
)

type memoryEntry struct {
	tbState *algorithm.TokenBucketState
	swState *algorithm.SlidingWindowState
}

// MemoryConfig configures the in-memory backend.
type MemoryConfig struct {
	Algorithm     int
	Limit         int
	Window        time.Duration
	Burst         int
	MaxKeys       int
	SweepInterval time.Duration
}

type MemoryBackend struct {
	cfg       MemoryConfig
	entries   *shardmap.ShardedMap[*memoryEntry]
	tbCfg     *algorithm.TokenBucketConfig
	swCfg     *algorithm.SlidingWindowConfig
	stopCh    chan struct{}
	closeOnce sync.Once
}

func NewMemoryBackend(cfg MemoryConfig) *MemoryBackend {
	if cfg.SweepInterval <= 0 {
		cfg.SweepInterval = 60 * time.Second
	}

	mb := &MemoryBackend{
		cfg:     cfg,
		entries: shardmap.New[*memoryEntry](),
		stopCh:  make(chan struct{}),
	}

	// Pre-compute algorithm configs (avoids per-call allocation)
	if cfg.Algorithm == 0 { // TokenBucket
		capacity := float64(cfg.Burst)
		if capacity == 0 {
			capacity = float64(cfg.Limit)
		}
		refillRate := float64(cfg.Limit) / cfg.Window.Seconds()
		mb.tbCfg = &algorithm.TokenBucketConfig{
			Capacity:   capacity,
			RefillRate: refillRate,
		}
	} else { // SlidingWindow
		mb.swCfg = &algorithm.SlidingWindowConfig{
			Limit:  cfg.Limit,
			Window: cfg.Window,
		}
	}

	go mb.sweeper()
	return mb
}

// GetAndUpdate atomically evaluates the rate limit for key.
func (mb *MemoryBackend) GetAndUpdate(ctx context.Context, key string, cost int) (*BackendResult, error) {
	// Check context before doing any work
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	now := time.Now()

	if mb.cfg.MaxKeys > 0 && mb.entries.Len() >= mb.cfg.MaxKeys {
		mb.enforceMaxKeys()
	}

	if mb.cfg.Algorithm == 0 { // TokenBucket
		entry := mb.entries.GetOrCreate(key, func() *memoryEntry {
			return &memoryEntry{tbState: &algorithm.TokenBucketState{}}
		})
		res := mb.tbCfg.Evaluate(entry.tbState, cost, now, mb.cfg.Window)
		return &BackendResult{
			Allowed:    res.Allowed,
			Remaining:  res.Remaining,
			RetryAfter: res.RetryAfter,
		}, nil
	}

	// SlidingWindow
	entry := mb.entries.GetOrCreate(key, func() *memoryEntry {
		return &memoryEntry{swState: &algorithm.SlidingWindowState{}}
	})
	res := mb.swCfg.Evaluate(entry.swState, cost, now)
	return &BackendResult{
		Allowed:    res.Allowed,
		Remaining:  res.Remaining,
		RetryAfter: res.RetryAfter,
	}, nil
}

func (mb *MemoryBackend) sweeper() {
	ticker := time.NewTicker(mb.cfg.SweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mb.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			// DeleteIf mutates under each shard's write lock; using Range+Delete
			// here would self-deadlock (RLock then Lock on the same shard).
			removed := mb.entries.DeleteIf(func(key string, entry *memoryEntry) bool {
				if entry.tbState != nil {
					return entry.tbState.IsExpired(now)
				}
				if entry.swState != nil {
					return entry.swState.IsExpired(now)
				}
				return true
			})
			mb.enforceMaxKeys()
			if removed > 0 {
				slog.Info("ratelimiter memory sweeper", "removed_keys", removed, "remaining_keys", mb.entries.Len())
			}
		}
	}
}

func (mb *MemoryBackend) Close() error {
	mb.closeOnce.Do(func() {
		close(mb.stopCh)
	})
	return nil
}

// enforceMaxKeys bounds memory usage by evicting expired entries first, then
// (if still over budget) the oldest entries regardless of expiry. Called from
// GetAndUpdate and the sweeper; safe under concurrency because DeleteIf takes
// per-shard write locks.
func (mb *MemoryBackend) enforceMaxKeys() {
	max := mb.cfg.MaxKeys
	if max <= 0 {
		return
	}
	now := time.Now()
	// Pass 1: drop expired entries.
	removed := mb.entries.DeleteIf(func(key string, entry *memoryEntry) bool {
		if entry.tbState != nil {
			return entry.tbState.IsExpired(now)
		}
		if entry.swState != nil {
			return entry.swState.IsExpired(now)
		}
		return true
	})
	if removed > 0 && mb.entries.Len() <= max {
		return
	}
	if mb.entries.Len() <= max {
		return
	}
	// Pass 2: still over budget — evict arbitrary entries (map iteration order,
	// which approximates random eviction) until within budget.
	overflow := mb.entries.Len() - max
	mb.entries.DeleteIf(func(key string, entry *memoryEntry) bool {
		if overflow <= 0 {
			return false
		}
		overflow--
		return true
	})
}
