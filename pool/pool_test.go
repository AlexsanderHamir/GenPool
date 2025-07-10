package pool_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// TestObject is a simple struct we'll use for testing.
type TestObject struct {
	ID    int
	Value string

	next       atomic.Pointer[TestObject]
	usageCount atomic.Int64
	inUse      atomic.Bool // Track if object is in use
}

func (o *TestObject) GetNext() *TestObject {
	if next := o.next.Load(); next != nil {
		return next
	}
	return nil
}

func (o *TestObject) SetNext(next *TestObject) {
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

// testAllocator creates a new TestObject for the pool.
func testAllocator() *TestObject {
	return &TestObject{ID: 1, Value: "test"}
}

// testCleaner resets a TestObject.
func testCleaner(obj *TestObject) {
	obj.ID = 0
	obj.Value = ""
}

func TestNewPool(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if p != nil {
			defer p.Close()
		}

		if err != nil {
			t.Errorf("NewPool() error = %v, want nil", err)
		}
		if p == nil {
			t.Error("NewPool() returned nil pool")
		}
	})
}

func TestPoolRetrieveOrCreate(t *testing.T) {
	t.Run("with allocator", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}

		defer p.Close()

		obj := p.RetrieveOrCreate()
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
		cfg := pool.DefaultConfig(testAllocator, testCleaner)
		cfg.Cleanup.Enabled = true
		cfg.Cleanup.Interval = 100 * time.Millisecond

		p, err := pool.NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}

		defer p.Close()

		obj1 := p.RetrieveOrCreate()
		if obj1 == nil {
			t.Fatal("RetrieveOrCreate() returned nil object")
		}

		obj1.IncrementUsage()
		obj1.IncrementUsage()

		p.Put(obj1)

		time.Sleep(1 * time.Second)

		if obj1.GetUsageCount() != 0 {
			t.Errorf("obj1 should have been cleaned up (usage count 0), got %d", obj1.GetUsageCount())
		}
	})
}
