package pool_test

import (
	"context"
	"sync"
	"sync/atomic"
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

// TestNewPool tests pool creation with valid parameters.
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

	t.Run("nil allocator", func(t *testing.T) {
		p, err := pool.NewPool(nil, testCleaner)
		if p != nil {
			defer p.Close()
		}

		if err == nil {
			t.Error("NewPool() with nil allocator should return error")
		}
	})

	t.Run("nil cleaner", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, nil)
		if p != nil {
			defer p.Close()
		}

		if err == nil {
			t.Error("NewPool() with nil cleaner should return error")
		}
	})
}

// TestNewPoolWithConfig tests pool creation with custom configuration.
func TestNewPoolWithConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := pool.DefaultConfig(testAllocator, testCleaner)
		p, err := pool.NewPoolWithConfig(cfg)
		if p != nil {
			defer p.Close()
		}

		if err != nil {
			t.Errorf("NewPoolWithConfig() error = %v, want nil", err)
		}
		if p == nil {
			t.Error("NewPoolWithConfig() returned nil pool")
		}
	})

	t.Run("nil allocator", func(t *testing.T) {
		cfg := pool.PoolConfig[TestObject, *TestObject]{
			Allocator: nil,
			Cleaner:   testCleaner,
		}
		p, err := pool.NewPoolWithConfig(cfg)
		if p != nil {
			defer p.Close()
		}

		if err == nil {
			t.Error("NewPoolWithConfig() with nil allocator should return error")
		}
	})

	t.Run("nil cleaner", func(t *testing.T) {
		cfg := pool.PoolConfig[TestObject, *TestObject]{
			Allocator: testAllocator,
			Cleaner:   nil,
		}
		p, err := pool.NewPoolWithConfig(cfg)
		if p != nil {
			defer p.Close()
		}

		if err == nil {
			t.Error("NewPoolWithConfig() with nil cleaner should return error")
		}
	})

	t.Run("invalid cleanup interval", func(t *testing.T) {
		cfg := pool.PoolConfig[TestObject, *TestObject]{
			Allocator: testAllocator,
			Cleaner:   testCleaner,
			Cleanup: pool.CleanupPolicy{
				Enabled:  true,
				Interval: -1 * time.Second,
			},
		}
		p, err := pool.NewPoolWithConfig(cfg)
		if p != nil {
			defer p.Close()
		}

		if err == nil {
			t.Error("NewPoolWithConfig() with invalid cleanup interval should return error")
		}
	})

	t.Run("invalid min usage count", func(t *testing.T) {
		cfg := pool.PoolConfig[TestObject, *TestObject]{
			Allocator: testAllocator,
			Cleaner:   testCleaner,
			Cleanup: pool.CleanupPolicy{
				Enabled:       true,
				Interval:      1 * time.Second,
				MinUsageCount: -1,
			},
		}
		p, err := pool.NewPoolWithConfig(cfg)
		if p != nil {
			defer p.Close()
		}

		if err == nil {
			t.Error("NewPoolWithConfig() with invalid min usage count should return error")
		}
	})
}

// TestPoolGet tests the Get method.
func TestPoolGet(t *testing.T) {
	t.Run("get new object", func(t *testing.T) {
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

		if obj.GetUsageCount() != 1 {
			t.Errorf("Get() usage count = %d, want 1", obj.GetUsageCount())
		}
	})

	t.Run("get reused object", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		obj1 := p.Get()
		p.Put(obj1)

		obj2 := p.Get()
		if obj2 == nil {
			t.Error("Get() returned nil object")
			return
		}

		// Should be the same object
		if obj2 != obj1 {
			t.Error("Get() should return the same object after Put()")
		}

		if obj2.GetUsageCount() != 2 {
			t.Errorf("Get() usage count = %d, want 2", obj2.GetUsageCount())
		}
	})
}

