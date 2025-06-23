## Explanation

During the cleanup cycle, each object's usage count is evaluated. Objects with usage count greater than or equal to `MinUsageCount` are retained, while those below the threshold may be evicted. If an object survives this round, its usage count is reset to 0. On the next cleanup pass, if it fails to meet the MinUsageCount threshold, it becomes eligible for eviction.

This two-pass approach ensures that objects which were heavily used in the past but are no longer actively accessed will eventually be removed. Without this mechanism, stale but once-popular objects could remain in the pool indefinitely, leading to memory bloat and poor cache hygiene.

By resetting the usage count only for retained objects, the system gives every object a fair chance to prove recent utility before eviction—encouraging temporal locality and keeping the pool fresh.


````go

    func (p *ShardedPool[T]) cleanupShard(shard *PoolShard[T]) {
	var current, prev T
	var kept int
	var zero T

	current, ok := shard.head.Load().(T)
	if !ok {
		return
	}

	if reflect.ValueOf(current).IsNil() {
		return
	}

	for !reflect.ValueOf(current).IsNil() {
		next := current.GetNext()
		usageCount := current.GetUsageCount()

		metMinUsageCount := usageCount >= p.cfg.Cleanup.MinUsageCount
		targetDisabled := p.cfg.Cleanup.TargetSize <= 0
		// Ensure at least 1 object per shard when TargetSize is set
		shardQuota := 1
		if p.cfg.Cleanup.TargetSize > 0 {
			shardQuota = max(1, p.cfg.Cleanup.TargetSize/numShards)
		}
		underShardQuota := kept < shardQuota

		shouldKeep := metMinUsageCount && (targetDisabled || underShardQuota)
		if shouldKeep {
			current.ResetUsage()
			prev = current
			kept++
		} else {
			if reflect.ValueOf(prev).IsNil() {
				if reflect.ValueOf(next).IsNil() {
					shard.head.Store(zero)
				} else {
					shard.head.Store(next)
				}
			} else {
				prev.SetNext(next)
			}

			current.SetNext(zero)
			p.cfg.Cleaner(current)
		}

		current = next.(T)
	}
}

```
````
