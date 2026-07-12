package eviction

import (
	"fmt"
	"sync"
	"testing"
)

func TestLRU_SetAndGet(t *testing.T) {
	c := NewLRU[string, int](3)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	for _, tc := range []struct {
		key  string
		want int
	}{
		{"a", 1}, {"b", 2}, {"c", 3},
	} {
		got, ok := c.Get(tc.key)
		if !ok || got != tc.want {
			t.Errorf("Get(%q) = %v, %v; want %v, true", tc.key, got, ok, tc.want)
		}
	}
}

func TestLRU_EvictsLRU(t *testing.T) {
	c := NewLRU[string, int](3)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	// Access "a" and "c" to make "b" the LRU.
	c.Get("a")
	c.Get("c")

	// Adding "d" should evict "b".
	c.Set("d", 4)

	if _, ok := c.Get("b"); ok {
		t.Error("expected 'b' to be evicted, but it was found")
	}
	if v, ok := c.Get("d"); !ok || v != 4 {
		t.Errorf("expected 'd'=4, got %v, %v", v, ok)
	}
}

func TestLRU_Update(t *testing.T) {
	c := NewLRU[string, int](2)
	c.Set("x", 10)
	c.Set("x", 20)

	v, ok := c.Get("x")
	if !ok || v != 20 {
		t.Errorf("expected x=20, got %v, %v", v, ok)
	}
	if c.Len() != 1 {
		t.Errorf("expected Len=1, got %d", c.Len())
	}
}

func TestLRU_Delete(t *testing.T) {
	c := NewLRU[string, int](3)
	c.Set("a", 1)
	c.Delete("a")

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be deleted")
	}
	if c.Len() != 0 {
		t.Errorf("expected Len=0, got %d", c.Len())
	}
}

func TestLRU_DeleteAbsent(t *testing.T) {
	c := NewLRU[string, int](3)
	c.Delete("nonexistent") // must not panic
}

func TestLRU_Metrics(t *testing.T) {
	c := NewLRU[string, int](3)
	c.Set("a", 1)
	c.Get("a") // hit
	c.Get("z") // miss

	s := c.Metrics()
	if s.Hits != 1 {
		t.Errorf("want 1 hit, got %d", s.Hits)
	}
	if s.Misses != 1 {
		t.Errorf("want 1 miss, got %d", s.Misses)
	}
}

func TestLRU_EvictionCount(t *testing.T) {
	c := NewLRU[string, int](2)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3) // evicts one

	if c.Metrics().Evictions != 1 {
		t.Errorf("want 1 eviction, got %d", c.Metrics().Evictions)
	}
}

func TestLRU_PanicOnZeroCapacity(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for capacity=0")
		}
	}()
	NewLRU[string, int](0)
}

func TestLRU_ConcurrentAccess(t *testing.T) {
	c := NewLRU[int, int](64)
	const goroutines = 20
	const ops = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := (g*ops + i) % 50
				c.Set(key, key*2)
				c.Get(key)
			}
		}()
	}
	wg.Wait()

	if c.Len() > 64 {
		t.Errorf("Len %d exceeds capacity 64", c.Len())
	}
}

func TestLRU_HitRate(t *testing.T) {
	c := NewLRU[string, int](10)
	for i := 0; i < 10; i++ {
		c.Set(fmt.Sprintf("k%d", i), i)
	}
	for i := 0; i < 5; i++ {
		c.Get(fmt.Sprintf("k%d", i)) // 5 hits
	}
	for i := 10; i < 15; i++ {
		c.Get(fmt.Sprintf("k%d", i)) // 5 misses
	}

	if c.Metrics().HitRate() != 0.5 {
		t.Errorf("want hit rate 0.5, got %f", c.Metrics().HitRate())
	}
}

func TestLRU_ZeroHitRate(t *testing.T) {
	c := NewLRU[string, int](3)
	if c.Metrics().HitRate() != 0 {
		t.Error("expected hit rate 0 with no lookups")
	}
}
