package pool

import (
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkObject is a simple struct we'll use for benchmarking
type BenchmarkObject struct {
	// user fields
	Name string   // 16 bytes
	Data []byte   // 24 bytes
	_    [24]byte // 24 bytes = 64 bytes

	// interface necessary fields (kept together since they're modified together)
	usageCount atomic.Int64 // 8 bytes
	next       atomic.Value // 16 bytes
	_          [40]byte     // 40 bytes padding to make struct 128 bytes (2 cache lines)
}

func performWorkload(obj *BenchmarkObject) {
	obj.Name = "test"

	// Simulate CPU-intensive work
	for range 1000 {
		obj.Data = append(obj.Data, byte(rand.Intn(256)))
	}

	// Simulate some I/O or network delay
	time.Sleep(time.Microsecond * 10)
}

func (o *BenchmarkObject) GetNext() Poolable {
	if next := o.next.Load(); next != nil {
		return next.(Poolable)
	}
	return nil
}

func (o *BenchmarkObject) SetNext(next Poolable) {
	o.next.Store(next)
}

func (o *BenchmarkObject) GetUsageCount() int64 {
	return o.usageCount.Load()
}

func (o *BenchmarkObject) IncrementUsage() {
	o.usageCount.Add(1)
}

func (o *BenchmarkObject) ResetUsage() {
	o.usageCount.Store(0)
}

// Helper functions for benchmarks
func allocator() *BenchmarkObject {
	return &BenchmarkObject{Name: "test"}
}

func cleaner(obj *BenchmarkObject) {
	obj.Name = ""
	obj.Data = obj.Data[:0]
}

func BenchmarkGenPool(b *testing.B) {
	runtime.SetBlockProfileRate(1)
	cfg := PoolConfig[*BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}

	b.SetParallelism(1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.RetrieveOrCreate()

			if obj == nil {
				b.Fatal("obj is nil")
			}

			performWorkload(obj)

			pool.Put(obj)
		}
	})
}
func BenchmarkSyncPool(b *testing.B) {
	runtime.SetBlockProfileRate(1)
	pool := &sync.Pool{
		New: func() any {
			return allocator()
		},
	}

	b.SetParallelism(1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.Get().(*BenchmarkObject)

			if obj == nil {
				b.Fatal("obj is nil")
			}

			performWorkload(obj)

			obj.Name = ""
			obj.Data = obj.Data[:0]

			pool.Put(obj)
		}
	})
}

// prof -benchmarks "[BenchmarkGenPoolNoCleanup,BenchmarkGenPoolAggressiveCleanup,BenchmarkGenPoolConservativeCleanup,BenchmarkGenPoolTargetSizeCleanup]" -profiles "[cpu,memory]" -tag "profiling" -count 1

// BenchmarkGenPoolNoCleanup benchmarks the pool with cleanup disabled
func BenchmarkGenPoolNoCleanup(b *testing.B) {
	cfg := PoolConfig[*BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: CleanupPolicy{
			Enabled: false,
		},
	}

	benchmarkPoolWithConfig(b, cfg)
}

// BenchmarkGenPoolAggressiveCleanup benchmarks the pool with aggressive cleanup
func BenchmarkGenPoolAggressiveCleanup(b *testing.B) {
	cfg := PoolConfig[*BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      500 * time.Millisecond,
			MinUsageCount: 1,
		},
	}

	benchmarkPoolWithConfig(b, cfg)
}

// BenchmarkGenPoolConservativeCleanup benchmarks the pool with conservative cleanup
func BenchmarkGenPoolConservativeCleanup(b *testing.B) {
	cfg := PoolConfig[*BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      5 * time.Minute,
			MinUsageCount: 100,
		},
	}

	benchmarkPoolWithConfig(b, cfg)
}

// BenchmarkGenPoolTargetSizeCleanup benchmarks the pool with target size cleanup
func BenchmarkGenPoolTargetSizeCleanup(b *testing.B) {
	cfg := PoolConfig[*BenchmarkObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      1 * time.Second,
			MinUsageCount: 5,
		},
	}

	benchmarkPoolWithConfig(b, cfg)
}

// benchmarkPoolWithConfig is a helper function to run benchmarks with a specific config
func benchmarkPoolWithConfig(b *testing.B, cfg PoolConfig[*BenchmarkObject]) {
	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}

	b.SetParallelism(1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.RetrieveOrCreate()

			if obj == nil {
				b.Fatal("obj is nil")
			}

			performWorkload(obj)

			pool.Put(obj)
		}
	})
}
