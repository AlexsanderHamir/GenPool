# Minimal Object Pool Design

This object pool uses an atomic singly linked list built from the objects themselves to manage the pool efficiently, reducing indirection and allocations. Each object maintains a pointer to the next one using `atomic.Value`, allowing lock-free access and updates.

The primary advantage of this approach is that it supports **efficient dynamic resizing** (growing and shrinking) of the pool without relying on locks or centralized data structures. Although the use of pointers and atomics sacrifices **some cache locality** compared to slice-based or array-backed pools, it still results in better throughput under concurrent access patterns, particularly in **high-contention environments**.

By storing the `next` pointer atomically inside each object, the pool can quickly push or pop objects to/from the list using low-cost atomic operations. This design minimizes allocation overhead during peak loads and avoids blocking other goroutines, unlike traditional mutex-based pools.
