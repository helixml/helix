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

	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/helix/helixclient"
	"github.com/helixml/helix/helix-org/store"
	"github.com/helixml/helix/helix-org/store/sqlite"
)

// projectFake is an extended fakeHelixClient with capture counters and
// configurable return values for the project-applier paths. It
// satisfies helixclient.Client by embedding the spawner-test fake.
type projectFake struct {
	fakeHelixClient

	mu sync.Mutex

	applyCalls            int
	lastApplyReq          helixclient.ProjectApplyRequest
	applyResponse         helixclient.ProjectApplyResponse
	applyErr              error
	getProjectCalls       int
	getProjectResp        helixclient.Project
	getProjectErr         error
	putSecretCalls        int
	putSecretLast         map[string]string // key -> value
	createGitRepoCalls    int
	createGitRepoErr      error
	attachRepoCalls       int
	createBranchCalls     int
	putFileCalls          int32
	putFileByPath         map[string]string
	putFileErr            error
	getAppCalls           int
	getAppResp            helixclient.App
	updateAppCalls        int
	updateAppLastReq      helixclient.AppRequest
	whoAmIResp            helixclient.UserStatus
}

func newProjectFake() *projectFake {
	// Helix's auto-provisioned Agent App always has one assistant —
	// AttachMCPToAppWithHeaders inserts the MCP entry on assistants[0].
	// Without this, the attach helper aborts before UpdateApp.
	const seededAppConfig = `{"helix":{"assistants":[{"name":"main"}]}}`
	return &projectFake{
		applyResponse:  helixclient.ProjectApplyResponse{ProjectID: "prj_test", AgentAppID: "app_test", Created: true},
		getProjectResp: helixclient.Project{ID: "prj_test", DefaultRepoID: ""},
		whoAmIResp:     helixclient.UserStatus{User: "u-test"},
		putFileByPath:  map[string]string{},
		putSecretLast:  map[string]string{},
		getAppResp:     helixclient.App{ID: "app_test", Config: []byte(seededAppConfig)},
	}
}

func (f *projectFake) ApplyProject(_ context.Context, req helixclient.ProjectApplyRequest) (helixclient.ProjectApplyResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applyCalls++
	f.lastApplyReq = req
	return f.applyResponse, f.applyErr
}

func (f *projectFake) GetProject(_ context.Context, _ string) (helixclient.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getProjectCalls++
	return f.getProjectResp, f.getProjectErr
}

func (f *projectFake) PutProjectSecret(_ context.Context, _, name, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putSecretCalls++
	f.putSecretLast[name] = value
	return nil
}

func (f *projectFake) CreateGitRepo(_ context.Context, req helixclient.CreateGitRepoRequest) (helixclient.GitRepo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createGitRepoCalls++
	if f.createGitRepoErr != nil {
		return helixclient.GitRepo{}, f.createGitRepoErr
	}
	return helixclient.GitRepo{ID: "repo-" + req.Name, Name: req.Name}, nil
}

func (f *projectFake) AttachRepoToProject(_ context.Context, _, _ string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attachRepoCalls++
	return nil
}

func (f *projectFake) CreateBranch(_ context.Context, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createBranchCalls++
	return nil
}

