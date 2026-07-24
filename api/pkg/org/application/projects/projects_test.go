package projects

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// fakePort records the orgID it was called with and returns canned results.
type fakePort struct {
	lastOrg     string
	lastProject string
	views       []runtime.ProjectView
	view        runtime.ProjectView
	err         error
}

func (f *fakePort) List(_ context.Context, org string) ([]runtime.ProjectView, error) {
	f.lastOrg = org
	return f.views, f.err
}
func (f *fakePort) Get(_ context.Context, org, projectID string) (runtime.ProjectView, error) {
	f.lastOrg, f.lastProject = org, projectID
	return f.view, f.err
}

type fakeMembers struct {
	err        error
	projectIDs []string
}

func (m *fakeMembers) GetBot(_ context.Context, org string, id orgchart.BotID) (orgchart.Bot, error) {
	if m.err != nil {
		return orgchart.Bot{}, m.err
	}
	return orgchart.Bot{ID: id, OrganizationID: org, ProjectIDs: m.projectIDs}, nil
}

type fakeAccess struct {
	projectID string
	err       error
}

func (a fakeAccess) OwnProjectID(_ context.Context, _ string, _ orgchart.BotID) (string, error) {
	return a.projectID, a.err
}

type fakeCaller struct{ id, org string }

func (c fakeCaller) ID() string             { return c.id }
func (c fakeCaller) OrganizationID() string { return c.org }

func TestList_ScopesToCallerOrg(t *testing.T) {
	t.Parallel()
	fp := &fakePort{views: []runtime.ProjectView{{ID: "prj_1"}}}
	svc := New(fp, nil)
	views, err := svc.List(context.Background(), fakeCaller{id: "w-pm", org: "org-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if fp.lastOrg != "org-1" {
		t.Errorf("org passed = %q, want org-1", fp.lastOrg)
	}
	if len(views) != 1 || views[0].ID != "prj_1" {
		t.Errorf("views = %+v", views)
	}
}

func TestList_FiltersToOwnAndExplicitProjects(t *testing.T) {
	t.Parallel()
	fp := &fakePort{views: []runtime.ProjectView{
		{ID: "prj_own"},
		{ID: "prj_allowed"},
		{ID: "prj_denied"},
	}}
	svc := New(fp, &fakeMembers{projectIDs: []string{"prj_allowed"}}, fakeAccess{projectID: "prj_own"})
	views, err := svc.List(context.Background(), fakeCaller{id: "w-pm", org: "org-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(views) != 2 || views[0].ID != "prj_own" || views[1].ID != "prj_allowed" {
		t.Fatalf("views = %+v, want own and explicitly allowed projects", views)
	}
}

func TestGet_RejectsProjectOutsideAccess(t *testing.T) {
	t.Parallel()
	fp := &fakePort{view: runtime.ProjectView{ID: "prj_denied"}}
	svc := New(fp, &fakeMembers{}, fakeAccess{projectID: "prj_own"})
	if _, err := svc.Get(context.Background(), fakeCaller{id: "w-pm", org: "org-1"}, "prj_denied"); err == nil {
		t.Fatal("expected project access rejection")
	}
	if fp.lastProject != "" {
		t.Fatalf("port called for denied project %q", fp.lastProject)
	}
}

func TestGet_ForwardsProjectID(t *testing.T) {
	t.Parallel()
	fp := &fakePort{view: runtime.ProjectView{ID: "prj_9"}}
	svc := New(fp, nil)
	if _, err := svc.Get(context.Background(), fakeCaller{id: "w-pm", org: "org-1"}, "prj_9"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fp.lastOrg != "org-1" || fp.lastProject != "prj_9" {
		t.Errorf("passed (%q,%q), want (org-1, prj_9)", fp.lastOrg, fp.lastProject)
	}
}

func TestRejectsCallerWithoutOrg(t *testing.T) {
	t.Parallel()
	svc := New(&fakePort{}, nil)
	if _, err := svc.List(context.Background(), fakeCaller{id: "w-pm", org: ""}); err == nil {
		t.Error("expected error when caller has no org")
	}
}

func TestRejectsNonMemberBot(t *testing.T) {
	t.Parallel()
	fp := &fakePort{}
	svc := New(fp, &fakeMembers{err: errors.New("not found")})
	if _, err := svc.List(context.Background(), fakeCaller{id: "w-intruder", org: "org-1"}); err == nil {
		t.Error("expected error when caller bot is not a member of the org")
	}
	if fp.lastOrg != "" {
		t.Errorf("port should not be called when membership fails, got org=%q", fp.lastOrg)
	}
}
