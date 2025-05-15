package pool

import (
	"errors"
	"sync"
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

// NonPointerObject is used to test the pointer type constraint
type NonPointerObject struct {
	ID         int
	Value      string
	Next       atomic.Value
	usageCount atomic.Int64
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
		if !errors.Is(err, ErrNotPointerType) {
			t.Errorf("NewPool() error = %v, want %v", err, ErrNotPointerType)
		}
	})
}

func TestPoolRetrieveOrCreate(t *testing.T) {
	t.Run("with allocator", func(t *testing.T) {
		pool, err := NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}

		obj, err := pool.RetrieveOrCreate()
		if err != nil {
			t.Errorf("RetrieveOrCreate() error = %v, want nil", err)
		}

		if obj == nil {
			t.Error("RetrieveOrCreate() returned nil object")
			return
		}

		if obj.ID != 1 || obj.Value != "test" {
			t.Errorf("RetrieveOrCreate() got = %+v, want ID=1, Value=test", obj)
		}

		if pool.Active() != 1 {
			t.Errorf("Active() = %d, want 1", pool.Active())
		}
	})

	t.Run("hard limit", func(t *testing.T) {
		cfg := DefaultConfig(testAllocator, testCleaner)
		cfg.HardLimit = 1
		pool, err := NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}

		obj1, err := pool.RetrieveOrCreate()
		if err != nil {
			t.Errorf("First RetrieveOrCreate() error = %v, want nil", err)
		}

		_, err = pool.RetrieveOrCreate()
		if !errors.Is(err, ErrHardLimitReached) {
			t.Errorf("Second RetrieveOrCreate() error = %v, want %v", err, ErrHardLimitReached)
		}

		if err := pool.Put(obj1); err != nil {
			t.Errorf("Put() error = %v, want nil", err)
		}

		obj2, err := pool.RetrieveOrCreate()
		if err != nil {
			t.Errorf("Third RetrieveOrCreate() error = %v, want nil", err)
		}
		if obj2 == nil {
			t.Error("Third RetrieveOrCreate() returned nil object")
		}
	})
}

func TestPoolConcurrent(t *testing.T) {
	cfg := DefaultConfig(testAllocator, testCleaner)
	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 10
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				obj, err := pool.RetrieveOrCreate()
				if err != nil {
					t.Errorf("RetrieveOrCreate() error = %v", err)
					return
				}
				time.Sleep(time.Millisecond)

				if err := pool.Put(obj); err != nil {
					t.Errorf("Put() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	// Verify pool state
	if pool.Active() != 0 {
		t.Errorf("Active() = %d, want 0", pool.Active())
	}
}

func TestPoolClose(t *testing.T) {
	t.Run("basic close", func(t *testing.T) {
		pool, err := NewPoolWithConfig(DefaultConfig(testAllocator, testCleaner))
		if err != nil {
			t.Fatal(err)
		}
		obj := &TestObject{ID: 1}
		pool.Put(obj)

		pool.Close()
		if pool.Size() != 0 {
			t.Errorf("Size() = %d, want 0 after Close()", pool.Size())
		}
		if pool.Active() != 0 {
			t.Errorf("Active() = %d, want 0 after Close()", pool.Active())
		}
	})
}

func TestPoolCleanupUsageCount(t *testing.T) {
	// Configure pool with cleanup enabled
	cfg := DefaultConfig(testAllocator, testCleaner)
	cfg.Cleanup.Enabled = true
	cfg.Cleanup.Interval = 100 * time.Millisecond // Short interval for testing
	cfg.Cleanup.MinUsageCount = 2                 // Objects used less than 2 times will be cleaned
	cfg.Cleanup.TargetSize = 0                    // No target size limit

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Create and return objects with different usage counts
	obj1, err := pool.RetrieveOrCreate() // Will be used once
	if err != nil {
		t.Errorf("RetrieveOrCreate() error = %v", err)
	}
	obj2, err := pool.RetrieveOrCreate() // Will be used twice
	if err != nil {
		t.Errorf("RetrieveOrCreate() error = %v", err)
	}
	obj3, err := pool.RetrieveOrCreate() // Will be used twice
	if err != nil {
		t.Errorf("RetrieveOrCreate() error = %v", err)
	}

	// Use obj2 and obj3 twice
	pool.Put(obj2)
	obj2, err = pool.RetrieveOrCreate()
	if err != nil {
		t.Errorf("RetrieveOrCreate() error = %v", err)
	}

	pool.Put(obj3)
	obj3, err = pool.RetrieveOrCreate()
	if err != nil {
		t.Errorf("RetrieveOrCreate() error = %v", err)
	}

	// Return all objects to pool
	pool.Put(obj1)
	pool.Put(obj2)
	pool.Put(obj3)

	// Wait for cleanup to run
	time.Sleep(1 * time.Second)

	// Verify pool state
	if pool.Size() != 0 {
		t.Errorf("Size() = %d, want 0 (obj1 should be cleaned)", pool.Size())
	}
}
