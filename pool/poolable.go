// Poolable interface and Fields embed. Types that embed Fields[T] get GetNext/SetNext,
// usage count, and shard index; implement Poolable so they can be used with ShardedPool.
package pool

import "sync/atomic"

// Allocator creates new objects for the pool.
type Allocator[T any] func() *T

// Cleaner prepares an object before it is returned to the pool.
type Cleaner[T any] func(*T)

// Poolable is the interface required to store objects in the pool.
type Poolable[T any] interface {
	*T
	GetNext() *T
	SetNext(next *T)
	GetUsageCount() int64
	IncrementUsage()
	ResetUsage()
	SetShardIndex(index int)
	GetShardIndex() int
}

// Fields provides the intrusive fields and Poolable implementation; embed in your type.
type Fields[T any] struct {
	usageCount atomic.Int64
	next       atomic.Pointer[T]
	shardIndex int
}

func (p *Fields[T]) GetNext() *T {
	return p.next.Load()
}

func (p *Fields[T]) SetNext(n *T) {
	p.next.Store(n)
}

func (p *Fields[T]) GetUsageCount() int64 {
	return p.usageCount.Load()
}

func (p *Fields[T]) IncrementUsage() {
	p.usageCount.Add(1)
}

func (p *Fields[T]) ResetUsage() {
	p.usageCount.Store(0)
}

func (p *Fields[T]) SetShardIndex(index int) {
	p.shardIndex = index
}

func (p *Fields[T]) GetShardIndex() int {
	return p.shardIndex
}
