# GenPool

GenPool delivers better performance than sync.Pool when your system tends to hold onto objects longer—that is, under moderate to high latency workloads—while giving you precise control over when and how aggressively memory is reclaimed.

> If your system rarely retains objects and has low concurrency, you’re unlikely to benefit from GenPool’s design.

[![GoDoc](https://godoc.org/github.com/AlexsanderHamir/GenPool?status.svg)](https://godoc.org/github.com/AlexsanderHamir/GenPool)
![Build](https://github.com/AlexsanderHamir/GenPool/actions/workflows/test.yml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/AlexsanderHamir/GenPool/badge.svg?branch=main)](https://coveralls.io/github/AlexsanderHamir/GenPool?branch=main)
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
- [Cleanup Levels](#cleanup-levels)
- [Total Manual Control](#total-manual-control)
- [Contributing](#contributing)
- [Complete Example](#complete-example)
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

// By embedding [PoolFields], Object automatically satisfies the Poolable interface.
// Objects are pooled using an atomic, sharded (per CPU) linked list.
type Object struct {
	Name       string
	Data       []byte
	pool.PoolFields[Object]
}

func allocator() *Object {
	return &Object{Name: "test"}
}

func cleaner(obj *Object) {
	obj.Name = ""
	obj.Data = obj.Data[:0]
}

func main() {

	// Create custom cleanup policy
	cleanupPolicy := pool.CleanupPolicy{
		Enabled:       true,
		Interval:      10 * time.Minute,
		MinUsageCount: 20, // eviction happens below this number of usage per object
	}

	// Create pool with custom configuration
	config := pool.PoolConfig[Object, *Object]{
		Cleanup:   cleanupPolicy,
		Allocator: allocator,
		Cleaner:   cleaner,
	}

	benchPool, err := pool.NewPoolWithConfig(config)
	if err != nil {
		panic(err)
	}

	defer benchPool.Close()

	obj := benchPool.RetrieveOrCreate()


	obj.Name = "Robert"
	obj.Data = append(obj.Data, 34)


	benchPool.Put(obj)
}
```

## Cleanup Levels

Use `DefaultCleanupPolicy(level)` to get a predefined setup.

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

## Total Manual Control

For advanced users who prefer full control over memory reclamation, GenPool allows you to **disable automatic cleanup** using the `GcDisable` policy, and the pool exposes its internal fields to allow for custom logic.

### Public Access for Manual Cleanup

The `ShardedPool` and `PoolShard` types expose the internals you need:

```go
type PoolShard[T any, P Poolable[T]] struct {
	Head atomic.Pointer[T] // Head of the linked list for this shard
}

type ShardedPool[T any, P Poolable[T]] struct {
	Shards    []*PoolShard[T, P] // All shards
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
```

### Development Guidelines

- Write tests for new functionality
- Run benchmarks to ensure no performance regressions
- Update documentation for user-facing changes
- Ensure all tests pass before submitting PRs

## Complete Example

For a fully working example with its own Go module, see the [example](./example) directory.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
