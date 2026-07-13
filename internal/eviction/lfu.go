package eviction

import (
	"go-cache/pkg/cache"
	"sync"
	"sync/atomic"
)

// lfuNode holds a single cache entry inside a frequency bucket's linked list.
type lfuNode[K comparable, V any] struct {
	key        K
	value      V
	freq       int
	prev, next *lfuNode[K, V]
}

// freqList is a doubly-linked list of lfuNodes sharing the same access
// frequency. head/tail are sentinels; newest entries sit just after head.
type freqList[K comparable, V any] struct {
	head, tail *lfuNode[K, V]
	length     int
}

func newFreqList[K comparable, V any]() *freqList[K, V] {
	h := &lfuNode[K, V]{}
	t := &lfuNode[K, V]{}
	h.next = t
	t.prev = h
	return &freqList[K, V]{head: h, tail: t}
}

func (fl *freqList[K, V]) pushFront(n *lfuNode[K, V]) {
	n.prev = fl.head
	n.next = fl.head.next
	fl.head.next.prev = n
	fl.head.next = n
	fl.length++
}

func (fl *freqList[K, V]) remove(n *lfuNode[K, V]) {
	n.prev.next = n.next
	n.next.prev = n.prev
	fl.length--
}

// removeLast removes and returns the LRU entry in this bucket (just before tail).
func (fl *freqList[K, V]) removeLast() *lfuNode[K, V] {
	if fl.length == 0 {
		return nil
	}
	last := fl.tail.prev
	fl.remove(last)
	return last
}

// LFU is a thread-safe Least-Frequently-Used cache. Ties in frequency are
// broken by recency (the less-recently used entry is evicted first).
type LFU[K comparable, V any] struct {
	mu      sync.Mutex
	cap     int
	minFreq int
	items   map[K]*lfuNode[K, V]
	buckets map[int]*freqList[K, V]

	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// NewLFU returns an initialised LFU cache with the given capacity.
// Panics if capacity is less than 1.
func NewLFU[K comparable, V any](capacity int) *LFU[K, V] {
	if capacity < 1 {
		panic("go-cache: LFU capacity must be >= 1")
	}
	return &LFU[K, V]{
		cap:     capacity,
		items:   make(map[K]*lfuNode[K, V], capacity),
		buckets: make(map[int]*freqList[K, V]),
	}
}

// Get returns the value for key and increments its frequency.
// Returns zero value and false on a miss.
func (c *LFU[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	node, ok := c.items[key]
	var value V
	if ok {
		value = node.value
		c.increment(node)
	}
	c.mu.Unlock()

	if ok {
		c.hits.Add(1)
		return value, true
	}
	c.misses.Add(1)
	return value, false
}

// Set inserts or updates key. Evicts the LFU entry if at capacity.
func (c *LFU[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[key]; ok {
		node.value = value
		c.increment(node)
		return
	}

	if len(c.items) >= c.cap {
		c.evict()
	}

	node := &lfuNode[K, V]{key: key, value: value, freq: 1}
	c.items[key] = node
	c.bucketFor(1).pushFront(node)
	c.minFreq = 1
}

// Delete removes key from the cache. No-op if absent.
func (c *LFU[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, ok := c.items[key]
	if !ok {
		return
	}
	c.bucketFor(node.freq).remove(node)
	delete(c.items, key)
}

// Len returns the number of entries currently in the cache.
func (c *LFU[K, V]) Len() int {
	c.mu.Lock()
	n := len(c.items)
	c.mu.Unlock()
	return n
}

// Metrics returns a snapshot of hit/miss/eviction counters.
func (c *LFU[K, V]) Metrics() cache.Stats {
	return cache.Stats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evictions.Load(),
	}
}

// ── internal helpers (must be called with c.mu held) ─────────────────────────

func (c *LFU[K, V]) bucketFor(freq int) *freqList[K, V] {
	if _, ok := c.buckets[freq]; !ok {
		c.buckets[freq] = newFreqList[K, V]()
	}
	return c.buckets[freq]
}

// increment moves node from its current frequency bucket to freq+1.
func (c *LFU[K, V]) increment(node *lfuNode[K, V]) {
	oldFreq := node.freq
	fl := c.bucketFor(oldFreq)
	fl.remove(node)

	// Advance minFreq when the min-frequency bucket becomes empty.
	if oldFreq == c.minFreq && fl.length == 0 {
		c.minFreq++
	}

	node.freq++
	c.bucketFor(node.freq).pushFront(node)
}

// evict removes the LFU (and LRU within that frequency) entry.
func (c *LFU[K, V]) evict() {
	fl := c.bucketFor(c.minFreq)
	node := fl.removeLast()
	if node == nil {
		return
	}
	delete(c.items, node.key)
	c.evictions.Add(1)
}
