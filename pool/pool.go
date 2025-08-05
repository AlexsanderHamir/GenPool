// Package pool creates a pool of your objects without additional wrapping and shards the linked list
// based on the number of available logical CPUs. This design improves performance under
// high concurrency and when there is significant work done between getting and returning objects.
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

// GcLevel offers different levels for clean up configuration.
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
		return CleanupPolicy{}
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

// Fields provides intrusive fields and logic for poolable objects.
// By embedding this struct in your types, you avoid having to implement
// pooling logic separately for each pool.
type Fields[T any] struct {
	usageCount atomic.Int64
	next       atomic.Pointer[T]
}

// GetNext implements interface function
func (p *Fields[T]) GetNext() *T {
	return p.next.Load()
}

// SetNext implements interface function
func (p *Fields[T]) SetNext(n *T) {
	p.next.Store(n)
}

// GetUsageCount implements interface function
func (p *Fields[T]) GetUsageCount() int64 {
	return p.usageCount.Load()
}

// IncrementUsage implements interface function
func (p *Fields[T]) IncrementUsage() {
	p.usageCount.Add(1)
}

// ResetUsage implements interface function
func (p *Fields[T]) ResetUsage() {
	p.usageCount.Store(0)
}

// Config holds configuration options for the pool.
type Config[T any, P Poolable[T]] struct {
	// Cleanup defines the cleanup policy for the pool
	Cleanup CleanupPolicy

	// Growth defined the growth policy for the pool
	Growth GrowthPolicy

	// Allocator is the function to create new objects
	Allocator Allocator[T]

	// Cleaner is the function to clean objects before returning them to the pool
	Cleaner Cleaner[T]

	// ShardNumOverride allows you to change [numShards] if its necessary for your use case
	ShardNumOverride int
}

// GrowthPolicy controls how the pool is allowed to grow.
// If unset, the pool will grow indefinitely, and any cleanup will rely solely on the CleanupPolicy.
type GrowthPolicy struct {
	// MaxPoolSize defines the maximum number of objects the pool is allowed to grow to.
	MaxPoolSize int64

	// Enable activates growth control. If disabled, the pool will grow and shrink freely based on your configuration.
	Enable bool
}

// DefaultConfig returns a default pool configuration for type T.
func DefaultConfig[T any, P Poolable[T]](allocator Allocator[T], cleaner Cleaner[T]) Config[T, P] {
	return Config[T, P]{
		Cleanup:   DefaultCleanupPolicy(GcModerate),
		Allocator: allocator,
		Cleaner:   cleaner,
	}
}

// Shard represents a single shard in the pool.
// It is 64 bytes in total to avoid false sharing across CPU cache lines.
type Shard[T any, P Poolable[T]] struct {
	Head  atomic.Pointer[T] // 8 bytes
	Cond  *sync.Cond        // 8 bytes
	Mutex *sync.Mutex       // 8 bytes

	// Padding to make the struct 64 bytes in total
	_ [64 - unsafe.Sizeof(atomic.Pointer[T]{}) -
		unsafe.Sizeof((*sync.Cond)(nil)) -
		unsafe.Sizeof((*sync.Mutex)(nil))]byte
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

	// blockedShards keeps track of how many goroutines are blocked and in which shards.
	blockedShards map[int]*atomic.Int64
}

