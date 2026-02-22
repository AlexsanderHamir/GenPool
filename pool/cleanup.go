package pool

import (
	"time"
)

// startCleaner starts the background cleanup goroutine.
func (p *ShardedPool[T, P]) startCleaner() {
	p.cleanWg.Add(1)
	go func() {
		defer p.cleanWg.Done()
		ticker := time.NewTicker(p.cfg.Cleanup.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.cleanup()
			case <-p.stopClean:
				return
			}
		}
	}()
}

func (p *ShardedPool[T, P]) cleanup() {
	if !p.cfg.Cleanup.Enabled {
		return
	}

	for _, shard := range p.Shards {
		p.cleanupShard(shard)
	}
}

func (p *ShardedPool[T, P]) cleanupShard(shard *Shard[T, P]) {
	oldHead := p.tryTakeOwnership(shard)
	if oldHead == nil {
		return
	}

	keptHead, keptTail, evictedCount := p.filterUsableObjects(oldHead)

	if evictedCount > 0 {
		p.CurrentPoolLength.Add(-int64(evictedCount))
	}

	if keptHead != nil {
		p.reinsertKeptObjects(shard, keptHead, keptTail)
	}
}

func (p *ShardedPool[T, P]) tryTakeOwnership(shard *Shard[T, P]) P {
	head := P(shard.Head.Load())
	if head == nil {
		return nil
	}
	if !shard.Head.CompareAndSwap(head, nil) {
		return nil
	}
	return head
}

// filterUsableObjects filters objects based on usage count and returns the kept head, kept tail, and number of evicted objects.
func (p *ShardedPool[T, P]) filterUsableObjects(head P) (keptHead, keptTail P, evictedCount int) {
	current := head

	for current != nil {
		next := current.GetNext()
		usageCount := current.GetUsageCount()

		if usageCount >= p.cfg.Cleanup.MinUsageCount {
			current.ResetUsage()
			if keptHead == nil {
				keptHead = current
			} else {
				keptTail.SetNext(current)
			}
			keptTail = current
		} else {
			current.SetNext(nil)
			evictedCount++
		}
		current = next
	}

	if keptHead == nil {
		return nil, nil, evictedCount
	}

	keptTail.SetNext(nil)
	return keptHead, keptTail, evictedCount
}

func (p *ShardedPool[T, P]) reinsertKeptObjects(shard *Shard[T, P], keptHead, keptTail P) {
	var shardID int
	for i, s := range p.Shards {
		if s == shard {
			shardID = i
			break
		}
	}

	current := keptHead
	for current != nil {
		current.SetShardIndex(shardID)
		current = current.GetNext()
	}

	for {
		currentHead := P(shard.Head.Load())
		if currentHead != nil {
			keptTail.SetNext(currentHead)
		}
		if shard.Head.CompareAndSwap(currentHead, keptHead) {
			break
		}
	}
}
