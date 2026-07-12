// Package eviction contains all cache eviction strategy implementations.
// Each type implements pkg/cache.Cache[K, V].
package eviction

import (
	"go-cache/pkg/cache"
	"sync"
	"sync/atomic"
)

// lruNode is one element in the doubly-linked list maintained by LRU.
type lruNode[K comparable, V any] struct {
	key        K
	value      V
	prev, next *lruNode[K, V]
}

// LRU is a thread-safe Least-Recently-Used cache. The most-recently accessed
// entry is kept at the front of the list; the least-recently used entry is at
// the back and is evicted when capacity is exceeded.
type LRU[K comparable, V any] struct {
	mu        sync.Mutex
	cap       int
	items     map[K]*lruNode[K, V]
	head      *lruNode[K, V] // sentinel (MRU side)
	tail      *lruNode[K, V] // sentinel (LRU side)
	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// NewLRU returns an initialised LRU cache with the given capacity.
// Panics if capacity is less than 1.
func NewLRU[K comparable, V any](capacity int) *LRU[K, V] {
	if capacity < 1 {
		panic("go-cache: LRU capacity must be >= 1")
	}
	head := &lruNode[K, V]{}
	tail := &lruNode[K, V]{}
	head.next = tail
	tail.prev = head
	return &LRU[K, V]{
		cap:   capacity,
		items: make(map[K]*lruNode[K, V], capacity),
		head:  head,
		tail:  tail,
	}
}

// Get returns the value associated with key and moves the node to the MRU
// position. Returns the zero value and false on a cache miss.
func (c *LRU[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	node, ok := c.items[key]
	var value V
	if ok {
		c.moveToFront(node)
		value = node.value // read value under the lock
	}
	c.mu.Unlock()

	if ok {
		c.hits.Add(1)
		return value, true
	}
	c.misses.Add(1)
	return value, false
}

// Set inserts or updates key. If at capacity, the LRU entry is evicted first.
func (c *LRU[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[key]; ok {
		node.value = value
		c.moveToFront(node)
		return
	}

	if len(c.items) >= c.cap {
		c.evictLRU()
	}

	node := &lruNode[K, V]{key: key, value: value}
	c.items[key] = node
	c.insertFront(node)
}

// Delete removes key from the cache. No-op if absent.
func (c *LRU[K, V]) Delete(key K) {
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
func (c *LRU[K, V]) Len() int {
	c.mu.Lock()
	n := len(c.items)
	c.mu.Unlock()
	return n
}

// Metrics returns a point-in-time snapshot of hit/miss/eviction counters.
func (c *LRU[K, V]) Metrics() cache.Stats {
	return cache.Stats{
		Hits:      c.hits.Load(),
		Misses:    c.misses.Load(),
		Evictions: c.evictions.Load(),
	}
}

// ── internal list helpers (must be called with c.mu held) ────────────────────

func (c *LRU[K, V]) insertFront(node *lruNode[K, V]) {
	node.prev = c.head
	node.next = c.head.next
	c.head.next.prev = node
	c.head.next = node
}

func (c *LRU[K, V]) removeNode(node *lruNode[K, V]) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (c *LRU[K, V]) moveToFront(node *lruNode[K, V]) {
	c.removeNode(node)
	c.insertFront(node)
}

func (c *LRU[K, V]) evictLRU() {
	lru := c.tail.prev
	if lru == c.head {
		return // empty list — should not happen
	}
	c.removeNode(lru)
	delete(c.items, lru.key)
	c.evictions.Add(1)
}