func (f *projectFake) PutFile(_ context.Context, _ string, req helixclient.PutFileRequest) error {
	atomic.AddInt32(&f.putFileCalls, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putFileByPath[req.Path] = req.Content
	return f.putFileErr
}

func (f *projectFake) WhoAmI(_ context.Context) (helixclient.UserStatus, error) {
	return f.whoAmIResp, nil
}

func (f *projectFake) GetApp(_ context.Context, _ string) (helixclient.App, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getAppCalls++
	return f.getAppResp, nil
}

func (f *projectFake) UpdateApp(_ context.Context, _ string, req helixclient.AppRequest) (helixclient.App, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateAppCalls++
	f.updateAppLastReq = req
	return helixclient.App{}, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newProjectTestStore(t *testing.T, roleContent string) (*store.Store, worker.ID) {
	t.Helper()
	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	r, err := role.New("r-eng", roleContent, nil, nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	if err := st.Roles.Create(ctx, r); err != nil {
		t.Fatalf("create role: %v", err)
	}
	pos, _ := domain.NewPosition("p-eng", "r-eng", nil)
	if err := st.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}
	w, err := domain.NewAIWorker("w-eng", []position.ID{"p-eng"}, "# Identity content")
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := st.Workers.Create(ctx, w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	return st, w.ID()
}

func newApplier(fc helixclient.Client, st *store.Store) *ProjectApplier {
	return &ProjectApplier{
		Client:      fc,
		Store:       st,
		HelixOrgURL: "http://helix-org:8081",
		AgentMD:     "# Org policy",
		Logger:      discardLogger(),
	}
}

// TestEnsureFreshAppliesProjectAndPushesFiles verifies the
// first-activation path: ApplyProject is called once, project secrets
// are written, a git repo is created and attached, the helix-specs
// branch is materialised, and role/identity/agent.md are pushed.
func TestEnsureFreshAppliesProjectAndPushesFiles(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: engineer")
	fc := newProjectFake()
	a := newApplier(fc, st)

	projectID, agentAppID, repoID, err := a.Ensure(context.Background(), wid)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if projectID != "prj_test" || agentAppID != "app_test" {
		t.Fatalf("ids = (%q,%q,%q); want (prj_test, app_test, repo-w-eng)", projectID, agentAppID, repoID)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.applyCalls != 1 {
		t.Errorf("ApplyProject calls = %d, want 1", fc.applyCalls)
	}
	if fc.lastApplyReq.Name != "w-eng" {
		t.Errorf("ApplyProject name = %q, want w-eng", fc.lastApplyReq.Name)
	}
	if fc.lastApplyReq.Spec.Agent == nil {
		t.Fatalf("ApplyProject Agent spec is nil")
	}
	if fc.lastApplyReq.Spec.Agent.Runtime != runtimehelix.Runtime {
		t.Errorf("Agent runtime = %q, want %q", fc.lastApplyReq.Spec.Agent.Runtime, runtimehelix.Runtime)
	}
	// Project secrets HELIX_ORG_URL + HELIX_WORKER_ID written.
	if fc.putSecretLast["HELIX_ORG_URL"] != "http://helix-org:8081" {
		t.Errorf("HELIX_ORG_URL = %q", fc.putSecretLast["HELIX_ORG_URL"])
	}
	if fc.putSecretLast["HELIX_WORKER_ID"] != "w-eng" {
		t.Errorf("HELIX_WORKER_ID = %q", fc.putSecretLast["HELIX_WORKER_ID"])
	}
	// Git repo created + attached.
	if fc.createGitRepoCalls != 1 {
		t.Errorf("CreateGitRepo calls = %d, want 1", fc.createGitRepoCalls)
	}
	if fc.attachRepoCalls != 1 {
		t.Errorf("AttachRepoToProject calls = %d, want 1", fc.attachRepoCalls)
	}
	// helix-specs branch created.
	if fc.createBranchCalls < 1 {
		t.Errorf("CreateBranch calls = %d, want >=1", fc.createBranchCalls)
	}
	// PutFile for the three canonical paths.
	if _, ok := fc.putFileByPath[".context/agent.md"]; !ok {
		t.Errorf("agent.md not pushed")
	}
	if _, ok := fc.putFileByPath["workers/w-eng/.context/role.md"]; !ok {
		t.Errorf("role.md not pushed")
	}
	if _, ok := fc.putFileByPath["workers/w-eng/.context/identity.md"]; !ok {
		t.Errorf("identity.md not pushed")
	}
	// Runtime state persisted.
	state, err := runtimehelix.LoadState(context.Background(), st, wid)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.ProjectID != "prj_test" || state.AgentAppID != "app_test" {
		t.Errorf("state = %+v", state)
	}
}

// TestEnsureWithPersistedProjectFastPaths checks that once the project
// triple is persisted, ApplyProject is NOT re-called but the role +
// identity files ARE re-pushed so update_role / update_identity hot
// edits land on every activation.
func TestEnsureWithPersistedProjectFastPaths(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role v1")
	if err := runtimehelix.SaveProject(context.Background(), st, wid, "prj_existing", "app_existing", "repo_existing"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	fc := newProjectFake()
	fc.getProjectResp = helixclient.Project{ID: "prj_existing", DefaultRepoID: "repo_existing"}
	a := newApplier(fc, st)

	pid, aid, rid, err := a.Ensure(context.Background(), wid)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if pid != "prj_existing" || aid != "app_existing" || rid != "repo_existing" {
		t.Errorf("returned ids = (%q,%q,%q)", pid, aid, rid)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.applyCalls != 0 {
		t.Errorf("ApplyProject must not be called on fast path; got %d", fc.applyCalls)
	}
	if fc.getProjectCalls < 1 {
		t.Errorf("GetProject calls = %d, want >=1", fc.getProjectCalls)
	}
	if fc.createBranchCalls < 1 {
		t.Errorf("republish must still create-branch; got %d", fc.createBranchCalls)
	}
	if _, ok := fc.putFileByPath["workers/w-eng/.context/role.md"]; !ok {
		t.Errorf("role.md must be republished on fast path")
	}
}

// TestEnsureClearsStateOnGetProject404 verifies that when GetProject
// returns ErrNotFound for a persisted project (operator deleted it
// directly), ClearProject is called and the full apply path runs.
func TestEnsureClearsStateOnGetProject404(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	if err := runtimehelix.SaveProject(context.Background(), st, wid, "prj_ghost", "app_ghost", "repo_ghost"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := runtimehelix.SaveSession(context.Background(), st, wid, "ses_ghost"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := newProjectFake()
	fc.getProjectErr = helixclient.ErrNotFound
	a := newApplier(fc, st)

	_, _, _, err := a.Ensure(context.Background(), wid)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	// State should have been cleared then re-applied: new project ID
	// comes from ApplyProject's response.
	state, err := runtimehelix.LoadState(context.Background(), st, wid)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.ProjectID != "prj_test" {
		t.Errorf("state.ProjectID = %q, want prj_test (fresh after clear)", state.ProjectID)
	}
	// Session pointer is wiped when ClearProject runs.
	if state.SessionID != "" {
		t.Errorf("session pointer must be cleared with project; got %q", state.SessionID)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.applyCalls != 1 {
		t.Errorf("ApplyProject calls = %d, want 1 (after clear)", fc.applyCalls)
	}
}

// TestEnsureGetProjectErrorIsFatal pins the contract: non-404 errors
// from GetProject during the fast-path verification propagate as fatal
// — we do NOT fall through to a redundant ApplyProject (which would
// silently duplicate state for a transient network blip).
func TestEnsureGetProjectErrorIsFatal(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	if err := runtimehelix.SaveProject(context.Background(), st, wid, "prj_existing", "app_existing", "repo_existing"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	fc := newProjectFake()
	fc.getProjectErr = errors.New("transient")
	a := newApplier(fc, st)

	if _, _, _, err := a.Ensure(context.Background(), wid); err == nil {
		t.Fatal("expected error on transient GetProject failure")
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.applyCalls != 0 {
		t.Errorf("ApplyProject must not run on transient GetProject error; got %d", fc.applyCalls)
	}
}

// TestEnsureAttachesMCPToAgentApp verifies the GetApp + UpdateApp pair
// runs when the project applier has an Agent App ID and an
// HelixOrgURL: the MCP entry on the app config carries the
// /workers/<id>/mcp URL.
func TestEnsureAttachesMCPToAgentApp(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	fc := newProjectFake()
	a := newApplier(fc, st)

	if _, _, _, err := a.Ensure(context.Background(), wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.getAppCalls < 1 || fc.updateAppCalls < 1 {
		t.Fatalf("MCP attach must call GetApp+UpdateApp; got get=%d update=%d", fc.getAppCalls, fc.updateAppCalls)
	}
	if !strings.Contains(string(fc.updateAppLastReq.Config), "/workers/w-eng/mcp") {
		t.Errorf("UpdateApp config missing MCP URL: %s", fc.updateAppLastReq.Config)
	}
}

// TestEnsureMCPAttachUsesBearerFromContext pins the per-call bearer
// behaviour: when ctx carries a bearer via WithBearerToken, the MCP
// attach uses that bearer in the Authorization header on the MCP
// entry. This is the chat-bridge / spawner per-user identity path.
func TestEnsureMCPAttachUsesBearerFromContext(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	fc := newProjectFake()
	a := newApplier(fc, st)

	ctx := helixclient.WithBearerToken(context.Background(), "k_bob")
	if _, _, _, err := a.Ensure(ctx, wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if !strings.Contains(string(fc.updateAppLastReq.Config), "Bearer k_bob") {
		t.Errorf("UpdateApp config missing per-call bearer; got %s", fc.updateAppLastReq.Config)
	}
}

// TestEnsureRolePropagatesFromFirstPosition checks that the content
// of role.md pushed to the helix-specs branch matches the canonical
// Role.Content from the Worker's first Position. This is what makes
// update_role hot edits land on the next activation.
func TestEnsureRolePropagatesFromFirstPosition(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role: ChiefEngineer")
	fc := newProjectFake()
	a := newApplier(fc, st)

	if _, _, _, err := a.Ensure(context.Background(), wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if got := fc.putFileByPath["workers/w-eng/.context/role.md"]; got != "# Role: ChiefEngineer" {
		t.Errorf("role.md content = %q", got)
	}
}

// TestEnsureSkipsRolePushIfNoPosition verifies the no-position
// degenerate case: no role.md push (content is empty), but the other
// files still go up so the worker has agent.md + identity.md.
func TestEnsureSkipsRolePushIfNoPosition(t *testing.T) {
	t.Parallel()
	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Worker with NO positions.
	w, _ := domain.NewAIWorker("w-orphan", nil, "# I am alone")
	if err := st.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	fc := newProjectFake()
	a := newApplier(fc, st)

	if _, _, _, err := a.Ensure(context.Background(), w.ID()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if _, ok := fc.putFileByPath["workers/w-orphan/.context/role.md"]; ok {
		t.Errorf("role.md must not be pushed without a position")
	}
	if _, ok := fc.putFileByPath["workers/w-orphan/.context/identity.md"]; !ok {
		t.Errorf("identity.md should still be pushed")
	}
}

// TestEnsureLogsButDoesNotFailOnPutFileError verifies file-push
// errors are non-fatal: Ensure returns success even if PutFile fails
// (recoverable on the next activation, but hard fail would block the
// dispatch chain).
func TestEnsureLogsButDoesNotFailOnPutFileError(t *testing.T) {
	t.Parallel()
	st, wid := newProjectTestStore(t, "# Role")
	fc := newProjectFake()
	fc.putFileErr = errors.New("disk full")
	a := newApplier(fc, st)

	if _, _, _, err := a.Ensure(context.Background(), wid); err != nil {
		t.Fatalf("Ensure must not fail on PutFile error; got %v", err)
	}
}
