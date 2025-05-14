## Explanation

During the cleanup cycle, each object’s usage count is evaluated. If an object survives this round, its usage count is reset to 0. On the next cleanup pass, if it fails to meet the MinUsageCount threshold, it becomes eligible for eviction.

This two-pass approach ensures that objects which were heavily used in the past but are no longer actively accessed will eventually be removed. Without this mechanism, stale but once-popular objects could remain in the pool indefinitely, leading to memory bloat and poor cache hygiene.

By resetting the usage count only for retained objects, the system gives every object a fair chance to prove recent utility before eviction—encouraging temporal locality and keeping the pool fresh.

````go

        shouldKeep := usageCount >= p.cfg.Cleanup.MinUsageCount && (p.cfg.Cleanup.TargetSize <= 0 || kept < p.cfg.Cleanup.TargetSize)
		if shouldKeep {
			// Reset usage count for kept objects
			current.ResetUsage()
			prev = current
			kept++
			fmt.Printf("[Cleanup] Keeping object (total kept: %d)\n", kept)
		} else {
			// Remove current object from list
			if reflect.ValueOf(prev).IsNil() {
				// We're at the head
				fmt.Println("[Cleanup] Removing head object")
				p.head.Store(next)
			} else {
				fmt.Println("[Cleanup] Removing object from middle of list")
				prev.SetNext(next)
			}
			if p.active.Load() > 0 {
				p.active.Add(-1)
			}
			removed++
		}
```
````
