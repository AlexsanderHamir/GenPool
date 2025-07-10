// Package alternative is for anybody that doesn't like the intrusive style here's an alternative,
// feel free to improve it and benchmark it to see if it matches your desired performance for your use case.
package alternative

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Common errors that may be returned by the pool.
var (
	// ErrNoAllocator is returned when attempting to get an object but no allocator is configured.
	ErrNoAllocator = errors.New("no allocator configured")

	// ErrNoCleaner is returned when attempting to create a pool but no cleaner is configured.
	ErrNoCleaner = errors.New("no cleaner configured")
)

// numShards attempts to get the approximate number of shards that is fitting for your CPU.
// It will not work well if you start with 2 logical cores and gradually move to 64.
var numShards = min(max(runtime.GOMAXPROCS(0), 8), 128)

// CleanupPolicy defines how the pool should clean up unused objects.
type CleanupPolicy struct {
	// Enabled determines if automatic cleanup is enabled
	Enabled bool
	// Interval is how often the cleanup should run
	Interval time.Duration
	// MinUsageCount is the number of usage below which an object will be evicted
	MinUsageCount int64
}

// DefaultCleanupPolicy returns a default cleanup configuration.
func DefaultCleanupPolicy() CleanupPolicy {
	return CleanupPolicy{
		Enabled:       false,
		Interval:      5 * time.Minute,
		MinUsageCount: 1,
	}
}

// Allocator is a function type that creates new objects for the pool.
type Allocator[T any] func() *T

// Cleaner is a function type that cleans up objects before they are returned to the pool.
type Cleaner[T any] func(*T)

// PoolObject wraps any type to make it suitable for pooling.
type PoolObject[T any] struct {
	Inner      *T
	usageCount atomic.Int64
	next       atomic.Pointer[PoolObject[T]]
}

// GetNext returns the next object in the pool's linked list.
func (p *PoolObject[T]) GetNext() *PoolObject[T] {
	if next := p.next.Load(); next != nil {
		return next
	}
	return nil
}

// SetNext sets the next object in the pool's linked list.
func (p *PoolObject[T]) SetNext(next *PoolObject[T]) {
	p.next.Store(next)
}

// GetUsageCount returns the number of times this object has been used.
func (p *PoolObject[T]) GetUsageCount() int64 {
	return p.usageCount.Load()
}

// IncrementUsage increments the usage count of this object.
func (p *PoolObject[T]) IncrementUsage() {
	p.usageCount.Add(1)
}

// ResetUsage resets the usage count to 0.
func (p *PoolObject[T]) ResetUsage() {
	p.usageCount.Store(0)
}

// PoolConfig holds configuration options for the pool.
type PoolConfig[T any] struct {
	// Cleanup defines the cleanup policy for the pool
	Cleanup CleanupPolicy
	// Allocator is the function to create new objects
	Allocator Allocator[T]
	// Cleaner is the function to clean objects before returning them to the pool
	Cleaner Cleaner[T]
}

// DefaultConfig returns a default pool configuration for type T.
func DefaultConfig[T any](allocator Allocator[T], cleaner Cleaner[T]) PoolConfig[T] {
	return PoolConfig[T]{
		Cleanup:   DefaultCleanupPolicy(),
		Allocator: allocator,
		Cleaner:   cleaner,
	}
}

// PoolShard represents a single shard in the pool.
type PoolShard[T any] struct {
	// head is the head of the linked list for this shard
	head atomic.Pointer[PoolObject[T]]

	// pad ensures each shard is on its own cache line
	_ [64 - unsafe.Sizeof(atomic.Pointer[PoolObject[T]]{})%64]byte
}

// ShardedPool is the main pool implementation using sharding for better concurrency.
type ShardedPool[T any] struct {
	// shards is a slice of pool shards, each on its own cache line
	shards []*PoolShard[T]

	// stopClean signals the cleanup goroutine to stop
	stopClean chan struct{}

	// cleanWg waits for the cleanup goroutine to finish
	cleanWg sync.WaitGroup

	// cfg holds the pool configuration
	cfg PoolConfig[T]
}

// NewPool creates a new sharded pool with the given configuration.
func NewPool[T any](allocator Allocator[T], cleaner Cleaner[T]) (*ShardedPool[T], error) {
	return NewPoolWithConfig(DefaultConfig(allocator, cleaner))
}

// NewPoolWithConfig creates a new sharded pool with the specified configuration.
func NewPoolWithConfig[T any](cfg PoolConfig[T]) (*ShardedPool[T], error) {
	if cfg.Allocator == nil {
		return nil, fmt.Errorf("%w: allocator is required", ErrNoAllocator)
	}
	if cfg.Cleaner == nil {
		return nil, fmt.Errorf("%w: cleaner is required", ErrNoCleaner)
	}

	p := &ShardedPool[T]{
		cfg:       cfg,
		stopClean: make(chan struct{}),
		shards:    make([]*PoolShard[T], numShards),
	}

	for i := range p.shards {
		p.shards[i] = &PoolShard[T]{}
		p.shards[i].head.Store(nil)
	}

	if cfg.Cleanup.Enabled {
		if cfg.Cleanup.Interval <= 0 {
			return nil, errors.New("cleanup interval must be greater than 0")
		}
		if cfg.Cleanup.MinUsageCount <= 0 {
			return nil, errors.New("minimum usage count must be greater than 0")
		}
		p.startCleaner()
	}

	return p, nil
}