// TestPoolGetN tests the GetN method.
func TestPoolGetN(t *testing.T) {
	t.Run("get multiple objects", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		objs := p.GetN(5)
		if len(objs) != 5 {
			t.Errorf("GetN(5) returned %d objects, want 5", len(objs))
		}

		for i, obj := range objs {
			if obj == nil {
				t.Errorf("GetN() returned nil object at index %d", i)
				continue
			}

			if obj.ID != 1 || obj.Value != "test" {
				t.Errorf("GetN() got = %+v at index %d, want ID=1, Value=test", obj, i)
			}

			if obj.GetUsageCount() != 1 {
				t.Errorf("GetN() usage count = %d at index %d, want 1", obj.GetUsageCount(), i)
			}
		}
	})

	t.Run("get zero objects", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		objs := p.GetN(0)
		if len(objs) != 0 {
			t.Errorf("GetN(0) returned %d objects, want 0", len(objs))
		}
	})

	t.Run("get negative count", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		objs := p.GetN(-1)
		if len(objs) != 0 {
			t.Errorf("GetN(-1) returned %d objects, want 0", len(objs))
		}
	})
}

// TestPoolGetNCheap tests the GetNCheap method.
func TestPoolGetNCheap(t *testing.T) {
	t.Run("get multiple objects cheap", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		// GetNCheap should create the FastPath channel
		if p.FastPath != nil {
			t.Error("FastPath should be nil initially")
		}

		p.GetNCheap(3)

		if p.FastPath == nil {
			t.Error("FastPath should be created after GetNCheap")
		}

		// Read from the channel to verify objects were sent
		obj1 := <-p.FastPath
		obj2 := <-p.FastPath
		obj3 := <-p.FastPath

		if obj1 == nil || obj2 == nil || obj3 == nil {
			t.Error("GetNCheap() should not send nil objects")
		}

		// Verify objects are valid
		for i, obj := range []*TestObject{obj1, obj2, obj3} {
			if obj.ID != 1 || obj.Value != "test" {
				t.Errorf("GetNCheap() got = %+v at index %d, want ID=1, Value=test", obj, i)
			}
		}
	})
}

// TestPoolPut tests the Put method.
func TestPoolPut(t *testing.T) {
	t.Run("put object", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		obj := p.Get()

		p.Put(obj)

		// Object should be cleaned
		if obj.ID != 0 || obj.Value != "" {
			t.Errorf("Put() did not clean object: ID=%d, Value=%s", obj.ID, obj.Value)
		}

		// Get the object back and verify it's the same
		obj2 := p.Get()
		if obj2 != obj {
			t.Error("Put() and Get() should return the same object")
		}
	})

	t.Run("put nil object", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		// This should not panic
		p.Put(nil)
	})
}

// TestPoolPutN tests the PutN method.
func TestPoolPutN(t *testing.T) {
	t.Run("put multiple objects", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		objs := p.GetN(3)
		p.PutN(objs)

		// All objects should be cleaned
		for i, obj := range objs {
			if obj.ID != 0 || obj.Value != "" {
				t.Errorf("PutN() did not clean object at index %d: ID=%d, Value=%s", i, obj.ID, obj.Value)
			}
		}

		// Get objects back and verify they're the same
		for i := range objs {
			obj := p.Get()
			if obj != objs[i] {
				t.Errorf("PutN() and Get() should return the same object at index %d", i)
			}
		}
	})

	t.Run("put empty slice", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		// This should not panic
		p.PutN([]*TestObject{})
	})
}

// TestPoolCleanup tests the cleanup functionality.
func TestPoolCleanup(t *testing.T) {
	t.Run("cleanup enabled", func(t *testing.T) {
		cfg := pool.DefaultConfig(testAllocator, testCleaner)
		cfg.Cleanup.Enabled = true
		cfg.Cleanup.Interval = 100 * time.Millisecond
		cfg.Cleanup.MinUsageCount = 2

		p, err := pool.NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		// Create objects with different usage counts
		obj1 := p.Get()       // usage count = 1
		obj2 := p.Get()       // usage count = 1
		obj2.IncrementUsage() // usage count = 2

		p.Put(obj1)
		p.Put(obj2)

		// Wait for cleanup to run
		time.Sleep(200 * time.Millisecond)

		// obj1 should be cleaned up (usage count < 2)
		// obj2 should remain (usage count >= 2)
		obj1Retrieved := p.Get()
		obj2Retrieved := p.Get()

		if obj1Retrieved == obj1 {
			t.Error("obj1 should have been cleaned up")
		}
		if obj2Retrieved != obj2 {
			t.Error("obj2 should not have been cleaned up")
		}
	})

	t.Run("cleanup disabled", func(t *testing.T) {
		cfg := pool.DefaultConfig(testAllocator, testCleaner)
		cfg.Cleanup.Enabled = false

		p, err := pool.NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		obj := p.Get()
		p.Put(obj)

		// Wait longer than any cleanup interval
		time.Sleep(1 * time.Second)

		// Object should still be in the pool
		objRetrieved := p.Get()
		if objRetrieved != obj {
			t.Error("Object should not be cleaned up when cleanup is disabled")
		}
	})
}

