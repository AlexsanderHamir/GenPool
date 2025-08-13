package test

import (
	"testing"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// TestObject is a simple struct we'll use for testing.
type TestObject struct {
	ID    int
	Value string

	pool.Fields[TestObject]
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
}

func TestPoolRetrieveOrCreate(t *testing.T) {
	p, err := pool.NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}

	defer p.Close()

	obj := p.Get()
	if obj == nil {
		t.Error("Get() returned nil object")
		return
	}

	if obj.ID != 1 || obj.Value != "test" {
		t.Errorf("Get() got = %+v, want ID=1, Value=test", obj)
	}
}

func createConfig[T any, P pool.Poolable[T]](interval time.Duration, minUsage int64, override int) *pool.Config[TestObject, *TestObject] {
	cfg := pool.DefaultConfig(testAllocator, testCleaner)
	cfg.Cleanup.Enabled = true
	cfg.Cleanup.Interval = interval * time.Millisecond
	cfg.Cleanup.MinUsageCount = minUsage

	if override > 0 {
		cfg.ShardNumOverride = override
	}

	return &cfg
}

func TestPoolCleanupUsageCount(t *testing.T) {

	t.Run("CleanupSuccess", func(t *testing.T) {
		cfg := createConfig[TestObject](100, 2, 0)
		p, err := pool.NewPoolWithConfig(*cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		obj1 := p.Get()
		if obj1 == nil {
			t.Fatal("Get() returned nil object")
		}

		obj1.IncrementUsage()
		obj1.IncrementUsage()
		p.Put(obj1)

		// Should clean in two passes.
		time.Sleep(1 * time.Second)
	})

	t.Run("CleanupFailure", func(t *testing.T) {
		cfg := createConfig[TestObject](1000, 2, 1)
		p, err := pool.NewPoolWithConfig(*cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		obj1 := p.Get()
		if obj1 == nil {
			t.Fatal("Get() returned nil object")
		}

		obj1.IncrementUsage()
		obj1.IncrementUsage()
		p.Put(obj1)

		// Should clean in two passes.
		time.Sleep(1 * time.Millisecond)

		if obj1.GetUsageCount() != 3 {
			t.Errorf("obj1 should not have been cleaned up, current usage count: %d", obj1.GetUsageCount())
		}
	})
}
