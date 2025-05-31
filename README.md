# Minimal Object Pool

GenPool is a high-performance object pool for Go. It leverages runtime_procPin to consistently assign each processor to its own pool shard, ensuring fixed shard access per logical processor. This design maximizes cache locality, minimizes contention, and reduces garbage collection pressure. Each shard operates independently, making GenPool ideal for systems that rapidly create and recycle objects at scale.

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
- [Why Use GenPool?](#why-use-genpool)
- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Use Cases](#use-cases)
- [API Reference](#api-reference)
- [Configuration](#configuration)
- [Contributing](#contributing)
- [License](#license)

## Why Use GenPool?

- **Lightweight**: Just 227 lines of code, with ongoing efforts to simplify and optimize further
- **Type Safety**: Leverages Go generics for compile-time type checking
- **Zero Dependencies**: Pure Go implementation with no external dependencies

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

> **Note:**  
> The benchmarks show that **GenPool delivers consistent performance across varying levels of concurrency**, often outperforming **SyncPool** in both **speed** and **memory efficiency**.  
> In the scenario with **1000 goroutines** on both GenPool and SyncPool, **GenPool’s worst run** was compared against **SyncPool’s best**, yet the results were still competitive:  
> GenPool consumed **8.3% less memory per operation**, with only a slight tradeoff in latency (around **2% higher on average**).  
>  
> This reinforces **GenPool’s strength in delivering predictable and efficient performance under high contention**, while also providing customizable cleanup—giving you full control over when and how aggressively objects are reclaimed.


> **Performance Tip**: For maximum performance in high-contention scenarios, ensure that your pooled objects have their interface fields (`usageCount` and `next`) on their own cache line by adding appropriate padding. This prevents false sharing and cache line bouncing between CPU cores. See the [benchmark test file](./pool/pool_benchmark_test.go) for an example implementation.

## Features

- 🔒 Type-safe implementation using Go generics
- ⚡ Lock-free operations using atomic operations
- 🔄 Configurable cleanup of unused objects
- 📊 Usage tracking for intelligent object eviction
- 🎯 Thread-safe operations

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
    // Create allocator function for new objects
    allocator := pool.Allocator[*BenchmarkObject](func() *BenchmarkObject {
        return &BenchmarkObject{
            Value: 0,
            Name:  "",
        }
    })

    // Create cleaner function for resetting objects
    cleaner := pool.Cleaner[*BenchmarkObject](func(obj *BenchmarkObject) {
        obj.Value = 0
        obj.Name = ""
    })

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

    pool, err := internal.NewPoolWithConfig(config)
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

## API Reference

Methods Available:

```go
// Core pool operations
RetrieveOrCreate() (T, error)  // Gets an object from the pool or creates a new one
Put(obj T)                     // Returns an object to the pool
```

Example usage:

```go
pool, _ := internal.NewPool(allocator, cleaner)

// Get an object
obj, err := pool.RetrieveOrCreate()
if err != nil {
    // Handle error
}

// Use the object
// ...

// Return to pool
pool.Put(obj)
```

#### PoolConfig[T Poolable]

Configuration struct for customizing pool behavior. All have sensible defaults:

```go
type PoolConfig[T Poolable] struct {
    Cleanup   CleanupPolicy // Cleanup configuration
    Allocator Allocator[T]  // Function to create new objects
    Cleaner   Cleaner[T]    // Function to reset objects
}
```

#### CleanupPolicy

Controls the pool's cleanup behavior to prevent memory bloat and manage resource usage:

```go
type CleanupPolicy struct {
    // Whether automatic cleanup is enabled
    // Default: false
    Enabled bool

    // How often the cleanup routine runs
    // Default: 5 minutes
    Interval time.Duration

    // Objects with usage count below this threshold
    // may be evicted during cleanup
    // Default: 5
    MinUsageCount int64

    // Target number of objects to maintain after cleanup
    TargetSize int
}
```

Example cleanup policy:

```go
cleanupPolicy := CleanupPolicy{
    Enabled:       true,
    Interval:      10 * time.Minute,
    MinUsageCount: 20,
    TargetSize:    200,
}
```

### Function Types

#### Allocator[T]

Function type for creating new objects. Must return a new instance of the pooled type.

```go
type Allocator[T Poolable] func() T
```

Example implementation:

```go
allocator := Allocator[*MyObject](func() *MyObject {
    return &MyObject{
        ID:    uuid.New(),
        State: "new",
        // Initialize other fields...
    }
})
```

#### Cleaner[T]

Function type for resetting objects before they are returned to the pool. This helps ensure objects are in a clean state for the next use.

```go
type Cleaner[T Poolable] func(T)
```

Example implementation:

```go
cleaner := Cleaner[*MyObject](func(obj *MyObject) {
    obj.State = "clean"
    obj.LastUsed = time.Time{}
    obj.Buffer = nil
    // Reset other fields to their initial state...
})
```

## Use Cases

### 1. Database Connection Pooling

```go
type DBConnection struct {
    conn *sql.DB
    // ... poolable fields
}

pool, _ := NewPool(
    Allocator[*DBConnection](func() *DBConnection {
        return &DBConnection{conn: createNewConnection()}
    }),
    Cleaner[*DBConnection](func(conn *DBConnection) {
        conn.conn.Ping() // Verify connection health
    }),
)
```

### 2. Game Development

```go
type GameObject struct {
    Position Vector3D
    // ... poolable fields
}

pool, _ := NewPool(
    Allocator[*GameObject](func() *GameObject {
        return &GameObject{Position: Vector3D{0, 0, 0}}
    }),
    Cleaner[*GameObject](func(obj *GameObject) {
        obj.Position = Vector3D{0, 0, 0}
    }),
)
```

### 3. HTTP Request Handling

```go
type RequestContext struct {
    Headers map[string]string
    // ... poolable fields
}

pool, _ := NewPool(
    Allocator[*RequestContext](func() *RequestContext {
        return &RequestContext{Headers: make(map[string]string)}
    }),
    Cleaner[*RequestContext](func(ctx *RequestContext) {
        for k := range ctx.Headers {
            delete(ctx.Headers, k)
        }
    }),
)
```

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
