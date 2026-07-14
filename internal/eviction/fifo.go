package eviction

import (
	"sync"
	"sync/atomic"

	"go-cache/pkg/cache"
)

// fifoNode is one element in the FIFO queue.
type fifoNode[K comparable, V any] struct {
	key        K
	value      V
	prev, next *fifoNode[K, V]
}

// FIFO is a thread-safe First-In-First-Out cache. The oldest inserted entry
// is evicted when capacity is exceeded. Access order has no effect on eviction.
type FIFO[K comparable, V any] struct {
	mu        sync.Mutex
	cap       int
	items     map[K]*fifoNode[K, V]
	head      *fifoNode[K, V] // sentinel (newest side)
	tail      *fifoNode[K, V] // sentinel (oldest side)
	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// NewFIFO returns an initialised FIFO cache with the given capacity.
// Panics if capacity is less than 1.
func NewFIFO[K comparable, V any](capacity int) *FIFO[K, V] {
	if capacity < 1 {
		panic("go-cache: FIFO capacity must be >= 1")
	}
	head := &fifoNode[K, V]{}
	tail := &fifoNode[K, V]{}
	head.next = tail
	tail.prev = head
	return &FIFO[K, V]{
		cap:   capacity,
		items: make(map[K]*fifoNode[K, V], capacity),
		head:  head,
		tail:  tail,
	}
}

// Get returns the value for key. Unlike LRU, this does not affect eviction
// order — FIFO evicts strictly by insertion order.
func (c *FIFO[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	node, ok := c.items[key]
	var value V
	if ok {
		value = node.value
	}
	c.mu.Unlock()

	if ok {
		c.hits.Add(1)
		return value, true
	}
	c.misses.Add(1)
	return value, false
}

// Set inserts or updates key. On update the insertion position is unchanged —
// the entry keeps its original place in the eviction queue. Evicts the oldest
// entry if at capacity.
func (c *FIFO[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[key]; ok {
		node.value = value
		return
	}

	if len(c.items) >= c.cap {
		c.evictOldest()
	}

	node := &fifoNode[K, V]{key: key, value: value}
	c.items[key] = node
	c.insertFront(node)
}

// Delete removes key from the cache. No-op if absent.
func (c *FIFO[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, ok := c.items[key]
	if !ok {
		return
	}
	c.removeNode(node)
	delete(c.items, key)
}

// Len returns the number of entries currently in the cache.
func (c *FIFO[K, V]) Len() int {
	c.mu.Lock()
	n := len(c.items)
	c.mu.Unlock()
	return n
}

// Metrics returns a snapshot of hit/miss/eviction counters.
func (c *FIFO[K, V]) Metrics() cache.Stats {
	return cache.Stats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evictions.Load(),
	}
}

// ── internal helpers (must be called with c.mu held) ─────────────────────────

func (c *FIFO[K, V]) insertFront(node *fifoNode[K, V]) {
	node.prev = c.head
	node.next = c.head.next
	c.head.next.prev = node
	c.head.next = node
}

func (c *FIFO[K, V]) removeNode(node *fifoNode[K, V]) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (c *FIFO[K, V]) evictOldest() {
	oldest := c.tail.prev
	if oldest == c.head {
		return
	}
	c.removeNode(oldest)
	delete(c.items, oldest.key)
	c.evictions.Add(1)
}
