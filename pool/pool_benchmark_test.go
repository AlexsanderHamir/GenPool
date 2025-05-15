package pool

import (
	"sync"
	"sync/atomic"
	"testing"
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

	b.SetParallelism(1000)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.RetrieveOrCreate()

			if obj == nil {
				b.Fatal("obj is nil")
			}

			// Do some heavy work
			doHeavyWork(obj)

			pool.Put(obj)
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

			if obj == nil {
				b.Fatal("obj is nil")
			}

			doHeavyWork(obj)

			obj.Value = 0
			pool.Put(obj)
		}
	})
}
