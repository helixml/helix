package helix

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeProjectService is the test stand-in for ProjectService. Counters
// + last-request captures let tests assert exactly what was called.
type fakeProjectService struct {
	mu sync.Mutex

	applyCalls              int
	applyStarted            chan struct{}
	applyContinue           chan struct{}
	lastApplyReq            types.ProjectApplyRequest
	applyResponse           types.ProjectApplyResponse
	applyErr                error
	getProjectCalls         int
	getProjectResp          types.Project
	getProjectErr           error
	getProjectErrOnce       bool
	updateProjectCalls      int
	updateProjectPatchLast  types.ProjectUpdateRequest
	updateProjectErr        error
	putSecretCalls          int
	putSecretLast           map[string]string
	deletedSecrets          []string
	listSecretsProjectID    string
	listSecretsResp         map[string]string
	listSecretsErr          error
	createGitRepoCalls      int
	createGitRepoErr        error
	createGitRepoNameReturn string // when set, CreateGitRepo returns this name (simulates auto-increment on collision)
	getGitRepoCalls         int
	getGitRepoErr           error
	deleteGitRepoIDs        []string
	attachRepoErr           error
	attachRepoCalls         int
	getAppCalls             int
	getAppErr               error
	appConfig               types.AppConfig
	updateAppCalls          int
	updateAppLastCfg        types.AppConfig
	whoAmIResp              string
	deleteProjectIDs        []string
	deleteAppIDs            []string
}

func newFakeProjectService() *fakeProjectService {
	// Helix's auto-provisioned Agent App always has one assistant —
	// AttachMCP inserts the MCP entry on assistants[0]. Without this,
	// the attach helper aborts before UpdateAppConfig.
	return &fakeProjectService{
		applyResponse:  types.ProjectApplyResponse{ProjectID: "prj_test", AgentAppID: "app_test", Created: true},
		getProjectResp: types.Project{ID: "prj_test", DefaultRepoID: ""},
		whoAmIResp:     "u-test",
		putSecretLast:  map[string]string{},
		appConfig: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{Name: "main"}},
			},
		},
	}
}

func (f *fakeProjectService) WhoAmI(_ context.Context) (string, error) { return f.whoAmIResp, nil }

func (f *fakeProjectService) ApplyProject(_ context.Context, req types.ProjectApplyRequest) (types.ProjectApplyResponse, error) {
	f.mu.Lock()
	f.applyCalls++
	f.lastApplyReq = req
	resp, err := f.applyResponse, f.applyErr
	started, continued := f.applyStarted, f.applyContinue
	f.mu.Unlock()
	if started != nil {
		started <- struct{}{}
	}
	if continued != nil {
		<-continued
	}
	return resp, err
}

func (f *fakeProjectService) GetProject(_ context.Context, _ string) (types.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getProjectCalls++
	err := f.getProjectErr
	if f.getProjectErrOnce {
		f.getProjectErr = nil
	}
	return f.getProjectResp, err
}

func (f *fakeProjectService) GetApp(_ context.Context, id string) (*types.App, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getAppCalls++
	return &types.App{ID: id, OrganizationID: "org-test", Config: f.appConfig}, f.getAppErr
}

func (f *fakeProjectService) UpdateProject(_ context.Context, id string, patch types.ProjectUpdateRequest) (types.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateProjectCalls++
	f.updateProjectPatchLast = patch
	// Start from the seeded GetProject response so updates are
	// visible to subsequent GetProject calls. Mirrors the in-proc
	// adapter's "return the post-update project" contract.
	updated := f.getProjectResp
	if patch.StartupScript != nil {
		updated.StartupScript = *patch.StartupScript
	}
	if patch.Name != nil {
		updated.Name = *patch.Name
	}
	if patch.Metadata != nil {
		updated.Metadata = *patch.Metadata
	}
	if patch.DefaultHelixAppID != nil {
		updated.DefaultHelixAppID = *patch.DefaultHelixAppID
	}
	f.getProjectResp = updated
	return updated, f.updateProjectErr
}

func (f *fakeProjectService) PutProjectSecret(_ context.Context, _, name, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putSecretCalls++
	f.putSecretLast[name] = value
	return nil
}

func (f *fakeProjectService) DeleteProjectSecret(_ context.Context, _, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletedSecrets = append(f.deletedSecrets, name)
	return nil
}

