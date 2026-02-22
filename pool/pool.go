// Package pool creates a pool of your objects without additional wrapping and shards the linked list
// based on the number of available logical CPUs. This design improves performance under
// high concurrency and when there is significant work done between getting and returning objects.
package pool

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// Shard represents a single shard in the pool.
// It is 64 bytes in total to avoid false sharing across CPU cache lines.
type Shard[T any, P Poolable[T]] struct {
	Head   atomic.Pointer[T] // 8 bytes
	Single atomic.Pointer[T] // 8 bytes - fast path for single object

	// Padding to avoid false sharing
	_ [128 - unsafe.Sizeof(atomic.Pointer[T]{})*2]byte
}

// ShardedPool is the main pool implementation using sharding for better concurrency.
type ShardedPool[T any, P Poolable[T]] struct {
	// shards is a slice of pool shards, each on its own cache line
	Shards []*Shard[T, P]

	// stopClean signals the cleanup goroutine to stop
	stopClean chan struct{}

	// cleanWg waits for the cleanup goroutine to finish
	cleanWg sync.WaitGroup

	// cfg holds the pool configuration
	cfg Config[T, P]

	// CurrentPoolLength changes at runtime, keeping track of how many uniqe objects have been created
	CurrentPoolLength atomic.Int64
}

// NewPool creates a new sharded pool with the given configuration.
func NewPool[T any, P Poolable[T]](allocator Allocator[T], cleaner Cleaner[T]) (*ShardedPool[T, P], error) {
	return NewPoolWithConfig(DefaultConfig[T, P](allocator, cleaner))
}

// NewPoolWithConfig creates a new sharded pool with the specified configuration.
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

// Get returns an object from the pool or creates a new one.
// Returns nil if MaxPoolSize is set, reached, and no reusable objects are available.
func (p *ShardedPool[T, P]) Get() P {
	shardID := runtimeProcPin()
	shard := p.Shards[shardID]
	runtimeProcUnpin()

	// Fast path: check single object first
	if single := shard.Single.Load(); single != nil {
		if shard.Single.CompareAndSwap(single, nil) {
			P(single).IncrementUsage()
			return single
		}
	}

	// Fast path: try to get object from shard
	for {
		oldHead := P(shard.Head.Load())
		if oldHead == nil {
			break // No objects available, fall through to allocation
		}

		next := oldHead.GetNext()
		if shard.Head.CompareAndSwap(oldHead, next) {
			oldHead.IncrementUsage()
			return oldHead
		}
		// CAS failed, retry
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

// Put returns an object to the pool.
func (p *ShardedPool[T, P]) Put(obj P) {
	p.cfg.Cleaner(obj)

	shardID := obj.GetShardIndex()
	shard := p.Shards[shardID]

	// Fast path: try single object first
	if shard.Single.CompareAndSwap(nil, obj) {
		return
	}

	for {
		oldHead := P(shard.Head.Load())
		obj.SetNext(oldHead) // set before CAS so Get() never sees obj with wrong next (#31, #32)
		if shard.Head.CompareAndSwap(oldHead, obj) {
			return
		}
	}
}

// clear removes all objects from the pool and decrements the pool length accordingly.
func (p *ShardedPool[T, P]) clear() {
	for _, shard := range p.Shards {
		for {
			current := P(shard.Head.Load())
			if current == nil {
				break
			}

			if shard.Head.CompareAndSwap(current, nil) {
				// We have successfully taken the list.
				// Now iterate and clean it.
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
				break // move to next shard
			}
			// Lost the race, try again on the same shard.
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
