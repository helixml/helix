package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// blockingInstances holds the delete cascade open until release is closed,
// standing in for a slow sandbox teardown.
type blockingInstances struct {
	started chan struct{}
	release chan struct{}
}

func (b blockingInstances) DeleteAll(ctx context.Context, _ string, _ orgchart.NodeID) error {
	close(b.started)
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestDeleteBot_ReturnsBeforeTeardownFinishes pins that DELETE /bots/{id}
// does not block on the teardown, and that while it runs the bot is hidden
// from reads, refuses edits, activation and chat, and a repeated delete is
// accepted.
func TestDeleteBot_ReturnsBeforeTeardownFinishes(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-mgr", "# mgr")
	seedBot(t, st, ctx, "b-ic", "# ic")
	if err := st.ReportingLines.Add(ctx, orgchart.ReportingLine{OrgID: "org-test", ManagerID: "b-mgr", ReportID: "b-ic"}); err != nil {
		t.Fatalf("add reporting line: %v", err)
	}
	inst := blockingInstances{started: make(chan struct{}), release: make(chan struct{})}
	deps.Lifecycle.Instances = inst
	ensurer := &fakeEnsurer{projectID: "prj-x", agentApp: "app-x"}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, ensurer, disp)
	deps.ProjectEnsurer = ensurer
	h := orgapi.Handler(deps)

	if rec := do(t, h, "DELETE", "/bots/b-mgr", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("delete: got %d, want 202; body=%s", rec.Code, rec.Body)
	}
	select {
	case <-inst.started:
	case <-time.After(5 * time.Second):
		t.Fatal("background teardown never started")
	}

	if rec := do(t, h, "GET", "/bots/b-mgr", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get deleting bot: got %d, want 404", rec.Code)
	}
	rec := do(t, h, "GET", "/bots", nil)
	var bots []orgapi.BotDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &bots); err != nil {
		t.Fatalf("decode bots: %v; body=%s", err, rec.Body)
	}
	if len(bots) != 1 || bots[0].ID != "b-ic" {
		t.Fatalf("list while deleting: got %+v, want only b-ic", bots)
	}
	if len(bots[0].ParentIDs) != 0 {
		t.Fatalf("b-ic still lists deleting manager: %v", bots[0].ParentIDs)
	}
	name := "renamed"
	if rec := do(t, h, "PATCH", "/bots/b-mgr", orgapi.UpdateBotRequest{Name: &name}); rec.Code != http.StatusConflict {
		t.Fatalf("patch deleting bot: got %d, want 409; body=%s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "DELETE", "/bots/b-mgr", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("repeat delete: got %d, want 202", rec.Code)
	}
	// Nothing may re-provision the bot the cascade is tearing down.
	for _, path := range []string{"/bots/b-mgr/activate", "/bots/b-mgr/chat"} {
		if rec := do(t, h, "POST", path, nil); rec.Code != http.StatusConflict {
			t.Fatalf("POST %s while deleting: got %d, want 409; body=%s", path, rec.Code, rec.Body)
		}
	}
	ensurer.mu.Lock()
	ensureCalls := ensurer.calls
	ensurer.mu.Unlock()
	disp.mu.Lock()
	dispatched := disp.manualCalls
	disp.mu.Unlock()
	if ensureCalls != 0 || dispatched != 0 {
		t.Fatalf("deleting bot was provisioned: ensure=%d dispatch=%d", ensureCalls, dispatched)
	}

	close(inst.release)
	deadline := time.Now().Add(5 * time.Second)
	for deps.Lifecycle.Deleting("org-test", "b-mgr") {
		if time.Now().After(deadline) {
			t.Fatal("background delete did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := st.Nodes.Get(ctx, "org-test", "b-mgr"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bot row after delete: err=%v, want ErrNotFound", err)
	}
}

func TestDeleteBot_MissingBotIs404(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)
	if rec := do(t, h, "DELETE", "/bots/b-nope", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing: got %d, want 404; body=%s", rec.Code, rec.Body)
	}
}
