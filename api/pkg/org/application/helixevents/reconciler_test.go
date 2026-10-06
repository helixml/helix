package helixevents_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/helixevents"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

const org = "org-1"

func fixedNow() func() time.Time {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// TestReconcile_CreatesSingleTrigger pins that a first Reconcile creates
// exactly one helix_events Trigger with the deterministic id.
func TestReconcile_CreatesSingleTrigger(t *testing.T) {
	t.Parallel()
	s := memory.New()
	rec := helixevents.New(helixevents.Deps{Triggers: s.Triggers, Now: fixedNow()})
	ctx := context.Background()

	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	list, err := s.Triggers.Find(ctx, store.WithOrg(org))
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("trigger count = %d, want 1 (%v)", len(list), list)
	}
	got := list[0]
	if got.ID != helixevents.TriggerID {
		t.Fatalf("trigger id = %q, want %q", got.ID, helixevents.TriggerID)
	}
	if got.Kind != transport.KindHelixEvents {
		t.Fatalf("transport kind = %q, want %q", got.Kind, transport.KindHelixEvents)
	}
}

// TestReconcile_Idempotent pins that repeated Reconcile never creates a
// second Trigger.
func TestReconcile_Idempotent(t *testing.T) {
	t.Parallel()
	s := memory.New()
	rec := helixevents.New(helixevents.Deps{Triggers: s.Triggers, Now: fixedNow()})
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := rec.Reconcile(ctx, org); err != nil {
			t.Fatalf("Reconcile #%d: %v", i, err)
		}
	}
	list, err := s.Triggers.Find(ctx, store.WithOrg(org))
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("trigger count = %d, want 1 after repeated reconcile", len(list))
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
