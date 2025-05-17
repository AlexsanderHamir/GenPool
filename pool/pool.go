package pool

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Common errors that may be returned by the pool
var (
	// ErrNotPointerType is returned when attempting to create a pool with a non-pointer type
	ErrNotPointerType = fmt.Errorf("type must be a pointer type")

	// ErrInvalidPoolType is returned when the pool's head contains an invalid type
	ErrInvalidPoolType = fmt.Errorf("invalid pool type")

	// ErrHardLimitReached is returned when attempting to create a new object would exceed the pool's hard limit
	ErrHardLimitReached = fmt.Errorf("hard limit reached")

	// ErrNoAllocator is returned when attempting to get an object but no allocator is configured
	ErrNoAllocator = fmt.Errorf("no allocator configured")

	// ErrAllocatorFailed is returned when the allocator function fails to create a new object
	ErrAllocatorFailed = fmt.Errorf("allocator failed")

	// ErrCleanerFailed is returned when the cleaner function fails to clean an object
	ErrCleanerFailed = fmt.Errorf("cleaner failed")

	// ErrNoObjectsAvailable is returned when no objects are available in the pool
	ErrNoObjectsAvailable = fmt.Errorf("no objects available")
)

// CleanupPolicy defines how the pool should clean up unused objects
type CleanupPolicy struct {
	// Enabled determines if automatic cleanup is enabled
	Enabled bool
	// Interval is how often the cleanup should run
	Interval time.Duration
	// MinUsageCount is the minimum number of times an object should be used before being considered for eviction
	MinUsageCount int64
	// TargetSize is the target number of objects to keep after cleanup
	// If 0, no target size is enforced
	TargetSize int
}

// DefaultCleanupPolicy returns a default cleanup configuration
func DefaultCleanupPolicy() CleanupPolicy {
	return CleanupPolicy{
		Enabled:       false,
		Interval:      5 * time.Minute,
		MinUsageCount: 10, // Objects used less than 10 times may be evicted
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

type Pool[T Poolable] struct {
	head atomic.Value
	_    [64 - unsafe.Sizeof(atomic.Value{})%64]byte

	stopClean chan struct{}
	cleanWg   sync.WaitGroup
	cfg       PoolConfig[T]
}

// NewPool creates a new pool with default configuration
// T must be a pointer type and implement Poolable
func NewPool[T Poolable](allocator Allocator[T], cleaner Cleaner[T]) (*Pool[T], error) {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("%w, got %T", ErrNotPointerType, zero)
	}

	cfg := DefaultConfig(allocator, cleaner)
	return NewPoolWithConfig(cfg)
}

// NewPoolWithConfig creates a new pool with the specified configuration
func NewPoolWithConfig[T Poolable](cfg PoolConfig[T]) (*Pool[T], error) {
	if cfg.Allocator == nil {
		return nil, fmt.Errorf("%w: allocator is required", ErrNoAllocator)
	}
	if cfg.Cleaner == nil {
		return nil, fmt.Errorf("%w: cleaner is required", ErrCleanerFailed)
	}

	p := &Pool[T]{
		cfg:       cfg,
		stopClean: make(chan struct{}),
	}

	var zero T
	p.head.Store(zero)

	if cfg.Cleanup.Enabled {
		if cfg.Cleanup.Interval <= 0 {
			return nil, fmt.Errorf("%w: cleanup interval must be greater than 0", ErrInvalidPoolType)
		}

		if cfg.Cleanup.MinUsageCount < 0 {
			return nil, fmt.Errorf("%w: minimum usage count must be greater than or equal to 0", ErrInvalidPoolType)
		}

		if cfg.Cleanup.TargetSize < 0 {
			return nil, fmt.Errorf("%w: target size must be greater than or equal to 0", ErrInvalidPoolType)
		}

		p.startCleaner()
	}

	return p, nil
}

// RetrieveOrCreate retrieves an object from the pool or creates a new one using the allocator
func (p *Pool[T]) RetrieveOrCreate() T {
	if obj, ok := p.retrieve(); ok {
		return obj
	}

	obj := p.cfg.Allocator()
	obj.IncrementUsage()

	return obj
}

// Put returns an object to the pool, cleaning it first
func (p *Pool[T]) Put(obj T) {
	p.cfg.Cleaner(obj)

	for {
		oldHead, ok := p.head.Load().(T)
		if !ok {
			return
		}

		// Set the next pointer to the old head (which may be nil)
		obj.SetNext(oldHead)
		if p.head.CompareAndSwap(oldHead, obj) {
			return
		}
	}
}

// retrieve retrieves a previously inserted object from the pool
func (p *Pool[T]) retrieve() (zero T, success bool) {
	for {
		oldHead, ok := p.head.Load().(T)
		if !ok {
			return zero, false
		}

		if reflect.ValueOf(oldHead).IsNil() {
			return zero, false
		}

		next := oldHead.GetNext()
		if p.head.CompareAndSwap(oldHead, next) {
			oldHead.SetNext(zero)
			oldHead.IncrementUsage() // Track usage when object is retrieved
			return oldHead, true
		}
	}
}

// Clear removes all objects from the pool
func (p *Pool[T]) Clear() {
	var zero T
	for {
		oldHead, ok := p.head.Load().(T)
		if !ok {
			return
		}

		if reflect.ValueOf(oldHead).IsNil() {
			return
		}
		if p.head.CompareAndSwap(oldHead, zero) {
			p.cfg.Cleaner(oldHead)
			oldHead.SetNext(zero)
			return
		}
	}
}

// Config returns the current pool configuration
func (p *Pool[T]) Config() PoolConfig[T] {
	return p.cfg
}

// startCleaner starts the background cleanup goroutine
func (p *Pool[T]) startCleaner() {
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
func (p *Pool[T]) cleanup() {
	if !p.cfg.Cleanup.Enabled {
		return
	}

	var current, prev T
	var kept int

	// Start from the head of the pool
	current, ok := p.head.Load().(T)
	if !ok {
		return
	}

	if reflect.ValueOf(current).IsNil() {
		return
	}

	// Traverse and clean the list
	for !reflect.ValueOf(current).IsNil() {
		next := current.GetNext()
		usageCount := current.GetUsageCount()
		shouldKeep := usageCount >= p.cfg.Cleanup.MinUsageCount && (p.cfg.Cleanup.TargetSize <= 0 || kept < p.cfg.Cleanup.TargetSize)

		if shouldKeep {
			// Reset usage count for kept objects
			current.ResetUsage()
			prev = current
			kept++
		} else {
			// Remove current object from list
			if reflect.ValueOf(prev).IsNil() {
				// We're at the head
				p.head.Store(next)
			} else {
				prev.SetNext(next)
			}
		}

		current = next.(T)
	}
}
