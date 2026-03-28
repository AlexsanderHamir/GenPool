package test

import (
	"sync"
	"testing"

	"github.com/AlexsanderHamir/GenPool/pool"
)

const (
	// benchParallelism is the RunParallel goroutine count. Raises contention on both pools;
	// GenPool is sharded by GOMAXPROCS, so this value strongly affects relative behavior.
	benchParallelism = 1000
)

// benchScenario is one row in the comparative matrix (same for GenPool and sync.Pool).
type benchScenario struct {
	name        string
	innerIters  int
	appendCount int
}

// Scenarios from pool-bound (read: overhead / weakness vs sync) to compute-bound.
var benchScenarios = []benchScenario{
	{name: "pool_only", innerIters: 0, appendCount: 0},
	{name: "low", innerIters: 500, appendCount: 32},
	{name: "medium", innerIters: 10_000, appendCount: 100},
	{name: "high", innerIters: 100_000, appendCount: 256},
	{name: "extreme", innerIters: 1_000_000, appendCount: 256},
}

// BenchmarkObject models a medium-sized pooled value (fields + embedded pool metadata).
type BenchmarkObject struct {
	Name   string
	Data   []byte
	Result int64
	_      [16]byte

	pool.Fields[BenchmarkObject]
}

// betweenGetAndPut is identical user work for both pools (deterministic, allocation-light
// once slice capacity is warm).
func betweenGetAndPut(obj *BenchmarkObject, innerIters, appendCount int) {
	obj.Name = "cpu_test"

	var result int64
	for i := range innerIters {
		result += int64(i * i * i)
		result ^= int64(i << 3)
		if i%1000 == 0 {
			result = result*31 + int64(i)
		}
	}
	obj.Result = result

	if cap(obj.Data) < appendCount {
		obj.Data = make([]byte, 0, appendCount)
	}
	obj.Data = obj.Data[:0]

	for i := range appendCount {
		obj.Data = append(obj.Data, byte(result>>uint(i%8)))
	}
}

func benchAllocator() *BenchmarkObject {
	return &BenchmarkObject{Name: "test"}
}

func benchCleaner(obj *BenchmarkObject) {
	obj.Name = ""
	obj.Data = obj.Data[:0]
}

func resetLikeCleaner(obj *BenchmarkObject) {
	obj.Name = ""
	obj.Data = obj.Data[:0]
}

func newGenPoolForBench(b *testing.B) (*pool.ShardedPool[BenchmarkObject, *BenchmarkObject], func()) {
	b.Helper()
	cfg := pool.Config[BenchmarkObject, *BenchmarkObject]{
		Allocator: benchAllocator,
		Cleaner:   benchCleaner,
	}
	p, err := pool.NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("GenPool: %v", err)
	}
	return p, func() { p.Close() }
}

func newSyncPoolForBench() *sync.Pool {
	return &sync.Pool{New: func() any { return benchAllocator() }}
}

func BenchmarkGenPool(b *testing.B) {
	for _, sc := range benchScenarios {
		b.Run(sc.name, func(b *testing.B) {
			p, cleanup := newGenPoolForBench(b)
			defer cleanup()

			b.SetParallelism(benchParallelism)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					obj := p.Get()
					betweenGetAndPut(obj, sc.innerIters, sc.appendCount)
					p.Put(obj)
				}
			})
		})
	}
}

func BenchmarkSyncPool(b *testing.B) {
	for _, sc := range benchScenarios {
		b.Run(sc.name, func(b *testing.B) {
			p := newSyncPoolForBench()

			b.SetParallelism(benchParallelism)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					obj := p.Get().(*BenchmarkObject)
					betweenGetAndPut(obj, sc.innerIters, sc.appendCount)
					resetLikeCleaner(obj)
					p.Put(obj)
				}
			})
		})
	}
}
