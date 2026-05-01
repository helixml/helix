package tools

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

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools/helixclient"
)

// fakeHelixClient is a deterministic stand-in for helixclient.Client.
// Each method records call counts and returns canned values so tests
// can assert ordering and content without standing up HTTP.
type fakeHelixClient struct {
	mu             sync.Mutex
	startCalls     int32
	outputCalls    int32
	subscribeCalls int32
	startSessionID string
	startErr       error
	outputs        []helixclient.Output
	updatesFactory func() <-chan helixclient.SessionUpdate
	lastStartReq   helixclient.StartChatRequest
}

func (f *fakeHelixClient) StartChat(_ context.Context, req helixclient.StartChatRequest) (helixclient.Session, error) {
	atomic.AddInt32(&f.startCalls, 1)
	f.mu.Lock()
	f.lastStartReq = req
	f.mu.Unlock()
	return helixclient.Session{ID: f.startSessionID}, f.startErr
}

func (f *fakeHelixClient) StartChatWithStatus(ctx context.Context, req helixclient.StartChatRequest) (helixclient.Session, bool, error) {
	s, err := f.StartChat(ctx, req)
	return s, false, err
}

func (f *fakeHelixClient) CreateGitRepo(_ context.Context, req helixclient.CreateGitRepoRequest) (helixclient.GitRepo, error) {
	return helixclient.GitRepo{ID: "repo-" + req.Name, Name: req.Name}, nil
}

func (f *fakeHelixClient) AttachRepoToProject(_ context.Context, _, _ string, _ bool) error {
	return nil
}

func (f *fakeHelixClient) CreateBranch(_ context.Context, _, _, _ string) error { return nil }

func (f *fakeHelixClient) GetOutput(_ context.Context, _ string) (helixclient.Output, error) {
	i := int(atomic.AddInt32(&f.outputCalls, 1)) - 1
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.outputs) {
		return f.outputs[len(f.outputs)-1], nil
	}
	return f.outputs[i], nil
}

func (f *fakeHelixClient) SubscribeUpdates(_ context.Context, _ string) (<-chan helixclient.SessionUpdate, error) {
	atomic.AddInt32(&f.subscribeCalls, 1)
	if f.updatesFactory != nil {
		return f.updatesFactory(), nil
	}
	ch := make(chan helixclient.SessionUpdate)
	close(ch)
	return ch, nil
}

func (f *fakeHelixClient) StopExternalAgent(_ context.Context, _ string) error { return nil }
func (f *fakeHelixClient) WhoAmI(_ context.Context) (helixclient.UserStatus, error) {
	return helixclient.UserStatus{}, nil
}
func (f *fakeHelixClient) ApplyProject(_ context.Context, _ helixclient.ProjectApplyRequest) (helixclient.ProjectApplyResponse, error) {
	return helixclient.ProjectApplyResponse{ProjectID: "prj_test", AgentAppID: "app_test", Created: true}, nil
}
func (f *fakeHelixClient) GetProject(_ context.Context, _ string) (helixclient.Project, error) {
	return helixclient.Project{ID: "prj_test", DefaultRepoID: "repo_test"}, nil
}
func (f *fakeHelixClient) DeleteProject(_ context.Context, _ string) error { return nil }
func (f *fakeHelixClient) GetSession(_ context.Context, _ string) (helixclient.Session, error) {
	return helixclient.Session{}, nil
}
func (f *fakeHelixClient) PutProjectSecret(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeHelixClient) PutFile(_ context.Context, _ string, _ helixclient.PutFileRequest) error {
	return nil
}
func (f *fakeHelixClient) GetFile(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (f *fakeHelixClient) CreateApp(_ context.Context, _ helixclient.AppRequest) (helixclient.App, error) {
	return helixclient.App{ID: "app_test"}, nil
}
func (f *fakeHelixClient) GetApp(_ context.Context, _ string) (helixclient.App, error) {
	return helixclient.App{}, nil
}
func (f *fakeHelixClient) UpdateApp(_ context.Context, _ string, _ helixclient.AppRequest) (helixclient.App, error) {
	return helixclient.App{}, nil
}

func newHelixTestStore(t *testing.T) (*store.Store, domain.WorkerID) {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	role, _ := domain.NewRole("r-eng", "# Role: Engineer", time.Now().UTC())
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("role: %v", err)
	}
	pos, _ := domain.NewPosition("p-eng", "r-eng", nil)
	if err := s.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("pos: %v", err)
	}
	worker, _ := domain.NewAIWorker("w-eng", []domain.PositionID{"p-eng"}, "# Persona")
	if err := s.Workers.Create(ctx, worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	return s, worker.ID()
}

