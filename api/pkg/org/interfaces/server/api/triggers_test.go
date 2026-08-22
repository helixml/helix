package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

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

// TestTriggerDTOReportsAttachedWorkers pins the Triggers list column the
// Topics page used to have: which Workers this Trigger actually starts.
// It is derived from attachments, not stored on the Trigger.
func TestTriggerDTOReportsAttachedWorkers(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedBot(t, st, context.Background(), "w-one", "# Worker")

	created := do(t, h, http.MethodPost, "/triggers", map[string]any{"name": "Incoming", "kind": "local"})
	var tr orgapi.TriggerDTO
	decode(t, created, &tr)

	// Before attaching, the Trigger starts nobody.
	var before orgapi.TriggerDTO
	decode(t, do(t, h, http.MethodGet, "/triggers/"+tr.ID, nil), &before)
	if len(before.AttachedWorkers) != 0 {
		t.Fatalf("attached_workers before attach = %v, want none", before.AttachedWorkers)
	}

	attach := do(t, h, http.MethodPost, "/agents/w-one/attachments", map[string]any{
		"source": map[string]any{"kind": "trigger", "trigger_id": tr.ID},
	})
	if attach.Code != http.StatusCreated {
		t.Fatalf("attach status=%d body=%s", attach.Code, attach.Body)
	}

	var after orgapi.TriggerDTO
	decode(t, do(t, h, http.MethodGet, "/triggers/"+tr.ID, nil), &after)
	if len(after.AttachedWorkers) != 1 || after.AttachedWorkers[0] != "w-one" {
		t.Fatalf("attached_workers = %v, want [w-one]", after.AttachedWorkers)
	}

	var listed orgapi.TriggerListResponse
	decode(t, do(t, h, http.MethodGet, "/triggers", nil), &listed)
	if len(listed.Triggers) != 1 || len(listed.Triggers[0].AttachedWorkers) != 1 {
		t.Fatalf("list attached_workers = %+v", listed.Triggers)
	}
}

// TestTriggerDTOPublicURLOnlyForProviderTriggers: the loopback warning on
// the detail page needs the effective public URL, but only a Trigger whose
// webhook payload URL must be internet-reachable has one.
func TestTriggerDTOPublicURLOnlyForProviderTriggers(t *testing.T) {
	n := 0
	deps, _, _ := newDepsClock(t,
		func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
		func() string { n++; return fmt.Sprintf("id-%d", n) },
	)
	deps.PublicServerURL = "https://helix.example.com"
	h := orgapi.Handler(deps)

	var local orgapi.TriggerDTO
	decode(t, do(t, h, http.MethodPost, "/triggers", map[string]any{"name": "Local", "kind": "local"}), &local)
	if local.EffectivePublicURL != "" {
		t.Fatalf("local effective_public_url = %q, want empty", local.EffectivePublicURL)
	}

	var gh orgapi.TriggerDTO
	decode(t, do(t, h, http.MethodPost, "/triggers", map[string]any{
		"name": "GitHub", "kind": "github",
		"config": map[string]any{"repo": "helixml/helix", "events": []string{"issues"}},
	}), &gh)
	if gh.EffectivePublicURL != "https://helix.example.com" {
		t.Fatalf("github effective_public_url = %q", gh.EffectivePublicURL)
	}
}
