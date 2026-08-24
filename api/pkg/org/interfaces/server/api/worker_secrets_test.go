package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/workersecrets"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// liveSourceResolver accepts every binding, so availability in these
// tests is decided purely by whether the catalog still lists the source
// — the same thing that happens when a project secret is deleted.
type liveSourceResolver struct{}

func (liveSourceResolver) Validate(context.Context, workersecret.Binding) error { return nil }
func (liveSourceResolver) Resolve(context.Context, workersecret.Binding) (workersecret.Resolved, error) {
	return workersecret.Resolved{}, nil
}

// newWorkerSecretsDeps seeds one Agent holding two grants — one whose
// source is still in the catalog, one whose source has been deleted.
func newWorkerSecretsDeps(t *testing.T) orgapi.Deps {
	t.Helper()
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	node, err := orgchart.NewNode("b-agent", "# Agent", nil, now, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}
	for _, b := range []workersecret.Binding{
		{Name: "LIVE_TOKEN", SecretID: "sec-live"},
		{Name: "DEAD_TOKEN", SecretID: "sec-deleted"},
	} {
		b.OrganizationID, b.WorkerID = "org-test", node.ID
		b.SourceKind, b.CreatedAt, b.UpdatedAt = workersecret.SourceHelixSecret, now, now
		if err := st.WorkerSecretBindings.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	catalog := func(context.Context, string, orgchart.NodeID) ([]workersecret.AvailableSource, error) {
		return []workersecret.AvailableSource{{
			Label: "LIVE_TOKEN", SourceKind: workersecret.SourceHelixSecret,
			SecretID: "sec-live", ProposedName: "LIVE_TOKEN",
		}}, nil
	}
	svc, err := workersecrets.New(st.WorkerSecretBindings, st.Nodes, liveSourceResolver{}, func() time.Time { return now }, nil, catalog)
	if err != nil {
		t.Fatal(err)
	}
	deps.WorkerSecrets = svc
	return deps
}

// A grant whose source has been deleted stays in the list and is marked
// unavailable, so the panel showing what an Agent holds can show the
// break instead of looking healthy.
func TestListWorkerSecrets_MarksDeletedSourceUnavailable(t *testing.T) {
	deps := newWorkerSecretsDeps(t)

	rec := do(t, orgapi.Handler(deps), http.MethodGet, "/agents/b-agent/secrets", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body)
	}
	var got []orgapi.WorkerSecretBindingDTO
	decode(t, rec, &got)
	if len(got) != 2 {
		t.Fatalf("got %d bindings, want 2: %+v", len(got), got)
	}
	byName := map[string]orgapi.WorkerSecretBindingDTO{}
	for _, b := range got {
		byName[b.Name] = b
	}
	if !byName["LIVE_TOKEN"].Available {
		t.Error("LIVE_TOKEN should be available; its source is still in the catalog")
	}
	if byName["DEAD_TOKEN"].Available {
		t.Error("DEAD_TOKEN should be unavailable; its source was deleted")
	}
}
