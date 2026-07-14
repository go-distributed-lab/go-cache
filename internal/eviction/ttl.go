package eviction

import (
	"sync"
	"sync/atomic"
	"time"

	"go-cache/pkg/cache"
)

// ttlEntry holds a value and its absolute expiry deadline.
type ttlEntry[V any] struct {
	value     V
	expiresAt time.Time
}

// TTL is a thread-safe expiration-based cache. Entries are evicted lazily —
// on Get or Set — with no background goroutines or ticker leaks.
// There is no hard capacity limit; expired entries are purged on every write.
type TTL[K comparable, V any] struct {
	mu        sync.Mutex
	ttl       time.Duration
	items     map[K]ttlEntry[V]
	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// NewTTL returns an initialised TTL cache where every entry expires after ttl.
// Panics if ttl is <= 0.
func NewTTL[K comparable, V any](ttl time.Duration) *TTL[K, V] {
	if ttl <= 0 {
		panic("go-cache: TTL duration must be > 0")
	}
	return &TTL[K, V]{
		ttl:   ttl,
		items: make(map[K]ttlEntry[V]),
	}
}

// Get returns the value for key if it exists and has not expired.
func (c *TTL[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	entry, ok := c.items[key]
	if ok && time.Now().After(entry.expiresAt) {
		delete(c.items, key)
		c.evictions.Add(1)
		ok = false
	}
	var value V
	if ok {
		value = entry.value
	}
	c.mu.Unlock()

	if ok {
		c.hits.Add(1)
		return value, true
	}
	c.misses.Add(1)
	return value, false
}

// Set inserts or updates key with a fresh TTL. No purge is performed —
// expired entries are reclaimed lazily by Get and Len.
func (c *TTL[K, V]) Set(key K, value V) {
	c.mu.Lock()
	c.items[key] = ttlEntry[V]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Delete removes key from the cache. No-op if absent or already expired.
func (c *TTL[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Len returns the number of non-expired entries currently in the cache.
func (c *TTL[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeExpired()
	return len(c.items)
}

// Metrics returns a snapshot of hit/miss/eviction counters.
func (c *TTL[K, V]) Metrics() cache.Stats {
	return cache.Stats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evictions.Load(),
	}
}

// purgeExpired removes all expired entries. Must be called with c.mu held.
func (c *TTL[K, V]) purgeExpired() {
	now := time.Now()
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
			c.evictions.Add(1)
		}
	}
}
