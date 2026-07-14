package eviction

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestTTL_SetAndGet(t *testing.T) {
	c := NewTTL[string, int](time.Second)
	c.Set("a", 1)

	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Errorf("expected a=1, got %v, %v", v, ok)
	}
}

func TestTTL_Expiry(t *testing.T) {
	c := NewTTL[string, int](50 * time.Millisecond)
	c.Set("a", 1)

	time.Sleep(100 * time.Millisecond)

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to have expired")
	}
}

func TestTTL_RefreshOnSet(t *testing.T) {
	c := NewTTL[string, int](100 * time.Millisecond)
	c.Set("a", 1)

	time.Sleep(60 * time.Millisecond)
	c.Set("a", 2) // refresh TTL

	time.Sleep(60 * time.Millisecond)
	// 120ms since first set but only 60ms since refresh — must still be alive.
	v, ok := c.Get("a")
	if !ok || v != 2 {
		t.Errorf("expected a=2 after refresh, got %v, %v", v, ok)
	}
}

func TestTTL_Delete(t *testing.T) {
	c := NewTTL[string, int](time.Second)
	c.Set("a", 1)
	c.Delete("a")

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be deleted")
	}
}

func TestTTL_DeleteAbsent(t *testing.T) {
	c := NewTTL[string, int](time.Second)
	c.Delete("nonexistent") // must not panic
}

func TestTTL_Metrics_Hit(t *testing.T) {
	c := NewTTL[string, int](time.Second)
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

func TestTTL_ExpiryCountedAsEviction(t *testing.T) {
	c := NewTTL[string, int](50 * time.Millisecond)
	c.Set("a", 1)
	c.Set("b", 2)

	time.Sleep(100 * time.Millisecond)

	c.Get("a") // triggers lazy eviction of "a"
	c.Get("b") // triggers lazy eviction of "b"

	s := c.Metrics()
	if s.Evictions != 2 {
		t.Errorf("want 2 evictions, got %d", s.Evictions)
	}
}

func TestTTL_LenExcludesExpired(t *testing.T) {
	c := NewTTL[string, int](50 * time.Millisecond)
	c.Set("a", 1)
	c.Set("b", 2)

	time.Sleep(100 * time.Millisecond)

	c.Set("c", 3) // does NOT purge anymore
	c.Len()       // Len() purges expired entries

	if c.Len() != 1 {
		t.Errorf("expected Len=1 after expiry, got %d", c.Len())
	}
}

func TestTTL_PanicOnZeroDuration(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for zero TTL")
		}
	}()
	NewTTL[string, int](0)
}

func TestTTL_ConcurrentAccess(t *testing.T) {
	c := NewTTL[int, int](time.Second)
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
}

func TestTTL_HitRate(t *testing.T) {
	c := NewTTL[string, int](time.Second)
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
