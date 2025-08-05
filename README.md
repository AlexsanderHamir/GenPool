# GenPool

GenPool outperforms sync.Pool in scenarios where objects are retained for longer periods, while also giving you fine-grained control over memory reclamation timing and aggressiveness.

> If your system rarely retains objects, you’re unlikely to benefit from GenPool’s design.

[![GoDoc](https://godoc.org/github.com/AlexsanderHamir/GenPool?status.svg)](https://godoc.org/github.com/AlexsanderHamir/GenPool)
![Build](https://github.com/AlexsanderHamir/GenPool/actions/workflows/test.yml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/AlexsanderHamir/GenPool/badge.png?branch=main)](https://coveralls.io/github/AlexsanderHamir/GenPool?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/AlexsanderHamir/GenPool)](https://goreportcard.com/report/github.com/AlexsanderHamir/GenPool)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Issues](https://img.shields.io/github/issues/AlexsanderHamir/GenPool)
![Last Commit](https://img.shields.io/github/last-commit/AlexsanderHamir/GenPool)
![Code Size](https://img.shields.io/github/languages/code-size/AlexsanderHamir/GenPool)
![Version](https://img.shields.io/github/v/tag/AlexsanderHamir/GenPool?sort=semver)
![Go Version](https://img.shields.io/badge/Go-1.24.3%2B-blue)

## Table of Contents

- [Performance](#performance)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Features](#features)
- [Cleanup Policy](#cleanup-policy)
- [Cleanup Levels](#cleanup-levels)
- [Growth Policy](#growth-policy)
- [Manual Control](#manual-control)
- [Contributing](#contributing)
- [License](#license)

## Performance

### Environment

- **GOOS:** darwin
- **GOARCH:** arm64
- **CPU:** Apple M1
- **Package:** `github.com/AlexsanderHamir/GenPool/pool`

### Benchmark Results

| Latency Level Workload | Metric         | GenPool   | SyncPool  | Delta Value | Delta %    |
| ---------------------- | -------------- | --------- | --------- | ----------- | ---------- |
| **High**               | Avg Iterations | 92,090    | 85,018    | +7,072      | +8.32%     |
|                        | Avg Time (ns)  | 12,268    | 13,070    | **-802**    | **-6.14%** |
| **Moderate**           | Avg Iterations | 869,492   | 840,131   | +29,361     | +3.49%     |
|                        | Avg Time (ns)  | 1,223.8   | 1,316.7   | **-92.9**   | **-7.05%** |
| **Low**                | Avg Iterations | 6,004,695 | 6,099,886 | -95,191     | -1.56%     |
|                        | Avg Time (ns)  | 197.46    | 194.26    | **+3.2**    | **+1.65%** |

> **Full benchmark details:** [GenPool vs sync.Pool](./benchmark_results_transparency)

### Summary

- As shown, **GenPool** performs better when objects are held for longer periods. Its performance degrades as retention time decreases, due to the overhead of its sharded design.

- For detailed results and interactive graphs, see the [Benchmark Results Transparency](/benchmark_results_transparency) page.
  - In short, the benchmarks revealed that across all scenarios—whether using a single shard or many, and under both high and low concurrency—the key factor influencing performance was how quickly objects were returned. The closer you're to doing nothing with the object, the more likely `sync.Pool` was to outperform GenPool.

### ⚙️ Performance Tip

For best results under contention make sure that `pool.PoolFields[Object]` is on a separate cache line from your fields (add padding if needed). This avoids false sharing and improves cache performance across cores. ([example](pool/pool_benchmark_test.go))

### References

For detailed technical explanations and implementation details, please refer to the [docs](./docs) directory:

- [Overall Design](./docs/overall_design.md) - Technical design and architecture overview
- [Cleanup Mechanism](./docs/cleanup.md) - Details about the pool's cleanup and eviction policies

## Installation

```bash
go get github.com/AlexsanderHamir/GenPool
```

## Quick Start

Here's a simple example of how to use the object pool:

```go
package main

import (
	"sync/atomic"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// By embedding [Fields], Object automatically satisfies the Poolable interface.
// Objects are pooled using an atomic, sharded (per CPU) linked list.
type Object struct {
	Name       string
	Data       []byte
	pool.Fields[Object]
}

func allocator() *Object {
	return &Object{Name: "test"}
}

// Used internally by PUT
func cleaner(obj *Object) {
	obj.Name = ""
	obj.Data = obj.Data[:0]

	// or
	// *obj = Object{}
}

func main() {

	// Create custom cleanup policy
	cleanupPolicy := pool.CleanupPolicy{
		Enabled:       true,
		Interval:      10 * time.Minute,
		MinUsageCount: 20, // eviction happens below this number of usage per object
	}

	// Create pool with custom configuration
	config := pool.Config[Object, *Object]{
		Cleanup:   cleanupPolicy,
		Allocator: allocator,
		Cleaner:   cleaner,
	}

	benchPool, err := pool.NewPoolWithConfig(config)
	if err != nil {
		panic(err)
	}

	defer benchPool.Close()

	obj := benchPool.Get()


	obj.Name = "Robert"
	obj.Data = append(obj.Data, 34)


	benchPool.Put(obj)
}
```

## Features

1. **Growth Control**
   Limit how large the pool can grow using the `GrowthPolicy`, giving you precise control over memory usage.

2. **Cleanup Control**
   Fine-tune how often and how aggressively the pool is cleaned up with `CleanupPolicy`.

3. **Set Cleaner Once**
   Provide a `cleaner` function to automatically reset or sanitize objects before reuse—no manual cleanup required.

4. **Shards Control**
   Configure the number of shards in the pool.
   → Sharding helps distribute load and significantly improves performance under heavy concurrency.

## Growth Policy

If no growth policy is provided, the pool will grow **indefinitely**. In this case, any resource control will rely entirely on the `CleanupPolicy`.

```go
// GrowthPolicy defines constraints on how the pool is allowed to grow.
type GrowthPolicy struct {
	// Enable determines whether growth limiting is active.
	// If false, the pool can grow and shrink without restriction.
	Enable bool

	// MaxPoolSize sets the upper limit on the number of objects the pool can hold.
	MaxPoolSize int64
}
```

## Cleanup Policy

If no cleanup policy is provided in the config, the zero value will be used by default, which means automatic cleanup is **disabled**.

```go
// CleanupPolicy defines how the pool should automatically clean up unused objects.
type CleanupPolicy struct {
	// Enabled indicates whether automatic cleanup is active.
	Enabled bool
	// Interval specifies how frequently the cleanup process should run.
	Interval time.Duration
	// MinUsageCount sets the usage threshold below which objects will be evicted.
	MinUsageCount int64
}
```

## Cleanup Levels

Use `DefaultCleanupPolicy(level)` to get a predefined [CleanupPolicy].

### 🎛️ Cleanup Levels

| Level        | Interval | MinUsageCount | When to Use                            |
| ------------ | -------- | ------------- | -------------------------------------- |
| `disable`    | —        | —             | Manual control / predictable workloads |
| `low`        | 10m      | 1             | High reuse, latency-sensitive          |
| `moderate`   | 2m       | 2             | Balanced default                       |
| `aggressive` | 30s      | 3             | Low memory tolerance / bursty usage    |

```go
Cleanup: pool.DefaultCleanupPolicy(pool.GcModerate)
```

## Manual Control

For advanced users who prefer full control over memory reclamation, GenPool allows you to **disable automatic cleanup** using the `GcDisable` policy, and the pool exposes its internal fields to allow for custom logic.

### Public Access for Manual Cleanup

The `ShardedPool` and `Shard` types expose the internals you need:

```go

type ShardedPool[T any, P Poolable[T]] struct {
	Shards    []*Shard[T, P] // All shards
}

type Shard[T any, P Poolable[T]] struct {
		Head atomic.Pointer[T] // Head of the linked list for this shard
}
```

You can safely traverse and modify these shards to implement your own retention, eviction, or tracking strategies.

## Contributing

We welcome contributions! Before you start contributing, please ensure you have:

- **Go 1.24.3 or later** installed
- **Git** for version control
- Basic understanding of Go testing and benchmarking

### Quick Setup

```bash
# Fork and clone the repository
git clone https://github.com/AlexsanderHamir/GenPool.git
cd GenPool

# Run tests to verify setup
go test -v ./...
go test -bench=. ./...

# Check for linter errors
 go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
 golangci-lint run
```

### Development Guidelines

1. Run benchmarks before anything to stablish a baseline.
2. Ensure any new functionality didn't regress the performance.
3. Write unit and black box tests for new functionality.
4. Update documentation for user-facing changes.
5. Ensure all tests pass before submitting PRs.
6. PRs will only be merged if it passed all required github actions.

> The best improvement is to do less!!!

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
