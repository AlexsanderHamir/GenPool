package internal

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkObject is a simple struct we'll use for benchmarking
type BenchmarkObject struct {
	Value      int
	next       atomic.Value
	usageCount atomic.Int64
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
func newBenchmarkObject() *BenchmarkObject {
	return &BenchmarkObject{Value: 42}
}

func cleanBenchmarkObject(obj *BenchmarkObject) {
	obj.Value = 0
}

func doHeavyWork(obj *BenchmarkObject) {
	for range 1000 {
		obj.Value = (obj.Value*31 + 17) % 1000
		if obj.Value%2 == 0 {
			obj.Value = obj.Value * 2
		} else {
			obj.Value = obj.Value * 3
		}
	}
}

// BenchmarkGetPutOurPool benchmarks basic Get/Put operations for our pool implementation
func BenchmarkGetPutOurPool(b *testing.B) {
	cfg := PoolConfig[*BenchmarkObject]{
		Allocator: func() *BenchmarkObject {
			return &BenchmarkObject{Value: 42}
		},
		Cleaner: cleanBenchmarkObject,
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}
	defer pool.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj, err := pool.RetrieveOrCreate()
			if err != nil {
				b.Fatalf("error retrieving object: %v", err)
			}

			if obj == nil {
				b.Fatal("obj is nil")
			}

			// Do some heavy work
			doHeavyWork(obj)

			if err := pool.Put(obj); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkGetPutSyncPool benchmarks basic Get/Put operations for sync.Pool
func BenchmarkGetPutSyncPool(b *testing.B) {
	pool := &sync.Pool{
		New: func() any {
			return newBenchmarkObject()
		},
	}

	b.SetParallelism(1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.Get().(*BenchmarkObject)
			doHeavyWork(obj)

			obj.Value = 0
			pool.Put(obj)
		}
	})
}

// BenchmarkGetPutOurPoolWithAggressiveShrinking benchmarks Get/Put operations with aggressive shrinking
func BenchmarkGetPutOurPoolWithAggressiveShrinking(b *testing.B) {
	cfg := PoolConfig[*BenchmarkObject]{
		Allocator: func() *BenchmarkObject {
			return &BenchmarkObject{Value: 42}
		},
		Cleaner: cleanBenchmarkObject,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      100 * time.Millisecond, // Aggressive cleanup interval
			MinUsageCount: 2,                      // Objects used less than 2 times will be cleaned
			TargetSize:    10,                     // Try to keep pool size around 10 objects
		},
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}
	defer pool.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj, err := pool.RetrieveOrCreate()
			if err != nil {
				b.Fatalf("error retrieving object: %v", err)
			}

			if obj == nil {
				b.Fatal("obj is nil")
			}

			// Do some heavy work
			doHeavyWork(obj)

			if err := pool.Put(obj); err != nil {
				b.Fatal(err)
			}
		}
	})
}
