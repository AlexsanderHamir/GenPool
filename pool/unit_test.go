package pool

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestObject is a simple struct we'll use for testing.
type TestObject struct {
	ID    int
	Value string
	Fields[TestObject]
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

// TestDefaultCleanupPolicy tests all GC levels and the default fallback
func TestDefaultCleanupPolicy(t *testing.T) {
	tests := []struct {
		name     string
		level    GcLevel
		expected CleanupPolicy
	}{
		{
			name:  "GcDisable",
			level: GcDisable,
			expected: CleanupPolicy{
				Enabled: false,
			},
		},
		{
			name:  "GcLow",
			level: GcLow,
			expected: CleanupPolicy{
				Enabled:       true,
				Interval:      10 * time.Minute,
				MinUsageCount: 1,
			},
		},
		{
			name:  "GcModerate",
			level: GcModerate,
			expected: CleanupPolicy{
				Enabled:       true,
				Interval:      2 * time.Minute,
				MinUsageCount: 2,
			},
		},
		{
			name:  "GcAggressive",
			level: GcAggressive,
			expected: CleanupPolicy{
				Enabled:       true,
				Interval:      30 * time.Second,
				MinUsageCount: 3,
			},
		},
		{
			name:  "UnknownLevel",
			level: "unknown",
			expected: CleanupPolicy{
				Enabled:       true,
				Interval:      2 * time.Minute,
				MinUsageCount: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultCleanupPolicy(tt.level)
			if result.Enabled != tt.expected.Enabled {
				t.Errorf("DefaultCleanupPolicy() Enabled = %v, want %v", result.Enabled, tt.expected.Enabled)
			}
			if result.Interval != tt.expected.Interval {
				t.Errorf("DefaultCleanupPolicy() Interval = %v, want %v", result.Interval, tt.expected.Interval)
			}
			if result.MinUsageCount != tt.expected.MinUsageCount {
				t.Errorf("DefaultCleanupPolicy() MinUsageCount = %v, want %v", result.MinUsageCount, tt.expected.MinUsageCount)
			}
		})
	}
}

// TestFields tests all the Fields methods
func TestFields(t *testing.T) {
	fields := &Fields[TestObject]{}

	// Test initial state
	if fields.GetNext() != nil {
		t.Error("GetNext() should return nil initially")
	}
	if fields.GetUsageCount() != 0 {
		t.Error("GetUsageCount() should return 0 initially")
	}

	// Test SetNext and GetNext
	obj1 := &TestObject{ID: 1}
	obj2 := &TestObject{ID: 2}
	fields.SetNext(obj1)
	if fields.GetNext() != obj1 {
		t.Error("GetNext() should return the set object")
	}

	// Test IncrementUsage
	fields.IncrementUsage()
	if fields.GetUsageCount() != 1 {
		t.Error("IncrementUsage() should increment usage count")
	}

	fields.IncrementUsage()
	if fields.GetUsageCount() != 2 {
		t.Error("IncrementUsage() should increment usage count again")
	}

	// Test ResetUsage
	fields.ResetUsage()
	if fields.GetUsageCount() != 0 {
		t.Error("ResetUsage() should reset usage count to 0")
	}

	// Test SetNext again
	fields.SetNext(obj2)
	if fields.GetNext() != obj2 {
		t.Error("GetNext() should return the newly set object")
	}
}

// TestDefaultConfig tests the DefaultConfig function
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig(testAllocator, testCleaner)

	if cfg.Allocator == nil {
		t.Error("DefaultConfig() Allocator should not be nil")
	}
	if cfg.Cleaner == nil {
		t.Error("DefaultConfig() Cleaner should not be nil")
	}
	if !cfg.Cleanup.Enabled {
		t.Error("DefaultConfig() Cleanup should be enabled by default")
	}
	if cfg.Cleanup.Interval != 2*time.Minute {
		t.Error("DefaultConfig() Cleanup interval should be 2 minutes")
	}
	if cfg.Cleanup.MinUsageCount != 2 {
		t.Error("DefaultConfig() MinUsageCount should be 2")
	}
}

// TestNewPool tests the NewPool function
func TestNewPool(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatalf("NewPool() error = %v, want nil", err)
	}
	defer pool.Close()

	if pool == nil {
		t.Error("NewPool() returned nil pool")
	}
}

