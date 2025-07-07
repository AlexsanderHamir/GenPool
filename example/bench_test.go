package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

type Object struct {
	Name       string
	Data       []byte
	usageCount atomic.Int64
	next       atomic.Value
}

func (o *Object) GetNext() pool.Poolable {
	if next := o.next.Load(); next != nil {
		return next.(pool.Poolable)
	}
	return nil
}

func (o *Object) SetNext(next pool.Poolable) {
	o.next.Store(next)
}

func (o *Object) GetUsageCount() int64 {
	return o.usageCount.Load()
}

func (o *Object) IncrementUsage() {
	o.usageCount.Add(1)
}

func (o *Object) ResetUsage() {
	o.usageCount.Store(0)
}

func allocator() *Object {
	return &Object{
		Name: "test",
		Data: make([]byte, 1024),
	}
}

func cleaner(obj *Object) {
	obj.Name = ""
	obj.Data = obj.Data[:0]
}

func createPool() *pool.ShardedPool[*Object] {
	config := pool.PoolConfig[*Object]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup: pool.CleanupPolicy{
			Enabled:       true,
			Interval:      500 * time.Millisecond,
			MinUsageCount: 10,
		},
	}
	benchPool, err := pool.NewPoolWithConfig(config)
	if err != nil {
		panic(err)
	}
	return benchPool
}

func BenchmarkGenPoolHeavy(b *testing.B) {
	p := createPool()

	var wg sync.WaitGroup
	const (
		numGoroutines = 100
		iterations    = 10000
	)

	b.ResetTimer()
	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range iterations {
				obj := p.RetrieveOrCreate()
				obj.Name = "Worker"
				obj.Data = append(obj.Data, byte(j%256))
				p.Put(obj)
			}
		}()
	}
	wg.Wait()
}
