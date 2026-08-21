package shardmap

import (
	"sync"
	"testing"
)

func TestSetGetDelete(t *testing.T) {
	sm := New[string]()

	sm.Set("a", "1")
	v, ok := sm.Get("a")
	if !ok || v != "1" {
		t.Fatalf("expected (1, true), got (%q, %v)", v, ok)
	}

	sm.Delete("a")
	if _, ok := sm.Get("a"); ok {
		t.Fatal("expected key deleted")
	}
}

func TestGetMissingKey(t *testing.T) {
	sm := New[int]()
	if _, ok := sm.Get("nope"); ok {
		t.Fatal("expected missing key")
	}
}

func TestDeleteMissingKeyIsNoop(t *testing.T) {
	sm := New[int]()
	sm.Delete("nope") // must not panic
}

func TestGetOrCreateCreatesOnce(t *testing.T) {
	sm := New[*int]()
	calls := 0

	factory := func() *int {
		calls++
		v := 42
		return &v
	}

	a := sm.GetOrCreate("k", factory)
	b := sm.GetOrCreate("k", factory)

	if calls != 1 {
		t.Errorf("factory called %d times, want 1", calls)
	}
	if a != b {
		t.Error("GetOrCreate should return the same instance")
	}
}

func TestDeleteIf(t *testing.T) {
	sm := New[int]()
	for _, k := range []string{"a", "b", "c", "d"} {
		sm.Set(k, 1)
	}

	removed := sm.DeleteIf(func(key string, v int) bool {
		return key == "a" || key == "d"
	})

	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
	if _, ok := sm.Get("a"); ok {
		t.Error("a should be removed")
	}
	if _, ok := sm.Get("d"); ok {
		t.Error("d should be removed")
	}
	if sm.Len() != 2 {
		t.Errorf("expected len 2, got %d", sm.Len())
	}
}

func TestRangeVisitsAll(t *testing.T) {
	sm := New[int]()
	for i := 0; i < 10; i++ {
		sm.Set(string(rune('a'+i)), i)
	}

	seen := map[string]int{}
	sm.Range(func(key string, v int) bool {
		seen[key] = v
		return true
	})
	if len(seen) != 10 {
		t.Errorf("expected 10 visited, got %d", len(seen))
	}
}

func TestRangeEarlyStop(t *testing.T) {
	sm := New[int]()
	for i := 0; i < 100; i++ {
		sm.Set(string(rune(i)), i)
	}

	count := 0
	sm.Range(func(key string, v int) bool {
		count++
		return false // stop after first
	})
	if count != 1 {
		t.Errorf("early stop expected 1 visit, got %d", count)
	}
}

func TestLenAcrossShards(t *testing.T) {
	sm := New[int]()
	for i := 0; i < 1000; i++ {
		sm.Set(string(rune(i%256))+string(rune(i/256)), i)
	}
	if got := sm.Len(); got != 1000 {
		t.Errorf("expected 1000, got %d", got)
	}
}

// TestConcurrentMixedOperations exercises Set/Get/GetOrCreate/DeleteIf/Len
// from many goroutines. Run with -race in CI to catch data races; the
// DeleteIf-under-write-lock design must never deadlock with GetOrCreate.
func TestConcurrentMixedOperations(t *testing.T) {
	sm := New[int]()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				key := string(rune('a' + (g+i)%26))
				switch (g + i) % 4 {
				case 0:
					sm.GetOrCreate(key, func() int { return i })
				case 1:
					sm.Set(key, i)
				case 2:
					sm.Get(key)
				case 3:
					sm.DeleteIf(func(k string, v int) bool { return k == key })
					sm.Len()
				}
			}
		}(g)
	}
	wg.Wait()
}