// TestNewPoolWithConfig tests the NewPoolWithConfig function with various configurations
func TestNewPoolWithConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config[TestObject, *TestObject]
		wantErr bool
	}{
		{
			name: "ValidConfig",
			cfg: Config[TestObject, *TestObject]{
				Allocator: testAllocator,
				Cleaner:   testCleaner,
				Cleanup: CleanupPolicy{
					Enabled:       true,
					Interval:      100 * time.Millisecond,
					MinUsageCount: 1,
				},
			},
			wantErr: false,
		},
		{
			name: "NoAllocator",
			cfg: Config[TestObject, *TestObject]{
				Allocator: nil,
				Cleaner:   testCleaner,
				Cleanup:   CleanupPolicy{},
			},
			wantErr: true,
		},
		{
			name: "NoCleaner",
			cfg: Config[TestObject, *TestObject]{
				Allocator: testAllocator,
				Cleaner:   nil,
				Cleanup:   CleanupPolicy{},
			},
			wantErr: true,
		},
		{
			name: "InvalidCleanupInterval",
			cfg: Config[TestObject, *TestObject]{
				Allocator: testAllocator,
				Cleaner:   testCleaner,
				Cleanup: CleanupPolicy{
					Enabled:       true,
					Interval:      0,
					MinUsageCount: 1,
				},
			},
			wantErr: true,
		},
		{
			name: "InvalidMinUsageCount",
			cfg: Config[TestObject, *TestObject]{
				Allocator: testAllocator,
				Cleaner:   testCleaner,
				Cleanup: CleanupPolicy{
					Enabled:       true,
					Interval:      100 * time.Millisecond,
					MinUsageCount: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "ShardNumOverride",
			cfg: Config[TestObject, *TestObject]{
				Allocator:        testAllocator,
				Cleaner:          testCleaner,
				ShardNumOverride: 4,
				Cleanup:          CleanupPolicy{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewPoolWithConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPoolWithConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if pool != nil {
				defer pool.Close()
			}
		})
	}
}

// TestGetShardCount tests the getShardCount function
func TestGetShardCount(t *testing.T) {
	// Test with no override
	cfg := Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
	}
	count := getShardCount(cfg)
	if count != numShards {
		t.Errorf("getShardCount() = %v, want %v", count, numShards)
	}

	// Test with override
	cfg.ShardNumOverride = 16
	count = getShardCount(cfg)
	if count != 16 {
		t.Errorf("getShardCount() with override = %v, want 16", count)
	}
}

// TestInitShards tests the initShards function
func TestInitShards(t *testing.T) {
	pool := &ShardedPool[TestObject, *TestObject]{
		Shards:        make([]*Shard[TestObject, *TestObject], 4),
		blockedShards: map[int]*atomic.Int64{},
	}

	initShards(pool)

	for i, shard := range pool.Shards {
		if shard == nil {
			t.Errorf("Shard %d should not be nil", i)
		}
		if shard.Head.Load() != nil {
			t.Errorf("Shard %d head should be nil initially", i)
		}
	}
}

// TestGetShard tests the getShard method
func TestGetShard(t *testing.T) {
	pool := &ShardedPool[TestObject, *TestObject]{
		Shards:        make([]*Shard[TestObject, *TestObject], 4),
		blockedShards: map[int]*atomic.Int64{},
	}

	initShards(pool)

	shard, _ := pool.getShard()
	if shard == nil {
		t.Error("getShard() should not return nil")
	}
}

// TestGet tests the Get method
func TestGet(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Test getting a new object
	obj := pool.Get()
	if obj == nil {
		t.Error("Get() returned nil object")
	}

	if obj.ID != 1 || obj.Value != "test" {
		t.Errorf("Get() got = %+v, want ID=1, Value=test", obj)
	}

	if obj.GetUsageCount() != 1 {
		t.Errorf("Get() usage count = %d, want 1", obj.GetUsageCount())
	}

	// Test getting from pool after returning
	pool.Put(obj)
	obj2 := pool.Get()
	if obj2 == nil {
		t.Error("Get() returned nil object after Put")
	}
	if obj2.GetUsageCount() != 2 {
		t.Errorf("Get() usage count after Put = %d, want 1", obj2.GetUsageCount())
	}
}

// TestGetN tests the GetN method
func TestGetN(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	objs := pool.GetN(3)
	if len(objs) != 3 {
		t.Errorf("GetN() returned %d objects, want 3", len(objs))
	}

	for i, obj := range objs {
		if obj == nil {
			t.Errorf("GetN() obj[%d] is nil", i)
		}
		if obj.GetUsageCount() != 1 {
			t.Errorf("GetN() obj[%d] usage count = %d, want 1", i, obj.GetUsageCount())
		}
	}
}

// TestPut tests the Put method
func TestPut(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	obj := pool.Get()

	pool.Put(obj)

	// Verify cleaner was called
	if obj.ID != 0 || obj.Value != "" {
		t.Errorf("Put() cleaner not called, got ID=%d, Value=%s", obj.ID, obj.Value)
	}
}

// TestPutN tests the PutN method
func TestPutN(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	objs := pool.GetN(3)
	pool.PutN(objs)

	// Verify all objects were cleaned
	for i, obj := range objs {
		if obj.ID != 0 || obj.Value != "" {
			t.Errorf("PutN() obj[%d] not cleaned, got ID=%d, Value=%s", i, obj.ID, obj.Value)
		}
	}
}

// TestRetrieveFromShard tests the retrieveFromShard method
func TestRetrieveFromShard(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	shard, _ := pool.getShard()

	// Test with empty shard
	obj, success := pool.retrieveFromShard(shard)
	if success {
		t.Error("retrieveFromShard() should return false for empty shard")
	}

	if obj != nil {
		t.Error("retrieveFromShard() should return nil for empty shard")
	}
}

// TestClear tests the clear method
func TestClear(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Add some objects to the pool
	objs := pool.GetN(3)
	pool.PutN(objs)

	// Clear the pool
	pool.clear()

	// Verify all shards are empty
	for i, shard := range pool.Shards {
		if shard.Head.Load() != nil {
			t.Errorf("clear() shard[%d] not empty", i)
		}
	}
}

// TestStartCleaner tests the startCleaner method
func TestStartCleaner(t *testing.T) {
	cfg := Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      10 * time.Millisecond,
			MinUsageCount: 1,
		},
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Verify cleaner was started
	if pool.stopClean == nil {
		t.Error("startCleaner() should initialize stopClean channel")
	}
}

