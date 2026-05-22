package chat

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/helixclient"
	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// fakeEnsurer is a fixed ProjectEnsurer that returns canned IDs so
// the bridge tests don't need a Helix or a store.
type fakeEnsurer struct {
	projectID, agentAppID, repoID string
}

func (f *fakeEnsurer) Ensure(_ context.Context, _ worker.ID) (string, string, string, error) {
	return f.projectID, f.agentAppID, f.repoID, nil
}

// fakeChatClient is a minimum-viable helixclient.Client used by the
// helix bridge tests. Captures StartChat / PostFollowup calls so the
// test can assert the bridge persists the session ID and switches
// to follow-up on subsequent messages.
type fakeChatClient struct {
	helixclient.Client
	startCalls     int
	sendCalls      int
	lastStartReq   runtimehelix.StartChatRequest
	lastSendSID    string
	lastSendBody   string
	startSessionID string
}

func (f *fakeChatClient) SendSessionMessage(_ context.Context, sid, content string, _ runtimehelix.SendMessageOptions) (runtimehelix.SendMessageResponse, error) {
	f.sendCalls++
	f.lastSendSID = sid
	f.lastSendBody = content
	return runtimehelix.SendMessageResponse{RequestID: "req_x", InteractionID: "ix_x"}, nil
}

func (f *fakeChatClient) ServerStatus(_ context.Context) (runtimehelix.ServerStatus, error) {
	return runtimehelix.ServerStatus{MaxConcurrentDesktops: 0}, nil // 0 = unlimited
}

func (f *fakeChatClient) StartChat(_ context.Context, req runtimehelix.StartChatRequest) (types.Session, error) {
	f.startCalls++
	f.lastStartReq = req
	if f.startSessionID == "" {
		f.startSessionID = "ses_test_1"
	}
	// Mirror the real client's behaviour: invoke OnSessionID the
	// moment the session ID is known. Without this, the bridge's
	// attachSession is never wired and follow-up sends can't see the
	// persisted SessionID.
	if req.OnSessionID != nil {
		req.OnSessionID(f.startSessionID)
	}
	return types.Session{ID: f.startSessionID}, nil
}

func (f *fakeChatClient) StartChatWithStatus(ctx context.Context, req runtimehelix.StartChatRequest) (types.Session, bool, error) {
	s, err := f.StartChat(ctx, req)
	return s, false, err
}

func (f *fakeChatClient) CreateGitRepo(_ context.Context, req helixclient.CreateGitRepoRequest) (helixclient.GitRepo, error) {
	return helixclient.GitRepo{ID: "repo-" + req.Name, Name: req.Name}, nil
}

func (f *fakeChatClient) AttachRepoToProject(_ context.Context, _, _ string, _ bool) error {
	return nil
}

func (f *fakeChatClient) CreateBranch(_ context.Context, _, _, _ string) error { return nil }

func (f *fakeChatClient) GetProject(_ context.Context, id string) (helixclient.Project, error) {
	return helixclient.Project{ID: id, OrganizationID: "org-test"}, nil
}

