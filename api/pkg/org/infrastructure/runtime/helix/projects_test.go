package helix

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeProjectStore is an in-memory ProjectStore for the projects impl tests.
type fakeProjectStore struct {
	projects map[string]*types.Project
}

func newFakeProjectStore() *fakeProjectStore {
	return &fakeProjectStore{projects: map[string]*types.Project{}}
}

func (f *fakeProjectStore) ListProjects(_ context.Context, q *store.ListProjectsQuery) ([]*types.Project, error) {
	var out []*types.Project
	for _, p := range f.projects {
		if q.OrganizationID != "" && p.OrganizationID != q.OrganizationID {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeProjectStore) GetProject(_ context.Context, id string) (*types.Project, error) {
	p, ok := f.projects[id]
	if !ok {
		return nil, errors.New("project not found")
	}
	return p, nil
}

func TestProjects_RejectsNilStore(t *testing.T) {
	t.Parallel()
	if _, err := NewProjects(nil); err == nil {
		t.Error("expected error on nil store")
	}
}

func TestProjects_ListScopesToOrg(t *testing.T) {
	t.Parallel()
	fs := newFakeProjectStore()
	fs.projects["prj_a"] = &types.Project{ID: "prj_a", OrganizationID: "org-1", Name: "A"}
	fs.projects["prj_b"] = &types.Project{ID: "prj_b", OrganizationID: "org-2", Name: "B"}
	p, err := NewProjects(fs)
	if err != nil {
		t.Fatalf("NewProjects: %v", err)
	}
	views, err := p.List(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(views) != 1 || views[0].ID != "prj_a" {
		t.Errorf("views = %+v, want only prj_a", views)
	}
}

func TestProjects_GetSameOrg(t *testing.T) {
	t.Parallel()
	fs := newFakeProjectStore()
	fs.projects["prj_a"] = &types.Project{ID: "prj_a", OrganizationID: "org-1", Name: "A", Status: "active"}
	p, _ := NewProjects(fs)
	view, err := p.Get(context.Background(), "org-1", "prj_a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if view.ID != "prj_a" || view.Status != "active" {
		t.Errorf("view = %+v", view)
	}
}

func TestProjects_GetCrossOrgRejected(t *testing.T) {
	t.Parallel()
	fs := newFakeProjectStore()
	fs.projects["prj_a"] = &types.Project{ID: "prj_a", OrganizationID: "org-2"}
	p, _ := NewProjects(fs)
	if _, err := p.Get(context.Background(), "org-1", "prj_a"); err == nil {
		t.Error("expected cross-org rejection")
	}
}
