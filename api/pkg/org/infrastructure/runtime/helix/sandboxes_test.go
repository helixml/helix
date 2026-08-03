package helix

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/hydra"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/datatypes"
)

type fakeSandboxController struct {
	sandboxes map[string]*types.Sandbox
	created   *types.CreateSandboxRequest
	createOrg string
	owner     string
	updated   *types.UpdateSandboxRequest
	deleted   string
}

func (f *fakeSandboxController) RuntimeNames() []string     { return []string{"node22", "ubuntu-desktop"} }
func (f *fakeSandboxController) DefaultRuntimeName() string { return "node22" }
func (f *fakeSandboxController) List(_ context.Context, orgID, projectID string) ([]*types.Sandbox, error) {
	var out []*types.Sandbox
	for _, value := range f.sandboxes {
		if value.OrganizationID == orgID && (projectID == "" || value.ProjectID == projectID) {
			out = append(out, value)
		}
	}
	return out, nil
}
func (f *fakeSandboxController) Get(_ context.Context, id string) (*types.Sandbox, error) {
	value := f.sandboxes[id]
	if value == nil {
		return nil, errors.New("not found")
	}
	return value, nil
}
func (f *fakeSandboxController) Create(_ context.Context, orgID, owner string, req *types.CreateSandboxRequest) (*types.Sandbox, error) {
	f.createOrg, f.owner, f.created = orgID, owner, req
	value := &types.Sandbox{
		ID: "sbx_new", OrganizationID: orgID, Owner: owner, Name: req.Name,
		Runtime: types.SandboxRuntime(req.Runtime), Status: types.SandboxStatusPending,
		Env: datatypes.JSON([]byte(`{"TOKEN":"secret"}`)), Tags: datatypes.JSON([]byte(`{"team":"ops"}`)),
	}
	f.sandboxes[value.ID] = value
	return value, nil
}
func (f *fakeSandboxController) Update(_ context.Context, id string, req *types.UpdateSandboxRequest) (*types.Sandbox, error) {
	f.updated = req
	value := f.sandboxes[id]
	if value == nil {
		return nil, errors.New("not found")
	}
	if req.Name != nil {
		value.Name = *req.Name
	}
	return value, nil
}
func (f *fakeSandboxController) Delete(_ context.Context, id string) error {
	f.deleted = id
	delete(f.sandboxes, id)
	return nil
}
func (f *fakeSandboxController) HydraClient(*types.Sandbox) (*hydra.RevDialClient, error) {
	return nil, errors.New("terminal not configured in CRUD test")
}

type fakeSandboxProjects map[string]*types.Project

func (f fakeSandboxProjects) GetProject(_ context.Context, id string) (*types.Project, error) {
	project := f[id]
	if project == nil {
		return nil, errors.New("not found")
	}
	return project, nil
}

func TestSandboxesCRUDUsesAuthenticatedOwnerAndOrgScope(t *testing.T) {
	t.Parallel()
	orgStore := orggorm.GetOrgTestDB(t)
	control := &fakeSandboxController{sandboxes: map[string]*types.Sandbox{
		"sbx_owned": {ID: "sbx_owned", OrganizationID: "org-a", Owner: "user-a", Runtime: "node22", Status: types.SandboxStatusRunning},
		"sbx_other": {ID: "sbx_other", OrganizationID: "org-b", Owner: "user-b", Runtime: "node22", Status: types.SandboxStatusRunning},
	}}
	port, err := NewSandboxes(orgStore, control, fakeSandboxProjects{
		"prj-a": {ID: "prj-a", OrganizationID: "org-a"},
		"prj-b": {ID: "prj-b", OrganizationID: "org-b"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := WithUserID(context.Background(), "user-authenticated")
	created, err := port.Create(ctx, "org-a", "chief-of-staff", runtime.CreateSandboxInput{
		Name: "ops", Runtime: "node22", ProjectID: "prj-a", Env: map[string]string{"TOKEN": "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if control.createOrg != "org-a" || control.owner != "user-authenticated" {
		t.Fatalf("create attribution = org %q owner %q", control.createOrg, control.owner)
	}
	if created.Tags["team"] != "ops" {
		t.Fatalf("created tags = %+v", created.Tags)
	}

	name := "renamed"
	updated, err := port.Update(ctx, "org-a", created.ID, runtime.UpdateSandboxInput{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || control.updated == nil {
		t.Fatalf("updated = %+v", updated)
	}
	if err := port.Delete(ctx, "org-a", created.ID); err != nil {
		t.Fatal(err)
	}
	if control.deleted != created.ID {
		t.Fatalf("deleted = %q, want %q", control.deleted, created.ID)
	}

	if _, err := port.Get(ctx, "org-a", "sbx_other"); err == nil {
		t.Fatal("cross-org sandbox get succeeded")
	}
	if _, err := port.Create(ctx, "org-a", "chief-of-staff", runtime.CreateSandboxInput{ProjectID: "prj-b"}); err == nil {
		t.Fatal("cross-org project association succeeded")
	}
}

func TestSandboxesRuntimeCatalogSorted(t *testing.T) {
	t.Parallel()
	port, err := NewSandboxes(orggorm.GetOrgTestDB(t), &fakeSandboxController{sandboxes: map[string]*types.Sandbox{}}, fakeSandboxProjects{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := port.ListRuntimes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Runtimes) != 2 || catalog.Runtimes[0] != "node22" || catalog.DefaultRuntime != "node22" {
		t.Fatalf("catalog = %+v", catalog)
	}
}
