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
![Go Version](https://img.shields.io/badge/Go-1.24.3%2B-blue)

## Table of Contents

- [Performance](#performance)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Contributing](#contributing)
- [Complete Example](#complete-example)
- [License](#license)

## Performance

Choose GenPool when you need finer control over object lifecycle, especially in scenarios where predictable reclamation and reuse patterns are critical, instead of aggressive object reclamantion.

For a detailed breakdown of the performance go to [GenPool vs SyncPool](./benchmark_results_transparency)

> In most benchmarks, GenPool performs on par with sync.Pool. Under high concurrency, GenPool often delivers slightly lower latency, reduced memory usage, and less contention—though the differences are typically small (e.g., ~20ns faster and ~1–3 bytes lighter per operation).

> **Performance Tip**: For maximum performance in high-contention scenarios, ensure that your pooled objects have their interface fields (`usageCount` and `next`) on their own cache line by adding appropriate padding. This prevents false sharing and cache line bouncing between CPU cores.

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
	"sync/atomic"
	"time"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// Your Object must implement the Poolable interface
// An atomic linked list is created and sharded (cpu number) out of the objects you want to pool.
type Object struct {
	Name       string
	Data       []byte
	usageCount atomic.Int64
	next       atomic.Value
}

func (o *Object) GetNext() pool.Poolable {
	if next := o.next.Load(); next != nil {
		return next.(pool.Poolable)
	}
	return nil
}

func (o *Object) SetNext(next pool.Poolable) {
	o.next.Store(next)
}

func (o *Object) GetUsageCount() int64 {
	return o.usageCount.Load()
}

func (o *Object) IncrementUsage() {
	o.usageCount.Add(1)
}

func (o *Object) ResetUsage() {
	o.usageCount.Store(0)
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
		MinUsageCount: 20,
	}

	// Create pool with custom configuration
	config := pool.PoolConfig[*Object]{
		Cleanup:   cleanupPolicy,
		Allocator: allocator,
		Cleaner:   cleaner,
	}

	benchPool, err := pool.NewPoolWithConfig(config)
	if err != nil {
		panic(err)
	}

	// Use the pool as before...
	obj := benchPool.RetrieveOrCreate()

	// Use the object
	obj.Name = "Robert"
	obj.Data = append(obj.Data, 34)

	// Return the object to the pool
	benchPool.Put(obj)
}
```

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

# Install dependencies
go mod download
go mod tidy

# Run tests to verify setup
go test -v ./...
go test -bench=. ./...
```

### Development Guidelines

- Write tests for new functionality
- Run benchmarks to ensure no performance regressions
- Follow Go code style guidelines
- Update documentation for user-facing changes
- Ensure all tests pass before submitting PRs

## Complete Example

For a fully working example with its own Go module, see the [example](./example) directory.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
