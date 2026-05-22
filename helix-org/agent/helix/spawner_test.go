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

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/helix-org/agent"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/helix/helixclient"
	"github.com/helixml/helix/helix-org/store"
	"github.com/helixml/helix/helix-org/store/sqlite"
)

// fakeHelixClient is a deterministic stand-in for helixclient.Client.
type fakeHelixClient struct {
	mu             sync.Mutex
	startCalls     int32
	sendCalls      int32
	outputCalls    int32
	subscribeCalls int32
	startSessionID string
	startErr       error
	sendErr        error
	outputs        []helixclient.Output
	updatesFactory func() <-chan helixclient.SessionUpdate
	lastStartReq   helixclient.StartChatRequest
	lastSendSID    string
	lastSendBody   string
}

func (f *fakeHelixClient) SendSessionMessage(_ context.Context, sid, content string, _ helixclient.SendMessageOptions) (helixclient.SendMessageResponse, error) {
	atomic.AddInt32(&f.sendCalls, 1)
	f.mu.Lock()
	f.lastSendSID = sid
	f.lastSendBody = content
	f.mu.Unlock()
	if f.sendErr != nil {
		return helixclient.SendMessageResponse{}, f.sendErr
	}
	return helixclient.SendMessageResponse{RequestID: "req_x", InteractionID: "ix_x"}, nil
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
func (f *fakeHelixClient) ServerStatus(_ context.Context) (helixclient.ServerStatus, error) {
	return helixclient.ServerStatus{MaxConcurrentDesktops: 0, ActiveConcurrentDesktops: 0}, nil
}
func (f *fakeHelixClient) ListProviders(_ context.Context) ([]string, error) {
	return []string{"openai", "anthropic"}, nil
}
func (f *fakeHelixClient) ListModelsForProvider(_ context.Context, _ string) ([]helixclient.Model, error) {
	return []helixclient.Model{{ID: "gpt-4o-mini", Enabled: true}, {ID: "claude-opus-4-6", Enabled: true}}, nil
}
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

func newHelixTestStore(t *testing.T) (*store.Store, worker.ID) {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	role, _ := role.New("r-eng", "# Role: Engineer", nil, nil, time.Now().UTC())
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("role: %v", err)
	}
	pos, _ := domain.NewPosition("p-eng", "r-eng", nil)
	if err := s.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("pos: %v", err)
	}
	worker, _ := domain.NewAIWorker("w-eng", []position.ID{"p-eng"}, "# Persona")
	if err := s.Workers.Create(ctx, worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	return s, worker.ID()
}

