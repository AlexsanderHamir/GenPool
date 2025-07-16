package test

import (
	"sync"
	"testing"
	"time"

	"sync/atomic"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// BenchmarkObject is a simple struct we'll use for benchmarking.
type BenchmarkObject struct {
	// user fields
	Name   string   // 16 bytes (pointer + length)
	Data   []byte   // 24 bytes (pointer + len + cap)
	Result int64    // 8 bytes - store computation result
	_      [16]byte // 16 bytes padding = 64 bytes total

	pool.Fields[BenchmarkObject]
}

func cpuIntensiveWorkload(obj *BenchmarkObject) {
	obj.Name = "cpu_test"

	// Heavier CPU work
	var result int64
	for i := range 10_000 {
		result += int64(i * i * i)
		result ^= int64(i << 3)
		if i%1000 == 0 {
			result = result*31 + int64(i)
		}
	}
	obj.Result = result

	// Minimal allocation - just set a small data payload
	if cap(obj.Data) < 100 {
		obj.Data = make([]byte, 0, 100)
	}
	obj.Data = obj.Data[:0]

	// Add some derived data
	for i := range 100 {
		obj.Data = append(obj.Data, byte(result>>uint(i%8)))
	}
}

// Helper functions for benchmarks.
func allocator() *BenchmarkObject {
	return &BenchmarkObject{Name: "test"}
}

func cleaner(obj *BenchmarkObject) {
	obj.Name = ""
	obj.Data = obj.Data[:0]
}

func BenchmarkGenPool(b *testing.B) {
	cfg := pool.Config[BenchmarkObject, *BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
	}

	p, err := pool.NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}

	defer p.Close()

	b.SetParallelism(1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := p.Get()

			cpuIntensiveWorkload(obj)

			p.Put(obj)
		}
	})
}

func BenchmarkSyncPool(b *testing.B) {
	p := &sync.Pool{
		New: func() any {
			return allocator()
		},
	}

	b.SetParallelism(1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := p.Get().(*BenchmarkObject)

			cpuIntensiveWorkload(obj)

			obj.Name = ""
			obj.Data = obj.Data[:0]

			p.Put(obj)
		}
	})
}

// prof -benchmarks "[BenchmarkGenPoolNoCleanup,BenchmarkGenPoolAggressiveCleanup,BenchmarkGenPoolConservativeCleanup,BenchmarkGenPoolTargetSizeCleanup]" -profiles "[cpu,memory]" -tag "profiling" -count 1

// BenchmarkGenPoolNoCleanup benchmarks the pool with cleanup disabled.
func BenchmarkGenPoolNoCleanup(b *testing.B) {
	cfg := pool.Config[BenchmarkObject, *BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: pool.CleanupPolicy{
			Enabled: false,
		},
	}

	benchmarkPoolWithConfig(b, cfg)
}

// BenchmarkGenPoolAggressiveCleanup benchmarks the pool with aggressive cleanup.
func BenchmarkGenPoolAggressiveCleanup(b *testing.B) {
	cfg := pool.Config[BenchmarkObject, *BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: pool.CleanupPolicy{
			Enabled:       true,
			Interval:      10 * time.Millisecond,
			MinUsageCount: 1,
		},
	}

	benchmarkPoolWithConfig(b, cfg)
}

// BenchmarkGenPoolConservativeCleanup benchmarks the pool with conservative cleanup.
func BenchmarkGenPoolConservativeCleanup(b *testing.B) {
	cfg := pool.Config[BenchmarkObject, *BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: pool.CleanupPolicy{
			Enabled:       true,
			Interval:      5 * time.Minute,
			MinUsageCount: 100,
		},
	}

	benchmarkPoolWithConfig(b, cfg)
}

// BenchmarkGenPoolTargetSizeCleanup benchmarks the pool with target size cleanup.
func BenchmarkGenPoolTargetSizeCleanup(b *testing.B) {
	cfg := pool.Config[BenchmarkObject, *BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: pool.CleanupPolicy{
			Enabled:       true,
			Interval:      1 * time.Second,
			MinUsageCount: 5,
		},
	}

	benchmarkPoolWithConfig(b, cfg)
}

// BenchmarkGenPoolCoreOps benchmarks the pool's core Get/Put overhead and allocation stats with minimal workload.
func BenchmarkGenPoolCoreOps(b *testing.B) {
	var allocs int64
	allocatorWithCount := func() *BenchmarkObject {
		atomic.AddInt64(&allocs, 1)
		return &BenchmarkObject{Name: "coreops"}
	}
	cleanerNoop := func(obj *BenchmarkObject) {
		*obj = BenchmarkObject{}
	}

	cfg := pool.Config[BenchmarkObject, *BenchmarkObject]{
		Allocator: allocatorWithCount,
		Cleaner:   cleanerNoop,
		Cleanup: pool.CleanupPolicy{
			Enabled: false,
		},
	}

	p, err := pool.NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}
	defer p.Close()

	b.Run("Serial", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			obj := p.Get()
			if obj == nil {
				b.Fatal("obj is nil")
			}
			p.Put(obj)
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.SetParallelism(1000)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				obj := p.Get()
				if obj == nil {
					b.Fatal("obj is nil")
				}
				p.Put(obj)
			}
		})
	})

	b.Logf("Total allocations: %d", allocs)
}

// benchmarkPoolWithConfig is a helper function to run benchmarks with a specific config.
func benchmarkPoolWithConfig(b *testing.B, cfg pool.Config[BenchmarkObject, *BenchmarkObject]) {
	p, err := pool.NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}

	defer p.Close()

	b.SetParallelism(1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := p.Get()

			if obj == nil {
				b.Fatal("obj is nil")
			}

			cpuIntensiveWorkload(obj)

			p.Put(obj)
		}
	})
}