// TestCleanup tests the cleanup method
func TestCleanup(t *testing.T) {
	cfg := Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      10 * time.Millisecond,
			MinUsageCount: 2,
		},
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Test cleanup with disabled cleanup
	pool.cfg.Cleanup.Enabled = false
	pool.cleanup() // Should return early

	// Test cleanup with enabled cleanup
	pool.cfg.Cleanup.Enabled = true
	pool.cleanup() // Should process all shards
}

// TestCleanupShard tests the cleanupShard method
func TestCleanupShard(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	shard, _ := pool.getShard()

	// Test with empty shard
	pool.cleanupShard(shard)

	// Test with populated shard
	obj := pool.Get()
	obj.IncrementUsage() // Usage count = 2
	pool.Put(obj)

	pool.cfg.Cleanup.MinUsageCount = 1
	pool.cleanupShard(shard)
}

// TestTryTakeOwnership tests the tryTakeOwnership method
func TestTryTakeOwnership(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	shard, _ := pool.getShard()

	// Test with empty shard
	obj := pool.tryTakeOwnership(shard)
	if obj != nil {
		t.Error("tryTakeOwnership() should return nil for empty shard")
	}
}

// TestFilterUsableObjects tests the filterUsableObjects method
func TestFilterUsableObjects(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Create a chain of objects with different usage counts
	obj1 := pool.Get()
	obj2 := pool.Get()
	obj3 := pool.Get()

	obj1.IncrementUsage() // Usage count = 2
	obj2.IncrementUsage() // Usage count = 2
	obj3.IncrementUsage() // Usage count = 2

	// Link them together
	obj1.SetNext(obj2)
	obj2.SetNext(obj3)
	obj3.SetNext(nil)

	pool.cfg.Cleanup.MinUsageCount = 2

	keptHead, keptTail, _ := pool.filterUsableObjects(obj1)

	if keptHead == nil {
		t.Error("filterUsableObjects() should return kept objects")
	}
	if keptTail == nil {
		t.Error("filterUsableObjects() should return kept tail")
	}

	// Verify usage counts were reset
	if keptHead.GetUsageCount() != 0 {
		t.Error("filterUsableObjects() should reset usage count")
	}
}

// TestReinsertKeptObjects tests the reinsertKeptObjects method
func TestReinsertKeptObjects(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	shard, _ := pool.getShard()

	// Create objects to reinsert
	obj1 := pool.Get()
	obj2 := pool.Get()
	obj1.SetNext(obj2)
	obj2.SetNext(nil)

	pool.reinsertKeptObjects(shard, obj1, obj2)

	// Verify objects were reinserted
	if shard.Head.Load() != obj1 {
		t.Error("reinsertKeptObjects() should reinsert objects")
	}
}

