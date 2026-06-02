package activation_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// TestQueueSerializesPerWorker is the core invariant the Spawner
// relies on: at most one in-flight Spawn per Worker. The number of
// Spawn calls is NOT asserted — coalescing is racy by design (a
// trigger arriving while a batch is mid-claim gets folded in), so
// the count varies between 1 and len(Enqueues). What must hold is
// that no two Spawn calls ever overlap for the same Worker.
func TestQueueSerializesPerWorker(t *testing.T) {
	t.Parallel()
	var inflight, peak int32
	released := make(chan struct{})
	firstStarted := make(chan struct{}, 1)

	spawn := func(_ context.Context, _ string, _ worker.ID, _ string, _ []activation.Trigger) error {
		cur := atomic.AddInt32(&inflight, 1)
		for {
			old := atomic.LoadInt32(&peak)
			if cur <= old || atomic.CompareAndSwapInt32(&peak, old, cur) {
				break
			}
		}
		select {
		case firstStarted <- struct{}{}:
		default:
		}
		<-released
		atomic.AddInt32(&inflight, -1)
		return nil
	}

	q := activation.NewQueue(spawn, nil)
	for i := 0; i < 4; i++ {
		q.Enqueue("org-test", "w-a", "/env/a", activation.Trigger{Kind: activation.TriggerEvent})
	}
	// Block until the first Spawn is actually running so we know any
	// trailing Enqueues are queued behind it rather than racing to
	// start a parallel call.
	<-firstStarted
	close(released)

	// Settle: wait for inflight to hit 0, i.e. the lane is fully
	// drained. Peak should never have exceeded 1.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&inflight) != 0 {
		select {
		case <-deadline:
			t.Fatalf("inflight never settled to 0; peak=%d", atomic.LoadInt32(&peak))
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if got := atomic.LoadInt32(&peak); got > 1 {
		t.Fatalf("peak inflight per worker = %d, want ≤ 1", got)
	}
}

// TestQueueCoalescesBurstIntoOneBatch — two triggers landing
// while the first activation is running become ONE batch on the next
// drain, not two separate Spawner calls.
func TestQueueCoalescesBurstIntoOneBatch(t *testing.T) {
	t.Parallel()
	var batches [][]activation.Trigger
	var mu sync.Mutex
	var done sync.WaitGroup
	done.Add(2) // first batch + the coalesced follow-up
	holdFirst := make(chan struct{})

	spawn := func(_ context.Context, _ string, _ worker.ID, _ string, triggers []activation.Trigger) error {
		mu.Lock()
		idx := len(batches)
		copied := make([]activation.Trigger, len(triggers))
		copy(copied, triggers)
		batches = append(batches, copied)
		mu.Unlock()
		if idx == 0 {
			<-holdFirst
		}
		done.Done()
		return nil
	}

	c := activation.NewQueue(spawn, nil)
	c.Enqueue("org-test", "w-a", "/env/a", activation.Trigger{Kind: activation.TriggerHire})
	// Two more triggers arrive while batch 0 is blocked.
	time.Sleep(20 * time.Millisecond)
	c.Enqueue("org-test", "w-a", "/env/a", activation.Trigger{Kind: activation.TriggerEvent, EventID: "e-1"})
	c.Enqueue("org-test", "w-a", "/env/a", activation.Trigger{Kind: activation.TriggerEvent, EventID: "e-2"})
	time.Sleep(20 * time.Millisecond)
	close(holdFirst)
	done.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want 2 (the burst coalesces)", len(batches))
	}
	if len(batches[0]) != 1 {
		t.Errorf("first batch size = %d, want 1", len(batches[0]))
	}
	if len(batches[1]) != 2 {
		t.Errorf("second batch size = %d, want 2 (e-1 + e-2 coalesced)", len(batches[1]))
	}
}

// TestQueueDifferentWorkersRunInParallel — two activations for
// distinct Workers must not block each other. Only the per-Worker
// queue is serialised.
func TestQueueDifferentWorkersRunInParallel(t *testing.T) {
	t.Parallel()
	started := make(chan worker.ID, 2)
	release := make(chan struct{})

	spawn := func(_ context.Context, _ string, w worker.ID, _ string, _ []activation.Trigger) error {
		started <- w
		<-release
		return nil
	}

	c := activation.NewQueue(spawn, nil)
	c.Enqueue("org-test", "w-a", "/env/a", activation.Trigger{Kind: activation.TriggerHire})
	c.Enqueue("org-test", "w-b", "/env/b", activation.Trigger{Kind: activation.TriggerHire})

	deadline := time.After(time.Second)
	got := map[worker.ID]struct{}{}
	for len(got) < 2 {
		select {
		case w := <-started:
			got[w] = struct{}{}
		case <-deadline:
			t.Fatalf("only %d workers started in parallel; got %+v", len(got), got)
		}
	}
	close(release)
}

// TestQueueNilSpawnerIsNoop confirms the standard nil-spawner
// behaviour the existing dispatcher honoured. Useful for test wirings
// that exercise the event-side fan-out without running real
// activations.
func TestQueueNilSpawnerIsNoop(t *testing.T) {
	t.Parallel()
	c := activation.NewQueue(nil, nil)
	c.Enqueue("org-test", "w-a", "/env/a", activation.Trigger{Kind: activation.TriggerHire})
	// No goroutine started, no panic; nothing to assert beyond
	// "didn't crash" and the test returning normally.
}
