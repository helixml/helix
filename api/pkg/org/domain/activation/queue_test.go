package activation_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// TestQueueSerializesPerWorker is the core invariant the Spawner
// relies on: at most one in-flight Spawn per Worker. The number of
// Spawn calls is NOT asserted here (one-per-trigger is covered by
// TestQueueDrainsTriggersOneAtATime); what must hold is that no two
// Spawn calls ever overlap for the same Worker.
func TestQueueSerializesPerWorker(t *testing.T) {
	t.Parallel()
	var inflight, peak int32
	released := make(chan struct{})
	firstStarted := make(chan struct{}, 1)

	spawn := func(_ context.Context, _ string, _ orgchart.WorkerID, _ []activation.Trigger) error {
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
		q.Enqueue("org-test", "w-a", activation.Trigger{Kind: activation.TriggerEvent})
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

// TestQueueDrainsTriggersOneAtATime — two triggers landing while the
// first activation is running are NOT coalesced. Each is delivered in
// its own Spawner call, one trigger per call, in arrival (FIFO) order.
// This is the context-bounding behaviour: a busy Stream can never fold
// its backlog into one oversized activation.
func TestQueueDrainsTriggersOneAtATime(t *testing.T) {
	t.Parallel()
	var batches [][]activation.Trigger
	var mu sync.Mutex
	var done sync.WaitGroup
	done.Add(3) // hire + e-1 + e-2, each its own activation
	holdFirst := make(chan struct{})

	spawn := func(_ context.Context, _ string, _ orgchart.WorkerID, triggers []activation.Trigger) error {
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
	c.Enqueue("org-test", "w-a", activation.Trigger{Kind: activation.TriggerHire})
	// Two more triggers arrive while the first activation is blocked.
	time.Sleep(20 * time.Millisecond)
	c.Enqueue("org-test", "w-a", activation.Trigger{Kind: activation.TriggerEvent, EventID: "e-1"})
	c.Enqueue("org-test", "w-a", activation.Trigger{Kind: activation.TriggerEvent, EventID: "e-2"})
	time.Sleep(20 * time.Millisecond)
	close(holdFirst)
	done.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 3 {
		t.Fatalf("activations = %d, want 3 (one per trigger, no coalescing)", len(batches))
	}
	for i, b := range batches {
		if len(b) != 1 {
			t.Errorf("activation %d carried %d triggers, want exactly 1", i, len(b))
		}
	}
	// FIFO order: hire, then e-1, then e-2.
	if batches[0][0].Kind != activation.TriggerHire {
		t.Errorf("activation 0 kind = %q, want hire", batches[0][0].Kind)
	}
	if batches[1][0].EventID != "e-1" {
		t.Errorf("activation 1 event = %q, want e-1", batches[1][0].EventID)
	}
	if batches[2][0].EventID != "e-2" {
		t.Errorf("activation 2 event = %q, want e-2", batches[2][0].EventID)
	}
}

// TestQueueDifferentWorkersRunInParallel — two activations for
// distinct Workers must not block each other. Only the per-Worker
// queue is serialised.
func TestQueueDifferentWorkersRunInParallel(t *testing.T) {
	t.Parallel()
	started := make(chan orgchart.WorkerID, 2)
	release := make(chan struct{})

	spawn := func(_ context.Context, _ string, w orgchart.WorkerID, _ []activation.Trigger) error {
		started <- w
		<-release
		return nil
	}

	c := activation.NewQueue(spawn, nil)
	c.Enqueue("org-test", "w-a", activation.Trigger{Kind: activation.TriggerHire})
	c.Enqueue("org-test", "w-b", activation.Trigger{Kind: activation.TriggerHire})

	deadline := time.After(time.Second)
	got := map[orgchart.WorkerID]struct{}{}
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
	c.Enqueue("org-test", "w-a", activation.Trigger{Kind: activation.TriggerHire})
	// No goroutine started, no panic; nothing to assert beyond
	// "didn't crash" and the test returning normally.
}

// TestQueueIsolatesSameWorkerIDAcrossOrgs is the regression test for the
// cross-tenant activation leak
// (design/2026-06-09-org-multitenancy-spawner-leak.md).
//
// Worker IDs are unique only within an org — every org's owner is
// "w-owner". The lane key must therefore include the org. When it
// didn't, two orgs' "w-owner" shared one lane: the second Enqueue
// folded into the first's pending list (no parallel runner) and
// overwrote lane.orgID, so activations ran under the wrong tenant.
//
// Here both orgs enqueue the SAME worker id and block in spawn. If they
// share a lane only one spawn starts and the second test-loop receive
// times out. With per-(org,worker) lanes both run in parallel, each
// under its own org.
func TestQueueIsolatesSameWorkerIDAcrossOrgs(t *testing.T) {
	t.Parallel()
	type call struct {
		org string
		w   orgchart.WorkerID
	}
	started := make(chan call, 2)
	release := make(chan struct{})

	spawn := func(_ context.Context, org string, w orgchart.WorkerID, _ []activation.Trigger) error {
		started <- call{org: org, w: w}
		<-release
		return nil
	}

	q := activation.NewQueue(spawn, nil)
	q.Enqueue("org-a", "w-owner", activation.Trigger{Kind: activation.TriggerHire})
	q.Enqueue("org-b", "w-owner", activation.Trigger{Kind: activation.TriggerHire})

	deadline := time.After(2 * time.Second)
	got := map[string]orgchart.WorkerID{}
	for len(got) < 2 {
		select {
		case c := <-started:
			got[c.org] = c.w
		case <-deadline:
			t.Fatalf("same worker id in two orgs did not run in parallel — they share a lane (cross-tenant); got %+v", got)
		}
	}
	close(release)

	if got["org-a"] != "w-owner" || got["org-b"] != "w-owner" {
		t.Fatalf("activations not delivered per-org: %+v", got)
	}
}
