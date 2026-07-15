package eviction

import (
	"fmt"
	"sync"
	"testing"
)

func TestSharded_SetAndGet(t *testing.T) {
	c := NewSharded[int](4, 64)
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

func TestSharded_Delete(t *testing.T) {
	c := NewSharded[int](4, 64)
	c.Set("a", 1)
	c.Delete("a")

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be deleted")
	}
}

func TestSharded_DeleteAbsent(t *testing.T) {
	c := NewSharded[int](4, 64)
	c.Delete("nonexistent") // must not panic
}

func TestSharded_Len(t *testing.T) {
	c := NewSharded[int](4, 64)
	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	if c.Len() != 3 {
		t.Errorf("expected Len=3, got %d", c.Len())
	}
}

func TestSharded_Metrics(t *testing.T) {
	c := NewSharded[int](4, 64)
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

func TestSharded_Eviction(t *testing.T) {
	// 4 shards, capacity 4 → 1 per shard.
	c := NewSharded[int](4, 4)

	// Fill each shard beyond its per-shard capacity to trigger evictions.
	for i := 0; i < 100; i++ {
		c.Set(fmt.Sprintf("key-%d", i), i)
	}

	if c.Metrics().Evictions == 0 {
		t.Error("expected evictions to occur")
	}
}

func TestSharded_PanicOnZeroShards(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for numShards=0")
		}
	}()
	NewSharded[int](0, 64)
}

func TestSharded_PanicCapacityLessThanShards(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when capacity < numShards")
		}
	}()
	NewSharded[int](8, 4)
}

func TestSharded_ConcurrentAccess(t *testing.T) {
	c := NewSharded[int](16, 256)
	const goroutines = 40
	const ops = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("key-%d", (g*ops+i)%100)
				c.Set(key, i)
				c.Get(key)
			}
		}()
	}
	wg.Wait()
}

func TestSharded_HitRate(t *testing.T) {
	c := NewSharded[int](4, 64)
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

func TestSharded_DistributionAcrossShards(t *testing.T) {
	c := NewSharded[int](8, 800)
	for i := 0; i < 800; i++ {
		c.Set(fmt.Sprintf("key-%d", i), i)
	}

	// Each shard should hold roughly capacity/numShards entries.
	// Allow wide tolerance for hash distribution variance.
	for i, shard := range c.shards {
		l := shard.Len()
		if l == 0 {
			t.Errorf("shard %d is empty — poor key distribution", i)
		}
	}
}
