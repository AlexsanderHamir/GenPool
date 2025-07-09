# Minimal Object Pool Design

This object pool uses an atomic singly linked list built from the provided objects themselves to manage the pool efficiently, reducing indirection and allocations. Each object maintains a pointer to the next one using `atomic.Pointer`, allowing lock-free access and updates.

The pool implements **sharding** by maintaining **X** independent linked lists up to the number of logical cores, two runtime functions are used `runtime_procPin` and `runtime_procUnpin`, which are low-level Go runtime functions that:

1. `runtime_procPin()`: Temporarily pins the current goroutine to its current processor (P) and returns the processor ID. This prevents the goroutine from being moved to a different processor during critical sections.

2. `runtime_procUnpin()`: Unpins the goroutine from its processor, allowing it to be scheduled on any available processor again.

These functions are used in the `getShard()` method to efficiently determine which shard a goroutine should access, ensuring better cache locality and reducing contention. The processor ID returned by `runtime_procPin()` is used as the shard index, providing a simple and effective way to distribute load across the available CPU cores.

The primary advantage of this approach is that it supports **efficient dynamic resizing** (growing and shrinking) of the pool without relying on locks or centralized data structures. Although the use of pointers and atomics sacrifices **some cache locality** compared to slice-based or array-backed pools, it still results in better throughput under concurrent access patterns, particularly in **high-contention environments**.

By storing the `next` pointer atomically inside each object, the pool can quickly push or pop objects to/from the list using low-cost atomic operations. This design minimizes allocation overhead during peak loads and avoids blocking other goroutines, unlike traditional mutex-based pools.

The combination of sharding and runtime optimizations makes this pool particularly effective for **highly concurrent workloads**, where multiple goroutines frequently acquire and release objects simultaneously. The sharded design ensures that even under extreme contention, the pool maintains predictable performance characteristics.
