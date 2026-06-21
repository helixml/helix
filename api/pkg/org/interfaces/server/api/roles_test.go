package api_test

import (
	"net/http"
	"testing"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// TestRESTCreateRole_EmptyToolsStaysEmpty pins that POST /roles with no
// tools creates a Role with an empty tool list. Operators add tools
// explicitly via the role detail page rather than receiving an implicit
// baseline injection.
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
		t.Errorf("new role should have empty tools list, got: %v", out.Tools)
	}
}

// TestRESTCreateRole_CallerToolsPreserved pins that caller-supplied tools
// are stored verbatim with no baseline injection.
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

	// Exactly the caller's three tools, in order, no extras.
	if len(out.Tools) != 3 || out.Tools[0] != "publish" || out.Tools[1] != "managers" || out.Tools[2] != "subscribe" {
		t.Errorf("caller tools not preserved verbatim: %v", out.Tools)
	}
}
