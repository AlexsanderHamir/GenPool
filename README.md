# Minimal Object Pool

![Build](https://github.com/AlexsanderHamir/GenPool/actions/workflows/test.yml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/AlexsanderHamir/GenPool/badge.svg?branch=main)](https://coveralls.io/github/AlexsanderHamir/GenPool?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/AlexsanderHamir/GenPool)](https://goreportcard.com/report/github.com/AlexsanderHamir/GenPool)
[![Go Reference](https://pkg.go.dev/badge/github.com/AlexsanderHamir/GenPool.svg)](https://pkg.go.dev/github.com/AlexsanderHamir/GenPool)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Issues](https://img.shields.io/github/issues/AlexsanderHamir/GenPool)
![Last Commit](https://img.shields.io/github/last-commit/AlexsanderHamir/GenPool)
![Code Size](https://img.shields.io/github/languages/code-size/AlexsanderHamir/GenPool)
![Version](https://img.shields.io/github/v/tag/AlexsanderHamir/GenPool?sort=semver)

A lightweight, type-safe object pool implementation in Go. This pool implementation aims to be minimalistic, it uses an atomic linked list to get the job done.

## GenPool vs sync.Pool

```
BenchmarkGetPutOurPool-8         1111809              1087 ns/op               0 B/op          0 allocs/op
BenchmarkGetPutSyncPool-8        1140043              1079 ns/op               0 B/op          0 allocs/op
```

## Features

- 🔒 Type-safe implementation using Go generics
- ⚡ Lock-free operations using atomic operations
- 🔄 Automatic cleanup of unused objects (configurable)
- 📊 Usage tracking for intelligent object eviction
- 🛡️ Hard limit enforcement to prevent memory explosion
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
    // your fields
    Value      int
    Name       string

    // must add these fields
    next       atomic.Value
    usageCount atomic.Int64
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

    // Create pool with default configuration
    pool, err := internal.NewPool(allocator, cleaner)
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    // Get an object from the pool
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

### Core Types

#### Pool[T Poolable]

The main pool type that manages object lifecycle and provides thread-safe operations.

#### PoolConfig[T Poolable]

Configuration struct for customizing pool behavior:

```go
type PoolConfig[T Poolable] struct {
    HardLimit int           // Maximum number of objects allowed in the pool
    Cleanup   CleanupPolicy // Cleanup configuration
    Allocator Allocator[T]  // Function to create new objects
    Cleaner   Cleaner[T]    // Function to reset objects
}
```

#### CleanupPolicy

Controls the pool's cleanup behavior:

```go
type CleanupPolicy struct {
    Enabled       bool          // Whether cleanup is enabled
    Interval      time.Duration // How often cleanup runs
    MinUsageCount int64        // Minimum usage count before eviction
    TargetSize    int          // Target pool size after cleanup
}
```

### Function Types

#### Allocator[T]

```go
type Allocator[T Poolable] func() T
```

Function type for creating new objects. Must return a new instance of the pooled type.

#### Cleaner[T]

```go
type Cleaner[T Poolable] func(T)
```

Function type for resetting objects to their initial state.

### Pool Methods

#### Creation

- `NewPool[T Poolable](allocator Allocator[T], cleaner Cleaner[T]) (*Pool[T], error)`

  - Creates a new pool with default configuration
  - Returns error if allocator or cleaner is nil

- `NewPoolWithConfig[T Poolable](config PoolConfig[T]) (*Pool[T], error)`
  - Creates a new pool with custom configuration
  - Returns error if configuration is invalid

#### Object Management

- `RetrieveOrCreate() T`

  - Gets an object from the pool or creates a new one
  - Returns error nil pool is closed or at hard limit

- `Put(obj T)`
  - Returns an object to the pool
  - No-op if pool is closed

#### Pool Information

- `Size() int`

  - Returns current number of objects in the pool

- `Active() int`
  - Returns number of objects currently in use

#### Pool Control

- `Clear()`

  - Removes all objects from the pool
  - Objects are not cleaned up, just removed from pool

- `Close()`
  - Stops cleanup goroutine and clears the pool
  - Should be called when pool is no longer needed

### Interface Requirements

#### Poolable

Objects stored in the pool must implement this interface:

```go
type Poolable interface {
    GetNext() Poolable
    SetNext(Poolable)
    GetUsageCount() int64
    IncrementUsage()
    ResetUsage()
}
```

## Advanced Configuration

The pool can be configured with custom settings:

```go
config := pool.PoolConfig[*MyObject]{
    HardLimit: 1000, // Maximum number of objects
    Cleanup: pool.CleanupPolicy{
        Enabled:       true,
        Interval:      5 * time.Minute,
        MinUsageCount: 10,
        TargetSize:    100,
    },
    Allocator: allocator,
    Cleaner:   cleaner,
}

pool, err := pool.NewPoolWithConfig(config)
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Versioning

This project follows [Semantic Versioning](https://semver.org/). For the versions available, see the [tags on this repository](https://github.com/AlexsanderHamir/GenPool/tags).
