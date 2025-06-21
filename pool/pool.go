package pool

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Common errors that may be returned by the pool
var (
	// ErrNonPointerType is returned when attempting to create a pool with a non-pointer type
	ErrNonPointerType = fmt.Errorf("type must be a pointer type")

	// ErrNoAllocator is returned when attempting to get an object but no allocator is configured
	ErrNoAllocator = fmt.Errorf("no allocator configured")

	// ErrNoCleaner is returned when attempting to create a pool but no cleaner is configured
	ErrNoCleaner = fmt.Errorf("no cleaner configured")

	// ErrCleanerFailed is returned when the cleaner function fails to clean an object
	ErrCleanerFailed = fmt.Errorf("cleaner failed")
)

var (
	numShards = runtime.GOMAXPROCS(0)
)

// CleanupPolicy defines how the pool should clean up unused objects
type CleanupPolicy struct {
	// Enabled determines if automatic cleanup is enabled
	Enabled bool
	// Interval is how often the cleanup should run
	Interval time.Duration
	// MinUsageCount is the number of usage below which an object will be evicted
	MinUsageCount int64
	// TargetSize is the target number of objects to attempt to keep during cleanup
	// If 0, no target size is enforced
	TargetSize int
}

// DefaultCleanupPolicy returns a default cleanup configuration
func DefaultCleanupPolicy() CleanupPolicy {
	return CleanupPolicy{
		Enabled:       false,
		Interval:      5 * time.Minute,
		MinUsageCount: 1,
		TargetSize:    0,
	}
}

// Allocator is a function type that creates new objects for the pool
type Allocator[T any] func() T

// Cleaner is a function type that cleans up objects before they are returned to the pool
type Cleaner[T any] func(T)

// Poolable is an interface that objects must implement to be stored in the pool
type Poolable interface {
	// GetNext returns the next object in the pool's linked list
	GetNext() Poolable
	// SetNext sets the next object in the pool's linked list
	SetNext(next Poolable)
	// GetUsageCount returns the number of times this object has been used
	GetUsageCount() int64
	// IncrementUsage increments the usage count of this object
	IncrementUsage()
	// ResetUsage resets the usage count to 0
	ResetUsage()
}

// PoolConfig holds configuration options for the pool
type PoolConfig[T Poolable] struct {
	// Cleanup defines the cleanup policy for the pool
	Cleanup CleanupPolicy
	// Allocator is the function to create new objects
	Allocator Allocator[T]
	// Cleaner is the function to clean objects before returning them to the pool
	Cleaner Cleaner[T]
}

// DefaultConfig returns a default pool configuration for type T
func DefaultConfig[T Poolable](allocator Allocator[T], cleaner Cleaner[T]) PoolConfig[T] {
	return PoolConfig[T]{
		Cleanup:   DefaultCleanupPolicy(),
		Allocator: allocator,
		Cleaner:   cleaner,
	}
}

// PoolShard represents a single shard in the pool
type PoolShard[T Poolable] struct {
	// head is the head of the linked list for this shard
	head atomic.Value // T

	// pad ensures each shard is on its own cache line
	_ [64 - unsafe.Sizeof(atomic.Value{})%64]byte
}

// ShardedPool is the main pool implementation using sharding for better concurrency
type ShardedPool[T Poolable] struct {
	// shards is a slice of pool shards, each on its own cache line
	shards []*PoolShard[T]

	// stopClean signals the cleanup goroutine to stop
	stopClean chan struct{}

	// cleanWg waits for the cleanup goroutine to finish
	cleanWg sync.WaitGroup

	// cfg holds the pool configuration
	cfg PoolConfig[T]
}

// NewPool creates a new sharded pool with the given configuration
func NewPool[T Poolable](allocator Allocator[T], cleaner Cleaner[T]) (*ShardedPool[T], error) {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("%w, got %T", ErrNonPointerType, zero)
	}

	cfg := DefaultConfig(allocator, cleaner)
	return NewPoolWithConfig(cfg)
}