// TestPoolConcurrentAccess tests concurrent access to the pool.
func TestPoolConcurrentAccess(t *testing.T) {
	t.Run("concurrent get and put", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		const numGoroutines = 100
		const numOperations = 1000

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					obj := p.Get()
					if obj == nil {
						t.Error("Get() returned nil object")
						return
					}
					p.Put(obj)
				}
			}()
		}

		wg.Wait()
	})

	t.Run("concurrent getn and putn", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		const numGoroutines = 10
		const numOperations = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					objs := p.GetN(5)
					if len(objs) != 5 {
						t.Errorf("GetN(5) returned %d objects", len(objs))
						return
					}
					p.PutN(objs)
				}
			}()
		}

		wg.Wait()
	})
}

// TestPoolClose tests the Close method.
func TestPoolClose(t *testing.T) {
	t.Run("close with cleanup enabled", func(t *testing.T) {
		cfg := pool.DefaultConfig(testAllocator, testCleaner)
		cfg.Cleanup.Enabled = true
		cfg.Cleanup.Interval = 100 * time.Millisecond

		p, err := pool.NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}

		// Add some objects to the pool
		obj1 := p.Get()
		obj2 := p.Get()
		p.Put(obj1)
		p.Put(obj2)

		// Close the pool
		p.Close()

		// Try to get an object after closing (should still work for new objects)
		obj3 := p.Get()
		if obj3 == nil {
			t.Error("Get() after Close() should still work for new objects")
		}
	})

	t.Run("close with cleanup disabled", func(t *testing.T) {
		cfg := pool.DefaultConfig(testAllocator, testCleaner)
		cfg.Cleanup.Enabled = false

		p, err := pool.NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}

		// Add some objects to the pool
		obj1 := p.Get()
		obj2 := p.Get()
		p.Put(obj1)
		p.Put(obj2)

		// Close the pool
		p.Close()

		// Try to get an object after closing (should still work for new objects)
		obj3 := p.Get()
		if obj3 == nil {
			t.Error("Get() after Close() should still work for new objects")
		}
	})
}

// TestPoolFields tests the PoolFields functionality.
func TestPoolFields(t *testing.T) {
	t.Run("usage count operations", func(t *testing.T) {
		var fields pool.PoolFields[TestObject]

		// Initial usage count should be 0
		if fields.GetUsageCount() != 0 {
			t.Errorf("Initial usage count = %d, want 0", fields.GetUsageCount())
		}

		// Increment usage count
		fields.IncrementUsage()
		if fields.GetUsageCount() != 1 {
			t.Errorf("Usage count after increment = %d, want 1", fields.GetUsageCount())
		}

		fields.IncrementUsage()
		if fields.GetUsageCount() != 2 {
			t.Errorf("Usage count after second increment = %d, want 2", fields.GetUsageCount())
		}

		// Reset usage count
		fields.ResetUsage()
		if fields.GetUsageCount() != 0 {
			t.Errorf("Usage count after reset = %d, want 0", fields.GetUsageCount())
		}
	})

	t.Run("next pointer operations", func(t *testing.T) {
		var fields pool.PoolFields[TestObject]

		// Initial next pointer should be nil
		if fields.GetNext() != nil {
			t.Error("Initial next pointer should be nil")
		}

		// Set next pointer
		obj := &TestObject{ID: 1}
		fields.SetNext(obj)
		if fields.GetNext() != obj {
			t.Error("GetNext() should return the set object")
		}

		// Set to nil
		fields.SetNext(nil)
		if fields.GetNext() != nil {
			t.Error("GetNext() should return nil after setting to nil")
		}
	})
}

