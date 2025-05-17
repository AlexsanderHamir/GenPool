# Minimal Object Pool

![Build](https://github.com/AlexsanderHamir/GenPool/actions/workflows/test.yml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/AlexsanderHamir/GenPool/badge.svg?branch=main)](https://coveralls.io/github/AlexsanderHamir/GenPool?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/AlexsanderHamir/GenPool)](https://goreportcard.com/report/github.com/AlexsanderHamir/GenPool)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Issues](https://img.shields.io/github/issues/AlexsanderHamir/GenPool)
![Last Commit](https://img.shields.io/github/last-commit/AlexsanderHamir/GenPool)
![Code Size](https://img.shields.io/github/languages/code-size/AlexsanderHamir/GenPool)
![Version](https://img.shields.io/github/v/tag/AlexsanderHamir/GenPool?sort=semver)

A lightweight, type-safe object pool implementation in Go. This pool implementation aims to be minimalistic, it uses an atomic linked list to get the job done.

## GenPool vs sync.Pool

```
BenchmarkSyncPool        760000              1579 ns/op               3.9 B/op        0 allocs/op
BenchmarkGenPool         742288              1620 ns/op               3 B/op          0 allocs/op
```

## Features

- 🔒 Type-safe implementation using Go generics
- ⚡ Lock-free operations using atomic operations
- 🔄 Automatic cleanup of unused objects (configurable)
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
    HardLimit int           // Maximum number of objects allowed in the pool
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
    // Default: 50% of HardLimit
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
