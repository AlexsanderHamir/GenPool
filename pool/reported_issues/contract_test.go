// Package reported_issues contains regression tests that validate
// behaviors previously reported through GitHub issues.
//
// Issues covered by this file:
// - https://github.com/AlexsanderHamir/GenPool/issues/31
// - https://github.com/AlexsanderHamir/GenPool/issues/32

package reported_issues

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/AlexsanderHamir/GenPool/pool"
)

// stageObject is used to test that objects are never reused before being cleaned.
// Stage must be "new" (fresh alloc), "used" (consumer handed it), or "reset" (after cleaner).
type stageObject struct {
	Stage string
	pool.Fields[stageObject]
}

// TestConcurrentGetPutCleanerContract reproduces the scenario where a pool is shared
// across goroutines: one goroutine Gets, marks the object "used", sends it to a channel;
// another receives and Puts. The cleaner expects to see "used" and sets "reset".
// This test verifies that no object is ever reused before being cleaned (Get must only
// see "new" or "reset") and that Put is never given an object that wasn't marked "used"
// by the consumer (Put's cleaner must only see "used").
// The test runs multiple rounds with multiple producer-consumer pairs per round
// to increase contention. Run with: go test -race -run TestConcurrentGetPutCleanerContract
func TestConcurrentGetPutCleanerContract(t *testing.T) {
	const runRounds = 10
	for round := range runRounds {
		t.Run(fmt.Sprintf("Round%d", round+1), func(t *testing.T) {
			runConcurrentGetPutCleanerContractRound(t)
		})
	}
}

func runConcurrentGetPutCleanerContractRound(t *testing.T) {
	const iterations = 100_000
	const numPairs = 4
	const chanCap = 200

	recorder := newStageContractRecorder(10)
	p, err := newStageObjectPool(recorder)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	v := make(chan *stageObject, chanCap)
	perPair := iterations / numPairs
	runStageContractConsumers(p, v, numPairs, perPair, recorder)
	runStageContractProducers(p, v, numPairs, recorder)
	recorder.report(t)
}

// stageContractRecorder records violations of the stage contract (Get sees new/reset, Put sees used).
type stageContractRecorder struct {
	mu        sync.Mutex
	sample    []string
	count     atomic.Int64
	maxSample int
}

func newStageContractRecorder(maxSample int) *stageContractRecorder {
	return &stageContractRecorder{maxSample: maxSample}
}

func (r *stageContractRecorder) record(msg string) {
	r.mu.Lock()
	r.count.Add(1)
	if len(r.sample) < r.maxSample {
		r.sample = append(r.sample, msg)
	}
	r.mu.Unlock()
}

func (r *stageContractRecorder) report(t *testing.T) {
	r.mu.Lock()
	sample := r.sample
	total := r.count.Load()
	r.mu.Unlock()
	if total > 0 {
		t.Errorf("cleaner/reuse contract violated %d time(s). Sample: %v", total, sample)
	}
}

// newStageObjectPool creates a pool whose cleaner enforces Stage "used" and sets "reset".
// Violations are recorded to recorder.
func newStageObjectPool(recorder *stageContractRecorder) (*pool.ShardedPool[stageObject, *stageObject], error) {
	allocator := func() *stageObject {
		return &stageObject{Stage: "new"}
	}
	cleaner := func(obj *stageObject) {
		if obj == nil {
			recorder.record("cleaner received nil")
			return
		}
		if obj.Stage != "used" {
			recorder.record(fmt.Sprintf("cleaner received obj.Stage=%q (expected \"used\")", obj.Stage))
		}
		obj.Stage = "reset"
	}
	cfg := pool.Config[stageObject, *stageObject]{
		Allocator: allocator,
		Cleaner:   cleaner,
		Cleanup:   pool.CleanupPolicy{Enabled: false},
	}
	return pool.NewPoolWithConfig(cfg)
}

// runStageContractConsumers starts numPairs goroutines that Get from pool, check Stage is new/reset, set used, and send to ch.
// Closes ch when all are done.
func runStageContractConsumers(p *pool.ShardedPool[stageObject, *stageObject], ch chan<- *stageObject, numPairs, perPair int, recorder *stageContractRecorder) {
	var wg sync.WaitGroup
	wg.Add(numPairs)
	for range numPairs {
		go func() {
			defer wg.Done()
			for i := 0; i < perPair; i++ {
				obj := p.Get()
				if obj == nil {
					continue
				}
				if obj.Stage != "new" && obj.Stage != "reset" {
					recorder.record(fmt.Sprintf("Get() returned obj.Stage=%q (expected \"new\" or \"reset\")", obj.Stage))
				}
				obj.Stage = "used"
				ch <- obj
			}
		}()
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
}

// runStageContractProducers starts numPairs goroutines that receive from ch, check Stage is used, and Put to pool.
// It blocks until the channel is closed and all items are Put.
func runStageContractProducers(p *pool.ShardedPool[stageObject, *stageObject], ch <-chan *stageObject, numPairs int, recorder *stageContractRecorder) {
	var wg sync.WaitGroup
	wg.Add(numPairs)
	for range numPairs {
		go func() {
			defer wg.Done()
			for obj := range ch {
				if obj.Stage != "used" {
					recorder.record(fmt.Sprintf("Put() called with obj.Stage=%q (expected \"used\")", obj.Stage))
				}
				p.Put(obj)
			}
		}()
	}
	wg.Wait()
}
