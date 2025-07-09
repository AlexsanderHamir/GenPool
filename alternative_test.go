package main

import (
	"testing"
)

type Object struct {
	Name string
	Data []byte
}

var allocator = func() *Object {
	return &Object{
		Name: "",
		Data: make([]byte, 0, 1024), // Pre-allocate capacity
	}
}

var cleaner = func(obj *Object) {
	obj.Name = ""
	obj.Data = obj.Data[:0] // Reset slice but keep capacity
}

func BenchmarkGenPool(b *testing.B) {
	cfg := PoolConfig[*Object]{
		Allocator: allocator,
		Cleaner:   cleaner,
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		b.Fatalf("error creating pool: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.RetrieveOrCreate()

			if obj == nil {
				b.Fatal("obj is nil")
			}

			pool.Put(obj)
		}
	})
}