func newHelixCfg(t *testing.T, fc *fakeHelixClient, s *store.Store) SpawnerConfig {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return SpawnerConfig{
		Client:            fc,
		HelixOrgURL:       "http://helix-org:8081",
		Provider:          "openai",
		Model:             "gpt-4o-mini",
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

func TestSpawnerStartsFreshAndPersistsSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []helixclient.Output{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	err := sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 1 {
		t.Errorf("StartChat calls: %d", got)
	}
	state, err := LoadState(context.Background(), s, wid)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.SessionID != "ses_new" {
		t.Errorf("session pointer = %q", state.SessionID)
	}
	// The Worker should have its per-project IDs persisted from the
	// fake's ApplyProject response.
	if state.ProjectID != "prj_test" || state.AgentAppID != "app_test" {
		t.Errorf("project IDs not persisted: project=%q agent_app=%q", state.ProjectID, state.AgentAppID)
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

// TestBridgeRendersEntryPatchEvents verifies that the bridge's
// EntryStream callback produces the same line shapes the claude
// bridge emits — assistant text, tool_use, tool_result.
func TestBridgeRendersEntryPatchEvents(t *testing.T) {
	t.Parallel()
	var got []string
	b := newBridge(func(s string) { got = append(got, s) })
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

// TestSpawnerFollowUpResumesPersistedSession verifies that a Worker
// with a persisted Helix session ID resumes that session rather than
// opening a fresh one — the activation prompt is sent against the
// existing session_id so the warm desktop container is reused. This
// is what makes follow-up activations land in seconds against a warm
// desktop. Resume goes through StartChatWithStatus with SessionID
// set (EnsureAndSend's resume path).
func TestSpawnerFollowUpResumesPersistedSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	// Pre-seed an existing project + session for this worker.
	if err := SaveProject(context.Background(), s, wid, "prj_test", "app_test", "repo_test"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), s, wid, "ses_existing"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeHelixClient{
		startSessionID: "ses_existing",
		outputs:        []helixclient.Output{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	if err := sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e-1"}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// The first StartChatWithStatus call (resume) carries the existing
	// SessionID. The session pointer in the store must remain unchanged.
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.lastStartReq.SessionID != "ses_existing" {
		t.Errorf("StartChatRequest.SessionID = %q (want ses_existing) — resume must target persisted session", fc.lastStartReq.SessionID)
	}
	state, _ := LoadState(context.Background(), s, wid)
	if state.SessionID != "ses_existing" {
		t.Errorf("session pointer changed to %q; resume must NOT open a fresh session", state.SessionID)
	}
}

// TestSpawnerRefusesWhenDesktopQuotaExceeded asserts the spawner fails
// fast with a useful error when Helix's `max_concurrent_desktops`
// would be exceeded by spinning up a new session. Important: only
// fires when there's no existing session — follow-ups reuse the warm
// container and must skip the check (covered by
// TestSpawnerFollowUpUsesSendSessionMessage).
func TestSpawnerRefusesWhenDesktopQuotaExceeded(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &quotaFullFakeClient{
		fakeHelixClient: fakeHelixClient{startSessionID: "ses_x"},
	}
	cfg := newHelixCfg(t, &fc.fakeHelixClient, s)
	cfg.Client = fc
	sp := Spawner(cfg)
	err := sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
	if err == nil {
		t.Fatal("expected error when quota exhausted")
	}
	if !strings.Contains(err.Error(), "quota reached") {
		t.Errorf("error %q does not mention quota", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 0 {
		t.Errorf("StartChat must NOT be called when quota is full; got %d", got)
	}
}

// quotaFullFakeClient overrides ServerStatus to report no available
// desktop slots, simulating Helix's `max_concurrent_desktops` cap.
type quotaFullFakeClient struct {
	fakeHelixClient
}

func (f *quotaFullFakeClient) ServerStatus(_ context.Context) (helixclient.ServerStatus, error) {
	return helixclient.ServerStatus{MaxConcurrentDesktops: 2, ActiveConcurrentDesktops: 2}, nil
}

// TestSpawnerColdStartReQueues verifies that when StartChatWithStatus
// reports hadStreamErr=true on the fresh open, EnsureAndSend re-issues
// the same prompt against the same session ID (belt-and-braces — the
// original race that made this critical was fixed in Helix; this
// retry is the fallback).
func TestSpawnerColdStartReQueues(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &coldStartFakeClient{
		fakeHelixClient: fakeHelixClient{
			startSessionID: "ses_new",
			outputs:        []helixclient.Output{{Status: "complete", Output: "ok"}},
		},
		hadWSError: true,
	}
	cfg := newHelixCfg(t, &fc.fakeHelixClient, s)
	cfg.Client = fc
	sp := Spawner(cfg)
	if err := sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Two StartChatWithStatus calls: the fresh open and the retry on
	// the same session. (Cold-start retry replaced the older
	// SendSessionMessage path in EnsureAndSend.)
	if got := atomic.LoadInt32(&fc.startCalls); got < 2 {
		t.Errorf("StartChat calls: %d (want >=2 — fresh open + cold-start retry)", got)
	}
	// Retry targets the freshly-opened session.
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.lastStartReq.SessionID != "ses_new" {
		t.Errorf("retry SessionID = %q (want ses_new — retry on same session)", fc.lastStartReq.SessionID)
	}
}

// coldStartFakeClient overrides StartChatWithStatus to return
// hadWSError=true, simulating Helix's "no agent WS yet" race.
type coldStartFakeClient struct {
	fakeHelixClient
	hadWSError bool
}

func (f *coldStartFakeClient) StartChatWithStatus(ctx context.Context, req helixclient.StartChatRequest) (helixclient.Session, bool, error) {
	s, err := f.StartChat(ctx, req)
	return s, f.hadWSError, err
}

func TestSpawnerTimeoutEmitsExitError(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		outputs:        []helixclient.Output{{Status: "waiting"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.ActivationTimeout = 30 * time.Millisecond
	sp := Spawner(cfg)
	err := sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

func TestSpawnerSemaphoreSerialises(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	gate := make(chan struct{})
	var inflight, peak int32
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		outputs:        []helixclient.Output{{Status: "complete", Output: "ok"}},
	}
	original := fc.outputs[0]
	fc.outputs = []helixclient.Output{original}

	cfg := newHelixCfg(t, fc, s)
	cfg.MaxInflight = 1
	cfg.ActivationTimeout = time.Second

	wrapped := &concurrencyClient{inner: fc, gate: gate, inflight: &inflight, peak: &peak}
	cfg.Client = wrapped
	sp := Spawner(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
		}()
	}
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
func (c *concurrencyClient) SendSessionMessage(ctx context.Context, sid, content string, opts helixclient.SendMessageOptions) (helixclient.SendMessageResponse, error) {
	return c.inner.SendSessionMessage(ctx, sid, content, opts)
}
func (c *concurrencyClient) ServerStatus(ctx context.Context) (helixclient.ServerStatus, error) {
	return c.inner.ServerStatus(ctx)
}
func (c *concurrencyClient) ListProviders(ctx context.Context) ([]string, error) {
	return c.inner.ListProviders(ctx)
}
func (c *concurrencyClient) ListModelsForProvider(ctx context.Context, provider string) ([]helixclient.Model, error) {
	return c.inner.ListModelsForProvider(ctx, provider)
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

// TestSpawnerSubscribesAndReconnectsOnDisconnect verifies the bridge
// reconnect loop: when the updates channel closes mid-activation, the
// spawner re-calls SubscribeUpdates for the lifetime of the activation
// context. H1.3b replaces this WebSocket subscription with a pubsub
// one; this test pins the reconnect contract before that lift.
//
// The bridge's reconnect delay starts at 1s so we run an artificially
// long activation (slow polls so pollUntilDone doesn't terminate) to
// give reconnect at least one chance to fire.
func TestSpawnerSubscribesAndReconnectsOnDisconnect(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)

	var subCount int32
	// Many waiting outputs so the activation lives long enough for
	// the bridge's 1s reconnect backoff to fire at least once.
	waiting := make([]helixclient.Output, 0, 8)
	for i := 0; i < 7; i++ {
		waiting = append(waiting, helixclient.Output{Status: "waiting"})
	}
	waiting = append(waiting, helixclient.Output{Status: "complete", Output: "ok"})
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		outputs:        waiting,
		updatesFactory: func() <-chan helixclient.SessionUpdate {
			ch := make(chan helixclient.SessionUpdate)
			// First subscription closes immediately so the bridge
			// reconnects. Subsequent subscriptions stay open until
			// activation completes.
			if atomic.AddInt32(&subCount, 1) == 1 {
				close(ch)
			}
			return ch
		},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.ActivationTimeout = 5 * time.Second
	cfg.PollInitial = 200 * time.Millisecond
	cfg.PollMax = 200 * time.Millisecond
	sp := Spawner(cfg)
	_ = sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})

	if got := atomic.LoadInt32(&fc.subscribeCalls); got < 2 {
		t.Errorf("SubscribeUpdates calls = %d, want >=2 (reconnect on disconnect)", got)
	}
}

// TestSpawnerPublishesTranscriptViaEntryStream verifies the bridge
// feeds session updates through EntryStream and republishes settled
// events as activation Stream events.
func TestSpawnerPublishesTranscriptViaEntryStream(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)

	updates := make(chan helixclient.SessionUpdate, 4)
	updates <- helixclient.SessionUpdate{EntryPatches: []helixclient.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "hi there"},
	}}
	updates <- helixclient.SessionUpdate{
		Interaction: &helixclient.Interaction{State: "complete"},
	}
	close(updates)

	fc := &fakeHelixClient{
		startSessionID: "ses_y",
		// Several waiting outputs so the bridge has time to consume
		// the updates channel before pollUntilDone terminates.
		outputs: []helixclient.Output{
			{Status: "waiting"}, {Status: "waiting"}, {Status: "complete", Output: "ok"},
		},
		updatesFactory: func() <-chan helixclient.SessionUpdate { return updates },
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.PollInitial = 50 * time.Millisecond
	cfg.ActivationTimeout = 2 * time.Second
	// Unique IDs so each publishActivationEvent insert succeeds; the
	// default fake helper hands back the same string each call and
	// the duplicate-PK on Events.Append silently drops later events.
	var idCounter int32
	cfg.NewID = func() string { return fmt.Sprintf("e-%d", atomic.AddInt32(&idCounter, 1)) }
	sp := Spawner(cfg)
	if err := sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// Read events on the activation stream. We expect at least one
	// "assistant: hi there" line from the EntryStream callback.
	events, err := s.Events.ListForStream(context.Background(), agent.ActivationStreamID(wid), 100)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var sawAssistant bool
	for _, e := range events {
		msg, err := e.Message()
		if err != nil {
			continue
		}
		if strings.Contains(msg.Body, "assistant: hi there") {
			sawAssistant = true
		}
	}
	if !sawAssistant {
		t.Fatalf("activation stream missing transcript line; events: %+v", events)
	}
}

// TestSpawnerOpensFreshOnStaleSession pins the resume-then-fallback
// path: when the persisted session ID resume fails (Helix reports
// hadStreamErr on a resume call), the spawner opens a fresh session
// and persists the new ID. H1.3c rewrites EnsureAndSend; this test
// ensures the fallback survives.
func TestSpawnerOpensFreshOnStaleSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, wid, "prj_test", "app_test", "repo_test"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), s, wid, "ses_stale"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &staleSessionFake{
		fakeHelixClient: fakeHelixClient{
			startSessionID: "ses_fresh",
			outputs:        []helixclient.Output{{Status: "complete", Output: "ok"}},
		},
	}
	cfg := newHelixCfg(t, &fc.fakeHelixClient, s)
	cfg.Client = fc
	sp := Spawner(cfg)
	if err := sp(context.Background(), wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e1"}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	state, _ := LoadState(context.Background(), s, wid)
	if state.SessionID != "ses_fresh" {
		t.Errorf("session pointer = %q, want ses_fresh (stale resume must fall through to fresh)", state.SessionID)
	}
}

// staleSessionFake reports hadStreamErr=true when a resume call is
// made (SessionID != "") — simulating Helix's "session no longer
// running" signal. Fresh opens (SessionID empty) succeed normally.
type staleSessionFake struct {
	fakeHelixClient
}

func (f *staleSessionFake) StartChatWithStatus(ctx context.Context, req helixclient.StartChatRequest) (helixclient.Session, bool, error) {
	s, err := f.StartChat(ctx, req)
	if req.SessionID != "" {
		// Resume path: report streamHadErr so SendToSession reports
		// "session no longer running" and EnsureAndSend falls through.
		return s, true, err
	}
	return s, false, err
}
