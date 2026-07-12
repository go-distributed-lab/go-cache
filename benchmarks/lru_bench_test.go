package benchmarks

import (
	"fmt"
	"go-cache/internal/eviction"
	"io"
	"testing"
)

func TestMain(m *testing.M) {
	_ = io.Discard
	m.Run()
}

func BenchmarkLRU_Set(b *testing.B) {
	c := eviction.NewLRU[string, int](1024)
	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(keys[i%len(keys)], i)
	}
}

func BenchmarkLRU_Get_Hit(b *testing.B) {
	c := eviction.NewLRU[string, int](1024)
	for i := 0; i < 1024; i++ {
		c.Set(fmt.Sprintf("key-%d", i), i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fmt.Sprintf("key-%d", i%1024))
	}
}

func BenchmarkLRU_Get_Miss(b *testing.B) {
	c := eviction.NewLRU[string, int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(fmt.Sprintf("miss-%d", i))
	}
}

func BenchmarkLRU_SetGet_Parallel(b *testing.B) {
	c := eviction.NewLRU[int, int](512)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				c.Set(i%512, i)
			} else {
				c.Get(i % 512)
			}
			i++
		}
	})
}