func (p *ShardedPool[T, P]) getMostBlockedShard() *Shard[T, P] {
	var mostBlockedShard *Shard[T, P]
	var maxBlocked int64 = -1

	for shardID, counter := range p.blockedShards {
		val := counter.Load()
		if val > maxBlocked {
			maxBlocked = val
			mostBlockedShard = p.Shards[shardID]
		}
	}

	return mostBlockedShard
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
		cfg:           cfg,
		stopClean:     make(chan struct{}),
		blockedShards: map[int]*atomic.Int64{},
		Shards:        make([]*Shard[T, P], getShardCount(cfg)),
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

func validateConfig[T any, P Poolable[T]](cfg Config[T, P]) error {
	if cfg.Allocator == nil {
		return fmt.Errorf("%w: allocator is required", ErrNoAllocator)
	}
	if cfg.Cleaner == nil {
		return fmt.Errorf("%w: cleaner is required", ErrNoCleaner)
	}

	return nil
}

func validateCleanupConfig[T any, P Poolable[T]](cfg Config[T, P]) error {
	if cfg.Cleanup.Interval <= 0 {
		return errors.New("cleanup interval must be greater than 0")
	}
	if cfg.Cleanup.MinUsageCount <= 0 {
		return errors.New("minimum usage count must be greater than 0")
	}
	return nil
}

func getShardCount[T any, P Poolable[T]](cfg Config[T, P]) int {
	if cfg.ShardNumOverride > 0 {
		numShards = cfg.ShardNumOverride
		return numShards
	}
	return numShards
}

func initShards[T any, P Poolable[T]](p *ShardedPool[T, P]) {
	for i := range p.Shards {
		mu := &sync.Mutex{}
		shard := &Shard[T, P]{
			Mutex: mu,
			Cond:  sync.NewCond(mu),
		}
		shard.Head.Store(nil)

		p.Shards[i] = shard
		p.blockedShards[i] = new(atomic.Int64)
	}
}

// getShard returns the shard for the current goroutine.
func (p *ShardedPool[T, P]) getShard() (*Shard[T, P], int) {
	// Use goroutine's processor ID for shard selection
	// This provides better locality for goroutines that frequently access the pool
	id := runtimeProcPin()
	runtimeProcUnpin()

	return p.Shards[id%numShards], id // ensure we don't get "index out of bounds error" if number of P's changes
}

// Get returns an object from the pool or creates a new one.
// Returns nil if MaxPoolSize is set, reached, and no reusable objects are available.
func (p *ShardedPool[T, P]) Get() P {
	shard, _ := p.getShard()

	// Try to get an object from the shard
	if obj, ok := p.retrieveFromShard(shard); ok {
		obj.IncrementUsage()
		return obj
	}

	if !p.cfg.Growth.Enable || p.CurrentPoolLength.Load() < p.cfg.Growth.MaxPoolSize {
		obj := P(p.cfg.Allocator())
		obj.IncrementUsage()
		p.CurrentPoolLength.Add(1)
		return obj
	}

	return nil
}

// GetBlock retrieves an object from the pool, blocking if necessary until one becomes available.
// It first attempts to reuse an object from the shard, then allocates a new one if the pool isn't full.
// If the pool has reached its maximum size, it blocks until another goroutine puts an object back.
func (p *ShardedPool[T, P]) GetBlock() P {
	shard, shardID := p.getShard()

	// Try fast path
	if obj, ok := p.retrieveFromShard(shard); ok {
		obj.IncrementUsage()
		return obj
	}

	// Try to allocate new one if allowed
	if !p.cfg.Growth.Enable || p.CurrentPoolLength.Load() < p.cfg.Growth.MaxPoolSize {
		obj := P(p.cfg.Allocator())
		obj.IncrementUsage()
		p.CurrentPoolLength.Add(1)
		return obj
	}

	// Block: resource exhausted, wait for one to be returned
	p.blockedShards[shardID].Add(1)
	shard.Mutex.Lock()
	defer shard.Mutex.Unlock()

	for {
		if obj, ok := p.retrieveFromShard(shard); ok {
			obj.IncrementUsage()
			return obj
		}
		shard.Cond.Wait()
	}
}

// PutBlock returns an object to the pool and signals a blocked goroutine, if any.
// It attempts to atomically insert the object at the head of the most blocked shard's list.
func (p *ShardedPool[T, P]) PutBlock(obj P) {
	p.cfg.Cleaner(obj)
	shard := p.getMostBlockedShard()

	for {
		oldHead := P(shard.Head.Load())

		if shard.Head.CompareAndSwap(oldHead, obj) {
			obj.SetNext(oldHead)
			shard.Cond.Signal()
			return
		}
	}
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

// Put returns an object to the pool.
func (p *ShardedPool[T, P]) Put(obj P) {
	p.cfg.Cleaner(obj)
	shard, _ := p.getShard()

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
func (p *ShardedPool[T, P]) retrieveFromShard(shard *Shard[T, P]) (zero P, success bool) {
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

// Clear removes all objects from the pool and decrements the pool length accordingly.
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

func (p *ShardedPool[T, P]) cleanupShard(shard *Shard[T, P]) {
	oldHead := p.tryTakeOwnership(shard)
	if oldHead == nil {
		return
	}

	keptHead, keptTail, evictedCount := p.filterUsableObjects(oldHead)

	if evictedCount > 0 {
		p.CurrentPoolLength.Add(-int64(evictedCount))
	}

	if keptHead != nil {
		p.reinsertKeptObjects(shard, keptHead, keptTail)
	}
}

func (p *ShardedPool[T, P]) tryTakeOwnership(shard *Shard[T, P]) P {
	head := P(shard.Head.Load())
	if head == nil {
		return nil
	}
	if !shard.Head.CompareAndSwap(head, nil) {
		return nil
	}
	return head
}

// filterUsableObjects filters objects based on usage count and returns the kept head, kept tail, and number of evicted objects.
func (p *ShardedPool[T, P]) filterUsableObjects(head P) (keptHead, keptTail P, evictedCount int) {
	current := head

	for current != nil {
		next := current.GetNext()
		usageCount := current.GetUsageCount()

		if usageCount >= p.cfg.Cleanup.MinUsageCount {
			current.ResetUsage()
			if keptHead == nil {
				keptHead = current
			} else {
				keptTail.SetNext(current)
			}
			keptTail = current
		} else {
			current.SetNext(nil)
			evictedCount++
		}
		current = next
	}

	if keptHead == nil {
		return nil, nil, evictedCount
	}

	keptTail.SetNext(nil)
	return keptHead, keptTail, evictedCount
}

func (p *ShardedPool[T, P]) reinsertKeptObjects(shard *Shard[T, P], keptHead, keptTail P) {
	for {
		currentHead := P(shard.Head.Load())
		if currentHead != nil {
			keptTail.SetNext(currentHead)
		}
		if shard.Head.CompareAndSwap(currentHead, keptHead) {
			break
		}
		// Retry on contention
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
