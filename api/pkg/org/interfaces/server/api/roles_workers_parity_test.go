package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/tools"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// mcpRegistry builds a tools registry over a fresh store with the given
// deterministic clock/id, for driving MCP tools in parity tests.
func mcpRegistry(t *testing.T, st *store.Store, clock func() time.Time, newID func() string) *tools.Registry {
	t.Helper()
	deps := tools.DefaultDeps(st)
	deps.Now = clock
	deps.NewID = newID
	reg := tools.NewRegistry()
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	return reg
}

func ownerCaller(t *testing.T) tool.Worker {
	t.Helper()
	c, err := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	if err != nil {
		t.Fatalf("new caller: %v", err)
	}
	return c
}

// TestCreateRoleParity_RESTvsMCP: the REST POST /roles handler and the
// MCP create_role tool share application/roles, so both must produce
// identical role rows — same content, same baseline-unioned tools, same
// streams.
func TestCreateRoleParity_RESTvsMCP(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	newID := func() string { return "fixed" }

	restDeps, restStore, _ := newDeps(t)
	restDeps.Now = clock
	restDeps.NewID = newID
	h := orgapi.Handler(restDeps)
	rec := do(t, h, "POST", "/roles", orgapi.CreateRoleRequest{
		ID:      "r-qa",
		Content: "# QA",
		Tools:   []string{"publish", "subscribe"},
		Streams: []string{"s-a"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("REST create role: %d body=%s", rec.Code, rec.Body)
	}

	mcpStore := orggorm.GetOrgTestDB(t)
	reg := mcpRegistry(t, mcpStore, clock, newID)
	createRole, _ := reg.Get(tools.CreateRoleName)
	args, _ := json.Marshal(map[string]any{
		"id":      "r-qa",
		"content": "# QA",
		"tools":   []string{"publish", "subscribe"},
		"streams": []string{"s-a"},
	})
	if _, err := createRole.Invoke(context.Background(), tool.Invocation{Caller: ownerCaller(t), Args: args}); err != nil {
		t.Fatalf("MCP create_role: %v", err)
	}

	restRole, err := restStore.Roles.Get(context.Background(), "org-test", "r-qa")
	if err != nil {
		t.Fatalf("REST role get: %v", err)
	}
	mcpRole, err := mcpStore.Roles.Get(context.Background(), "org-test", "r-qa")
	if err != nil {
		t.Fatalf("MCP role get: %v", err)
	}
	if restRole.Content != mcpRole.Content {
		t.Errorf("Content differs: REST=%q MCP=%q", restRole.Content, mcpRole.Content)
	}
	if !sameNames(restRole.Tools, mcpRole.Tools) {
		t.Errorf("Tools differ: REST=%v MCP=%v", restRole.Tools, mcpRole.Tools)
	}
	if len(restRole.Streams) != len(mcpRole.Streams) || (len(restRole.Streams) > 0 && restRole.Streams[0] != mcpRole.Streams[0]) {
		t.Errorf("Streams differ: REST=%v MCP=%v", restRole.Streams, mcpRole.Streams)
	}
}

// TestUpdateIdentityParity_RESTvsMCP: REST POST /workers/{id}/identity
// and MCP update_identity share application/workers — both leave the
// worker's IdentityContent in the same state.
func TestUpdateIdentityParity_RESTvsMCP(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	newID := func() string { return "fixed" }
	ctx := context.Background()

	seed := func(st *store.Store) {
		role, _ := orgchart.NewRole("r-eng", "# Eng", []tool.Name{"publish"}, nil, clock(), "org-test")
		if err := st.Roles.Create(ctx, role); err != nil {
			t.Fatalf("seed role: %v", err)
		}
		wk, _ := orgchart.NewAIWorker("w-mark", "r-eng", "original", "org-test")
		if err := st.Workers.Create(ctx, wk); err != nil {
			t.Fatalf("seed worker: %v", err)
		}
	}

	restDeps, restStore, _ := newDeps(t)
	restDeps.Now = clock
	restDeps.NewID = newID
	seed(restStore)
	h := orgapi.Handler(restDeps)
	rec := do(t, h, "POST", "/workers/w-mark/identity", orgapi.UpdateWorkerIdentityRequest{Identity: "rewritten persona"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("REST update identity: %d body=%s", rec.Code, rec.Body)
	}

	mcpStore := orggorm.GetOrgTestDB(t)
	seed(mcpStore)
	reg := mcpRegistry(t, mcpStore, clock, newID)
	updateIdentity, _ := reg.Get(tools.UpdateIdentityName)
	args, _ := json.Marshal(map[string]any{"workerId": "w-mark", "content": "rewritten persona"})
	if _, err := updateIdentity.Invoke(ctx, tool.Invocation{Caller: ownerCaller(t), Args: args}); err != nil {
		t.Fatalf("MCP update_identity: %v", err)
	}

	restW, _ := restStore.Workers.Get(ctx, "org-test", "w-mark")
	mcpW, _ := mcpStore.Workers.Get(ctx, "org-test", "w-mark")
	if restW.IdentityContent() != mcpW.IdentityContent() {
		t.Errorf("identity differs: REST=%q MCP=%q", restW.IdentityContent(), mcpW.IdentityContent())
	}
	if restW.IdentityContent() != "rewritten persona" {
		t.Errorf("identity not applied: %q", restW.IdentityContent())
	}
}

func sameNames(a, b []tool.Name) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
