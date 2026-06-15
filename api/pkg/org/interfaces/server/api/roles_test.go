package api_test

import (
	"net/http"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/tools"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// TestRESTCreateRole_EmptyToolsGetsBaseline pins the bug fix discovered
// during the in-browser demo of helixml/helix#2546: the chart UI's
// "New Role" dialog only collects ID + content (no tools picker) and
// posts to POST /roles with an empty tools list. Before the fix the
// resulting Role had no tools, so its Workers had no MCP surface at
// all. Now the REST handler unions BaseReadTools the same way the MCP
// create_role tool does.
func TestRESTCreateRole_EmptyToolsGetsBaseline(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/roles", orgapi.CreateRoleRequest{
		ID:      "r-qa-engineer",
		Content: "# QA Engineer",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rec.Code, rec.Body)
	}
	var out orgapi.RoleDTO
	decode(t, rec, &out)

	want := make(map[string]bool, len(tools.BaseReadTools))
	for _, name := range tools.BaseReadTools {
		want[name] = true
	}
	got := make(map[string]bool, len(out.Tools))
	for _, name := range out.Tools {
		got[name] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("baseline tool %q missing from REST-created role; got: %v", name, out.Tools)
		}
	}
}

// TestRESTCreateRole_UnionWithCallerTools pins the union semantics for
// the REST path — caller-supplied tools are preserved alongside the
// baseline.
func TestRESTCreateRole_UnionWithCallerTools(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/roles", orgapi.CreateRoleRequest{
		ID:      "r-mixed",
		Content: "# Mixed",
		Tools:   []string{"publish", "managers", "subscribe"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rec.Code, rec.Body)
	}
	var out orgapi.RoleDTO
	decode(t, rec, &out)

	// Caller's order preserved, dedup on `managers` (also in baseline).
	if len(out.Tools) < 3 || out.Tools[0] != "publish" || out.Tools[1] != "managers" || out.Tools[2] != "subscribe" {
		t.Errorf("caller tools not preserved at head of list: %v", out.Tools)
	}
	// Count managers exactly once.
	var managersCount int
	for _, n := range out.Tools {
		if n == "managers" {
			managersCount++
		}
	}
	if managersCount != 1 {
		t.Errorf("managers should appear exactly once after dedup; got %d in %v", managersCount, out.Tools)
	}
	// Every baseline name present.
	got := make(map[string]bool, len(out.Tools))
	for _, name := range out.Tools {
		got[name] = true
	}
	for _, name := range tools.BaseReadTools {
		if !got[name] {
			t.Errorf("baseline tool %q missing from union; got: %v", name, out.Tools)
		}
	}
}
