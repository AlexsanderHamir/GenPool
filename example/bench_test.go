package main

import (
	"sync"
	"testing"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

type Object struct {
	Name string
	Data []byte

	pool.PoolFields[Object]
}

func allocator() *Object {
	return &Object{
		Name: "test",
		Data: make([]byte, 1024),
	}
}

func cleaner(obj *Object) {
	// manual
	obj.Name = ""
	obj.Data = obj.Data[:0]

	// or simply fo this, look at the link below for any doubts:
	// https://www.reddit.com/r/golang/comments/1lvjmar/comment/n2ekhq5
	*obj = Object{}

}

func createPool() *pool.ShardedPool[Object, *Object] {
	config := pool.PoolConfig[Object, *Object]{
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
	defer p.Close()

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
