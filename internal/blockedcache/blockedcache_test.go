package blockedcache

import (
	"sync"
	"testing"
	"time"
)

func TestGetAbsentKey(t *testing.T) {
	c := New(100)
	if c.Get("missing") {
		t.Fatal("absent key must not be blocked")
	}
}

func TestSetGetWithinTTL(t *testing.T) {
	c := New(100)
	c.Set("k", 500*time.Millisecond)
	if !c.Get("k") {
		t.Fatal("key should be blocked within TTL")
	}
}

func TestExpiresAfterTTL(t *testing.T) {
	c := New(100)
	c.Set("k", 30*time.Millisecond)

	time.Sleep(60 * time.Millisecond)

	if c.Get("k") {
		t.Fatal("key should be expired")
	}
}

func TestTTLCappedAtOneSecond(t *testing.T) {
	c := New(100)
	c.Set("k", 5*time.Second) // must be capped to 1s

	time.Sleep(1100 * time.Millisecond)

	if c.Get("k") {
		t.Fatal("TTL should be capped at 1s")
	}
}

func TestNegativeTTLStillCaches(t *testing.T) {
	c := New(100)
	c.Set("k", -1*time.Second) // clamped to default 1s
	if !c.Get("k") {
		t.Fatal("negative TTL should clamp to 1s and cache")
	}
}

func TestMaxKeysSkipsWhenFull(t *testing.T) {
	c := New(2)
	c.Set("a", time.Minute)
	c.Set("b", time.Minute)
	c.Set("c", time.Minute) // cache full, nothing expired => skip

	if c.Get("c") {
		t.Error("c should not be cached when full")
	}
	if !c.Get("a") || !c.Get("b") {
		t.Error("existing entries must survive")
	}
}

func TestMaxKeysEvictsExpiredFirst(t *testing.T) {
	c := New(2)
	c.Set("a", 20*time.Millisecond)
	c.Set("b", time.Minute)

	time.Sleep(50 * time.Millisecond) // a expires

	c.Set("c", time.Minute) // evicts expired a, then caches c

	if c.Get("a") {
		t.Error("expired a should have been evicted")
	}
	if !c.Get("c") {
		t.Error("c should be cached after eviction")
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New(1000)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := string(rune('a' + (g+i)%26))
				c.Set(key, 10*time.Millisecond)
				c.Get(key)
			}
		}(g)
	}
	wg.Wait()
}
