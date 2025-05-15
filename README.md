# Minimal Object Pool

![Build](https://github.com/AlexsanderHamir/GenPool/actions/workflows/test.yml/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/AlexsanderHamir/GenPool/badge.png?branch=main)](https://coveralls.io/github/AlexsanderHamir/GenPool?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/AlexsanderHamir/GenPool)](https://goreportcard.com/report/github.com/AlexsanderHamir/GenPool)
[![Go Reference](https://pkg.go.dev/badge/github.com/AlexsanderHamir/GenPool.svg)](https://pkg.go.dev/github.com/AlexsanderHamir/GenPool)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Issues](https://img.shields.io/github/issues/AlexsanderHamir/GenPool)
![Last Commit](https://img.shields.io/github/last-commit/AlexsanderHamir/GenPool)
![Code Size](https://img.shields.io/github/languages/code-size/AlexsanderHamir/GenPool)
![Version](https://img.shields.io/github/v/tag/AlexsanderHamir/GenPool?sort=semver)

A lightweight, type-safe object pool implementation in Go. This pool implementation aims to be minimalistic, it uses an atomic linked list to get the job done.

## Features

- 🔒 Type-safe implementation using Go generics
- ⚡ Lock-free operations using atomic operations
- 🔄 Automatic cleanup of unused objects (configurable)
- 📊 Usage tracking for intelligent object eviction
- 🛡️ Hard limit enforcement to prevent memory leaks
- 🎯 Thread-safe operations

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

// Your Object must implemen the Poolable interface
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
    // Create allocator and cleaner functions
    allocator := func() *BenchmarkObject {
        return &BenchmarkObject{
            Value: 0,
        }
    }

    cleaner := func(obj *BenchmarkObject) {
        obj.Value = 0
        obj.Name = ""
    }

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

    benchmarkObj.Value = 42
    benchmarkObj.Name = "Robert"


    // Return the object to the pool
    pool.Put(benchmarkObj)
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

## API Reference

### Main Types

- `Pool[T Poolable]`: The main pool type
- `PoolConfig[T Poolable]`: Configuration for the pool
- `CleanupPolicy`: Configuration for cleanup behavior
- `Allocator[T]`: Function type for creating new objects
- `Cleaner[T]`: Function type for cleaning objects

### Key Methods

- `NewPool[T]`: Create a new pool with default configuration
- `NewPoolWithConfig[T]`: Create a pool with custom configuration
- `RetrieveOrCreate()`: Get an object from the pool or create new using the provided allocator
- `Put(T)`: Return an object to the pool
- `Size()`: Get current pool size
- `Active()`: Get number of active objects
- `Clear()`: Remove all objects from pool
- `Close()`: Stop cleanup and clear pool

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Versioning

This project follows [Semantic Versioning](https://semver.org/). For the versions available, see the [tags on this repository](https://github.com/AlexsanderHamir/GenPool/tags).
