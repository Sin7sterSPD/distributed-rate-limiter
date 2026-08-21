package blockedcache

import (
	"sync"
	"time"

	"github.com/Sin7sterSPD/distributed-rate-limiter/internal/shardmap"
)

// BlockedCache is a tiny TTL cache of recent "rejected" decisions.
//
// Rationale (design notes, Flow 3): a client hammering a rate-limited endpoint
// at 1000 req/s would otherwise hit Redis on every rejection. Caching the
// rejection locally for a short TTL means only ~1 backend call per second per
// blocked key. Only rejections are cached — allowances always hit the backend,
// so the cache can only ever over-block briefly, never over-admit.
type BlockedCache struct {
	mu      sync.Mutex
	entries *shardmap.ShardedMap[time.Time] // key -> blocked-until
	maxKeys int
}

func New(maxKeys int) *BlockedCache {
	if maxKeys <= 0 {
		maxKeys = 10_000
	}
	return &BlockedCache{
		entries: shardmap.New[time.Time](),
		maxKeys: maxKeys,
	}
}

// Get returns true if key is currently known-blocked.
func (c *BlockedCache) Get(key string) bool {
	until, ok := c.entries.Get(key)
	if !ok {
		return false
	}
	if time.Now().After(until) {
		c.entries.Delete(key)
		return false
	}
	return true
}

// Set marks key as blocked for ttl (clamped to (0, 1s]).
func (c *BlockedCache) Set(key string, ttl time.Duration) {
	if ttl <= 0 {
		ttl = time.Second
	}
	if ttl > time.Second {
		ttl = time.Second
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries.Len() >= c.maxKeys {
		// Cheap bound: drop expired entries; if still full, skip caching.
		now := time.Now()
		c.entries.DeleteIf(func(k string, until time.Time) bool {
			return now.After(until)
		})
		if c.entries.Len() >= c.maxKeys {
			return
		}
	}
	c.entries.Set(key, time.Now().Add(ttl))
}