// ListProjectSecrets records the projectID it was asked for and returns
// the scripted response, so a test can assert ListWorkerProjectSecrets
// resolved the worker to the right project before reading.
func (f *fakeProjectService) ListProjectSecrets(_ context.Context, projectID string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listSecretsProjectID = projectID
	if f.listSecretsErr != nil {
		return nil, f.listSecretsErr
	}
	out := make(map[string]string, len(f.listSecretsResp))
	for k, v := range f.listSecretsResp {
		out[k] = v
	}
	return out, nil
}

func (f *fakeProjectService) CreateGitRepo(_ context.Context, req types.GitRepositoryCreateRequest) (types.GitRepository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createGitRepoCalls++
	if f.createGitRepoErr != nil {
		return types.GitRepository{}, f.createGitRepoErr
	}
	name := req.Name
	if f.createGitRepoNameReturn != "" {
		name = f.createGitRepoNameReturn
	}
	return types.GitRepository{ID: "repo-" + name, Name: name}, nil
}

func (f *fakeProjectService) GetGitRepo(_ context.Context, repoID string) (types.GitRepository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getGitRepoCalls++
	if f.getGitRepoErr != nil {
		return types.GitRepository{}, f.getGitRepoErr
	}
	return types.GitRepository{ID: repoID, Name: repoID}, nil
}

func (f *fakeProjectService) DeleteGitRepo(_ context.Context, repoID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteGitRepoIDs = append(f.deleteGitRepoIDs, repoID)
	return nil
}

func (f *fakeProjectService) AttachRepoToProject(_ context.Context, _, _ string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attachRepoCalls++
	return f.attachRepoErr
}

func (f *fakeProjectService) CreateBranch(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *fakeProjectService) GetAppConfig(_ context.Context, _ string) (types.AppConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getAppCalls++
	return f.appConfig, nil
}

func (f *fakeProjectService) UpdateAppConfig(_ context.Context, _ string, cfg types.AppConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateAppCalls++
	f.updateAppLastCfg = cfg
	return nil
}

func (f *fakeProjectService) DeleteProject(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteProjectIDs = append(f.deleteProjectIDs, id)
	return nil
}

func (f *fakeProjectService) DeleteApp(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteAppIDs = append(f.deleteAppIDs, id)
	return nil
}

// fakeGitForProject is the ProjectGit stand-in. Same shape as
// fakeGitWriter but with an additional path map.
type fakeGitForProject struct {
	mu            sync.Mutex
	putFileCalls  int32
	putFileByPath map[string]string
	putFileErr    error
}

func newFakeGitForProject() *fakeGitForProject {
	return &fakeGitForProject{putFileByPath: map[string]string{}}
}

