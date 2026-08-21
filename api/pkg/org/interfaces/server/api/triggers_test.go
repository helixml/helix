package api_test

import (
	"context"
	"net/http"
	"testing"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

func TestTriggerCRUDAndStaleRevision(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	created := do(t, h, http.MethodPost, "/triggers", map[string]any{"name": "Incoming", "kind": "local"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body)
	}
	var tr orgapi.TriggerDTO
	decode(t, created, &tr)
	if tr.ID == "" || tr.Revision == "" || tr.Name != "Incoming" {
		t.Fatalf("created trigger=%+v", tr)
	}

	updated := do(t, h, http.MethodPut, "/triggers/"+tr.ID, map[string]any{"name": "Renamed", "kind": "local", "revision": tr.Revision})
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body)
	}

	stale := do(t, h, http.MethodPut, "/triggers/"+tr.ID, map[string]any{"name": "Lost update", "kind": "local", "revision": tr.Revision})
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body)
	}
	var apiErr orgapi.APIError
	decode(t, stale, &apiErr)
	if apiErr.Code != "stale_resource" {
		t.Fatalf("stale code=%q", apiErr.Code)
	}
}

func TestAgentAttachmentRequiresExactProcessorOutput(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedBot(t, st, context.Background(), "w-one", "# Worker")

	invalid := do(t, h, http.MethodPost, "/agents/w-one/attachments", map[string]any{"source": map[string]any{"kind": "processor_output", "processor_id": "p-one"}})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body)
	}
	var apiErr orgapi.APIError
	decode(t, invalid, &apiErr)
	if apiErr.Code != "validation_failed" {
		t.Fatalf("invalid code=%q", apiErr.Code)
	}
}
