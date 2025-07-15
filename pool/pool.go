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

// Different levels for clean up configuration.
// These presets control how aggressively GenPool reclaims memory.
// Note: Go's GC may still run unless you explicitly suppress it via debug.SetGCPercent(-1)
type GcLevel string

var (
	// GcDisable disables GenPool's cleanup completely.
	// Objects will stay in the pool indefinitely unless manually cleared.
	GcDisable GcLevel = "disable"

	// GcLow performs cleanup at long intervals with minimal aggression.
	// Good for low-latency, high-reuse scenarios.
	GcLow GcLevel = "low"

	// GcModerate performs cleanup at regular intervals and evicts objects
	// that are lightly used. Balances reuse and memory usage.
	GcModerate GcLevel = "moderate"

	// GcAggressive enables frequent cleanup and removes objects
	// that are not reused often. Best for memory-constrained environments.
	GcAggressive GcLevel = "aggressive"
)

// numShards determines how many shards the pool will use based on available CPU resources.
// It uses GOMAXPROCS(0) to detect how many logical CPUs the Go scheduler is using.
// The number is clamped between 8 and 128 to avoid poor performance due to under- or over-sharding.
//
// NOTE: This value is computed once at startup.
// If your application starts with a small CPU quota (e.g., 2 cores in a container)
// and later scales up to a higher CPU count (e.g., 64 cores),
// numShards will NOT automatically adjust. This could lead to suboptimal performance
// because the pool may not fully utilize the additional cores.
var numShards = min(max(runtime.GOMAXPROCS(0), 8), 128)

// CleanupPolicy defines how the pool should clean up unused objects.
type CleanupPolicy struct {
	// Enabled determines if automatic cleanup is enabled.
	Enabled bool
	// Interval is how often the cleanup should run.
	Interval time.Duration
	// MinUsageCount is the number of usage BELOW which an object will be evicted.
	MinUsageCount int64
}

