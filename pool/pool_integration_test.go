package pool_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// TestObjectWithResources is a more complex test object that simulates real-world usage.
type TestObjectWithResources struct {
	// Real-world fields
	ID         int
	Value      string
	Data       []byte
	IsValid    bool
	CreatedAt  time.Time
	LastUsedAt time.Time

	pool.PoolFields[TestObjectWithResources]
}

func newTestObjectWithResources() *TestObjectWithResources {
	return &TestObjectWithResources{
		ID:        1,
		Value:     "test",
		Data:      make([]byte, 1024),
		IsValid:   true,
		CreatedAt: time.Now(),
	}
}

func cleanTestObjectWithResources(obj *TestObjectWithResources) {
	obj.ID = 0
	obj.Value = ""
	obj.Data = obj.Data[:0]
	obj.IsValid = false
}

// TestPoolStress tests the pool under heavy concurrent load.
func TestPoolStress(t *testing.T) {
	cfg := pool.DefaultConfig(newTestObjectWithResources, cleanTestObjectWithResources)
	p, err := pool.NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	defer p.Close()

	const (
		goroutines = 1000
		duration   = 5 * time.Second
	)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Track errors and panics
	var (
		errorsMu sync.Mutex
		errors   []error
	)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Errorf("goroutine %d panic: %v", id, r))
					errorsMu.Unlock()
				}
			}()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					obj := p.RetrieveOrCreate()
					if obj == nil {
						errorsMu.Lock()
						errors = append(errors, fmt.Errorf("goroutine %d: RetrieveOrCreate returned nil", id))
						errorsMu.Unlock()
						continue
					}

					time.Sleep(time.Millisecond)

					p.Put(obj)
				}
			}
		}(i)
	}

	wg.Wait()

	if len(errors) > 0 {
		t.Errorf("Encountered %d errors during stress test:", len(errors))
		for _, err := range errors {
			t.Logf("Error: %v", err)
		}
	}
}

// TestPoolObjectLifecycle verifies proper object lifecycle management.
func TestPoolObjectLifecycle(t *testing.T) {
	cfg := pool.DefaultConfig(newTestObjectWithResources, cleanTestObjectWithResources)
	cfg.Cleanup.Enabled = true
	cfg.Cleanup.Interval = 100 * time.Millisecond
	cfg.Cleanup.MinUsageCount = 2

	p, err := pool.NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	defer p.Close()

	// Test object creation and initial state
	obj := p.RetrieveOrCreate()
	if !obj.IsValid {
		t.Error("New object should be valid")
	}

	if obj.GetUsageCount() != 1 {
		t.Error("New object should have usage count 1")
	}

	// Test object reuse
	p.Put(obj)

	obj2 := p.RetrieveOrCreate()
	if obj2 != obj {
		t.Error("Expected to get the same object back")
	}

	if obj2.GetUsageCount() != 2 {
		t.Errorf("Expected usage count 2, got %d", obj2.GetUsageCount())
	}
}

// TestPoolConfigurationValidation verifies configuration validation.
func TestPoolConfigurationValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  pool.PoolConfig[TestObjectWithResources, *TestObjectWithResources]
		wantErr bool
	}{
		{
			name: "valid config",
			config: pool.PoolConfig[TestObjectWithResources, *TestObjectWithResources]{
				Allocator: newTestObjectWithResources,
				Cleaner:   cleanTestObjectWithResources,
			},
			wantErr: false,
		},
		{
			name: "nil allocator",
			config: pool.PoolConfig[TestObjectWithResources, *TestObjectWithResources]{
				Allocator: nil,
				Cleaner:   cleanTestObjectWithResources,
			},
			wantErr: true,
		},
		{
			name: "nil cleaner",
			config: pool.PoolConfig[TestObjectWithResources, *TestObjectWithResources]{
				Allocator: newTestObjectWithResources,
				Cleaner:   nil,
			},
			wantErr: true,
		},
		{
			name: "invalid cleanup interval",
			config: pool.PoolConfig[TestObjectWithResources, *TestObjectWithResources]{
				Allocator: newTestObjectWithResources,
				Cleaner:   cleanTestObjectWithResources,
				Cleanup: pool.CleanupPolicy{
					Enabled:  true,
					Interval: -1 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid MinUsageCount",
			config: pool.PoolConfig[TestObjectWithResources, *TestObjectWithResources]{
				Allocator: newTestObjectWithResources,
				Cleaner:   cleanTestObjectWithResources,
				Cleanup: pool.CleanupPolicy{
					Enabled:       true,
					Interval:      1 * time.Second,
					MinUsageCount: -1,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := pool.NewPoolWithConfig(tt.config)
			if p != nil {
				defer p.Close()
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("NewPoolWithConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestPoolObjectReuse verifies that objects are properly reused and cleaned.
func TestPoolObjectReuse(t *testing.T) {
	cfg := pool.DefaultConfig(newTestObjectWithResources, cleanTestObjectWithResources)
	cfg.Cleanup.Enabled = true
	cfg.Cleanup.Interval = 100 * time.Millisecond
	cfg.Cleanup.MinUsageCount = 1

	p, err := pool.NewPoolWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	defer p.Close()

	// Create and track objects
	objects := make(map[*TestObjectWithResources]int)
	const iterations = 100

	for range iterations {
		obj := p.RetrieveOrCreate()
		objects[obj]++
		p.Put(obj)
	}

	// Verify object reuse
	uniqueObjects := len(objects)
	if uniqueObjects > iterations/2 {
		t.Errorf("Too many unique objects created: %d, expected fewer than %d", uniqueObjects, iterations/2)
	}
}
