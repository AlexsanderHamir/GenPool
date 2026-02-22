// Config, cleanup policy, growth policy, and validation for the pool.
package pool

import (
	"errors"
	"fmt"
	"runtime"
	"time"
)

// Common errors that may be returned by the pool.
var (
	// ErrNoAllocator is returned when attempting to get an object but no allocator is configured.
	ErrNoAllocator = errors.New("no allocator configured")

	// ErrNoCleaner is returned when attempting to create a pool but no cleaner is configured.
	ErrNoCleaner = errors.New("no cleaner configured")
)

// GcLevel offers different levels for clean up configuration.
// These presets control how aggressively GenPool reclaims memory.
// Note: Go's GC may still run unless you explicitly suppress it via debug.SetGCPercent(-1)
type GcLevel string

var (
	// GcDisable disables GenPool's cleanup completely.
	// Objects will stay in the pool indefinitely unless manually cleared.
	GcDisable GcLevel = "disable"

	// GcLow performs cleanup at long intervals with minimal aggression.
	// Good for low-latency, high-reuse scenarios.
	GcLow GcLevel = "low"

	// GcModerate performs cleanup at regular intervals and evicts objects
	// that are lightly used. Balances reuse and memory usage.
	GcModerate GcLevel = "moderate"

	// GcAggressive enables frequent cleanup and removes objects
	// that are not reused often. Best for memory-constrained environments.
	GcAggressive GcLevel = "aggressive"
)

// The number of shards is tied to GOMAXPROCS (max OS threads running Go code in parallel).
// To reduce sharding, adjust GOMAXPROCS via runtime.GOMAXPROCS(n) before creating the pool.
var (
	numShards = runtime.GOMAXPROCS(0)
)

// CleanupPolicy defines how the pool should clean up unused objects.
type CleanupPolicy struct {
	// Enabled determines if automatic cleanup is enabled.
	Enabled bool
	// Interval is how often the cleanup should run.
	Interval time.Duration
	// MinUsageCount is the number of usage BELOW which an object will be evicted.
	MinUsageCount int64
}

// DefaultCleanupPolicy returns a default cleanup configuration based on specified level.
func DefaultCleanupPolicy(level GcLevel) CleanupPolicy {
	switch level {
	case GcDisable:
		return CleanupPolicy{}
	case GcLow:
		return CleanupPolicy{
			Enabled:       true,
			Interval:      10 * time.Minute,
			MinUsageCount: 1,
		}
	case GcModerate:
		return CleanupPolicy{
			Enabled:       true,
			Interval:      2 * time.Minute,
			MinUsageCount: 2,
		}
	case GcAggressive:
		return CleanupPolicy{
			Enabled:       true,
			Interval:      30 * time.Second,
			MinUsageCount: 3,
		}
	default:
		// Fallback to moderate if unrecognized
		return CleanupPolicy{
			Enabled:       true,
			Interval:      2 * time.Minute,
			MinUsageCount: 2,
		}
	}
}

// Config holds configuration options for the pool.
type Config[T any, P Poolable[T]] struct {
	// Cleanup defines the cleanup policy for the pool
	Cleanup CleanupPolicy

	// Growth defined the growth policy for the pool
	Growth GrowthPolicy

	// Allocator is the function to create new objects
	Allocator Allocator[T]

	// Cleaner is the function to clean objects before returning them to the pool
	Cleaner Cleaner[T]
}

// GrowthPolicy controls how the pool is allowed to grow.
// If unset, the pool will grow indefinitely, and any cleanup will rely solely on the CleanupPolicy.
type GrowthPolicy struct {
	// MaxPoolSize defines the maximum number of objects the pool is allowed to grow to.
	MaxPoolSize int64

	// Enable activates growth control. If disabled, the pool will grow and shrink freely based on your configuration.
	Enable bool
}

// DefaultConfig returns a default pool configuration for type T.
func DefaultConfig[T any, P Poolable[T]](allocator Allocator[T], cleaner Cleaner[T]) Config[T, P] {
	return Config[T, P]{
		Cleanup:   DefaultCleanupPolicy(GcModerate),
		Allocator: allocator,
		Cleaner:   cleaner,
	}
}

func validateConfig[T any, P Poolable[T]](cfg Config[T, P]) error {
	if cfg.Allocator == nil {
		return fmt.Errorf("%w: allocator is required", ErrNoAllocator)
	}
	if cfg.Cleaner == nil {
		return fmt.Errorf("%w: cleaner is required", ErrNoCleaner)
	}

	return nil
}

func validateCleanupConfig[T any, P Poolable[T]](cfg Config[T, P]) error {
	if cfg.Cleanup.Interval <= 0 {
		return errors.New("cleanup interval must be greater than 0")
	}
	if cfg.Cleanup.MinUsageCount <= 0 {
		return errors.New("minimum usage count must be greater than 0")
	}
	return nil
}
