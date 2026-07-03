package helixevents_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/helixevents"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

const org = "org-1"

func fixedNow() func() time.Time {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// TestReconcile_CreatesSingleTopic pins that a first Reconcile creates
// exactly one helix_events topic with the deterministic id.
func TestReconcile_CreatesSingleTopic(t *testing.T) {
	t.Parallel()
	s := memory.New()
	rec := helixevents.New(helixevents.Deps{Topics: s.Topics, Now: fixedNow()})
	ctx := context.Background()

	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	list, err := s.Topics.List(ctx, org)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("topic count = %d, want 1 (%v)", len(list), list)
	}
	got := list[0]
	if got.ID != helixevents.TopicID {
		t.Fatalf("topic id = %q, want %q", got.ID, helixevents.TopicID)
	}
	if got.Transport.Kind != transport.KindHelixEvents {
		t.Fatalf("transport kind = %q, want %q", got.Transport.Kind, transport.KindHelixEvents)
	}
}

// TestReconcile_Idempotent pins that repeated Reconcile never creates a
// second topic.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()
	s := memory.New()
	rec := helixevents.New(helixevents.Deps{Topics: s.Topics, Now: fixedNow()})
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := rec.Reconcile(ctx, org); err != nil {
			t.Fatalf("Reconcile #%d: %v", i, err)
		}
	}
	list, err := s.Topics.List(ctx, org)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("topic count = %d, want 1 after repeated reconcile", len(list))
	}
}

// TestReconcile_NilDeps pins the nil-safe no-op contract.
func TestReconcile_NilDeps(t *testing.T) {
	t.Parallel()
	var rec *helixevents.Reconciler
	if err := rec.Reconcile(context.Background(), org); err != nil {
		t.Fatalf("nil Reconciler Reconcile = %v, want nil", err)
	}
	unwired := helixevents.New(helixevents.Deps{})
	if err := unwired.Reconcile(context.Background(), org); err != nil {
		t.Fatalf("unwired Reconciler Reconcile = %v, want nil", err)
	}
}
