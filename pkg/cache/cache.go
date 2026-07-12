// Package cache defines the Cache interface and shared types used across all
// eviction-strategy implementations.
package cache

// Cache is a generic key-value store that enforces a capacity limit via an
// eviction strategy. K must be comparable so it can be used as a map key.
type Cache[K comparable, V any] interface {
	// Get returns the value for key and true, or the zero value and false if
	// the key is not present or has expired.
	Get(key K) (V, bool)

	// Set inserts or updates key with value. If the cache is at capacity the
	// implementation evicts one entry according to its strategy before inserting.
	Set(key K, value V)

	// Delete removes key from the cache. It is a no-op if the key is absent.
	Delete(key K)

	// Len returns the number of entries currently held in the cache.
	Len() int

	// Metrics returns a snapshot of runtime counters (hits, misses, evictions).
	Metrics() Stats
}

// Stats is a point-in-time snapshot of cache performance counters.
type Stats struct {
	Hits      uint64
	Misses    uint64
	Evictions uint64
}

// HitRate returns the ratio of hits to total lookups. Returns 0 if no lookups
// have occurred yet.
func (s Stats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total)
}