// TestDefaultCleanupPolicy tests the DefaultCleanupPolicy function.
func TestDefaultCleanupPolicy(t *testing.T) {
	tests := []struct {
		name     string
		level    pool.GcLevel
		enabled  bool
		interval time.Duration
		minCount int64
	}{
		{"disable", pool.GcDisable, false, 0, 0},
		{"low", pool.GcLow, true, 10 * time.Minute, 1},
		{"moderate", pool.GcModerate, true, 2 * time.Minute, 2},
		{"aggressive", pool.GcAggressive, true, 30 * time.Second, 3},
		{"unknown", "unknown", true, 2 * time.Minute, 2}, // fallback to moderate
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := pool.DefaultCleanupPolicy(tt.level)

			if policy.Enabled != tt.enabled {
				t.Errorf("DefaultCleanupPolicy(%s).Enabled = %v, want %v", tt.level, policy.Enabled, tt.enabled)
			}

			if policy.Interval != tt.interval {
				t.Errorf("DefaultCleanupPolicy(%s).Interval = %v, want %v", tt.level, policy.Interval, tt.interval)
			}

			if policy.MinUsageCount != tt.minCount {
				t.Errorf("DefaultCleanupPolicy(%s).MinUsageCount = %d, want %d", tt.level, policy.MinUsageCount, tt.minCount)
			}
		})
	}
}

// TestForceShardCount tests the ForceShardCount function.
func TestForceShardCount(t *testing.T) {
	// Test that we can force the shard count
	pool.ForceShardCount(16)

	// Create a pool and verify it uses the forced shard count
	p, err := pool.NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if len(p.Shards) != 16 {
		t.Errorf("Pool should have 16 shards, got %d", len(p.Shards))
	}

	// Reset to a reasonable value
	pool.ForceShardCount(8)
}

// TestPoolStressBasic tests the pool under heavy load.
func TestPoolStressBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	cfg := pool.DefaultConfig(testAllocator, testCleaner)
	cfg.Cleanup.Enabled = true
	cfg.Cleanup.Interval = 50 * time.Millisecond
	cfg.Cleanup.MinUsageCount = 1

	p, err := pool.NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	const (
		numGoroutines = 100
		duration      = 2 * time.Second
	)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var errors int64

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					obj := p.Get()
					if obj == nil {
						atomic.AddInt64(&errors, 1)
						continue
					}

					// Simulate some work
					obj.ID = obj.ID + 1
					obj.Value = obj.Value + "x"

					p.Put(obj)
				}
			}
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&errors) > 0 {
		t.Errorf("Encountered %d errors during stress test", atomic.LoadInt64(&errors))
	}
}

// TestPoolObjectReuseBasic tests that objects are properly reused.
func TestPoolObjectReuseBasic(t *testing.T) {
	p, err := pool.NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// Track unique objects
	objects := make(map[*TestObject]int)
	const iterations = 1000

	for i := 0; i < iterations; i++ {
		obj := p.Get()
		objects[obj]++
		p.Put(obj)
	}

	// We should have reused objects (fewer unique objects than iterations)
	uniqueCount := len(objects)
	if uniqueCount >= iterations {
		t.Errorf("Expected object reuse, got %d unique objects out of %d iterations", uniqueCount, iterations)
	}

	// Most objects should have been used multiple times
	var totalUsage int64
	for _, usage := range objects {
		totalUsage += int64(usage)
	}

	avgUsage := float64(totalUsage) / float64(uniqueCount)
	if avgUsage < 2.0 {
		t.Errorf("Expected average usage > 2.0, got %f", avgUsage)
	}
}

// TestPoolEdgeCases tests various edge cases.
func TestPoolEdgeCases(t *testing.T) {
	t.Run("get from empty pool", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		// Pool should be empty initially
		obj := p.Get()
		if obj == nil {
			t.Error("Get() from empty pool should create new object")
		}
	})

	t.Run("put object multiple times", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		obj := p.Get()
		p.Put(obj)
		p.Put(obj) // Should not cause issues

		// Should still be able to get the object
		obj2 := p.Get()
		if obj2 == nil {
			t.Error("Get() should still work after multiple Put() calls")
		}
	})

	t.Run("get after close", func(t *testing.T) {
		p, err := pool.NewPool(testAllocator, testCleaner)
		if err != nil {
			t.Fatal(err)
		}

		p.Close()

		// Should still be able to get new objects
		obj := p.Get()
		if obj == nil {
			t.Error("Get() after Close() should still work for new objects")
		}
	})
}
