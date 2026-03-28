// Package test benchmarks GenPool against [sync.Pool]. See pool_benchmark_test.go.
// Correctness lives in package pool.
//
// # Methodology
//
// One benchmark operation is:
//
//  1. Get an object
//  2. Run the same synthetic work on [BenchmarkObject] for both pools
//  3. Put it back
//
// GenPool: Cleaner runs on Put.
//
// [sync.Pool]: same fields cleared by hand before Put (matches the cleaner).
//
// Same allocator for both. Parallelism: testing.B.RunParallel with benchParallelism
// workers. GenPool: NumShards 0 → [runtime.GOMAXPROCS] shards at pool creation.
//
// Compare runs with the same sub-benchmark suffix (e.g. /pool_only). Winner: lower
// ns/op.
//
// # Levels
//
// Parameters are innerIters (hot int loop) and appendCount (bytes appended into Data;
// slice capacity is reused when already large enough). Source of truth: benchScenarios.
//
//	Name        innerIters   appendCount   Role
//
//	pool_only            0             0   Almost no user work — pool + reset cost.
//	low                500            32   Light CPU + tiny buffer.
//	medium          10_000           100   Medium CPU + buffer.
//	high           100_000           256   Heavy CPU + larger buffer.
//	extreme      1_000_000           256   Heaviest CPU; same append size as high.
//
// # Commands
//
// From the module root:
//
//	go test -bench . -benchmem ./test/
//	go test -bench . -benchmem -count=N ./test/
//	go test -bench BenchmarkGenPool/pool_only -cpuprofile=cpu.prof ./test/
//
// Windows PowerShell (same commands as above work). Broken example:
//
//	go test -bench=. ./test/
//
// That can attach "./test/" to the bench pattern and test package "." instead of
// ./test/. Use `-bench .` (space) or `"-bench=."`.
//
// Do not enable runtime block profiling in these benchmarks when comparing wall time.
//
// BENCHMARKS.md: optional checked-in numbers; update when inputs or the machine line
// change.
package test
