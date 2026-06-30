package dispatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestDispatchTranscriptEventSpawnsSubscribedWorker pins the auto-
// spawn-on-transcript-event wiring: when a Message event lands
// on an transcript (`s-transcript-<workerID>`) and a position
// is subscribed to that topic, every AI Worker currently filling that
// position must get an activation enqueued — which is what triggers
// the helix Spawner to provision the per-Worker project and open a
// fresh chat session (the "Human Desktop").
//
// This is the regression we need to pin after the position-anchored
// subscriptions refactor (7f3bc73e2). The fan-out path is:
//
//	publish on s-transcript-<X>
//	  → Dispatch lists Subscriptions.ListForTopic
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
func TestDispatchTranscriptEventSpawnsSubscribedWorker(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)

	// A fresh AI worker w-newhire at its own Position. The activation
	// topic `s-transcript-w-newhire` is the per-Worker transcript
	// topic that hire_worker creates in production.
	seedBot(t, s, "w-newhire")
	topicID := activation.TranscriptID("w-newhire")
	seedWebhookTopic(t, s, topicID, transport.LocalTransport())
	// The new worker's OWN position is subscribed to its OWN activation
	// topic, so any event published to s-transcript-w-newhire fans
	// out to w-newhire and triggers a Spawn — the desktop auto-starts
	// without the operator having to click "Open Human Desktop".
	seedSubscription(t, s, "w-newhire", topicID)

	// A publisher in a different position (so the publisher-self-skip
	// rule doesn't suppress the activation).
	seedBot(t, s, "w-publisher")

	e, err := streaming.NewMessageEvent(
		"e-activation-1",
		topicID,
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
		t.Fatalf("activations = %d, want 1 (event on s-transcript-<W> must spawn W); got %+v", len(got), got)
	}
	if got[0].BotID != "w-newhire" {
		t.Fatalf("spawned worker = %q, want %q", got[0].BotID, "w-newhire")
	}
}

// TestDispatchHireEnqueuesSpawn pins the second half of the auto-spawn
// wiring: a brand-new AI hire (no transcript yet, no subscription to
// itself) still gets a TriggerHire activation from DispatchHire that
// drives the Spawner to provision the project + open the first
// session.
//
// hire_worker calls Dispatcher.DispatchHire directly (not via a topic
// publish) so this is the only path that exercises the
// `dispatch.DispatchHire → Queue.Enqueue → Spawn` chain at the unit
// layer. Without it, a newly-hired AI worker would never get its
// first activation — the operator would have to click "Open Human
// Desktop" by hand to bootstrap the session.
func TestDispatchHireEnqueuesSpawn(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)

	seedBot(t, s, "w-fresh-hire")

	d.DispatchHire(context.Background(), "org-test", "w-fresh-hire", activation.ID("a-hire-1"))

	got := drainActivations(t, rec, 500*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("activations = %d, want 1 (DispatchHire must spawn the worker); got %+v", len(got), got)
	}
	if got[0].BotID != "w-fresh-hire" {
		t.Fatalf("spawned worker = %q, want %q", got[0].BotID, "w-fresh-hire")
	}
	if len(got[0].Triggers) != 1 || got[0].Triggers[0].Kind != activation.TriggerHire {
		t.Fatalf("trigger kind = %+v, want one TriggerHire", got[0].Triggers)
	}
	if got[0].Triggers[0].ActivationID != activation.ID("a-hire-1") {
		t.Fatalf("ActivationID = %q, want %q", got[0].Triggers[0].ActivationID, activation.ID("a-hire-1"))
	}
}

// TestDispatchManualEnqueuesSpawn pins the manual-activation wiring:
// the worker UI's "Start Desktop" button hits POST /workers/{id}/activate,
// which calls Dispatcher.DispatchManual; the dispatcher must enqueue a
// TriggerManual activation against the worker's per-Worker queue with
// the caller-supplied pre-allocated audit-row ID. Without this, the
// activate endpoint becomes a no-op and the desktop stays in whatever
// MCP-clobbered state it was already in.
func TestDispatchManualEnqueuesSpawn(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)
	seedBot(t, s, "w-manual")

	d.DispatchManual(context.Background(), "org-test", "w-manual", activation.ID("a-manual-1"))

	got := drainActivations(t, rec, 500*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("activations = %d, want 1 (DispatchManual must spawn the worker); got %+v", len(got), got)
	}
	if got[0].BotID != "w-manual" {
		t.Fatalf("spawned worker = %q, want %q", got[0].BotID, "w-manual")
	}
	if len(got[0].Triggers) != 1 || got[0].Triggers[0].Kind != activation.TriggerManual {
		t.Fatalf("trigger kind = %+v, want one TriggerManual", got[0].Triggers)
	}
	if got[0].Triggers[0].ActivationID != activation.ID("a-manual-1") {
		t.Fatalf("ActivationID = %q, want %q", got[0].Triggers[0].ActivationID, activation.ID("a-manual-1"))
	}
}
