package internal

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestObject is a simple struct we'll use for testing
type TestObject struct {
	ID    int
	Value string
	next  atomic.Value // Stores Poolable
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

// NonPointerObject is used to test the pointer type constraint
type NonPointerObject struct {
	ID    int
	Value string
	Next  atomic.Value
}

func (o NonPointerObject) GetNext() Poolable {
	return nil
}

func (o NonPointerObject) SetNext(next Poolable) {}

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