func (f *fakeChatClient) SubscribeUpdates(ctx context.Context, _ string) (<-chan types.WebsocketEvent, error) {
	ch := make(chan types.WebsocketEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (f *fakeChatClient) StopExternalAgent(_ context.Context, _ string) error { return nil }
func (f *fakeChatClient) GetSession(_ context.Context, _ string) (types.Session, error) {
	return types.Session{}, nil
}

func newTestHelixBridge(t *testing.T, fc *fakeChatClient) *HelixBridge {
	t.Helper()
	b, err := NewHelix(HelixConfig{
		Client:      fc,
		Ensure:      &fakeEnsurer{projectID: "prj_x", agentAppID: "app_x"},
		WorkerID:    "w-owner",
		SessionRole: "exploratory",
		CWD:         t.TempDir(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewHelix: %v", err)
	}
	return b
}

// TestHelixBridgeStartsThenFollowsUp verifies the core invariant: the
// first Send opens a fresh Helix session via /sessions/chat (no
// SessionID in request), subsequent Sends resume that same session
// (SessionID set in request). EnsureAndSend's behaviour: both fresh
// and resume go through StartChatWithStatus; the SessionID field on
// the request distinguishes them.
func TestHelixBridgeStartsThenFollowsUp(t *testing.T) {
	t.Parallel()
	fc := &fakeChatClient{startSessionID: "ses_42"}
	b := newTestHelixBridge(t, fc)
	srv := httptest.NewServer(b.SendHandler())
	defer srv.Close()

	post := func(msg string) *http.Response {
		resp, err := http.PostForm(srv.URL, url.Values{"message": {msg}})
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		return resp
	}

	resp1 := post("hello")
	if resp1.StatusCode != 200 {
		t.Fatalf("first send: %d", resp1.StatusCode)
	}
	body, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close() //nolint:errcheck,gosec // test cleanup
	if !strings.Contains(string(body), "hello") {
		t.Errorf("expected user-bubble echo, got %q", body)
	}
	// First turn: fresh open. The bridge runs sends on a detached
	// goroutine, so spin until we observe the call.
	if !waitFor(func() bool { return fc.startCalls >= 1 }) {
		t.Fatalf("first turn never reached StartChat (got %d)", fc.startCalls)
	}
	if fc.lastStartReq.SessionID != "" {
		t.Errorf("first turn SessionID = %q, want empty (fresh open)", fc.lastStartReq.SessionID)
	}

	resp2 := post("again")
	resp2.Body.Close() //nolint:errcheck,gosec // test cleanup
	// Second turn must resume — observe a second StartChatWithStatus
	// call with SessionID = ses_42.
	if !waitFor(func() bool { return fc.startCalls >= 2 && fc.lastStartReq.SessionID == "ses_42" }) {
		t.Fatalf("followup did not resume: startCalls=%d lastSID=%q", fc.startCalls, fc.lastStartReq.SessionID)
	}
}

// waitFor polls predicate p for up to 1s. Returns true if p returns
// true before the timeout. The chat bridge's sends run on a detached
// goroutine, so tests need to wait for side-effects rather than
// asserting synchronously after the HTTP request returns.
func waitFor(p func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if p() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return p()
}

// TestHelixBridgeNewResetsSession verifies that POST /ui/chat/new
// clears the session pointer so the next Send opens a fresh Helix
// session rather than following up on the prior one.
func TestHelixBridgeNewResetsSession(t *testing.T) {
	t.Parallel()
	fc := &fakeChatClient{startSessionID: "ses_a"}
	b := newTestHelixBridge(t, fc)
	send := httptest.NewServer(b.SendHandler())
	newSrv := httptest.NewServer(b.NewHandler())
	defer send.Close()
	defer newSrv.Close()

	if r, _ := http.PostForm(send.URL, url.Values{"message": {"first"}}); r != nil {
		r.Body.Close() //nolint:errcheck,gosec // test cleanup
	}
	// Click "New chat".
	if r, _ := http.PostForm(newSrv.URL, url.Values{}); r != nil {
		r.Body.Close() //nolint:errcheck,gosec // test cleanup
	}
	if !b.HistoryStartsFresh() {
		t.Errorf("HistoryStartsFresh = false after New (want true)")
	}
	// Next send should open a brand-new session.
	fc.startSessionID = "ses_b"
	if r, _ := http.PostForm(send.URL, url.Values{"message": {"second"}}); r != nil {
		r.Body.Close() //nolint:errcheck,gosec // test cleanup
	}
	if !waitFor(func() bool { return fc.startCalls >= 2 }) {
		t.Errorf("StartChat calls: %d (want 2)", fc.startCalls)
	}
}

func TestHelixBridgeRejectsMissingConfig(t *testing.T) {
	t.Parallel()
	if _, err := NewHelix(HelixConfig{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := NewHelix(HelixConfig{Client: &fakeChatClient{}}); err == nil {
		t.Fatal("expected error")
	}
}

// TestHelixBridgeResumesPersistedSessionOnBoot verifies the
// LoadSessionID hook: on the first send after process boot, the
// bridge picks up the previously-persisted session pointer and
// resumes (rather than opening a fresh container).
func TestHelixBridgeResumesPersistedSessionOnBoot(t *testing.T) {
	t.Parallel()
	fc := &fakeChatClient{startSessionID: "ses_persisted"}
	b, err := NewHelix(HelixConfig{
		Client:      fc,
		Ensure:      &fakeEnsurer{projectID: "prj_x", agentAppID: "app_x"},
		WorkerID:    "w-owner",
		SessionRole: "exploratory",
		CWD:         t.TempDir(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		LoadSessionID: func(_ context.Context, _ worker.ID) (string, error) {
			return "ses_persisted", nil
		},
		SaveSessionID: func(_ context.Context, _ worker.ID, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("NewHelix: %v", err)
	}
	srv := httptest.NewServer(b.SendHandler())
	defer srv.Close()
	r, _ := http.PostForm(srv.URL, url.Values{"message": {"hello"}})
	if r != nil {
		r.Body.Close() //nolint:errcheck,gosec // test cleanup
	}
	// First send after boot must resume the persisted session — the
	// StartChatRequest carries SessionID = ses_persisted.
	if !waitFor(func() bool { return fc.startCalls >= 1 && fc.lastStartReq.SessionID == "ses_persisted" }) {
		t.Fatalf("first send did not resume persisted session: startCalls=%d sid=%q", fc.startCalls, fc.lastStartReq.SessionID)
	}
}

// TestHelixBridgePersistsSessionIDOnFreshOpen verifies the
// SaveSessionID hook fires on a fresh open: the bridge saves the new
// pointer so a restart can pick it up.
func TestHelixBridgePersistsSessionIDOnFreshOpen(t *testing.T) {
	t.Parallel()
	fc := &fakeChatClient{startSessionID: "ses_fresh"}
	var (
		savedMu sync.Mutex
		saved   string
	)
	b, err := NewHelix(HelixConfig{
		Client:      fc,
		Ensure:      &fakeEnsurer{projectID: "prj_x", agentAppID: "app_x"},
		WorkerID:    "w-owner",
		SessionRole: "exploratory",
		CWD:         t.TempDir(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		SaveSessionID: func(_ context.Context, _ worker.ID, sid string) error {
			savedMu.Lock()
			saved = sid
			savedMu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewHelix: %v", err)
	}
	srv := httptest.NewServer(b.SendHandler())
	defer srv.Close()
	r, _ := http.PostForm(srv.URL, url.Values{"message": {"hello"}})
	if r != nil {
		r.Body.Close() //nolint:errcheck,gosec // test cleanup
	}
	if !waitFor(func() bool {
		savedMu.Lock()
		defer savedMu.Unlock()
		return saved == "ses_fresh"
	}) {
		savedMu.Lock()
		defer savedMu.Unlock()
		t.Fatalf("SaveSessionID was not called with the fresh ID; got %q", saved)
	}
}

// TestHelixBridgeRecordsActivationRow pins B5.11: every chat send
// persists an Activation row keyed to the target Worker, with the
// row Completed by the time the goroutine returns. Mirrors what the
// AI-Worker Spawner does in B5.6 — the chat surface uses the same
// audit shape regardless of which Worker is the target.
func TestHelixBridgeRecordsActivationRow(t *testing.T) {
	t.Parallel()
	fc := &fakeChatClient{startSessionID: "ses_42"}

	repo := &fakeActivationRepo{}
	b, err := NewHelix(HelixConfig{
		Client:      fc,
		Ensure:      &fakeEnsurer{projectID: "prj_x", agentAppID: "app_x"},
		WorkerID:    "w-owner",
		SessionRole: "exploratory",
		CWD:         t.TempDir(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Activations: repo,
		NewID:       func() string { return "id-1" },
		Now:         func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewHelix: %v", err)
	}
	srv := httptest.NewServer(b.SendHandler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL, url.Values{"message": {"hello"}})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close() //nolint:errcheck,gosec // test cleanup

	if !waitFor(func() bool { return repo.completedCount() == 1 }) {
		t.Fatalf("activation row never completed; created=%d, completed=%d", repo.createdCount(), repo.completedCount())
	}
	if got := repo.createdCount(); got != 1 {
		t.Fatalf("created rows = %d, want 1", got)
	}
	created := repo.firstCreated()
	if created.ID != "a-id-1" {
		t.Errorf("activation.ID = %q, want a-id-1", created.ID)
	}
	if created.WorkerID != "w-owner" {
		t.Errorf("activation.WorkerID = %q, want w-owner", created.WorkerID)
	}
	if created.TranscriptStreamID != activation.StreamID("w-owner") {
		t.Errorf("activation.TranscriptStreamID = %q, want %q", created.TranscriptStreamID, activation.StreamID("w-owner"))
	}
	if len(created.Triggers) == 0 {
		t.Fatal("created row has no triggers")
	}
	completed := repo.firstCompleted()
	if completed.id != "a-id-1" {
		t.Errorf("completed.id = %q, want a-id-1", completed.id)
	}
	if completed.outcome.Status != activation.StatusOK {
		t.Errorf("completed.Outcome.Status = %q, want ok", completed.outcome.Status)
	}
}

// fakeActivationRepo records Create + Complete calls for chat tests.
// Implements activation.Repository.
type fakeActivationRepo struct {
	mu        sync.Mutex
	created   []*activation.Activation
	completed []fakeActivationComplete
}

type fakeActivationComplete struct {
	id      activation.ID
	outcome activation.Outcome
	endedAt time.Time
}

func (r *fakeActivationRepo) Create(_ context.Context, a *activation.Activation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, a)
	return nil
}

func (r *fakeActivationRepo) Complete(_ context.Context, id activation.ID, outcome activation.Outcome, endedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completed = append(r.completed, fakeActivationComplete{id: id, outcome: outcome, endedAt: endedAt})
	return nil
}

func (r *fakeActivationRepo) Get(_ context.Context, _ activation.ID) (*activation.Activation, error) {
	return nil, nil
}

func (r *fakeActivationRepo) ListForWorker(_ context.Context, _ worker.ID, _ int) ([]*activation.Activation, error) {
	return nil, nil
}

func (r *fakeActivationRepo) createdCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.created)
}

func (r *fakeActivationRepo) completedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.completed)
}

func (r *fakeActivationRepo) firstCreated() *activation.Activation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created[0]
}

func (r *fakeActivationRepo) firstCompleted() fakeActivationComplete {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.completed[0]
}

// TestHelixBridgeIsWorkerAgnostic pins B5.12: the bridge's code path
// is the same whether the target Worker is the owner or any other
// hired AI Worker. Constructing a bridge with WorkerID="w-alice"
// produces an Activation row keyed to w-alice, not w-owner — proof
// that there is no special-case branch for the owner Worker
// anywhere in the bridge.
func TestHelixBridgeIsWorkerAgnostic(t *testing.T) {
	t.Parallel()
	fc := &fakeChatClient{startSessionID: "ses_alice"}
	repo := &fakeActivationRepo{}
	b, err := NewHelix(HelixConfig{
		Client:      fc,
		Ensure:      &fakeEnsurer{projectID: "prj_alice", agentAppID: "app_alice"},
		WorkerID:    "w-alice",
		SessionRole: "exploratory",
		CWD:         t.TempDir(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Activations: repo,
		NewID:       func() string { return "alice-1" },
		Now:         func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewHelix: %v", err)
	}
	srv := httptest.NewServer(b.SendHandler())
	defer srv.Close()

	resp, err := http.PostForm(srv.URL, url.Values{"message": {"hi alice"}})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close() //nolint:errcheck,gosec // test cleanup

	if !waitFor(func() bool { return repo.completedCount() == 1 }) {
		t.Fatalf("activation never recorded for w-alice: created=%d completed=%d",
			repo.createdCount(), repo.completedCount())
	}
	created := repo.firstCreated()
	if created.WorkerID != "w-alice" {
		t.Errorf("created.WorkerID = %q, want w-alice (the chat target)", created.WorkerID)
	}
	if created.TranscriptStreamID != activation.StreamID("w-alice") {
		t.Errorf("created.TranscriptStreamID = %q, want %q",
			created.TranscriptStreamID, activation.StreamID("w-alice"))
	}
}
