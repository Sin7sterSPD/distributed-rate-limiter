package shardmap

import (
	"hash/fnv"
	"sync"
)

const numShards = 256

// Shard holds one segment of the total keyspace with its own lock.
type Shard[V any] struct {
	mu    sync.RWMutex
	items map[string]V
}

// ShardedMap is a concurrency-safe map split into N shards.
// Type parameter V is the value type stored per key.
type ShardedMap[V any] struct {
	shards [numShards]*Shard[V]
}

func New[V any]() *ShardedMap[V] {
	sm := &ShardedMap[V]{}
	for i := 0; i < numShards; i++ {
		sm.shards[i] = &Shard[V]{items: make(map[string]V)}
	}
	return sm
}

// shard returns the shard responsible for key using FNV hash.
func (sm *ShardedMap[V]) shard(key string) *Shard[V] {
	h := fnv.New32a()
	h.Write([]byte(key))
	return sm.shards[h.Sum32()%numShards]
}

// Get retrieves the value for key. ok is false if the key does not exist.
func (sm *ShardedMap[V]) Get(key string) (V, bool) {
	s := sm.shard(key)
	s.mu.RLock()
	v, ok := s.items[key]
	s.mu.RUnlock()
	return v, ok
}

// Set stores value for key.
func (sm *ShardedMap[V]) Set(key string, value V) {
	s := sm.shard(key)
	s.mu.Lock()
	s.items[key] = value
	s.mu.Unlock()
}

func (sm *ShardedMap[V]) GetOrCreate(key string, create func() V) V {
	s := sm.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.items[key]; ok {
		return v
	}
	v := create()
	s.items[key] = v
	return v
}

// Delete removes key from the map.
func (sm *ShardedMap[V]) Delete(key string) {
	s := sm.shard(key)
	s.mu.Lock()
	delete(s.items, key)
	s.mu.Unlock()
}

func (sm *ShardedMap[V]) Range(fn func(key string, value V) bool) {
	for _, s := range sm.shards {
		s.mu.RLock()
		for k, v := range s.items {
			if !fn(k, v) {
				s.mu.RUnlock()
				return
			}
		}
		s.mu.RUnlock()
	}
}

// Len returns the total number of keys across all shards.
func (sm *ShardedMap[V]) Len() int {
	total := 0
	for _, s := range sm.shards {
		s.mu.RLock()
		total += len(s.items)
		s.mu.RUnlock()
	}
	return total
}
