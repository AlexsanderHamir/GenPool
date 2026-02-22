package pool

import "sync/atomic"

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
	// SetShardIndex sets the shard index for this object
	SetShardIndex(index int)
	// GetShardIndex returns the shard index for this object
	GetShardIndex() int
}

// Fields provides intrusive fields and logic for poolable objects.
// By embedding this struct in your types, you avoid having to implement
// pooling logic separately for each pool.
type Fields[T any] struct {
	usageCount atomic.Int64
	next       atomic.Pointer[T]
	shardIndex int // Track which shard this object belongs to
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

// SetShardIndex sets the shard index for this object
func (p *Fields[T]) SetShardIndex(index int) {
	p.shardIndex = index
}

// GetShardIndex returns the shard index for this object
func (p *Fields[T]) GetShardIndex() int {
	return p.shardIndex
}