// DefaultCleanupPolicy returns a default cleanup configuration based on specified level.
func DefaultCleanupPolicy(level GcLevel) CleanupPolicy {
	switch level {
	case GcDisable:
		return CleanupPolicy{
			Enabled:       false,
			Interval:      0,
			MinUsageCount: 0,
		}
	case GcLow:
		return CleanupPolicy{
			Enabled:       true,
			Interval:      10 * time.Minute,
			MinUsageCount: 1,
		}
	case GcModerate:
		return CleanupPolicy{
			Enabled:       true,
			Interval:      2 * time.Minute,
			MinUsageCount: 2,
		}
	case GcAggressive:
		return CleanupPolicy{
			Enabled:       true,
			Interval:      30 * time.Second,
			MinUsageCount: 3,
		}
	default:
		// Fallback to moderate if unrecognized
		return CleanupPolicy{
			Enabled:       true,
			Interval:      2 * time.Minute,
			MinUsageCount: 2,
		}
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

// PoolFields provides intrusive fields and logic for a poolable object.
// This struct can be embedded in user types.
type PoolFields[T any] struct {
	usageCount atomic.Int64
	next       atomic.Pointer[T]
}

func (p *PoolFields[T]) GetNext() *T {
	return p.next.Load()
}

func (p *PoolFields[T]) SetNext(n *T) {
	p.next.Store(n)
}

func (p *PoolFields[T]) GetUsageCount() int64 {
	return p.usageCount.Load()
}

func (p *PoolFields[T]) IncrementUsage() {
	p.usageCount.Add(1)
}

func (p *PoolFields[T]) ResetUsage() {
	p.usageCount.Store(0)
}

// PoolConfig holds configuration options for the pool.
type PoolConfig[T any, P Poolable[T]] struct {
	// Cleanup defines the cleanup policy for the pool
	Cleanup CleanupPolicy
	// Allocator is the function to create new objects
	Allocator Allocator[T]
	// Cleaner is the function to clean objects before returning them to the pool
	Cleaner Cleaner[T]
	// ShardNumOverride allows you to change [numShards] if its necessary for your use case
	ShardNumOverride int
}

// DefaultConfig returns a default pool configuration for type T.
func DefaultConfig[T any, P Poolable[T]](allocator Allocator[T], cleaner Cleaner[T]) PoolConfig[T, P] {
	return PoolConfig[T, P]{
		Cleanup:   DefaultCleanupPolicy(GcModerate),
		Allocator: allocator,
		Cleaner:   cleaner,
	}
}

// PoolShard represents a single shard in the pool.
type PoolShard[T any, P Poolable[T]] struct {
	// head is the head of the linked list for this shard
	Head atomic.Pointer[T]

	// pad ensures each shard is on its own cache line
	_ [64 - unsafe.Sizeof(atomic.Pointer[T]{})%64]byte
}

// ShardedPool is the main pool implementation using sharding for better concurrency.
type ShardedPool[T any, P Poolable[T]] struct {
	// shards is a slice of pool shards, each on its own cache line
	Shards []*PoolShard[T, P]

	// stopClean signals the cleanup goroutine to stop
	stopClean chan struct{}

	// cleanWg waits for the cleanup goroutine to finish
	cleanWg sync.WaitGroup

	// cfg holds the pool configuration
	cfg PoolConfig[T, P]

	// Its used by [GenNCheap], avoids creating slices.
	FastPath chan P
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

	if cfg.ShardNumOverride > 0 {
		numShards = cfg.ShardNumOverride
	}

	p := &ShardedPool[T, P]{
		cfg:       cfg,
		stopClean: make(chan struct{}),
		Shards:    make([]*PoolShard[T, P], numShards),
		FastPath:  make(chan P, 1),
	}

	for i := range p.Shards {
		p.Shards[i] = &PoolShard[T, P]{}
		p.Shards[i].Head.Store(nil)
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

	return p.Shards[id%numShards] // ensure we don't get "index out of bounds error" if number of P's changes
}

// Get gets an object from the pool or creates a new one.
func (p *ShardedPool[T, P]) Get() P {
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

// GetN returns N objects.
// This implementation creates memory, don't use it in the hot path,
// "make" always makes things much slower.
func (p *ShardedPool[T, P]) GetN(n int) []P {
	objs := make([]P, n) // WARNING
	for i := range n {
		objs[i] = p.Get()
	}

	return objs
}

// GetNCheap its the same as GetN, but uses a channel instead of creating a new slice
// every time is called.
func (p *ShardedPool[T, P]) GetNCheap(n int) {
	for range n {
		p.FastPath <- p.Get()
	}
}

// Put returns an object to the pool.
func (p *ShardedPool[T, P]) Put(obj P) {
	p.cfg.Cleaner(obj)
	shard := p.getShard()

	for {
		oldHead := P(shard.Head.Load())

		if shard.Head.CompareAndSwap(oldHead, obj) {
			obj.SetNext(oldHead)
			return
		}
	}
}

// PutN returns N objects.
func (p *ShardedPool[T, P]) PutN(objs []P) {
	for _, obj := range objs {
		p.Put(obj)
	}
}

// retrieveFromShard gets an object from a specific shard.
func (p *ShardedPool[T, P]) retrieveFromShard(shard *PoolShard[T, P]) (zero P, success bool) {
	for {
		oldHead := P(shard.Head.Load())
		if oldHead == nil {
			return zero, false
		}

		next := oldHead.GetNext()
		if shard.Head.CompareAndSwap(oldHead, next) {
			return oldHead, true
		}
	}
}

// Clear removes all objects from the pool.
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

	for _, shard := range p.Shards {
		p.cleanupShard(shard)
	}
}

func (p *ShardedPool[T, P]) cleanupShard(shard *PoolShard[T, P]) {
	// Atomically take the entire list from the shard.
	oldHead := P(shard.Head.Load())
	if oldHead == nil {
		return
	}

	if !shard.Head.CompareAndSwap(oldHead, nil) {
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
			currentHead := P(shard.Head.Load())
			var nextForTail P
			if currentHead != nil {
				nextForTail = currentHead
			}
			keptTail.SetNext(nextForTail)

			if shard.Head.CompareAndSwap(currentHead, keptHead) {
				break
			}
			// Contention: The head of the shard's list was modified. Retry.
		}
	}
}

// Close stops the cleanup goroutine and clears the pool.
func (p *ShardedPool[T, P]) Close() {
	if p.cfg.Cleanup.Enabled {
		close(p.FastPath)
		close(p.stopClean)
		p.cleanWg.Wait()
		p.clear()
	}
}

//go:linkname runtime_procPin runtime.procPin
func runtime_procPin() int

//go:linkname runtime_procUnpin runtime.procUnpin
func runtime_procUnpin()
