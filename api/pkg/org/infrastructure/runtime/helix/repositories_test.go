package helix

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/types"
)

type fakeRepoStore struct {
	repos    map[string]*types.GitRepository
	projects map[string]*types.Project
	// projectID → set of repoIDs
	attached map[string]map[string]bool

	listOrgCalls int
}

func newFakeRepoStore() *fakeRepoStore {
	return &fakeRepoStore{
		repos:    map[string]*types.GitRepository{},
		projects: map[string]*types.Project{},
		attached: map[string]map[string]bool{},
	}
}

func (f *fakeRepoStore) ListGitRepositories(_ context.Context, req *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error) {
	f.listOrgCalls++
	var out []*types.GitRepository
	if req.ProjectID != "" {
		for repoID := range f.attached[req.ProjectID] {
			if r := f.repos[repoID]; r != nil {
				if req.OrganizationID == "" || r.OrganizationID == req.OrganizationID || r.OrganizationID == "" {
					out = append(out, r)
				}
			}
		}
		return out, nil
	}
	for _, r := range f.repos {
		if req.OrganizationID != "" && r.OrganizationID != req.OrganizationID {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeRepoStore) GetGitRepository(_ context.Context, id string) (*types.GitRepository, error) {
	r, ok := f.repos[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return r, nil
}

func (f *fakeRepoStore) GetProject(_ context.Context, id string) (*types.Project, error) {
	p, ok := f.projects[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return p, nil
}

func (f *fakeRepoStore) AttachRepositoryToProject(_ context.Context, projectID, repoID string) error {
	if f.attached[projectID] == nil {
		f.attached[projectID] = map[string]bool{}
	}
	f.attached[projectID][repoID] = true
	return nil
}

func (f *fakeRepoStore) DetachRepositoryFromProject(_ context.Context, projectID, repoID string) error {
	delete(f.attached[projectID], repoID)
	return nil
}

func (f *fakeRepoStore) SetProjectPrimaryRepository(_ context.Context, projectID, repoID string) error {
	p, ok := f.projects[projectID]
	if !ok {
		return errors.New("project not found")
	}
	p.DefaultRepoID = repoID
	return nil
}

func TestRepositories_ListOrgScoped(t *testing.T) {
	t.Parallel()
	orgStore := orggorm.GetOrgTestDB(t)
	fake := newFakeRepoStore()
	fake.repos["r1"] = &types.GitRepository{ID: "r1", Name: "alpha", OrganizationID: "org-a"}
	fake.repos["r2"] = &types.GitRepository{ID: "r2", Name: "beta", OrganizationID: "org-b"}

	port, err := NewRepositories(orgStore, fake)
	if err != nil {
		t.Fatal(err)
	}
	got, err := port.List(context.Background(), "org-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "r1" {
		t.Fatalf("got %+v, want only r1", got)
	}
}

func TestRepositories_AttachDetachForBot(t *testing.T) {
	t.Parallel()
	orgStore := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	orgID := "org-test"
	botID := orgchart.BotID("b-coder")

	// Seed bot + runtime state with a project.
	if err := orgStore.Bots.Create(ctx, mustBot(t, botID, orgID)); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, orgStore, orgID, botID, "prj_1", "app_1", "repo_seed"); err != nil {
		t.Fatal(err)
	}

	fake := newFakeRepoStore()
	fake.projects["prj_1"] = &types.Project{ID: "prj_1", OrganizationID: orgID, DefaultRepoID: "repo_seed"}
	fake.repos["repo_seed"] = &types.GitRepository{ID: "repo_seed", Name: "seed", OrganizationID: orgID}
	fake.repos["repo_work"] = &types.GitRepository{ID: "repo_work", Name: "work", OrganizationID: orgID}
	fake.attached["prj_1"] = map[string]bool{"repo_seed": true}

	port, err := NewRepositories(orgStore, fake)
	if err != nil {
		t.Fatal(err)
	}

	// Attach work as primary.
	afterAttach, err := port.AttachToBot(ctx, orgID, botID, "repo_work", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterAttach) != 2 {
		t.Fatalf("after attach: got %d repos, want 2", len(afterAttach))
	}
	var workPrimary bool
	for _, v := range afterAttach {
		if v.ID == "repo_work" && v.Primary {
			workPrimary = true
		}
	}
	if !workPrimary {
		t.Fatalf("repo_work should be primary after attach: %+v", afterAttach)
	}
	if fake.projects["prj_1"].DefaultRepoID != "repo_work" {
		t.Fatalf("default_repo_id = %q, want repo_work", fake.projects["prj_1"].DefaultRepoID)
	}

	// Detach primary → remaining list without work, default cleared.
	afterDetach, err := port.DetachFromBot(ctx, orgID, botID, "repo_work")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range afterDetach {
		if v.ID == "repo_work" {
			t.Fatalf("repo_work still listed after detach: %+v", afterDetach)
		}
	}
	if fake.projects["prj_1"].DefaultRepoID != "" {
		t.Fatalf("default_repo_id should be cleared, got %q", fake.projects["prj_1"].DefaultRepoID)
	}
}

func TestRepositories_NoProjectYet(t *testing.T) {
	t.Parallel()
	orgStore := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	orgID := "org-test"
	botID := orgchart.BotID("b-new")
	if err := orgStore.Bots.Create(ctx, mustBot(t, botID, orgID)); err != nil {
		t.Fatal(err)
	}
	fake := newFakeRepoStore()
	port, err := NewRepositories(orgStore, fake)
	if err != nil {
		t.Fatal(err)
	}
	_, err = port.ListForBot(ctx, orgID, botID)
	if !errors.Is(err, runtime.ErrBotProjectNotReady) {
		t.Fatalf("err = %v, want ErrBotProjectNotReady", err)
	}
}

func mustBot(t *testing.T, id orgchart.BotID, orgID string) orgchart.Bot {
	t.Helper()
	b, err := orgchart.NewBot(id, "# bot", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