// NewPoolWithConfig creates a new sharded pool with the specified configuration
func NewPoolWithConfig[T Poolable](cfg PoolConfig[T]) (*ShardedPool[T], error) {
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

	var zero T
	for i := range p.shards {
		p.shards[i] = &PoolShard[T]{}
		p.shards[i].head.Store(zero)
	}

	if cfg.Cleanup.Enabled {
		if cfg.Cleanup.Interval <= 0 {
			return nil, errors.New("cleanup interval must be greater than 0")
		}
		if cfg.Cleanup.MinUsageCount <= 0 {
			return nil, errors.New("minimum usage count must be greater than 0")
		}
		if cfg.Cleanup.TargetSize < 0 {
			return nil, errors.New("target size can't be negative")
		}
		p.startCleaner()
	}

	return p, nil
}

// getShard returns the shard for the current goroutine
func (p *ShardedPool[T]) getShard() *PoolShard[T] {
	// Use goroutine's processor ID for shard selection
	// This provides better locality for goroutines that frequently access the pool
	id := runtime_procPin()
	runtime_procUnpin()

	return p.shards[id]
}

// RetrieveOrCreate gets an object from the pool or creates a new one
func (p *ShardedPool[T]) RetrieveOrCreate() T {
	shard := p.getShard()

	// Try to get an object from the shard
	if obj, ok := p.retrieveFromShard(shard); ok {
		obj.IncrementUsage()
		return obj
	}

	// Create a new object if none available
	obj := p.cfg.Allocator()
	obj.IncrementUsage()
	return obj
}

// Put returns an object to the pool
func (p *ShardedPool[T]) Put(obj T) {
	shard := p.getShard()

	for {
		oldHead := shard.head.Load().(T)

		obj.SetNext(oldHead)
		if shard.head.CompareAndSwap(oldHead, obj) {
			// Only clean the object after successfully adding it to the pool
			p.cfg.Cleaner(obj)
			return
		}
	}
}

// retrieveFromShard gets an object from a specific shard
func (p *ShardedPool[T]) retrieveFromShard(shard *PoolShard[T]) (zero T, success bool) {
	for {
		oldHead := shard.head.Load().(T)

		if reflect.ValueOf(oldHead).IsNil() {
			return zero, false
		}

		next := oldHead.GetNext()
		if shard.head.CompareAndSwap(oldHead, next) {
			return oldHead, true
		}
	}
}

// Clear removes all objects from the pool
func (p *ShardedPool[T]) clear() {
	var zero T
	for _, shard := range p.shards {
		current := shard.head.Load().(T)

		if !reflect.ValueOf(current).IsNil() {
			if shard.head.CompareAndSwap(current, zero) {
				for !reflect.ValueOf(current).IsNil() {
					next := current.GetNext()
					current.SetNext(zero)
					p.cfg.Cleaner(current)
					current = next.(T)
				}
			}
		}
	}
}

// startCleaner starts the background cleanup goroutine
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

// cleanup removes idle objects based on the cleanup policy
func (p *ShardedPool[T]) cleanup() {
	if !p.cfg.Cleanup.Enabled {
		return
	}

	for _, shard := range p.shards {
		p.cleanupShard(shard)
	}
}

func (p *ShardedPool[T]) cleanupShard(shard *PoolShard[T]) {
	var current, prev T
	var kept int
	var zero T

	current = shard.head.Load().(T)

	if reflect.ValueOf(current).IsNil() {
		return
	}

	for !reflect.ValueOf(current).IsNil() {
		next := current.GetNext()
		usageCount := current.GetUsageCount()

		metMinUsageCount := usageCount >= p.cfg.Cleanup.MinUsageCount
		targetDisabled := p.cfg.Cleanup.TargetSize <= 0

		shardQuota := 1
		if p.cfg.Cleanup.TargetSize > 0 {
			shardQuota = max(1, p.cfg.Cleanup.TargetSize/numShards)
		}
		underShardQuota := kept < shardQuota

		shouldKeep := metMinUsageCount && (targetDisabled || underShardQuota)
		if shouldKeep {
			current.ResetUsage()
			prev = current
			kept++
		} else {
			if reflect.ValueOf(prev).IsNil() {
				if reflect.ValueOf(next).IsNil() {
					shard.head.Store(zero)
				} else {
					shard.head.Store(next)
				}
			} else {
				prev.SetNext(next)
			}

			current.SetNext(zero)
		}

		current = next.(T)
	}
}

// Close stops the cleanup goroutine and clears the pool
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
