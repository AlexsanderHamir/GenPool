// Package pool provides a sharded object pool without extra wrapping. Sharding is
// by GOMAXPROCS for better concurrency when there is significant work between Get and Put.
//
// This file defines the pool types (Shard, ShardedPool), construction (NewPool,
// NewPoolWithConfig), Get/Put, clear/Close, and runtime proc pinning linknames.
package pool

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// Shard is a single pool shard; padding avoids false sharing across cache lines.
type Shard[T any, P Poolable[T]] struct {
	Head   atomic.Pointer[T]
	Single atomic.Pointer[T]

	_ [128 - unsafe.Sizeof(atomic.Pointer[T]{})*2]byte
}

// ShardedPool is the main pool implementation.
type ShardedPool[T any, P Poolable[T]] struct {
	Shards []*Shard[T, P]

	stopClean chan struct{}
	cleanWg   sync.WaitGroup
	cfg       Config[T, P]

	CurrentPoolLength atomic.Int64
}

// NewPool creates a sharded pool with the given allocator and cleaner.
func NewPool[T any, P Poolable[T]](allocator Allocator[T], cleaner Cleaner[T]) (*ShardedPool[T, P], error) {
	return NewPoolWithConfig(DefaultConfig[T, P](allocator, cleaner))
}

// NewPoolWithConfig creates a sharded pool with the given config.
func NewPoolWithConfig[T any, P Poolable[T]](cfg Config[T, P]) (*ShardedPool[T, P], error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	pool := &ShardedPool[T, P]{
		cfg:       cfg,
		stopClean: make(chan struct{}),
		Shards:    make([]*Shard[T, P], numShards),
	}

	initShards(pool)

	if cfg.Cleanup.Enabled {
		if err := validateCleanupConfig(cfg); err != nil {
			return nil, err
		}
		pool.startCleaner()
	}

	return pool, nil
}

func initShards[T any, P Poolable[T]](p *ShardedPool[T, P]) {
	for i := range p.Shards {
		shard := &Shard[T, P]{}
		shard.Head.Store(nil)
		shard.Single.Store(nil)

		p.Shards[i] = shard
	}
}

// Get returns an object from the pool or allocates a new one. Returns nil if
// MaxPoolSize is set, reached, and no reusable object is available.
func (p *ShardedPool[T, P]) Get() P {
	shardID := runtimeProcPin()
	shard := p.Shards[shardID]
	runtimeProcUnpin()

	if single := shard.Single.Load(); single != nil {
		if shard.Single.CompareAndSwap(single, nil) {
			P(single).IncrementUsage()
			return single
		}
	}

	for {
		oldHead := P(shard.Head.Load())
		if oldHead == nil {
			break
		}

		next := oldHead.GetNext()
		if shard.Head.CompareAndSwap(oldHead, next) {
			oldHead.IncrementUsage()
			return oldHead
		}
	}

	// Direct allocation path
	if !p.cfg.Growth.Enable {
		obj := P(p.cfg.Allocator())
		obj.SetShardIndex(shardID)
		obj.IncrementUsage()
		p.CurrentPoolLength.Add(1)
		return obj
	}

	if p.CurrentPoolLength.Load() >= p.cfg.Growth.MaxPoolSize {
		return nil
	}

	obj := P(p.cfg.Allocator())
	obj.SetShardIndex(shardID)
	obj.IncrementUsage()
	p.CurrentPoolLength.Add(1)
	return obj
}

func (p *ShardedPool[T, P]) Put(obj P) {
	p.cfg.Cleaner(obj)

	shardID := obj.GetShardIndex()
	shard := p.Shards[shardID]

	if shard.Single.CompareAndSwap(nil, obj) {
		return
	}

	for {
		oldHead := P(shard.Head.Load())
		obj.SetNext(oldHead) // before CAS so Get never sees wrong next (#31, #32)
		if shard.Head.CompareAndSwap(oldHead, obj) {
			return
		}
	}
}

// clear removes all objects from the pool and updates CurrentPoolLength.
func (p *ShardedPool[T, P]) clear() {
	for _, shard := range p.Shards {
		for {
			current := P(shard.Head.Load())
			if current == nil {
				break
			}

			if shard.Head.CompareAndSwap(current, nil) {
				removedCount := int64(0)
				for current != nil {
					next := current.GetNext()
					current.SetNext(nil)
					p.cfg.Cleaner(current)
					removedCount++
					current = next
				}
				if removedCount > 0 {
					p.CurrentPoolLength.Add(-removedCount)
				}
				break
			}
		}
	}
}

// Close stops the cleanup goroutine and clears the pool.
func (p *ShardedPool[T, P]) Close() {
	if p.cfg.Cleanup.Enabled {
		close(p.stopClean)
		p.cleanWg.Wait()
		p.clear()
	}
}

//go:linkname runtimeProcPin runtime.procPin
func runtimeProcPin() int

//go:linkname runtimeProcUnpin runtime.procUnpin
func runtimeProcUnpin()
