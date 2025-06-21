# GenPool

GenPool delivers sync.Pool-level performance with the added benefit of configurable object reclamation, letting you fine-tune reuse and lifecycle management.

[![GoDoc](https://godoc.org/github.com/AlexsanderHamir/GenPool?status.svg)](https://godoc.org/github.com/AlexsanderHamir/GenPool)
![Build](https://github.com/AlexsanderHamir/GenPool/actions/workflows/test.yml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/AlexsanderHamir/GenPool/badge.svg?branch=main)](https://coveralls.io/github/AlexsanderHamir/GenPool?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/AlexsanderHamir/GenPool)](https://goreportcard.com/report/github.com/AlexsanderHamir/GenPool)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Issues](https://img.shields.io/github/issues/AlexsanderHamir/GenPool)
![Last Commit](https://img.shields.io/github/last-commit/AlexsanderHamir/GenPool)
![Code Size](https://img.shields.io/github/languages/code-size/AlexsanderHamir/GenPool)
![Version](https://img.shields.io/github/v/tag/AlexsanderHamir/GenPool?sort=semver)
![Go Version](https://img.shields.io/badge/Go-1.23.4%2B-blue)

## Table of Contents

- [Performance](#performance)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Contributing](#contributing)
- [License](#license)

## Performance

The following benchmarks compare GenPool with Go's `sync.Pool` (which is tightly integrated with the runtime and optimized for short-lived object reuse).

### Benchmark Summary (100 runs each)

#### Scenario: GenPool (100 goroutines) vs SyncPool (1000 goroutines)

| Metric          | GenPool (100) | SyncPool (1k) | Difference |
| --------------- | ------------- | ------------- | ---------- |
| Average Latency | **1553.0ns**  | 1572.1ns      | **-1.2%**  |
| Median Latency  | **1542.0ns**  | 1534.0ns      | **+0.5%**  |
| P95 Latency     | **1570.0ns**  | 1659.0ns      | **-5.4%**  |
| P99 Latency     | **1588.0ns**  | 1822.0ns      | **-12.8%** |
| Memory/Op       | **1.7 B**     | 3.4 B         | **-50.0%** |
| Allocs/Op       | 0             | 0             | 0%         |

#### Scenario: GenPool (1000 goroutines) vs SyncPool (100 goroutines)

| **Metric**          | **GenPool (1k)** | **SyncPool (100)** | **Difference** |
| ------------------- | ---------------- | ------------------ | -------------- |
| **Average Latency** | **1548.7 ns**    | 1575.5 ns          | **-1.7%**      |
| **Median Latency**  | **1547.0 ns**    | 1579.0 ns          | **-2.0%**      |
| **P95 Latency**     | **1590.0 ns**    | 1590.0 ns          | **0%**         |
| **P99 Latency**     | **1600.0 ns**    | 1599.0 ns          | **+0.1%**      |
| **Memory/Op**       | **3.2 B**        | 2.0 B              | **+60%**       |
| **Allocs/Op**       | **0**            | 0                  | **0%**         |

#### Scenario: GenPool (1000 goroutines) vs SyncPool (1000 goroutines)

| Metric          | GenPool (1k) | SyncPool (1k) | Difference |
| --------------- | ------------ | ------------- | ---------- |
| Average Latency | 1578.7ns     | 1552.6ns      | **+1.7%**  |
| Median Latency  | 1574.0ns     | 1535.0ns      | **+2.5%**  |
| P95 Latency     | 1622.0ns     | 1590.0ns      | **+2.0%**  |
| P99 Latency     | 1680.0ns     | 1648.0ns      | **+1.9%**  |
| Memory/Op       | 3.3 B        | 3.6 B         | **-8.3%**  |
| Allocs/Op       | 0            | 0             | 0%         |

> **Benchmark Analysis**:  
> Across many benchmarks, the performance differences between GenPool and sync.Pool mostly disappear. GenPool tends to show slightly lower latency under high concurrency, but the gap is minimal—typically around 20 nanoseconds. Use GenPool if you need more control over when and how aggressively objects are cleaned up.

> **Performance Tip**: For maximum performance in high-contention scenarios, ensure that your pooled objects have their interface fields (`usageCount` and `next`) on their own cache line by adding appropriate padding. This prevents false sharing and cache line bouncing between CPU cores. See the [benchmark test file](./pool/pool_benchmark_test.go) for an example implementation.

## References

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
    "fmt"
    "github.com/AlexsanderHamir/GenPool/pool"
)

// Your Object must implement the Poolable interface
type BenchmarkObject struct {
 // user fields
	Name string   // 16 bytes
	Data []byte   // 24 bytes
	_    [24]byte // 24 bytes = 64 bytes

	// interface necessary fields (kept together since they're modified together)
	usageCount atomic.Int64 // 8 bytes
	next       atomic.Value // 16 bytes
	_          [40]byte     // 40 bytes padding to make struct 128 bytes (2 cache lines)
}

func (o *BenchmarkObject) GetNext() Poolable {
    if next := o.next.Load(); next != nil {
        return next.(Poolable)
    }
    return nil
}

func (o *BenchmarkObject) SetNext(next Poolable) {
    o.next.Store(next)
}

func (o *BenchmarkObject) GetUsageCount() int64 {
    return o.usageCount.Load()
}

func (o *BenchmarkObject) IncrementUsage() {
    o.usageCount.Add(1)
}

func (o *BenchmarkObject) ResetUsage() {
    o.usageCount.Store(0)
}

// Example using NewPoolWithConfig with custom configuration
func main() {
    func allocator() *BenchmarkObject {
	    return &BenchmarkObject{Name: "test"}
    }

    func cleaner(obj *BenchmarkObject) {
	    obj.Name = ""
	    obj.Data = obj.Data[:0]
    }

    // Create custom cleanup policy
    cleanupPolicy := pool.CleanupPolicy{
        Enabled:       true,
        Interval:      10 * time.Minute,
        MinUsageCount: 20,
        TargetSize:    200,
    }

    // Create pool with custom configuration
    config := pool.PoolConfig[*BenchmarkObject]{
        Cleanup:   cleanupPolicy,
        Allocator: allocator,
        Cleaner:   cleaner,
    }

    pool, err := pool.NewPoolWithConfig(config)
    if err != nil {
        panic(err)
    }

    // Use the pool as before...
    obj, err := pool.RetrieveOrCreate()
    if err != nil {
        panic(err)
    }

    // Use the object
    obj.Value = 42
    obj.Name = "Robert"

    // Return the object to the pool
    pool.Put(obj)
}
```

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