func newHelixCfg(t *testing.T, fc *fakeHelixClient, s *store.Store) HelixSpawnerConfig {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return HelixSpawnerConfig{
		Client:            fc,
		HelixOrgURL:       "http://helix-org:8081",
		Provider:          "openai",
		Model:             "gpt-4o-mini",
		Runtime:           "claude_code",
		ActivationTimeout: 2 * time.Second,
		MaxInflight:       2,
		PollInitial:       time.Millisecond,
		PollMax:           5 * time.Millisecond,
		Logger:            logger,
		Store:             s,
		Now:               func() time.Time { return time.Now().UTC() },
		NewID:             func() string { return "id" },
	}
}

func TestHelixSpawnerStartsFreshAndPersistsSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []helixclient.Output{{Status: "complete", Output: "ok"}},
	}
	sp := HelixSpawner(newHelixCfg(t, fc, s))
	err := sp(context.Background(), wid, "/ignored", []Trigger{{Kind: TriggerHire}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 1 {
		t.Errorf("StartChat calls: %d", got)
	}
	w, _ := s.Workers.Get(context.Background(), wid)
	if w.HelixSessionID() != "ses_new" {
		t.Errorf("session pointer = %q", w.HelixSessionID())
	}
	// The Worker should have its per-project IDs persisted from the
	// fake's ApplyProject response.
	if w.HelixProjectID() != "prj_test" || w.HelixAgentAppID() != "app_test" {
		t.Errorf("project IDs not persisted: project=%q agent_app=%q", w.HelixProjectID(), w.HelixAgentAppID())
	}
	// StartChat must point at the per-Worker project, not at any
	// global one.
	if fc.lastStartReq.ProjectID != "prj_test" {
		t.Errorf("StartChat ProjectID = %q (want prj_test)", fc.lastStartReq.ProjectID)
	}
	if fc.lastStartReq.AppID != "app_test" {
		t.Errorf("StartChat AppID = %q (want app_test)", fc.lastStartReq.AppID)
	}
}

// Live-followup behaviour was dropped along with PostFollowup /
// SessionAlive when we moved to the per-Worker-project model — every
// activation opens a fresh chat session against the Worker's project.
// Tests that exercised reuse / stale-pointer recovery were removed
// here; the remaining tests cover the active code paths.

// TestHelixBridgeRendersEntryPatchEvents verifies that the bridge's
// EntryStream callback produces the same line shapes the claude
// bridge emits — assistant text, tool_use, tool_result. The
// underlying dedup is the EntryStream's responsibility (covered in
// helixclient/patches_test.go).
func TestHelixBridgeRendersEntryPatchEvents(t *testing.T) {
	t.Parallel()
	var got []string
	b := newHelixBridge(func(s string) { got = append(got, s) })
	b.stream.Apply(helixclient.SessionUpdate{EntryPatches: []helixclient.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "hi", PatchOffset: 0},
	}})
	b.stream.Apply(helixclient.SessionUpdate{EntryPatches: []helixclient.EntryPatch{
		{Index: 1, MessageID: "t1", Type: "tool_call", Patch: `{"x":1}`, ToolName: "publish", ToolStatus: "Completed"},
	}})
	b.stream.Flush()
	if len(got) < 3 {
		t.Fatalf("expected ≥3 events, got %d: %v", len(got), got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "assistant: hi") {
		t.Errorf("missing assistant: %v", got)
	}
	if !strings.Contains(joined, "tool_use publish: {\"x\":1}") {
		t.Errorf("missing tool_use: %v", got)
	}
	if !strings.Contains(joined, "tool_result: ") {
		t.Errorf("missing tool_result: %v", got)
	}
}

