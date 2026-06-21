package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// newCreateRoleCaller sets up the minimal env create_role needs:
// in-memory store, a deterministic clock, a deterministic ID generator,
// and a caller Worker whose OrganizationID create_role reads.
func newCreateRoleCaller(t *testing.T, orgID string) (Config, orgchart.Worker) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	deps := DefaultDeps(st)
	deps.Now = func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
	var counter int
	deps.NewID = func() string {
		counter++
		return "id-create-role-test"
	}
	caller, err := orgchart.NewHumanWorker("w-owner", "r-owner", "", orgID)
	if err != nil {
		t.Fatalf("new caller: %v", err)
	}
	return deps, caller
}

// invokeCreateRole runs the tool and reads back the created Role from
// the store so tests can assert on Role.Tools directly.
func invokeCreateRole(t *testing.T, deps Config, caller orgchart.Worker, args string) orgchart.Role {
	t.Helper()
	ctx := context.Background()
	out, err := (&CreateRole{deps: deps.Build()}).Invoke(ctx, tool.Invocation{
		Caller: caller,
		Args:   json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("create_role invoke: %v", err)
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	role, err := deps.Store.Roles.Get(ctx, caller.OrganizationID(), orgchart.RoleID(resp.ID))
	if err != nil {
		t.Fatalf("get back role: %v", err)
	}
	return role
}

// TestCreateRoleEmptyToolsStaysEmpty verifies that omitting `tools` from
// create_role produces a Role with an empty tool list. Operators choose tools
// explicitly via the role editor.
func TestCreateRoleEmptyToolsStaysEmpty(t *testing.T) {
	t.Parallel()
	deps, caller := newCreateRoleCaller(t, "org-test")
	role := invokeCreateRole(t, deps, caller, `{"id":"r-empty","content":"# Empty role"}`)
	if len(role.Tools) != 0 {
		t.Fatalf("expected empty tools, got %v", role.Tools)
	}
}

// TestCreateRoleCallerToolsPreserved verifies that caller-supplied tools are
// stored exactly as supplied — no baseline is merged, no dedup.
func TestCreateRoleCallerToolsPreserved(t *testing.T) {
	t.Parallel()
	deps, caller := newCreateRoleCaller(t, "org-test")
	role := invokeCreateRole(t, deps, caller,
		`{"id":"r-qa","content":"# QA","tools":["publish","managers","subscribe"]}`)
	want := []tool.Name{PublishName, ManagersName, SubscribeName}
	if len(role.Tools) != len(want) {
		t.Fatalf("tools = %v, want %v", role.Tools, want)
	}
	for i, n := range want {
		if role.Tools[i] != n {
			t.Fatalf("tools[%d] = %q, want %q", i, role.Tools[i], n)
		}
	}
}
