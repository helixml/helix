package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// TestFire_RemovesWorkersActivationStream pins the regression behind
// "we still see s-activations-w-ai-1 and s-activations-w-test-ai
// even though those workers are gone": the Fire cascade tore down
// subscriptions, grants, environment, runtime state, and the worker
// row — but left the per-Worker activation Stream
// (s-activations-<workerID>) lying around, so the Streams page kept
// rendering ghost rows for workers that no longer existed and the
// chart's orphan strip filled up with dashed pseudo-nodes.
//
// Activation events themselves are still audit-retained (the
// `org_events` rows survive); only the Stream row is cleaned up so
// the UI surfaces stop showing it as an active channel.
func TestFire_RemovesWorkersActivationStream(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-test"

	// Seed a position + worker + their activation stream the same way
	// hire_worker would.
	pos, err := orgchart.NewPosition("p-ai", "r-owner", nil, orgID)
	if err != nil {
		t.Fatalf("new position: %v", err)
	}
	if err := st.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}
	worker, err := orgchart.NewAIWorker("w-ghost", pos.ID, "# Ghost", orgID)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := st.Workers.Create(ctx, worker); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	streamID := activation.StreamID(worker.ID())
	stream, err := streaming.NewStream(
		streamID, "Activations: w-ghost", "test",
		worker.ID(), time.Now().UTC(),
		transport.Transport{}, orgID,
	)
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}

	// Sanity: the stream is there before we fire.
	if _, err := st.Streams.Get(ctx, orgID, streamID); err != nil {
		t.Fatalf("precondition: activation stream not seeded: %v", err)
	}

	svc := &lifecycle.Service{Store: st, Owner: "w-owner"}
	if err := svc.Fire(ctx, orgID, worker.ID()); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	// The stream row must be gone.
	if _, err := st.Streams.Get(ctx, orgID, streamID); err == nil {
		t.Fatalf("activation stream %q still exists after Fire — orphan regression", streamID)
	}
}