// TestClose tests the Close method
func TestClose(t *testing.T) {
	cfg := Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      10 * time.Millisecond,
			MinUsageCount: 1,
		},
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Add some objects
	objs := pool.GetN(3)
	pool.PutN(objs)

	// Close the pool
	pool.Close()

	// Verify channels are closed
	select {
	case <-pool.stopClean:
		// Channel is closed, which is expected
	default:
		t.Error("Close() should close stopClean channel")
	}
}

// TestCloseWithoutCleanup tests Close when cleanup is disabled
func TestCloseWithoutCleanup(t *testing.T) {
	cfg := Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
		Cleanup: CleanupPolicy{
			Enabled: false,
		},
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Close should not panic
	pool.Close()
}

// TestConcurrentAccess tests concurrent access to the pool
func TestConcurrentAccess(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Test concurrent Get operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			obj := pool.Get()
			if obj != nil {
				pool.Put(obj)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestErrorMessages tests error message formatting
func TestErrorMessages(t *testing.T) {
	// Test ErrNoAllocator
	err := validateConfig(Config[TestObject, *TestObject]{
		Allocator: nil,
		Cleaner:   testCleaner,
	})
	if err == nil {
		t.Error("validateConfig() should return error for nil allocator")
	}
	if !errors.Is(err, ErrNoAllocator) {
		t.Errorf("validateConfig() error = %v, want ErrNoAllocator", err)
	}

	// Test ErrNoCleaner
	err = validateConfig(Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   nil,
	})
	if err == nil {
		t.Error("validateConfig() should return error for nil cleaner")
	}
	if !errors.Is(err, ErrNoCleaner) {
		t.Errorf("validateConfig() error = %v, want ErrNoCleaner", err)
	}

	// Test invalid cleanup interval
	err = validateCleanupConfig(Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      0,
			MinUsageCount: 1,
		},
	})
	if err == nil {
		t.Error("validateCleanupConfig() should return error for zero interval")
	}

	// Test invalid min usage count
	err = validateCleanupConfig(Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      100 * time.Millisecond,
			MinUsageCount: 0,
		},
	})
	if err == nil {
		t.Error("validateCleanupConfig() should return error for zero min usage count")
	}
}

// TestFilterUsableObjectsEdgeCases tests edge cases in filterUsableObjects
func TestFilterUsableObjectsEdgeCases(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Test with objects that should be discarded (usage count < MinUsageCount)
	obj1 := pool.Get()
	obj2 := pool.Get()
	obj3 := pool.Get()

	// Set usage counts: obj1=1 (should be discarded), obj2=2 (should be kept), obj3=1 (should be discarded)
	obj1.IncrementUsage() // Usage count = 2
	obj2.IncrementUsage() // Usage count = 2
	obj3.IncrementUsage() // Usage count = 2

	// Link them together
	obj1.SetNext(obj2)
	obj2.SetNext(obj3)
	obj3.SetNext(nil)

	// Set MinUsageCount to 3, so all objects should be discarded
	pool.cfg.Cleanup.MinUsageCount = 3

	keptHead, keptTail, _ := pool.filterUsableObjects(obj1)

	// All objects should be discarded
	if keptHead != nil {
		t.Error("filterUsableObjects() should return nil when all objects are discarded")
	}
	if keptTail != nil {
		t.Error("filterUsableObjects() should return nil tail when all objects are discarded")
	}

	// Verify objects were properly cleaned up
	if obj1.GetNext() != nil {
		t.Error("filterUsableObjects() should set next to nil for discarded objects")
	}
	if obj2.GetNext() != nil {
		t.Error("filterUsableObjects() should set next to nil for discarded objects")
	}
	if obj3.GetNext() != nil {
		t.Error("filterUsableObjects() should set next to nil for discarded objects")
	}
}

