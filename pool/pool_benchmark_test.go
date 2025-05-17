package pool

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkObject is a simple struct we'll use for benchmarking
type BenchmarkObject struct {
	Name string
	Data []byte

	next       atomic.Value
	usageCount atomic.Int64
}

func performWorkload(obj *BenchmarkObject) {
	obj.Name = "test"

	// Simulate CPU-intensive work
	for range 1000 {
		obj.Data = append(obj.Data, byte(rand.Intn(256)))
	}

	// Simulate some I/O or network delay
	time.Sleep(time.Microsecond * 100)
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
	return &BenchmarkObject{Name: "test"}
}

func cleanBenchmarkObject(obj *BenchmarkObject) {
	obj.Name = ""
	obj.Data = obj.Data[:0]
}

// BenchmarkPool benchmarks basic Get/Put operations for our pool implementation
// go test -run=^$ -bench=^BenchmarkGenPool$ -benchmem -cpuprofile=cpu.out -memprofile=mem.out -trace=trace.out -mutexprofile=mutex.out
func BenchmarkGenPool(b *testing.B) {
	cfg := PoolConfig[*BenchmarkObject]{
		Allocator: newBenchmarkObject,
		Cleaner:   cleanBenchmarkObject,
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

// BenchmarkSyncPool benchmarks basic Get/Put operations for sync.Pool
// go test -run=^$ -bench=^BenchmarkSyncPool$ -benchmem -cpuprofile=cpu.out -memprofile=mem.out -trace=trace.out -mutexprofile=mutex.out
func BenchmarkSyncPool(b *testing.B) {
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

			performWorkload(obj)

			obj.Name = ""
			obj.Data = obj.Data[:0]

			pool.Put(obj)
		}
	})
}
