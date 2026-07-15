package eviction

import (
	"hash/fnv"
	"sync/atomic"

	"go-cache/pkg/cache"
)

// Sharded is a cache that distributes keys across N independent LRU shards
// using FNV-1a hashing. Each shard has its own mutex, so concurrent operations
// on different keys never contend. Total capacity is spread evenly across shards.
//
// K must be string — hashing arbitrary comparable types without reflection
// requires the key to be serialisable; string is the zero-overhead choice.
type Sharded[V any] struct {
	shards    []*LRU[string, V]
	numShards uint64
	hits      atomic.Uint64
	misses    atomic.Uint64
}

// NewSharded returns a sharded cache with the given number of shards and total
// capacity. Capacity is divided evenly across shards (minimum 1 per shard).
// Panics if numShards < 1 or capacity < numShards.
func NewSharded[V any](numShards, capacity int) *Sharded[V] {
	if numShards < 1 {
		panic("go-cache: Sharded numShards must be >= 1")
	}
	if capacity < numShards {
		panic("go-cache: Sharded capacity must be >= numShards")
	}

	perShard := capacity / numShards
	shards := make([]*LRU[string, V], numShards)
	for i := range shards {
		shards[i] = NewLRU[string, V](perShard)
	}

	return &Sharded[V]{
		shards:    shards,
		numShards: uint64(numShards),
	}
}

// Get returns the value for key and true, or zero value and false on a miss.
func (c *Sharded[V]) Get(key string) (V, bool) {
	value, ok := c.shard(key).Get(key)
	if ok {
		c.hits.Add(1)
	} else {
		c.misses.Add(1)
	}
	return value, ok
}

// Set inserts or updates key in the appropriate shard.
func (c *Sharded[V]) Set(key string, value V) {
	c.shard(key).Set(key, value)
}

// Delete removes key from the appropriate shard.
func (c *Sharded[V]) Delete(key string) {
	c.shard(key).Delete(key)
}

// Len returns the total number of entries across all shards.
func (c *Sharded[V]) Len() int {
	total := 0
	for _, s := range c.shards {
		total += s.Len()
	}
	return total
}

// Metrics returns aggregated hit/miss/eviction counters across all shards.
// Shard-level evictions are included via per-shard Metrics().
func (c *Sharded[V]) Metrics() cache.Stats {
	var evictions uint64
	for _, s := range c.shards {
		evictions += s.Metrics().Evictions
	}
	return cache.Stats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: evictions,
	}
}

// shard returns the LRU shard responsible for key using FNV-1a hashing.
func (c *Sharded[V]) shard(key string) *LRU[string, V] {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return c.shards[h.Sum64()%c.numShards]
}
