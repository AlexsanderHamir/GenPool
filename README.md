# GenPool

A sharded, generic object pool for Go with configurable growth and cleanup.

The goal is **heavy reuse**: keep objects in the pool and in hot paths, and avoid allocating new ones or throwing them away unless you have to. Growth caps and cleanup exist for when you *do* need to bound memory or evict cold entries.

**Benchmarks:** In our [GenPool vs `sync.Pool` suite](test/BENCHMARKS.md), the longer pooled objects stay out (more CPU work between `Get` and `Put`), the more `sync.Pool` gives up its edge and GenPool catches up or moves ahead. That pattern is from CPU-only work; real code with I/O in between usually widens the gap further.

[![Go Reference](https://pkg.go.dev/badge/github.com/AlexsanderHamir/GenPool/pool.svg)](https://pkg.go.dev/github.com/AlexsanderHamir/GenPool/pool)
[![Build](https://github.com/AlexsanderHamir/GenPool/actions/workflows/test.yml/badge.svg)](https://github.com/AlexsanderHamir/GenPool/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/AlexsanderHamir/GenPool)](https://goreportcard.com/report/github.com/AlexsanderHamir/GenPool)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Install

```bash
go get github.com/AlexsanderHamir/GenPool
```

## Quick start

Embed `pool.Fields[YourType]`, provide an **allocator** and **cleaner** (run on each `Put`), then `NewPool` / `Get` / `Put` / `Close`.

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

Use `NewPoolWithConfig` for cleanup intervals, growth caps, and shard count — see the [user guide](docs/user-guide.md).

## Documentation

| Doc | Description |
| --- | --- |
| [docs/user-guide.md](docs/user-guide.md) | Concepts, configuration, benchmarks, manual control, contributing |
| [docs/overall_design.md](docs/overall_design.md) | Design notes |
| [docs/cleanup.md](docs/cleanup.md) | Cleanup behavior |
| [test/BENCHMARKS.md](test/BENCHMARKS.md) | GenPool vs `sync.Pool` sample numbers |
| [benchmark_results_transparency/](benchmark_results_transparency/) | Archived benchmark outputs |

## License

[MIT](LICENSE)
