package pool

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestObject is a simple struct we'll use for testing
type TestObject struct {
	ID         int
	Value      string
	next       atomic.Value // Stores Poolable
	usageCount atomic.Int64
	inUse      atomic.Bool // Track if object is in use
}

func (o *TestObject) GetNext() Poolable {
	if next := o.next.Load(); next != nil {
		return next.(Poolable)
	}
	return nil
}

func (o *TestObject) SetNext(next Poolable) {
	o.next.Store(next)
}

func (o *TestObject) GetUsageCount() int64 {
	return o.usageCount.Load()
}

func (o *TestObject) IncrementUsage() {
	o.usageCount.Add(1)
}

func (o *TestObject) ResetUsage() {
	o.usageCount.Store(0)
}

func (o *TestObject) IsInUse() bool {
	return o.inUse.Load()
}

func (o *TestObject) SetInUse(inUse bool) bool {
	return o.inUse.CompareAndSwap(!inUse, inUse)
}

// NonPointerObject is used to test the pointer type constraint
type NonPointerObject struct {
	ID         int
	Value      string
	Next       atomic.Value
	usageCount atomic.Int64
	inUse      atomic.Bool
}

func (o NonPointerObject) GetNext() Poolable {
	return nil
}

func (o NonPointerObject) SetNext(next Poolable) {}

func (o NonPointerObject) GetUsageCount() int64 {
	return o.usageCount.Load()
}

func (o NonPointerObject) IncrementUsage() {
	o.usageCount.Add(1)
}

func (o NonPointerObject) ResetUsage() {
	o.usageCount.Store(0)
}

func (o NonPointerObject) IsInUse() bool {
	return o.inUse.Load()
}

func (o NonPointerObject) SetInUse(inUse bool) bool {
	return o.inUse.CompareAndSwap(!inUse, inUse)
}

// testAllocator creates a new TestObject for the pool
func testAllocator() *TestObject {
	return &TestObject{ID: 1, Value: "test"}
}

// testCleaner resets a TestObject
func testCleaner(obj *TestObject) {
	obj.ID = 0
	obj.Value = ""
}

func TestNewPool(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		pool, err := NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Errorf("NewPool() error = %v, want nil", err)
		}
		if pool == nil {
			t.Error("NewPool() returned nil pool")
		}
	})

	t.Run("non-pointer type", func(t *testing.T) {
		_, err := NewPool(
			func() NonPointerObject { return NonPointerObject{} },
			func(obj NonPointerObject) {},
		)
		if !errors.Is(err, ErrNonPointerType) {
			t.Errorf("NewPool() error = %v, want %v", err, ErrNonPointerType)
		}
	})
}

func TestPoolRetrieveOrCreate(t *testing.T) {
	t.Run("with allocator", func(t *testing.T) {
		pool, err := NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}

		obj := pool.RetrieveOrCreate()
		if obj == nil {
			t.Error("RetrieveOrCreate() returned nil object")
			return
		}

		if obj.ID != 1 || obj.Value != "test" {
			t.Errorf("RetrieveOrCreate() got = %+v, want ID=1, Value=test", obj)
		}
	})
}

func TestPoolCleanupUsageCount(t *testing.T) {
	t.Run("should cleanup low usage objects", func(t *testing.T) {
		cfg := DefaultConfig(testAllocator, testCleaner)
		cfg.Cleanup.Enabled = true
		cfg.Cleanup.Interval = 100 * time.Millisecond

		pool, err := NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}

		obj1 := pool.RetrieveOrCreate()
		if obj1 == nil {
			t.Fatal("RetrieveOrCreate() returned nil object")
		}

		obj1.IncrementUsage()
		obj1.IncrementUsage()

		pool.Put(obj1)

		time.Sleep(1 * time.Second)

		if obj1.GetUsageCount() != 0 {
			t.Errorf("obj1 should have been cleaned up (usage count 0), got %d", obj1.GetUsageCount())
		}
	})
}