// TestFilterUsableObjectsMixed tests filtering with mixed usage counts
func TestFilterUsableObjectsMixed(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Create objects with different usage counts
	obj1 := pool.Get()
	obj2 := pool.Get()
	obj3 := pool.Get()
	obj4 := pool.Get()

	// Set usage counts: obj1=1 (discard), obj2=3 (keep), obj3=1 (discard), obj4=3 (keep)
	obj1.IncrementUsage() // Usage count = 2
	obj2.IncrementUsage() // Usage count = 2
	obj2.IncrementUsage() // Usage count = 3
	obj3.IncrementUsage() // Usage count = 2
	obj4.IncrementUsage() // Usage count = 2
	obj4.IncrementUsage() // Usage count = 3

	// Link them together
	obj1.SetNext(obj2)
	obj2.SetNext(obj3)
	obj3.SetNext(obj4)
	obj4.SetNext(nil)

	// Set MinUsageCount to 3, so only obj2 and obj4 should be kept
	pool.cfg.Cleanup.MinUsageCount = 3

	keptHead, keptTail, _ := pool.filterUsableObjects(obj1)

	// Should keep obj2 and obj4
	if keptHead != obj2 {
		t.Error("filterUsableObjects() should return obj2 as kept head")
	}
	if keptTail != obj4 {
		t.Error("filterUsableObjects() should return obj4 as kept tail")
	}

	// Verify usage counts were reset
	if keptHead.GetUsageCount() != 0 {
		t.Error("filterUsableObjects() should reset usage count for kept objects")
	}
	if keptTail.GetUsageCount() != 0 {
		t.Error("filterUsableObjects() should reset usage count for kept objects")
	}

	// Verify discarded objects have nil next
	if obj1.GetNext() != nil {
		t.Error("filterUsableObjects() should set next to nil for discarded objects")
	}
	if obj3.GetNext() != nil {
		t.Error("filterUsableObjects() should set next to nil for discarded objects")
	}
}

// TestReinsertKeptObjectsContention tests the retry logic in reinsertKeptObjects
func TestReinsertKeptObjectsContention(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	shard, _ := pool.getShard()

	// Create objects to reinsert
	obj1 := pool.Get()
	obj2 := pool.Get()
	obj1.SetNext(obj2)
	obj2.SetNext(nil)

	// Add some objects to the shard first
	existingObj := pool.Get()
	pool.Put(existingObj)

	// Start a goroutine to create contention by swapping the head repeatedly
	stop := make(chan struct{})
	var contended atomic.Bool

	go func() {
		for !contended.Load() {
			shard.Head.CompareAndSwap(existingObj, nil)
			runtime.Gosched()
		}
		stop <- struct{}{}
	}()

	// Call reinsertKeptObjects, which should eventually succeed after contention
	pool.reinsertKeptObjects(shard, obj1, obj2)
	contended.Store(true)
	<-stop

	// Verify objects were reinserted
	if shard.Head.Load() != obj1 {
		t.Error("reinsertKeptObjects() should reinsert objects")
	}
}

