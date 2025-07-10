package pool

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

// Poolable is an interface that objects must implement to be stored in the pool.
type Poolable[T any] interface {
	*T
	// GetNext returns the next object in the pool's linked list
	GetNext() *T
	// SetNext sets the next object in the pool's linked list
	SetNext(next *T)
	// GetUsageCount returns the number of times this object has been used
	GetUsageCount() int64
	// IncrementUsage increments the usage count of this object
	IncrementUsage()
	// ResetUsage resets the usage count to 0
	ResetUsage()
}

// PoolConfig holds configuration options for the pool.
type PoolConfig[T any, P Poolable[T]] struct {
	// Cleanup defines the cleanup policy for the pool
	Cleanup CleanupPolicy
	// Allocator is the function to create new objects
	Allocator Allocator[T]
	// Cleaner is the function to clean objects before returning them to the pool
	Cleaner Cleaner[T]
}

// DefaultConfig returns a default pool configuration for type T.
func DefaultConfig[T any, P Poolable[T]](allocator Allocator[T], cleaner Cleaner[T]) PoolConfig[T, P] {
	return PoolConfig[T, P]{
		Cleanup:   DefaultCleanupPolicy(),
		Allocator: allocator,
		Cleaner:   cleaner,
	}
}

// PoolShard represents a single shard in the pool.
type PoolShard[T any, P Poolable[T]] struct {
	// head is the head of the linked list for this shard
	head atomic.Pointer[T]

	// pad ensures each shard is on its own cache line
	_ [64 - unsafe.Sizeof(atomic.Pointer[T]{})%64]byte
}

// ShardedPool is the main pool implementation using sharding for better concurrency.
type ShardedPool[T any, P Poolable[T]] struct {
	// shards is a slice of pool shards, each on its own cache line
	shards []*PoolShard[T, P]

	// stopClean signals the cleanup goroutine to stop
	stopClean chan struct{}

	// cleanWg waits for the cleanup goroutine to finish
	cleanWg sync.WaitGroup

	// cfg holds the pool configuration
	cfg PoolConfig[T, P]
}

// NewPool creates a new sharded pool with the given configuration.
func NewPool[T any, P Poolable[T]](allocator Allocator[T], cleaner Cleaner[T]) (*ShardedPool[T, P], error) {
	return NewPoolWithConfig(DefaultConfig[T, P](allocator, cleaner))
}

// NewPoolWithConfig creates a new sharded pool with the specified configuration.
func NewPoolWithConfig[T any, P Poolable[T]](cfg PoolConfig[T, P]) (*ShardedPool[T, P], error) {
	if cfg.Allocator == nil {
		return nil, fmt.Errorf("%w: allocator is required", ErrNoAllocator)
	}
	if cfg.Cleaner == nil {
		return nil, fmt.Errorf("%w: cleaner is required", ErrNoCleaner)
	}

	p := &ShardedPool[T, P]{
		cfg:       cfg,
		stopClean: make(chan struct{}),
		shards:    make([]*PoolShard[T, P], numShards),
	}

	for i := range p.shards {
		p.shards[i] = &PoolShard[T, P]{}
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
func (p *ShardedPool[T, P]) getShard() *PoolShard[T, P] {
	// Use goroutine's processor ID for shard selection
	// This provides better locality for goroutines that frequently access the pool
	id := runtime_procPin()
	runtime_procUnpin()

	return p.shards[id%numShards] // ensure we don't get "index out of bounds error" if number of P's changes
}

// RetrieveOrCreate gets an object from the pool or creates a new one.
func (p *ShardedPool[T, P]) RetrieveOrCreate() P {
	shard := p.getShard()

	// Try to get an object from the shard
	if obj, ok := p.retrieveFromShard(shard); ok {
		obj.IncrementUsage()
		return obj
	}

	// Create a new object if none available
	obj := P(p.cfg.Allocator())
	obj.IncrementUsage()
	return obj
}

// Put returns an object to the pool.
func (p *ShardedPool[T, P]) Put(obj P) {
	p.cfg.Cleaner(obj)
	shard := p.getShard()

	for {
		oldHead := P(shard.head.Load())

		if shard.head.CompareAndSwap(oldHead, obj) {
			obj.SetNext(oldHead)
			return
		}
	}
}

// retrieveFromShard gets an object from a specific shard.
func (p *ShardedPool[T, P]) retrieveFromShard(shard *PoolShard[T, P]) (zero P, success bool) {
	for {
		oldHead := P(shard.head.Load())
		if oldHead == nil {
			return zero, false
		}

		next := oldHead.GetNext()
		if shard.head.CompareAndSwap(oldHead, next) {
			return oldHead, true
		}
	}
}

// Clear removes all objects from the pool.
func (p *ShardedPool[T, P]) clear() {
	for _, shard := range p.shards {
		for {
			current := P(shard.head.Load())
			if current == nil {
				break
			}

			if shard.head.CompareAndSwap(current, nil) {
				// We have successfully taken the list.
				// Now iterate and clean it.
				for current != nil {
					next := current.GetNext()
					current.SetNext(nil)
					p.cfg.Cleaner(current)
					current = next
				}
				break // move to next shard
			}
			// Lost the race, try again on the same shard.
		}
	}
}

// startCleaner starts the background cleanup goroutine.
func (p *ShardedPool[T, P]) startCleaner() {
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
func (p *ShardedPool[T, P]) cleanup() {
	if !p.cfg.Cleanup.Enabled {
		return
	}

	for _, shard := range p.shards {
		p.cleanupShard(shard)
	}
}

func (p *ShardedPool[T, P]) cleanupShard(shard *PoolShard[T, P]) {
	// Atomically take the entire list from the shard.
	oldHead := P(shard.head.Load())
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
	var keptHead P

	keptTail := P(new(T))

	for current != nil {
		next := current.GetNext()
		usageCount := current.GetUsageCount()

		if usageCount >= p.cfg.Cleanup.MinUsageCount {
			// This item is kept.
			current.ResetUsage()
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
		keptTail.SetNext(nil) // Terminate our list of kept items.

		// Atomically prepend the list of kept items to the shard's current list.
		for {
			currentHead := P(shard.head.Load())
			var nextForTail P
			if currentHead != nil {
				nextForTail = currentHead
			}
			keptTail.SetNext(nextForTail)

			if shard.head.CompareAndSwap(currentHead, keptHead) {
				break
			}
			// Contention: The head of the shard's list was modified. Retry.
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

//go:linkname runtime_procPin runtime.procPin
func runtime_procPin() int

//go:linkname runtime_procUnpin runtime.procUnpin
func runtime_procUnpin()
