package eviction

import (
	"fmt"
	"sync"
	"testing"
)

func TestFIFO_SetAndGet(t *testing.T) {
	c := NewFIFO[string, int](3)
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

func TestFIFO_EvictsOldest(t *testing.T) {
	c := NewFIFO[string, int](3)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	// Access "a" — must NOT affect eviction order in FIFO.
	c.Get("a")
	c.Get("a")

	// "a" was inserted first so it must be evicted.
	c.Set("d", 4)

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be evicted (oldest insertion)")
	}
	if v, ok := c.Get("d"); !ok || v != 4 {
		t.Errorf("expected 'd'=4, got %v, %v", v, ok)
	}
}

func TestFIFO_UpdateKeepsPosition(t *testing.T) {
	c := NewFIFO[string, int](3)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	// Update "a" — it should keep its original (oldest) position.
	c.Set("a", 99)

	// Adding "d" must still evict "a".
	c.Set("d", 4)

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be evicted even after update")
	}
	if v, ok := c.Get("d"); !ok || v != 4 {
		t.Errorf("expected 'd'=4, got %v, %v", v, ok)
	}
}

func TestFIFO_Delete(t *testing.T) {
	c := NewFIFO[string, int](3)
	c.Set("a", 1)
	c.Delete("a")

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be deleted")
	}
	if c.Len() != 0 {
		t.Errorf("expected Len=0, got %d", c.Len())
	}
}

func TestFIFO_DeleteAbsent(t *testing.T) {
	c := NewFIFO[string, int](3)
	c.Delete("nonexistent") // must not panic
}

func TestFIFO_Metrics(t *testing.T) {
	c := NewFIFO[string, int](3)
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

func TestFIFO_EvictionCount(t *testing.T) {
	c := NewFIFO[string, int](2)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	if c.Metrics().Evictions != 1 {
		t.Errorf("want 1 eviction, got %d", c.Metrics().Evictions)
	}
}

func TestFIFO_PanicOnZeroCapacity(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for capacity=0")
		}
	}()
	NewFIFO[string, int](0)
}

func TestFIFO_ConcurrentAccess(t *testing.T) {
	c := NewFIFO[int, int](64)
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

func TestFIFO_HitRate(t *testing.T) {
	c := NewFIFO[string, int](10)
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
