# GenPool

A sharded, generic object pool for Go with configurable growth and cleanup. It outperforms `sync.Pool` when objects are held longer between Get and Put, at the cost of extra overhead when retention is very short.

[![GoDoc](https://godoc.org/github.com/AlexsanderHamir/GenPool?status.svg)](https://godoc.org/github.com/AlexsanderHamir/GenPool)
[![Build](https://github.com/AlexsanderHamir/GenPool/actions/workflows/test.yml/badge.svg)](https://github.com/AlexsanderHamir/GenPool/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/AlexsanderHamir/GenPool)](https://goreportcard.com/report/github.com/AlexsanderHamir/GenPool)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Concepts](#concepts)
- [Configuration](#configuration)
- [Performance](#performance)
- [Manual Control](#manual-control)
- [Contributing](#contributing)
- [License](#license)

---

## Installation

```bash
go get github.com/AlexsanderHamir/GenPool
```

## Quick Start

1. Define a type that embeds `pool.Fields[YourType]` so it implements `Poolable`.
2. Provide an allocator and a cleaner (called on each Put).
3. Create the pool with `NewPool` or `NewPoolWithConfig` and call Get/Put.

```go
package main

import (
	"github.com/AlexsanderHamir/GenPool/pool"
)

type Object struct {
	Name string
	Data []byte
	pool.Fields[Object]
}

func main() {
	allocator := func() *Object { return &Object{} }
	cleaner := func(obj *Object) { obj.Name = ""; obj.Data = obj.Data[:0] }

	p, err := pool.NewPool(allocator, cleaner)
	if err != nil {
		panic(err)
	}
	defer p.Close()

	obj := p.Get()
	obj.Name = "example"
	obj.Data = append(obj.Data, 1, 2, 3)
	p.Put(obj)
}
```

With custom cleanup and growth:

```go
config := pool.Config[Object, *Object]{
	Cleanup:   pool.DefaultCleanupPolicy(pool.GcModerate),
	Allocator: allocator,
	Cleaner:   cleaner,
	Growth: pool.GrowthPolicy{
		Enable:      true,
		MaxPoolSize: 1000,
	},
}
p, err := pool.NewPoolWithConfig(config)
```

## Concepts

- **Sharding** — The pool is split by GOMAXPROCS; each shard has its own lock-free list. Set `runtime.GOMAXPROCS(n)` before creating the pool to control shard count.
- **Growth** — Without a growth policy, the pool grows unbounded. With `GrowthPolicy.Enable` and `MaxPoolSize`, Get returns `nil` when the cap is reached.
- **Cleanup** — Optional background goroutine that periodically evicts objects whose usage count is below `MinUsageCount`. Disabled by default; enable via `CleanupPolicy` or `DefaultCleanupPolicy(level)`.

## Configuration

### Cleanup policy

| Level        | Interval | MinUsageCount | Use case                    |
| ------------ | -------- | ------------- | --------------------------- |
| `GcDisable`  | —        | —             | No automatic cleanup         |
| `GcLow`      | 10m      | 1             | High reuse, low churn       |
| `GcModerate` | 2m       | 2             | Default balance             |
| `GcAggressive` | 30s    | 3             | Memory-sensitive / bursty  |

```go
Cleanup: pool.DefaultCleanupPolicy(pool.GcModerate)
```

Or build a policy manually:

```go
Cleanup: pool.CleanupPolicy{
	Enabled:       true,
	Interval:      2 * time.Minute,
	MinUsageCount: 2,
}
```

### Growth policy

Limit pool size so Get returns `nil` when full:

```go
Growth: pool.GrowthPolicy{
	Enable:      true,
	MaxPoolSize: 5000,
}
```

If `Enable` is false, the pool has no size limit and relies on cleanup (if enabled) for reclaiming memory.

## Performance

Benchmarks (darwin/arm64, Apple M1) show GenPool ahead when objects are held longer; `sync.Pool` can win when retention is very short.

- **Run benchmarks locally:** `go test -bench=. ./test/` (see [test/pool_benchmark_test.go](test/pool_benchmark_test.go)).
- **Published results and analysis:** [benchmark_results_transparency](./benchmark_results_transparency) — raw outputs per scenario and [conclusion](benchmark_results_transparency/conclusion.md).

**Tip:** Keep `pool.Fields[Object]` on its own cache line (e.g. add padding) to reduce false sharing under contention.

**Docs:** [Design](./docs/overall_design.md) · [Cleanup](./docs/cleanup.md)

## Manual Control

Disable automatic cleanup with `GcDisable`. You can then implement custom eviction by using the exported `ShardedPool.Shards` and each `Shard`'s `Head` (and `Single`) to traverse or clear lists. The pool does not lock these; coordinate with Get/Put usage as needed.

## Contributing

- **Go 1.24+** required.
- Run tests and benchmarks before changing behavior: `go test ./...` and `go test -bench=. ./...`.
- Add tests for new behavior and update docs for user-facing changes.
- All CI checks must pass for PRs to be merged.

```bash
git clone https://github.com/AlexsanderHamir/GenPool.git
cd GenPool
go test -v ./...
golangci-lint run  # optional
```

## License

MIT. See [LICENSE](LICENSE).