// getShard returns the shard for the current goroutine.
func (p *ShardedPool[T]) getShard() *PoolShard[T] {
	// Use goroutine's processor ID for shard selection.
	// This provides better locality for goroutines that frequently access the pool.
	id := runtime_procPin()
	runtime_procUnpin()

	return p.shards[id%numShards] // ensure we don't get "index out of bounds error" if number of P's changes.
}

// RetrieveOrCreate gets an object from the pool or creates a new one.
func (p *ShardedPool[T]) RetrieveOrCreate() *T {
	shard := p.getShard()

	// Try to get an object from the shard
	if obj := p.retrieveFromShard(shard); obj != nil {
		obj.IncrementUsage()
		return obj.Inner
	}

	// Create a new object if none available
	return p.cfg.Allocator()
}

// Put returns an object to the pool.
func (p *ShardedPool[T]) Put(obj *T) {
	p.cfg.Cleaner(obj)

	// Wrap the object in a PoolObject
	poolObj := &PoolObject[T]{
		Inner: obj,
	}

	shard := p.getShard()

	for {
		oldHead := shard.head.Load()
		poolObj.SetNext(oldHead)

		if shard.head.CompareAndSwap(oldHead, poolObj) {
			return
		}
	}
}

// retrieveFromShard gets an object from a specific shard.
func (p *ShardedPool[T]) retrieveFromShard(shard *PoolShard[T]) *PoolObject[T] {
	for {
		oldHead := shard.head.Load()
		if oldHead == nil {
			return nil
		}

		next := oldHead.GetNext()
		if shard.head.CompareAndSwap(oldHead, next) {
			return oldHead
		}
	}
}

// Clear removes all objects from the pool.
func (p *ShardedPool[T]) clear() {
	for _, shard := range p.shards {
		for {
			current := shard.head.Load()
			if current == nil {
				break
			}

			if shard.head.CompareAndSwap(current, nil) {
				// We have successfully taken the list.
				// Now iterate and clean it.
				for current != nil {
					next := current.GetNext()
					current.SetNext(nil)
					p.cfg.Cleaner(current.Inner)
					current = next
				}
				break // move to next shard
			}
			// Lost the race, try again on the same shard.
		}
	}
}

// startCleaner starts the background cleanup goroutine.
func (p *ShardedPool[T]) startCleaner() {
	p.cleanWg.Add(1)
	go func() {
		defer p.cleanWg.Done()
		ticker := time.NewTicker(p.cfg.Cleanup.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.cleanup()
			case <-p.stopClean:
				return
			}
		}
	}()
}

// cleanup removes idle objects based on the [CleanupPolicy].
func (p *ShardedPool[T]) cleanup() {
	if !p.cfg.Cleanup.Enabled {
		return
	}

	for _, shard := range p.shards {
		p.cleanupShard(shard)
	}
}

func (p *ShardedPool[T]) cleanupShard(shard *PoolShard[T]) {
	// Atomically take the entire list from the shard.
	oldHead := shard.head.Load()
	if oldHead == nil {
		return
	}

	if !shard.head.CompareAndSwap(oldHead, nil) {
		// The list was modified by another goroutine. We'll just skip this cleanup cycle for this shard
		// and try again on the next tick. This is a simple, low-contention strategy.
		return
	}

	// We now have exclusive ownership of the list starting at oldHead.
	current := oldHead
	var keptHead, keptTail *PoolObject[T]

	for current != nil {
		next := current.GetNext()
		usageCount := current.GetUsageCount()

		if usageCount >= p.cfg.Cleanup.MinUsageCount {
			// This item is kept.
			current.ResetUsage()
			current.SetNext(nil) // Clear the next pointer

			if keptHead == nil {
				keptHead = current
			} else {
				keptTail.SetNext(current)
			}
			keptTail = current
		} else {
			// This item is discarded
			current.SetNext(nil)
		}
		current = next
	}

	// If any items were kept, we need to add them back to the shard's list.
	if keptHead != nil {
		// Atomically prepend the list of kept items to the shard's current list.
		for {
			currentHead := shard.head.Load()
			keptTail.SetNext(currentHead)

			if shard.head.CompareAndSwap(currentHead, keptHead) {
				break
			}
			// Contention: The head of the shard's list was modified. Retry.
		}
	}
}

// Close stops the cleanup goroutine and clears the pool.
func (p *ShardedPool[T]) Close() {
	if p.cfg.Cleanup.Enabled {
		close(p.stopClean)
		p.cleanWg.Wait()
		p.clear()
	}
}

//go:linkname runtime_procPin runtime.procPin
func runtime_procPin() int

//go:linkname runtime_procUnpin runtime.procUnpin
func runtime_procUnpin()
