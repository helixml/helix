package api_test

import (
	"net/http"
	"testing"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// TestRESTCreateRole_EmptyToolsStaysEmpty verifies that POST /roles with no
// tools list produces a Role with an empty tool set. Operators choose tools
// explicitly via the role detail editor.
func TestRESTCreateRole_EmptyToolsStaysEmpty(t *testing.T) {
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

	if len(out.Tools) != 0 {
		t.Errorf("expected empty tools on new role, got: %v", out.Tools)
	}
}

// TestRESTCreateRole_CallerToolsPreserved pins that caller-supplied tools are
// stored verbatim — no baseline is merged, no dedup beyond what the caller
// sends.
func TestRESTCreateRole_CallerToolsPreserved(t *testing.T) {
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

	if len(out.Tools) != 3 || out.Tools[0] != "publish" || out.Tools[1] != "managers" || out.Tools[2] != "subscribe" {
		t.Errorf("caller tools not stored as-is: %v", out.Tools)
	}
}
