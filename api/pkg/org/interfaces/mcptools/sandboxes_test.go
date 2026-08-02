package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	orgsandboxes "github.com/helixml/helix/api/pkg/org/application/sandboxes"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

type sandboxCaller struct{ id, orgID string }

func (c sandboxCaller) ID() string             { return c.id }
func (c sandboxCaller) OrganizationID() string { return c.orgID }

type fakeSandboxesPort struct {
	created runtime.CreateSandboxInput
	updated runtime.UpdateSandboxInput
	deleted string
}

func (f *fakeSandboxesPort) ListRuntimes(context.Context) (runtime.SandboxRuntimeCatalog, error) {
	return runtime.SandboxRuntimeCatalog{Runtimes: []string{"node22"}, DefaultRuntime: "node22"}, nil
}
func (f *fakeSandboxesPort) List(context.Context, string, string) ([]runtime.SandboxView, error) {
	return []runtime.SandboxView{{ID: "sbx_1", OrganizationID: "org-a", Owner: "user-a", Runtime: "node22", Status: "running"}}, nil
}
func (f *fakeSandboxesPort) Get(context.Context, string, string) (runtime.SandboxView, error) {
	return runtime.SandboxView{ID: "sbx_1", OrganizationID: "org-a", Owner: "user-a", Runtime: "node22", Status: "running"}, nil
}
func (f *fakeSandboxesPort) Create(_ context.Context, orgID string, _ orgchart.NodeID, in runtime.CreateSandboxInput) (runtime.SandboxView, error) {
	f.created = in
	return runtime.SandboxView{ID: "sbx_new", OrganizationID: orgID, Owner: "user-a", Runtime: in.Runtime, Status: "pending"}, nil
}
func (f *fakeSandboxesPort) Update(_ context.Context, orgID, sandboxID string, in runtime.UpdateSandboxInput) (runtime.SandboxView, error) {
	f.updated = in
	return runtime.SandboxView{ID: sandboxID, OrganizationID: orgID, Owner: "user-a", Runtime: "node22", Status: "running", Name: *in.Name}, nil
}
func (f *fakeSandboxesPort) Delete(_ context.Context, _ string, sandboxID string) error {
	f.deleted = sandboxID
	return nil
}

func TestSandboxToolsCRUD(t *testing.T) {
	t.Parallel()
	port := &fakeSandboxesPort{}
	deps := Deps{Sandboxes: orgsandboxes.New(port, nil)}
	caller := sandboxCaller{id: "chief-of-staff", orgID: "org-a"}

	created, err := NewCreateSandbox(deps).Invoke(context.Background(), tool.Invocation{
		Caller: caller,
		Args:   json.RawMessage(`{"name":"ops","runtime":"node22","env":{"TOKEN":"secret"},"tags":{"team":"ops"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if port.created.Name != "ops" || port.created.Env["TOKEN"] != "secret" {
		t.Fatalf("create input = %+v", port.created)
	}
	if string(created) == "" || containsJSONValue(created, "secret") {
		t.Fatalf("create result leaked environment value: %s", created)
	}

	name := "renamed"
	updated, err := NewUpdateSandbox(deps).Invoke(context.Background(), tool.Invocation{
		Caller: caller,
		Args:   json.RawMessage(`{"sandbox_id":"sbx_new","name":"renamed"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if port.updated.Name == nil || *port.updated.Name != name || len(updated) == 0 {
		t.Fatalf("update input = %+v result = %s", port.updated, updated)
	}
	zero := 0
	zeroArgs, err := json.Marshal(updateSandboxArgs{SandboxID: "sbx_new", TimeoutSeconds: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewUpdateSandbox(deps).Invoke(context.Background(), tool.Invocation{Caller: caller, Args: zeroArgs}); err == nil {
		t.Fatal("zero timeout update succeeded")
	}

	deleted, err := NewDeleteSandbox(deps).Invoke(context.Background(), tool.Invocation{
		Caller: caller,
		Args:   json.RawMessage(`{"sandbox_id":"sbx_new"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if port.deleted != "sbx_new" || string(deleted) != `{"deleted_sandbox_id":"sbx_new"}` {
		t.Fatalf("delete = %q, result = %s", port.deleted, deleted)
	}
}

func containsJSONValue(raw json.RawMessage, value string) bool {
	var decoded struct {
		Env map[string]string `json:"env"`
	}
	if json.Unmarshal(raw, &decoded) != nil {
		return true
	}
	for _, got := range decoded.Env {
		if got == value {
			return true
		}
	}
	return false
}

func TestSandboxToolsRegisteredAndGrantedToChiefOfStaff(t *testing.T) {
	t.Parallel()
	names := []tool.Name{
		ListSandboxRuntimesName,
		ListSandboxesName,
		GetSandboxName,
		CreateSandboxName,
		UpdateSandboxName,
		DeleteSandboxName,
	}

	store := orggorm.GetOrgTestDB(t)
	registry := NewRegistry()
	config := DefaultDeps(store)
	injectTestPublishing(&config)
	if err := RegisterBuiltins(registry, config.Build()); err != nil {
		t.Fatal(err)
	}
	owner := make(map[tool.Name]bool)
	for _, name := range OwnerBotTools() {
		owner[name] = true
	}
	base := make(map[tool.Name]bool)
	for _, name := range BaseReadTools {
		base[name] = true
	}
	for _, name := range names {
		if _, err := registry.Get(name); err != nil {
			t.Errorf("tool %q not registered: %v", name, err)
		}
		if !owner[name] {
			t.Errorf("OwnerBotTools missing %q", name)
		}
		if base[name] {
			t.Errorf("sandbox tool %q must not be universal", name)
		}
	}
}
