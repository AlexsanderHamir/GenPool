package pool_test

import (
	"testing"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// TestObject is a simple struct we'll use for testing.
type TestObject struct {
	ID    int
	Value string

	pool.PoolFields[TestObject]
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

func TestPoolGet(t *testing.T) {
	t.Run("with allocator", func(t *testing.T) {
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

		obj1 := p.Get()
		if obj1 == nil {
			t.Fatal("Get() returned nil object")
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