func (f *fakeGitForProject) CreateBranch(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *fakeGitForProject) CreateOrUpdateFileContents(_ context.Context, _, path, _ string, content []byte, _, _, _ string) (string, error) {
	atomic.AddInt32(&f.putFileCalls, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putFileByPath[path] = string(content)
	return "sha-test", f.putFileErr
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newProjectTestStore(t *testing.T, roleContent string) (*store.Store, orgchart.NodeID) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	// The Bot's Content is its canonical instruction text. Keep the `w-eng`
	// handle so runtime instruction path assertions stay meaningful.
	b, err := orgchart.NewNode("w-eng", roleContent, nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	if err := st.Nodes.Create(ctx, b); err != nil {
		t.Fatalf("create bot: %v", err)
	}
	return st, b.ID
}

func newApplier(svc ProjectService, ws *Workspace, st *store.Store) *WorkerProject {
	_ = ws
	return &WorkerProject{
		Service: svc,
		Store:   st,
		Logger:  discardLogger(),
	}
}

// newApplierGit wraps fakeGitForProject in a Workspace so tests stay
// terse — the test fakes still capture branch + put calls.
func newApplierGit(svc ProjectService, git *fakeGitForProject, st *store.Store) *WorkerProject {
	ws := NewWorkspace(git, st, "helix-specs", "helix-org", "ho@example.com")
	return newApplier(svc, ws, st)
}

// TestEnsureFreshAppliesProjectAndPushesFiles verifies the
// first-activation path.
func TestEnsureFreshAppliesProjectAndPushesFiles(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: engineer")
	svc := newFakeProjectService()
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	projectID, agentAppID, repoID, err := a.Ensure(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if projectID != "prj_test" || agentAppID != "app_test" {
		t.Fatalf("ids = (%q,%q,%q); want (prj_test, app_test, repo-w-eng)", projectID, agentAppID, repoID)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.applyCalls != 1 {
		t.Errorf("ApplyProject calls = %d, want 1", svc.applyCalls)
	}
	if svc.updateProjectCalls != 1 || svc.updateProjectPatchLast.Metadata == nil || !svc.updateProjectPatchLast.Metadata.OrgMembersAccess {
		t.Errorf("fresh project was not marked for org member access: calls=%d patch=%+v", svc.updateProjectCalls, svc.updateProjectPatchLast)
	}
	if svc.lastApplyReq.Name != "w-eng" {
		t.Errorf("ApplyProject name = %q, want w-eng", svc.lastApplyReq.Name)
	}
	if svc.lastApplyReq.Spec.Agent == nil || svc.lastApplyReq.Spec.Agent.Runtime != Runtime {
		t.Errorf("Agent runtime = %+v, want %q", svc.lastApplyReq.Spec.Agent, Runtime)
	}
	if _, ok := svc.putSecretLast["HELIX_ORG_URL"]; ok {
		t.Errorf("HELIX_ORG_URL must not be stored as a project secret")
	}
	if _, ok := svc.putSecretLast["HELIX_WORKER_ID"]; ok {
		t.Errorf("HELIX_WORKER_ID must not be stored as a project secret")
	}
	if len(svc.deletedSecrets) != 2 || svc.deletedSecrets[0] != "HELIX_ORG_URL" || svc.deletedSecrets[1] != "HELIX_WORKER_ID" {
		t.Errorf("deleted secrets = %v, want [HELIX_ORG_URL HELIX_WORKER_ID]", svc.deletedSecrets)
	}
	if svc.createGitRepoCalls != 1 {
		t.Errorf("CreateGitRepo calls = %d, want 1", svc.createGitRepoCalls)
	}
	if svc.attachRepoCalls != 1 {
		t.Errorf("AttachRepoToProject calls = %d, want 1", svc.attachRepoCalls)
	}
	if got := atomic.LoadInt32(&git.putFileCalls); got != 0 {
		t.Errorf("WorkerProject published %d deprecated role files; instruction publishing belongs to Spawner", got)
	}
	state, err := LoadState(context.Background(), st, "org-test", wid)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.ProjectID != "prj_test" || state.AgentID != "app_test" {
		t.Errorf("state = %+v", state)
	}
	// RepoID is part of the contract — without it the desktop bringup
	// has no folder for Zed to open and times out. See
	// TestEnsureRequiresRepoToBeAttached for the explicit pin and the
	// historical context that justifies the assertion.
	if state.RepoID == "" {
		t.Errorf("state.RepoID is empty — Ensure must attach a repo, otherwise HELIX_REPOSITORIES is empty and the desktop bringup script aborts")
	}
	if repoID == "" {
		t.Errorf("returned repoID is empty — same reasoning")
	}
}

func TestEnsureConcurrentAppliesProjectOnce(t *testing.T) {
	st, wid := newProjectTestStore(t, "# Role: engineer")
	svc := newFakeProjectService()
	svc.getProjectResp.Name = "w-eng"
	svc.applyStarted = make(chan struct{}, 2)
	svc.applyContinue = make(chan struct{})
	a := newApplierGit(svc, newFakeGitForProject(), st)

	done := make(chan error, 2)
	go func() {
		_, _, _, err := a.Ensure(context.Background(), "org-test", wid)
		done <- err
	}()
	<-svc.applyStarted
	go func() {
		_, _, _, err := a.Ensure(context.Background(), "org-test", wid)
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)
	close(svc.applyContinue)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("Ensure: %v", err)
		}
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.applyCalls != 1 {
		t.Fatalf("ApplyProject calls = %d, want 1", svc.applyCalls)
	}
}

// TestEnsureRequiresRepoToBeAttached is the red-then-green test for
// the workaround-removed-fail-loud refactor.
//
// Symptom: zed_external sessions for helix-org workers timed out
// because HELIX_REPOSITORIES landed empty in the desktop env, and
// `desktop/shared/helix-workspace-setup.sh` aborts when it has no
// folders to clone. A first-pass fix in that script papered over the
// issue by falling back to ~/work, but the actual contract is "every
// Worker gets its own repo, period" — that's how Helix projects work
// generally and how the helix-org alpha worked before the
// infrastructure/runtime refactor.
//
// This test pins the contract on the application service side. If
// CreateGitRepo or AttachRepoToProject fail (or the inProc adapter
// stops wiring them), Ensure must fail loudly instead of silently
// returning a project with empty RepoID, so the issue surfaces at
// activation time with a clear error rather than as a downstream
// bringup-script timeout.
func TestEnsureRequiresRepoToBeAttached(t *testing.T) {
	t.Parallel()

	t.Run("CreateGitRepo failure aborts", func(t *testing.T) {
		st, wid := newProjectTestStore(t, "# Role: engineer")
		svc := newFakeProjectService()
		svc.createGitRepoErr = errors.New("boom")
		git := newFakeGitForProject()
		a := newApplierGit(svc, git, st)

		_, _, _, err := a.Ensure(context.Background(), "org-test", wid)
		if err == nil {
			t.Fatal("Ensure returned nil error when CreateGitRepo failed; must propagate so the activation surface knows the worker has no usable workspace")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("error does not wrap the underlying cause: %v", err)
		}
	})

	t.Run("AttachRepoToProject failure aborts", func(t *testing.T) {
		st, wid := newProjectTestStore(t, "# Role: engineer")
		svc := newFakeProjectService()
		svc.attachRepoErr = errors.New("nope")
		git := newFakeGitForProject()
		a := newApplierGit(svc, git, st)

		_, _, _, err := a.Ensure(context.Background(), "org-test", wid)
		if err == nil {
			t.Fatal("Ensure returned nil error when AttachRepoToProject failed; must propagate")
		}
		if !strings.Contains(err.Error(), "nope") {
			t.Errorf("error does not wrap the underlying cause: %v", err)
		}
	})

	t.Run("WhoAmI returning empty owner is a configuration error", func(t *testing.T) {
		st, wid := newProjectTestStore(t, "# Role: engineer")
		svc := newFakeProjectService()
		svc.whoAmIResp = ""
		git := newFakeGitForProject()
		a := newApplierGit(svc, git, st)

		_, _, _, err := a.Ensure(context.Background(), "org-test", wid)
		if err == nil {
			t.Fatal("Ensure returned nil error when WhoAmI gave an empty owner; without an owner we can't create a repo at all, so this must fail loudly")
		}
	})
}

// TestEnsureDeletesOrphanRepoOnAttachFailure: a repo created but not
// attachable is an orphan with nothing pointing at it. It must be deleted so a
// retry doesn't leak it (CreateGitRepo auto-increments, so a retry would make
// `<worker>-2` beside the orphan).
func TestEnsureDeletesOrphanRepoOnAttachFailure(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: engineer")
	svc := newFakeProjectService()
	svc.attachRepoErr = errors.New("attach nope")
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err == nil {
		t.Fatal("Ensure must fail when attach fails")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.deleteGitRepoIDs) != 1 || svc.deleteGitRepoIDs[0] != "repo-w-eng" {
		t.Fatalf("orphan repo not deleted: deleteGitRepoIDs = %v, want [repo-w-eng]", svc.deleteGitRepoIDs)
	}
}

// TestEnsureDeletesRacedDuplicateRepo: when CreateGitRepo returns a name we
// didn't ask for (it auto-incremented because a same-named repo already
// existed — a lost cross-process create race), the duplicate must be deleted,
// not kept.
func TestEnsureDeletesRacedDuplicateRepo(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: engineer")
	svc := newFakeProjectService()
	svc.createGitRepoNameReturn = "w-eng-2" // simulate auto-increment on collision
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err == nil {
		t.Fatal("Ensure must fail (retry) when it loses a create race")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.deleteGitRepoIDs) != 1 || svc.deleteGitRepoIDs[0] != "repo-w-eng-2" {
		t.Fatalf("raced duplicate not deleted: deleteGitRepoIDs = %v, want [repo-w-eng-2]", svc.deleteGitRepoIDs)
	}
	if svc.attachRepoCalls != 0 {
		t.Errorf("must not attach a raced duplicate; attachRepoCalls = %d", svc.attachRepoCalls)
	}
}

// TestEnsureFastPathReprovisionsDeletedRepo: on the persisted-project fast
// path, a DefaultRepoID/state repo that has been deleted out-of-band must be
// re-provisioned, not handed back as a dead id.
func TestEnsureFastPathReprovisionsDeletedRepo(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role v1")
	if err := SaveProject(context.Background(), st, "org-test", wid, "prj_existing", "app_existing", "repo_gone"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{ID: "prj_existing", DefaultRepoID: "repo_gone"}
	svc.getGitRepoErr = fmt.Errorf("%w: deleted out of band", ErrRepoNotFound)
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	_, _, rid, err := a.Ensure(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if rid != "repo-w-eng" {
		t.Fatalf("fast path did not re-provision the deleted repo: rid = %q, want repo-w-eng", rid)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.createGitRepoCalls != 1 {
		t.Errorf("deleted repo must be re-created; createGitRepoCalls = %d, want 1", svc.createGitRepoCalls)
	}
	// The new repo id must be persisted so the next activation is clean.
	state, err := LoadState(context.Background(), st, "org-test", wid)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.RepoID != "repo-w-eng" {
		t.Errorf("re-provisioned repo id not persisted: state.RepoID = %q, want repo-w-eng", state.RepoID)
	}
}

// TestEnsureWithPersistedProjectFastPaths checks the persisted-project
// fast path.
//
// The fast path must:
//
//   - return the stored IDs (no fresh provisioning — no
//     CreateGitRepo / AttachRepoToProject calls)
//   - leave the existing app configuration untouched; worker.* values
//     are provisioning defaults and the app UI/API owns later edits.
//   - leave runtime instruction publishing to the activation spawner, which
//     resolves linked Agent content and org-level overrides.
func TestEnsureWithPersistedProjectFastPaths(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role v1")
	if err := SaveProject(context.Background(), st, "org-test", wid, "prj_existing", "app_existing", "repo_existing"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{ID: "prj_existing", DefaultRepoID: "repo_existing"}
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	pid, aid, rid, err := a.Ensure(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	// The fast path returns the stored IDs without reapplying the project.
	if pid != "prj_existing" || aid != "app_existing" || rid != "repo_existing" {
		t.Errorf("returned ids = (%q,%q,%q)", pid, aid, rid)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.applyCalls != 0 {
		t.Errorf("ApplyProject must not overwrite existing app config; got %d calls", svc.applyCalls)
	}
	if svc.updateProjectCalls < 1 || svc.updateProjectPatchLast.Metadata == nil || !svc.updateProjectPatchLast.Metadata.OrgMembersAccess {
		t.Errorf("existing project was not marked for org member access: calls=%d patch=%+v", svc.updateProjectCalls, svc.updateProjectPatchLast)
	}
	if svc.getProjectCalls < 1 {
		t.Errorf("GetProject calls = %d, want >=1", svc.getProjectCalls)
	}
	// Fast path must NOT create a new repo. Repo creation is
	// first-activation provisioning.
	if svc.createGitRepoCalls != 0 {
		t.Errorf("fast path must not create a new repo; got %d", svc.createGitRepoCalls)
	}
	if svc.attachRepoCalls != 0 {
		t.Errorf("fast path must not attach a repo; got %d", svc.attachRepoCalls)
	}
	if got := atomic.LoadInt32(&git.putFileCalls); got != 0 {
		t.Errorf("fast path published %d deprecated role files", got)
	}
}

// TestEnsureFastPathPreservesAgentSpec verifies that provisioning defaults
// cannot overwrite an existing bot app on start.
func TestEnsureFastPathPreservesAgentSpec(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: engineer")
	if err := SaveProject(context.Background(), st, "org-test", wid, "prj_existing", "app_existing", "repo_existing"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{ID: "prj_existing", DefaultRepoID: "repo_existing"}
	git := newFakeGitForProject()

	a := newApplierGit(svc, git, st)
	// These values deliberately differ from the existing app's settings.
	// Ensure must not submit them to ApplyProject.
	a.Runtime = "claude_code"
	a.Credentials = "api_key"
	a.Provider = "OpenRouter"
	a.Model = "anthropic/claude-3-haiku"

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.applyCalls != 0 {
		t.Fatalf("ApplyProject must not run for an existing app; got %d calls", svc.applyCalls)
	}
}

func TestEnsureFastPathPreservesValidProjectDefaultAndConvergesLinks(t *testing.T) {
	st, wid := newProjectTestStore(t, "# Role")
	ctx := context.Background()
	bot, err := st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Update(ctx, bot.WithAgentID("app-new")); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, st, "org-test", wid, "prj_existing", "app-legacy", "repo_existing"); err != nil {
		t.Fatal(err)
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{
		ID: "prj_existing", OrganizationID: "org-test",
		DefaultHelixAppID: "app-explicit", DefaultRepoID: "repo_existing",
	}

	if _, agentAppID, _, err := newApplierGit(svc, newFakeGitForProject(), st).Ensure(ctx, "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	} else if agentAppID != "app-explicit" {
		t.Fatalf("returned app = %q, want app-explicit", agentAppID)
	}
	bot, err = st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if bot.AgentID != "app-explicit" {
		t.Fatalf("bot app = %q, want app-explicit", bot.AgentID)
	}
	state, err := LoadState(ctx, st, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if state.AgentID != "app-explicit" {
		t.Fatalf("runtime app = %q, want app-explicit", state.AgentID)
	}
}

func TestEnsureFastPathDoesNotAdoptProjectDefaultClaimedByAnotherBot(t *testing.T) {
	st, wid := newProjectTestStore(t, "# Role")
	ctx := context.Background()
	bot, err := st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Update(ctx, bot.WithAgentID("app-current")); err != nil {
		t.Fatal(err)
	}
	other, err := orgchart.NewNode("w-other", "# Other", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, other.WithAgentID("app-claimed")); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, st, "org-test", wid, "prj_existing", "app-legacy", "repo_existing"); err != nil {
		t.Fatal(err)
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{
		ID: "prj_existing", OrganizationID: "org-test",
		DefaultHelixAppID: "app-claimed", DefaultRepoID: "repo_existing",
	}

	if _, agentAppID, _, err := newApplierGit(svc, newFakeGitForProject(), st).Ensure(ctx, "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	} else if agentAppID != "app-current" {
		t.Fatalf("returned app = %q, want app-current", agentAppID)
	}
	bot, err = st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if bot.AgentID != "app-current" {
		t.Fatalf("bot app = %q, want app-current", bot.AgentID)
	}
	state, err := LoadState(ctx, st, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if state.AgentID != "app-current" {
		t.Fatalf("runtime app = %q, want app-current", state.AgentID)
	}
}

func TestEnsureFastPathPreservesLinksWhenProjectDefaultAppReadFails(t *testing.T) {
	st, wid := newProjectTestStore(t, "# Role")
	ctx := context.Background()
	bot, err := st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Update(ctx, bot.WithAgentID("app-current")); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, st, "org-test", wid, "prj_existing", "app-runtime", "repo_existing"); err != nil {
		t.Fatal(err)
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{
		ID: "prj_existing", OrganizationID: "org-test",
		DefaultHelixAppID: "app-explicit", DefaultRepoID: "repo_existing",
	}
	svc.getAppErr = errors.New("database unavailable")

	if _, _, _, err := newApplierGit(svc, newFakeGitForProject(), st).Ensure(ctx, "org-test", wid); err == nil ||
		!strings.Contains(err.Error(), "get project default agent app app-explicit") {
		t.Fatalf("Ensure error = %v", err)
	}
	bot, err = st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if bot.AgentID != "app-current" {
		t.Fatalf("bot app changed to %q", bot.AgentID)
	}
	state, err := LoadState(ctx, st, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if state.AgentID != "app-runtime" {
		t.Fatalf("runtime app changed to %q", state.AgentID)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.updateProjectCalls != 0 {
		t.Fatalf("project updated %d times", svc.updateProjectCalls)
	}
	if svc.getProjectResp.DefaultHelixAppID != "app-explicit" {
		t.Fatalf("project default changed to %q", svc.getProjectResp.DefaultHelixAppID)
	}
}

func TestEnsureFastPathRepairsStaleRuntimeAgentApp(t *testing.T) {
	st, wid := newProjectTestStore(t, "# Role")
	ctx := context.Background()
	bot, err := st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Update(ctx, bot.WithAgentID("app-canonical")); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, st, "org-test", wid, "prj_existing", "app-legacy", "repo_existing"); err != nil {
		t.Fatal(err)
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{
		ID: "prj_existing", OrganizationID: "org-test",
		DefaultHelixAppID: "app-canonical", DefaultRepoID: "repo_existing",
	}

	if _, _, _, err := newApplierGit(svc, newFakeGitForProject(), st).Ensure(ctx, "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	state, err := LoadState(ctx, st, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if state.AgentID != "app-canonical" {
		t.Fatalf("runtime app = %q, want app-canonical", state.AgentID)
	}
}

// TestEnsureClearsStateOnGetProject404 verifies that on
// ErrProjectNotFound, ClearProject runs and a fresh apply follows.
func TestEnsureClearsStateOnGetProject404(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	if err := SaveProject(context.Background(), st, "org-test", wid, "prj_ghost", "app_ghost", "repo_ghost"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), st, "org-test", wid, "ses_ghost"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	svc := newFakeProjectService()
	svc.getProjectErr = ErrProjectNotFound
	svc.getProjectErrOnce = true
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	_, _, _, err := a.Ensure(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	state, err := LoadState(context.Background(), st, "org-test", wid)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.ProjectID != "prj_test" {
		t.Errorf("state.ProjectID = %q, want prj_test (fresh after clear)", state.ProjectID)
	}
	if state.SessionID != "" {
		t.Errorf("session pointer must be cleared with project; got %q", state.SessionID)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.applyCalls != 1 {
		t.Errorf("ApplyProject calls = %d, want 1 (after clear)", svc.applyCalls)
	}
}

// TestEnsureGetProjectErrorIsFatal pins the contract: non-404 GetProject
// errors propagate as fatal.
func TestEnsureGetProjectErrorIsFatal(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	if err := SaveProject(context.Background(), st, "org-test", wid, "prj_existing", "app_existing", "repo_existing"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	svc := newFakeProjectService()
	svc.getProjectErr = errors.New("transient")
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err == nil {
		t.Fatal("expected error on transient GetProject failure")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.applyCalls != 0 {
		t.Errorf("ApplyProject must not run on transient GetProject error; got %d", svc.applyCalls)
	}
}

// TestEnsureDoesNotTouchAgentAppMCPs pins that MCP configuration is
// generated from session metadata instead of persisted by project setup.
func TestEnsureDoesNotTouchAgentAppMCPs(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	svc := newFakeProjectService()
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.getAppCalls != 0 {
		t.Errorf("Ensure must not read agent app config; GetAppConfig calls = %d", svc.getAppCalls)
	}
	if svc.updateAppCalls != 0 {
		t.Errorf("Ensure must not write agent app config; UpdateAppConfig calls = %d", svc.updateAppCalls)
	}
}

// TestEnsureDoesNotPublishDeprecatedRoleFile pins the single-instruction-file
// contract: project provisioning must not recreate role.md.
func TestEnsureDoesNotPublishDeprecatedRoleFile(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: ChiefEngineer")
	svc := newFakeProjectService()
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if got := atomic.LoadInt32(&git.putFileCalls); got != 0 {
		t.Errorf("WorkerProject published %d files, want 0", got)
	}
}

// TestEnsureScopesProjectToParamOrg_NotStructOrgID is the unit-level
// pin for the cross-tenant leak fix
// (design/2026-06-09-org-multitenancy-spawner-leak.md).
//
// WorkerProject.Ensure takes an orgID parameter AND carries an OrgID
// struct field. They are normally equal, but the production spawner
// used to freeze one org's identity onto a process-wide SpawnerConfig
// and replay it for every org — so a WorkerProject built for org A
// would .Ensure() worker activations for org B. If Ensure stamps the
// project with the struct field (a.OrgID) instead of the orgID it was
// invoked for, org B's worker project lands in org A — the root of the
// leak. This test forces the two apart and asserts the parameter wins.
func TestEnsureScopesProjectToParamOrg_NotStructOrgID(t *testing.T) {
	t.Parallel()
	// Worker exists in org-test (newProjectTestStore seeds there).
	st, wid := newProjectTestStore(t, "# Role: engineer")
	svc := newFakeProjectService()
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)
	// Frozen, WRONG org on the struct — simulates a reused/cached config.
	a.OrgID = "org-OTHER-TENANT"

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.lastApplyReq.OrganizationID != "org-test" {
		t.Fatalf("ApplyProject OrganizationID = %q, want org-test — Ensure must scope to the org it was invoked for, not the struct's frozen OrgID (cross-tenant leak)", svc.lastApplyReq.OrganizationID)
	}
}