// TestClearRaceCondition tests the race condition handling in clear()
func TestClearRaceCondition(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Add some objects to the pool
	objs := pool.GetN(5)
	pool.PutN(objs)

	// Create a goroutine that continuously adds objects while we clear
	done := make(chan bool)
	go func() {
		for i := 0; i < 10; i++ {
			obj := pool.Get()
			pool.Put(obj)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Clear the pool while objects are being added
	pool.clear()

	// Wait for the goroutine to finish
	<-done

	// Clear again to ensure all objects added during the race are also cleared
	pool.clear()

	// Verify all shards are empty after clear
	for i, shard := range pool.Shards {
		if shard.Head.Load() != nil {
			t.Errorf("clear() shard[%d] not empty after race condition test", i)
		}
	}
}

// TestCleanupShardWithDiscardedObjects tests cleanup when objects are discarded
func TestCleanupShardWithDiscardedObjects(t *testing.T) {
	cfg := Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      10 * time.Millisecond,
			MinUsageCount: 3, // High threshold to force discarding
		},
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Add objects with low usage count
	obj1 := pool.Get()
	obj2 := pool.Get()
	obj3 := pool.Get()

	// Set usage counts to 1 (below MinUsageCount of 3)
	obj1.IncrementUsage() // Usage count = 2
	obj2.IncrementUsage() // Usage count = 2
	obj3.IncrementUsage() // Usage count = 2

	pool.Put(obj1)
	pool.Put(obj2)
	pool.Put(obj3)

	// Trigger cleanup
	pool.cleanup()

	// All objects should be discarded due to low usage count
	// The pool should be empty after cleanup
	shard, _ := pool.getShard()
	if shard.Head.Load() != nil {
		t.Error("cleanup() should discard objects with usage count below MinUsageCount")
	}
}

// TestConcurrentPutAndCleanup tests concurrent Put operations during cleanup
func TestConcurrentPutAndCleanup(t *testing.T) {
	cfg := Config[TestObject, *TestObject]{
		Allocator: testAllocator,
		Cleaner:   testCleaner,
		Cleanup: CleanupPolicy{
			Enabled:       true,
			Interval:      10 * time.Millisecond,
			MinUsageCount: 1,
		},
	}

	pool, err := NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Start multiple goroutines putting objects
	done := make(chan bool, 5)
	for range 5 {
		go func() {
			for range 10 {
				obj := pool.Get()
				obj.IncrementUsage() // Increase usage count
				pool.Put(obj)
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to finish
	for range 5 {
		<-done
	}

	// Trigger cleanup manually
	pool.cleanup()

	// Verify pool is still functional
	obj := pool.Get()
	if obj == nil {
		t.Error("Pool should still be functional after concurrent operations")
	}
	pool.Put(obj)
}

// TestTryTakeOwnershipRaceCondition tests the race condition in tryTakeOwnership
func TestTryTakeOwnershipRaceCondition(t *testing.T) {
	pool, err := NewPool(testAllocator, testCleaner)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	shard, _ := pool.getShard()

	// Add some objects to the shard
	obj1 := pool.Get()
	obj2 := pool.Get()
	pool.Put(obj1)
	pool.Put(obj2)

	// Create a goroutine that continuously modifies the shard head
	done := make(chan bool)
	go func() {
		for range 20 {
			obj := pool.Get()
			pool.Put(obj)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Try to take ownership multiple times to trigger race conditions
	for range 10 {
		result := pool.tryTakeOwnership(shard)
		if result != nil {
			// If we successfully took ownership, put the object back
			pool.Put(result)
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Wait for the goroutine to finish
	<-done

	// Verify the pool is still functional
	obj := pool.Get()
	if obj == nil {
		t.Error("Pool should still be functional after race condition test")
	}
	pool.Put(obj)
}

// TestGrowthPolicy tests the GrowthPolicy (MaxPoolSize and Enable)
func TestGrowthPolicy(t *testing.T) {
	t.Run("GrowthEnabled_MaxPoolSize", func(t *testing.T) {
		cfg := DefaultConfig(testAllocator, testCleaner)
		cfg.Cleanup.Enabled = false
		cfg.Growth.Enable = true
		cfg.Growth.MaxPoolSize = 2

		pool, err := NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatalf("NewPoolWithConfig() error = %v", err)
		}
		defer pool.Close()

		// Should be able to get up to MaxPoolSize new objects
		obj1 := pool.Get()
		if obj1 == nil {
			t.Error("Get() returned nil for first object")
		}
		obj2 := pool.Get()
		if obj2 == nil {
			t.Error("Get() returned nil for second object")
		}

		// Should not be able to get a third new object (unless one is returned)
		obj3 := pool.Get()
		if obj3 != nil {
			t.Error("Get() should return nil when MaxPoolSize is reached and no reusable objects are available")
		}

		// Return one object, should be able to get again
		pool.Put(obj1)
		obj4 := pool.Get()
		if obj4 == nil {
			t.Error("Get() should return object after one is returned to pool")
		}
	})

	t.Run("BlocksWhenPoolAtMax", func(t *testing.T) {
		cfg := DefaultConfig(testAllocator, testCleaner)
		cfg.Cleanup.Enabled = false
		cfg.Growth.Enable = true
		cfg.Growth.MaxPoolSize = 2

		pool, err := NewPoolWithConfig(cfg)
		if err != nil {
			t.Fatalf("NewPoolWithConfig() error = %v", err)
		}
		defer pool.Close()

		// Fill pool to max size
		obj1 := pool.GetBlock()
		obj2 := pool.GetBlock()
		if obj1 == nil || obj2 == nil {
			t.Fatal("failed to allocate initial objects")
		}

		blockedCh := make(chan *TestObject, 1)

		// This should block until an object is returned
		go func() {
			obj3 := pool.GetBlock()
			blockedCh <- obj3
		}()

		// Ensure blocking actually happens (give the goroutine time to hit Wait)
		time.Sleep(100 * time.Millisecond)

		select {
		case <-blockedCh:
			t.Error("GetBlock() returned early — expected it to block")
		default:
			// Expected: still blocked
		}

		// Return one object to unblock the waiting goroutine
		pool.PutBlock(obj1)

		select {
		case obj3 := <-blockedCh:
			if obj3 == nil {
				t.Error("GetBlock() unblocked but returned nil object")
			}
		case <-time.After(5 * time.Second):
			t.Error("GetBlock() did not unblock after object was returned")
		}
	})

}
