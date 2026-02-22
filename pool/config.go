// Config, cleanup policy, growth policy, validation, and shard count. Used when
// building a pool via NewPoolWithConfig.
package pool

import (
	"errors"
	"fmt"
	"runtime"
	"time"
)

// Common errors returned by the pool.
var (
	ErrNoAllocator = errors.New("no allocator configured")
	ErrNoCleaner   = errors.New("no cleaner configured")
)

// GcLevel selects how aggressively the pool reclaims memory. Go's GC may still run.
type GcLevel string

var (
	GcDisable   GcLevel = "disable"
	GcLow       GcLevel = "low"
	GcModerate  GcLevel = "moderate"
	GcAggressive GcLevel = "aggressive"
)

// numShards is GOMAXPROCS at init. Set runtime.GOMAXPROCS(n) before NewPool to change.
var (
	numShards = runtime.GOMAXPROCS(0)
)

// CleanupPolicy configures automatic eviction of underused objects.
type CleanupPolicy struct {
	Enabled       bool
	Interval      time.Duration
	MinUsageCount int64
}

// DefaultCleanupPolicy returns a CleanupPolicy for the given level; unknown levels use moderate.
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
		return CleanupPolicy{
			Enabled:       true,
			Interval:      2 * time.Minute,
			MinUsageCount: 2,
		}
	}
}

// Config holds pool configuration.
type Config[T any, P Poolable[T]] struct {
	Cleanup   CleanupPolicy
	Growth    GrowthPolicy
	Allocator Allocator[T]
	Cleaner   Cleaner[T]
}

// GrowthPolicy limits pool size when Enable is true.
type GrowthPolicy struct {
	MaxPoolSize int64
	Enable      bool
}

// DefaultConfig returns a config with moderate cleanup and the given allocator/cleaner.
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
