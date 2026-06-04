package dispatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestDispatchActivationStreamSpawnsSubscribedWorker pins the auto-
// spawn-on-activation-stream-event wiring: when a Message event lands
// on an activation Stream (`s-activations-<workerID>`) and a position
// is subscribed to that stream, every AI Worker currently filling that
// position must get an activation enqueued — which is what triggers
// the helix Spawner to provision the per-Worker project and open a
// fresh chat session (the "Human Desktop").
//
// This is the regression we need to pin after the position-anchored
// subscriptions refactor (7f3bc73e2). The fan-out path is:
//
//	publish on s-activations-<X>
//	  → Dispatch lists Subscriptions.ListForStream
//	  → resolves each (position, current AI workers in that position)
//	  → Queue.Enqueue per worker
//	  → Spawn callback fires
//
// Before the refactor, subscriptions were keyed on a Worker directly,
// so the wiring was structurally simpler ("sub.WorkerID → spawn"). The
// refactor inserted the position-hop in between; this test exercises
// the full chain to catch a regression that drops the spawn at the
// end of it (e.g. a position with workers but no Enqueue, or a worker
// resolved via the position-map but skipped before the Enqueue line).
func TestDispatchActivationStreamSpawnsSubscribedWorker(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)

	// A fresh AI worker w-newhire at its own Position. The activation
	// stream `s-activations-w-newhire` is the per-Worker transcript
	// stream that hire_worker creates in production.
	seedAIWorker(t, s, "w-newhire")
	streamID := activation.StreamID("w-newhire")
	seedWebhookStream(t, s, streamID, transport.LocalTransport())
	// The new worker's OWN position is subscribed to its OWN activation
	// stream, so any event published to s-activations-w-newhire fans
	// out to w-newhire and triggers a Spawn — the desktop auto-starts
	// without the operator having to click "Open Human Desktop".
	seedSubscription(t, s, "w-newhire", streamID)

	// A publisher in a different position (so the publisher-self-skip
	// rule doesn't suppress the activation).
	seedAIWorker(t, s, "w-publisher")

	e, err := streaming.NewMessageEvent(
		"e-activation-1",
		streamID,
		"w-publisher",
		streaming.Message{From: "w-publisher", Body: "first turn"},
		time.Now().UTC(),
		"org-test",
	)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	if err := s.Events.Append(context.Background(), e); err != nil {
		t.Fatalf("append event: %v", err)
	}

	d.Dispatch(context.Background(), e)

	got := drainActivations(t, rec, 500*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("activations = %d, want 1 (event on s-activations-<W> must spawn W); got %+v", len(got), got)
	}
	if got[0].WorkerID != "w-newhire" {
		t.Fatalf("spawned worker = %q, want %q", got[0].WorkerID, "w-newhire")
	}
}

// TestDispatchHireEnqueuesSpawn pins the second half of the auto-spawn
// wiring: a brand-new AI hire (no transcript yet, no subscription to
// itself) still gets a TriggerHire activation from DispatchHire that
// drives the Spawner to provision the project + open the first
// session.
//
// hire_worker calls Dispatcher.DispatchHire directly (not via a stream
// publish) so this is the only path that exercises the
// `dispatch.DispatchHire → Queue.Enqueue → Spawn` chain at the unit
// layer. Without it, a newly-hired AI worker would never get its
// first activation — the operator would have to click "Open Human
// Desktop" by hand to bootstrap the session.
func TestDispatchHireEnqueuesSpawn(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)

	seedAIWorker(t, s, "w-fresh-hire")

	d.DispatchHire(context.Background(), "org-test", "w-fresh-hire", "/tmp/env-fresh-hire", activation.ID("a-hire-1"))

	got := drainActivations(t, rec, 500*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("activations = %d, want 1 (DispatchHire must spawn the worker); got %+v", len(got), got)
	}
	if got[0].WorkerID != "w-fresh-hire" {
		t.Fatalf("spawned worker = %q, want %q", got[0].WorkerID, "w-fresh-hire")
	}
	if len(got[0].Triggers) != 1 || got[0].Triggers[0].Kind != activation.TriggerHire {
		t.Fatalf("trigger kind = %+v, want one TriggerHire", got[0].Triggers)
	}
	if got[0].Triggers[0].ActivationID != activation.ID("a-hire-1") {
		t.Fatalf("ActivationID = %q, want %q", got[0].Triggers[0].ActivationID, activation.ID("a-hire-1"))
	}
}
