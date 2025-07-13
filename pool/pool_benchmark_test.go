package pool_test

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// BenchmarkObject is a simple struct we'll use for benchmarking.
type BenchmarkObject struct {
	// user fields
	Name string   // 16 bytes
	Data []byte   // 24 bytes
	_    [24]byte // 24 bytes = 64 bytes

	pool.PoolFields[BenchmarkObject]
}

func highLatencyWorkload(obj *BenchmarkObject) {
	obj.Name = "test"

	// Simulate heavy CPU work
	for range 10_000 {
		obj.Data = append(obj.Data, rand.N[byte](255))
	}

	// Simulate high I/O or network delay
	time.Sleep(10 * time.Millisecond)
}

func moderateLatencyWorkload(obj *BenchmarkObject) {
	obj.Name = "test"

	// Simulate moderate CPU work
	for i := 0; i < 1_000; i++ {
		obj.Data = append(obj.Data, rand.N[byte](255))
	}

	// Simulate moderate delay
	time.Sleep(100 * time.Microsecond)
}

func lowLatencyWorkload(obj *BenchmarkObject) {
	obj.Name = "test"

	// Simulate light CPU work
	for i := 0; i < 100; i++ {
		obj.Data = append(obj.Data, rand.N[byte](255))
	}

	// Simulate minimal delay
	time.Sleep(5 * time.Microsecond)
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
	cfg := pool.PoolConfig[BenchmarkObject, *BenchmarkObject]{
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

			lowLatencyWorkload(obj)

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

			lowLatencyWorkload(obj)

			obj.Name = ""
			obj.Data = obj.Data[:0]

			p.Put(obj)
		}
	})
}

// prof -benchmarks "[BenchmarkGenPoolNoCleanup,BenchmarkGenPoolAggressiveCleanup,BenchmarkGenPoolConservativeCleanup,BenchmarkGenPoolTargetSizeCleanup]" -profiles "[cpu,memory]" -tag "profiling" -count 1

// BenchmarkGenPoolNoCleanup benchmarks the pool with cleanup disabled.
func BenchmarkGenPoolNoCleanup(b *testing.B) {
	cfg := pool.PoolConfig[BenchmarkObject, *BenchmarkObject]{
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
	cfg := pool.PoolConfig[BenchmarkObject, *BenchmarkObject]{
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
	cfg := pool.PoolConfig[BenchmarkObject, *BenchmarkObject]{
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
	cfg := pool.PoolConfig[BenchmarkObject, *BenchmarkObject]{
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

// benchmarkPoolWithConfig is a helper function to run benchmarks with a specific config.
func benchmarkPoolWithConfig(b *testing.B, cfg pool.PoolConfig[BenchmarkObject, *BenchmarkObject]) {
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

			highLatencyWorkload(obj)

			p.Put(obj)
		}
	})
}