func TestHelixSpawnerTimeoutEmitsExitError(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		// Always "waiting" so the loop never terminates on its own.
		outputs: []helixclient.Output{{Status: "waiting"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.ActivationTimeout = 30 * time.Millisecond
	sp := HelixSpawner(cfg)
	err := sp(context.Background(), wid, "/ignored", []Trigger{{Kind: TriggerHire}})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

func TestHelixSpawnerSemaphoreSerialises(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	gate := make(chan struct{})
	var inflight, peak int32
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		outputs:        []helixclient.Output{{Status: "complete", Output: "ok"}},
	}
	// Decorate GetOutput so we can observe concurrency. Reuse fakeHelixClient
	// by wrapping start to bump counters.
	original := fc.outputs[0]
	fc.outputs = []helixclient.Output{original}

	cfg := newHelixCfg(t, fc, s)
	cfg.MaxInflight = 1
	cfg.ActivationTimeout = time.Second

	// Wrap client to capture overlap during the polling phase.
	wrapped := &concurrencyClient{inner: fc, gate: gate, inflight: &inflight, peak: &peak}
	cfg.Client = wrapped
	sp := HelixSpawner(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sp(context.Background(), wid, "/ignored", []Trigger{{Kind: TriggerHire}})
		}()
	}
	// Let both goroutines reach the StartChat call. Then unblock.
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if got := atomic.LoadInt32(&peak); got > 1 {
		t.Errorf("peak inflight = %d (want <=1)", got)
	}
}

type concurrencyClient struct {
	inner    helixclient.Client
	gate     chan struct{}
	inflight *int32
	peak     *int32
}

func (c *concurrencyClient) StartChat(ctx context.Context, req helixclient.StartChatRequest) (helixclient.Session, error) {
	cur := atomic.AddInt32(c.inflight, 1)
	for {
		p := atomic.LoadInt32(c.peak)
		if cur <= p || atomic.CompareAndSwapInt32(c.peak, p, cur) {
			break
		}
	}
	defer atomic.AddInt32(c.inflight, -1)
	<-c.gate
	return c.inner.StartChat(ctx, req)
}
func (c *concurrencyClient) StartChatWithStatus(ctx context.Context, req helixclient.StartChatRequest) (helixclient.Session, bool, error) {
	s, err := c.StartChat(ctx, req)
	return s, false, err
}
func (c *concurrencyClient) CreateGitRepo(ctx context.Context, req helixclient.CreateGitRepoRequest) (helixclient.GitRepo, error) {
	return c.inner.CreateGitRepo(ctx, req)
}
func (c *concurrencyClient) AttachRepoToProject(ctx context.Context, projectID, repoID string, primary bool) error {
	return c.inner.AttachRepoToProject(ctx, projectID, repoID, primary)
}
func (c *concurrencyClient) CreateBranch(ctx context.Context, repoID, branch, baseBranch string) error {
	return c.inner.CreateBranch(ctx, repoID, branch, baseBranch)
}
func (c *concurrencyClient) GetOutput(ctx context.Context, sid string) (helixclient.Output, error) {
	return c.inner.GetOutput(ctx, sid)
}
func (c *concurrencyClient) SubscribeUpdates(ctx context.Context, sid string) (<-chan helixclient.SessionUpdate, error) {
	return c.inner.SubscribeUpdates(ctx, sid)
}
func (c *concurrencyClient) StopExternalAgent(ctx context.Context, sid string) error {
	return c.inner.StopExternalAgent(ctx, sid)
}
func (c *concurrencyClient) WhoAmI(ctx context.Context) (helixclient.UserStatus, error) {
	return c.inner.WhoAmI(ctx)
}
func (c *concurrencyClient) ApplyProject(ctx context.Context, req helixclient.ProjectApplyRequest) (helixclient.ProjectApplyResponse, error) {
	return c.inner.ApplyProject(ctx, req)
}
func (c *concurrencyClient) GetProject(ctx context.Context, id string) (helixclient.Project, error) {
	return c.inner.GetProject(ctx, id)
}
func (c *concurrencyClient) DeleteProject(ctx context.Context, id string) error {
	return c.inner.DeleteProject(ctx, id)
}
func (c *concurrencyClient) GetSession(ctx context.Context, id string) (helixclient.Session, error) {
	return c.inner.GetSession(ctx, id)
}
func (c *concurrencyClient) PutProjectSecret(ctx context.Context, projectID, name, value string) error {
	return c.inner.PutProjectSecret(ctx, projectID, name, value)
}
func (c *concurrencyClient) PutFile(ctx context.Context, repoID string, req helixclient.PutFileRequest) error {
	return c.inner.PutFile(ctx, repoID, req)
}
func (c *concurrencyClient) GetFile(ctx context.Context, repoID, path, branch string) (string, error) {
	return c.inner.GetFile(ctx, repoID, path, branch)
}
func (c *concurrencyClient) CreateApp(ctx context.Context, req helixclient.AppRequest) (helixclient.App, error) {
	return c.inner.CreateApp(ctx, req)
}
func (c *concurrencyClient) GetApp(ctx context.Context, id string) (helixclient.App, error) {
	return c.inner.GetApp(ctx, id)
}
func (c *concurrencyClient) UpdateApp(ctx context.Context, id string, req helixclient.AppRequest) (helixclient.App, error) {
	return c.inner.UpdateApp(ctx, id, req)
}
