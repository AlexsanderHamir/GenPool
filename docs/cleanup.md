## Explanation

During the cleanup cycle, each object's usage count is evaluated. Objects with usage count greater than or equal to `MinUsageCount` are retained, while those below the threshold may be evicted. If an object survives this round, its usage count is reset to 0. On the next cleanup pass, if it fails to meet the MinUsageCount threshold, it becomes eligible for eviction.

This two-pass approach ensures that objects which were heavily used in the past but are no longer actively accessed will eventually be removed. Without this mechanism, stale but once-popular objects could remain in the pool indefinitely, leading to memory bloat and poor cache hygiene.

By resetting the usage count only for retained objects, the system gives every object a fair chance to prove recent utility before eviction—encouraging temporal locality and keeping the pool fresh.

````go
func (p *ShardedPool[T, P]) cleanupShard(shard *PoolShard[T, P]) {
	var current, prev T
	var kept int

	current := shard.head.Load()
	if current == nil {
		return
	}

	for current != nil {
		next := current.GetNext()
		usageCount := current.GetUsageCount()

		metMinUsageCount := usageCount >= p.cfg.Cleanup.MinUsageCount
		targetDisabled := p.cfg.Cleanup.TargetSize <= 0

		// Ensure at least 1 object per shard when TargetSize is set
		shardQuota := 1
		if p.cfg.Cleanup.TargetSize > 0 {
			shardQuota = max(shardQuota, p.cfg.Cleanup.TargetSize/numShards)
		}
		underShardQuota := kept < shardQuota

		shouldKeep := metMinUsageCount && (targetDisabled || underShardQuota)
		if shouldKeep {
			current.ResetUsage()
			prev = current
			kept++
		} else {
			if prev == nil {
				if next == nil {
					shard.head.Store(nil)
				} else {
					shard.head.Store(next)
				}
			} else {
				prev.SetNext(next)
			}

			current.SetNext(nil)
			p.cfg.Cleaner(current)
		}

		current = next
	}
}

```
````
