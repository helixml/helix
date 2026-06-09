package helix

import (
	"context"
	"errors"
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

	applyCalls         int
	lastApplyReq       types.ProjectApplyRequest
	applyResponse      types.ProjectApplyResponse
	applyErr           error
	getProjectCalls    int
	getProjectResp     types.Project
	getProjectErr      error
	updateProjectCalls     int
	updateProjectPatchLast types.ProjectUpdateRequest
	updateProjectErr       error
	putSecretCalls     int
	putSecretLast      map[string]string
	createGitRepoCalls int
	createGitRepoErr   error
	attachRepoErr      error
	attachRepoCalls    int
	getAppCalls        int
	appConfig          types.AppConfig
	updateAppCalls     int
	updateAppLastCfg   types.AppConfig
	whoAmIResp         string
	deleteProjectIDs   []string
	deleteAppIDs       []string
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
	defer f.mu.Unlock()
	f.applyCalls++
	f.lastApplyReq = req
	return f.applyResponse, f.applyErr
}

func (f *fakeProjectService) GetProject(_ context.Context, _ string) (types.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getProjectCalls++
	return f.getProjectResp, f.getProjectErr
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

func (f *fakeProjectService) CreateGitRepo(_ context.Context, req types.GitRepositoryCreateRequest) (types.GitRepository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createGitRepoCalls++
	if f.createGitRepoErr != nil {
		return types.GitRepository{}, f.createGitRepoErr
	}
	return types.GitRepository{ID: "repo-" + req.Name, Name: req.Name}, nil
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
	branchCalls   int32
	putFileCalls  int32
	putFileByPath map[string]string
	putFileErr    error
}

func newFakeGitForProject() *fakeGitForProject {
	return &fakeGitForProject{putFileByPath: map[string]string{}}
}

func (f *fakeGitForProject) CreateBranch(_ context.Context, _, _, _ string) error {
	atomic.AddInt32(&f.branchCalls, 1)
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

func newProjectTestStore(t *testing.T, roleContent string) (*store.Store, orgchart.WorkerID) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	r, err := orgchart.NewRole("r-eng", roleContent, nil, nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	if err := st.Roles.Create(ctx, r); err != nil {
		t.Fatalf("create role: %v", err)
	}
	w, err := orgchart.NewAIWorker("w-eng", "r-eng", "# Identity content", "org-test")
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := st.Workers.Create(ctx, w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	return st, w.ID()
}

func newApplier(svc ProjectService, ws *Workspace, st *store.Store) *WorkerProject {
	return &WorkerProject{
		Service:     svc,
		Workspace:   ws,
		Store:       st,
		HelixOrgURL: "http://helix-org:8081",
		AgentMD:     "# Org policy",
		Logger:      discardLogger(),
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
	if svc.lastApplyReq.Name != "w-eng" {
		t.Errorf("ApplyProject name = %q, want w-eng", svc.lastApplyReq.Name)
	}
	if svc.lastApplyReq.Spec.Agent == nil || svc.lastApplyReq.Spec.Agent.Runtime != Runtime {
		t.Errorf("Agent runtime = %+v, want %q", svc.lastApplyReq.Spec.Agent, Runtime)
	}
	if svc.putSecretLast["HELIX_ORG_URL"] != "http://helix-org:8081" {
		t.Errorf("HELIX_ORG_URL = %q", svc.putSecretLast["HELIX_ORG_URL"])
	}
	if svc.putSecretLast["HELIX_WORKER_ID"] != "w-eng" {
		t.Errorf("HELIX_WORKER_ID = %q", svc.putSecretLast["HELIX_WORKER_ID"])
	}
	if svc.createGitRepoCalls != 1 {
		t.Errorf("CreateGitRepo calls = %d, want 1", svc.createGitRepoCalls)
	}
	if svc.attachRepoCalls != 1 {
		t.Errorf("AttachRepoToProject calls = %d, want 1", svc.attachRepoCalls)
	}
	if atomic.LoadInt32(&git.branchCalls) < 1 {
		t.Errorf("CreateBranch calls = %d, want >=1", atomic.LoadInt32(&git.branchCalls))
	}
	git.mu.Lock()
	defer git.mu.Unlock()
	for _, p := range []string{".context/agent.md", "workers/w-eng/.context/role.md", "workers/w-eng/.context/identity.md"} {
		if _, ok := git.putFileByPath[p]; !ok {
			t.Errorf("path %q not pushed", p)
		}
	}
	state, err := LoadState(context.Background(), st, "org-test", wid)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.ProjectID != "prj_test" || state.AgentAppID != "app_test" {
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

// TestEnsureWithPersistedProjectFastPaths checks the persisted-project
// fast path.
//
// The fast path must:
//
//   - return the stored IDs (no fresh provisioning)
//   - NOT create a new repo or re-publish canonical files (those
//     would clobber any external edits the operator has made on the
//     helix-specs branch since the last apply — canonical-content
//     updates flow through Workspace.MirrorFile)
//
// It MUST re-call ApplyProject with the current Runtime/Provider/
// Model/Credentials, so a change in worker.* via the Settings page
// propagates to existing workers on the next activation. ApplyProject
// is upsert-by-name and idempotent — calling it on every Ensure with
// the fresh spec is the single mechanism that auto-applies settings
// drift to pre-existing workers. See
// TestEnsureFastPathRefreshesAgentSpec for the spec assertion.
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
	// The fast path returns the STORED ids even though ApplyProject is
	// re-called. (The fake's applyResp would otherwise overwrite them;
	// the prod code keeps the persisted state.)
	if pid != "prj_existing" || aid != "app_existing" || rid != "repo_existing" {
		t.Errorf("returned ids = (%q,%q,%q)", pid, aid, rid)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.applyCalls != 1 {
		t.Errorf("ApplyProject MUST be called on fast path to refresh worker.* settings drift; got %d", svc.applyCalls)
	}
	if svc.getProjectCalls < 1 {
		t.Errorf("GetProject calls = %d, want >=1", svc.getProjectCalls)
	}
	// Fast path must NOT create a new repo, change branches, or re-push
	// canonical files. Repo + files are first-activation provisioning.
	if svc.createGitRepoCalls != 0 {
		t.Errorf("fast path must not create a new repo; got %d", svc.createGitRepoCalls)
	}
	if svc.attachRepoCalls != 0 {
		t.Errorf("fast path must not attach a repo; got %d", svc.attachRepoCalls)
	}
	if atomic.LoadInt32(&git.branchCalls) != 0 {
		t.Errorf("fast path must not create-branch; got %d", atomic.LoadInt32(&git.branchCalls))
	}
	git.mu.Lock()
	defer git.mu.Unlock()
	if _, ok := git.putFileByPath["workers/w-eng/.context/role.md"]; ok {
		t.Errorf("fast path must not republish role.md (would clobber external edits)")
	}
}

// TestEnsureFastPathRefreshesAgentSpec pins the auto-apply behaviour:
// on a pre-existing worker, calling Ensure again with a new applier
// config (different runtime / provider / model / credentials)
// re-applies the project spec so Helix's per-Worker agent app picks
// up the new settings.
//
// Repro: operator opens /helix-org/settings, flips worker.credentials
// from subscription to api_key with a provider+model. Without this
// refresh, the existing w-owner agent app stays in subscription mode
// forever (its spec was baked at first-apply time) — which is the
// "how do users update settings for pre-existing workers?" gap.
func TestEnsureFastPathRefreshesAgentSpec(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: engineer")
	if err := SaveProject(context.Background(), st, "org-test", wid, "prj_existing", "app_existing", "repo_existing"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{ID: "prj_existing", DefaultRepoID: "repo_existing"}
	git := newFakeGitForProject()

	a := newApplierGit(svc, git, st)
	// Simulate the operator having flipped worker.credentials → api_key
	// via the settings page since the worker was first provisioned.
	a.Runtime = "claude_code"
	a.Credentials = "api_key"
	a.Provider = "OpenRouter"
	a.Model = "anthropic/claude-3-haiku"

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.applyCalls != 1 {
		t.Fatalf("ApplyProject must be called on the fast path to refresh spec; got %d", svc.applyCalls)
	}
	got := svc.lastApplyReq.Spec.Agent
	if got == nil {
		t.Fatalf("lastApplyReq has no Agent spec")
	}
	if got.Runtime != "claude_code" {
		t.Errorf("Runtime = %q, want claude_code", got.Runtime)
	}
	if got.Credentials != "api_key" {
		t.Errorf("Credentials = %q, want api_key", got.Credentials)
	}
	if got.Provider != "OpenRouter" {
		t.Errorf("Provider = %q, want OpenRouter", got.Provider)
	}
	if got.Model != "anthropic/claude-3-haiku" {
		t.Errorf("Model = %q, want anthropic/claude-3-haiku", got.Model)
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

// TestEnsureDoesNotTouchAgentAppMCPs pins the new contract: MCP
// attachment is NOT WorkerProject.Ensure's responsibility. It moved
// out into runtimehelix.AttachHelixOrgMCP, called explicitly by the
// Spawner (per-activation) and dynamicProjectApplier (per owner-chat
// ensureWorker). Ensure mutates the project + repo + helix-specs
// files only.
//
// Why this matters: the helix project-apply path wholesale-replaces
// agentApp.Config.Helix on update, so any MCP attached during Ensure
// is clobbered on the next re-apply. Keeping MCP attachment outside
// Ensure means there's exactly one place that writes the MCP entry,
// and it's the last write before the desktop boots.
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

// TestEnsureRolePropagatesFromFirstPosition.
func TestEnsureRolePropagatesFromFirstPosition(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: ChiefEngineer")
	svc := newFakeProjectService()
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	git.mu.Lock()
	defer git.mu.Unlock()
	if got := git.putFileByPath["workers/w-eng/.context/role.md"]; got != "# Role: ChiefEngineer" {
		t.Errorf("role.md content = %q", got)
	}
}

// TestEnsureSkipsRolePushIfRoleMissing.
func TestEnsureSkipsRolePushIfRoleMissing(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	w, _ := orgchart.NewAIWorker("w-orphan", "r-missing", "# I am alone", "org-test")
	if err := st.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	svc := newFakeProjectService()
	git := newFakeGitForProject()
	a := newApplierGit(svc, git, st)

	if _, _, _, err := a.Ensure(context.Background(), "org-test", w.ID()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	git.mu.Lock()
	defer git.mu.Unlock()
	if _, ok := git.putFileByPath["workers/w-orphan/.context/role.md"]; ok {
		t.Errorf("role.md must not be pushed without a position")
	}
	if _, ok := git.putFileByPath["workers/w-orphan/.context/identity.md"]; !ok {
		t.Errorf("identity.md should still be pushed")
	}
}

// TestEnsureLogsButDoesNotFailOnPutFileError.
func TestEnsureLogsButDoesNotFailOnPutFileError(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	svc := newFakeProjectService()
	git := newFakeGitForProject()
	git.putFileErr = errors.New("disk full")
	a := newApplierGit(svc, git, st)

	if _, _, _, err := a.Ensure(context.Background(), "org-test", wid); err != nil {
		t.Fatalf("Ensure must not fail on PutFile error; got %v", err)
	}
}
