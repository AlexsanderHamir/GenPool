# GenPool user guide

Concepts, configuration, benchmarks, and contribution notes. For API details see [pkg.go.dev](https://pkg.go.dev/github.com/AlexsanderHamir/GenPool/pool).

## Concepts

- **Sharding** — The pool is split into multiple shards, each with its own lock-free list to reduce contention. Shard count is `Config.NumShards`, defaulting to `runtime.GOMAXPROCS(0)` at creation (override with a positive value for testing or tuning).
- **Growth** — Without a growth policy, the pool grows unbounded. With `GrowthPolicy.Enable` and `MaxPoolSize`, `Get` returns `nil` when the cap is reached.
- **Cleanup** — Optional background goroutine that periodically evicts objects whose usage count is below `MinUsageCount`. Disabled by default; enable via `CleanupPolicy` or `DefaultCleanupPolicy(level)`.

## Configuration

### Cleanup policy

| Level          | Interval | MinUsageCount | Use case                    |
| -------------- | -------- | ------------- | --------------------------- |
| `GcDisable`    | —        | —             | No automatic cleanup        |
| `GcLow`        | 10m      | 1             | High reuse, low churn       |
| `GcModerate`   | 2m       | 2             | Default balance             |
| `GcAggressive` | 30s      | 3             | Memory-sensitive / bursty   |

```go
Cleanup: pool.DefaultCleanupPolicy(pool.GcModerate)
```

Or build a policy manually:

```go
import "time"

// In Config:
Cleanup: pool.CleanupPolicy{
	Enabled:       true,
	Interval:      2 * time.Minute,
	MinUsageCount: 2,
}
```

### Growth policy

Limit pool size so `Get` returns `nil` when full:

```go
Growth: pool.GrowthPolicy{
	Enable:      true,
	MaxPoolSize: 5000,
}
```

If `Enable` is false, the pool has no size limit and relies on cleanup (if enabled) for reclaiming memory.

### Example with cleanup and growth

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

## Performance

Benchmarks compare GenPool and `sync.Pool` under identical workloads in [`test/pool_benchmark_test.go`](../test/pool_benchmark_test.go). Methodology and scenario names are documented in [`test/doc.go`](../test/doc.go).

- **Sample table (one machine):** [`test/BENCHMARKS.md`](../test/BENCHMARKS.md)
- **Archived raw outputs and write-up:** [`benchmark_results_transparency/`](../benchmark_results_transparency) and [`conclusion.md`](../benchmark_results_transparency/conclusion.md)

Run locally (from repo root):

```bash
go test -bench . -benchmem ./test/
```

On Windows PowerShell, use `-bench .` (space) or `"-bench=."` so `./test/` is not merged into the benchmark pattern.

**Tip:** Keep `pool.Fields[YourType]` on its own cache line (e.g. add padding) to reduce false sharing under contention.

**Further reading:** [Overall design](./overall_design.md) · [Cleanup](./cleanup.md)

## Manual control

Disable automatic cleanup with `GcDisable`. You can then implement custom eviction using the exported `ShardedPool.Shards` and each `Shard`'s `Head` (and `Single`) to traverse or clear lists. The pool does not lock these; coordinate with `Get`/`Put` usage as needed.

## Contributing

- **Go 1.24+** required.
- Run tests and benchmarks before changing behavior: `go test ./...` and `go test -bench . -benchmem ./test/`.
- Add tests for new behavior and update docs for user-facing changes.
- All CI checks must pass for PRs to be merged.

```bash
git clone https://github.com/AlexsanderHamir/GenPool.git
cd GenPool
go test -v ./...
golangci-lint run  # optional
```
