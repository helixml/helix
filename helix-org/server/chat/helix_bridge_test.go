package chat

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/tools/helixclient"
)

// fakeEnsurer is a fixed ProjectEnsurer that returns canned IDs so
// the bridge tests don't need a Helix or a store.
type fakeEnsurer struct {
	projectID, agentAppID, repoID string
}

func (f *fakeEnsurer) Ensure(_ context.Context, _ domain.WorkerID) (string, string, string, error) {
	return f.projectID, f.agentAppID, f.repoID, nil
}

// fakeChatClient is a minimum-viable helixclient.Client used by the
// helix bridge tests. Captures StartChat / PostFollowup calls so the
// test can assert the bridge persists the session ID and switches
// to follow-up on subsequent messages.
type fakeChatClient struct {
	helixclient.Client
	startCalls     int
	lastStartReq   helixclient.StartChatRequest
	startSessionID string
}

func (f *fakeChatClient) StartChat(_ context.Context, req helixclient.StartChatRequest) (helixclient.Session, error) {
	f.startCalls++
	f.lastStartReq = req
	if f.startSessionID == "" {
		f.startSessionID = "ses_test_1"
	}
	return helixclient.Session{ID: f.startSessionID}, nil
}

func (f *fakeChatClient) StartChatWithStatus(ctx context.Context, req helixclient.StartChatRequest) (helixclient.Session, bool, error) {
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

func (f *fakeChatClient) SubscribeUpdates(ctx context.Context, _ string) (<-chan helixclient.SessionUpdate, error) {
	ch := make(chan helixclient.SessionUpdate)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func newTestHelixBridge(t *testing.T, fc *fakeChatClient) *HelixBridge {
	t.Helper()
	b, err := NewHelix(HelixConfig{
		Client:      fc,
		Ensure:      &fakeEnsurer{projectID: "prj_x", agentAppID: "app_x"},
		OwnerID:     "w-owner",
		SessionRole: "owner-chat",
		AgentType:   "helix_basic",
		CWD:         t.TempDir(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewHelix: %v", err)
	}
	return b
}

// TestHelixBridgeStartsThenFollowsUp verifies the core invariant: the
// first Send opens a fresh Helix session, subsequent Sends continue
// the same session_id. (The bridge issues both via StartChat — the
// followup path sets SessionID — so provider/model ride along.)
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
	if fc.startCalls != 1 || fc.lastStartReq.SessionID != "" {
		t.Errorf("first turn: startCalls=%d sid=%q (want 1, empty)", fc.startCalls, fc.lastStartReq.SessionID)
	}

	resp2 := post("again")
	resp2.Body.Close() //nolint:errcheck,gosec // test cleanup
	if fc.startCalls != 2 {
		t.Errorf("second turn StartChat calls: %d (want 2)", fc.startCalls)
	}
	if fc.lastStartReq.SessionID != "ses_42" {
		t.Errorf("followup SessionID: %q (want ses_42)", fc.lastStartReq.SessionID)
	}
	if got, want := fc.lastStartReq.Messages[0].Content.Parts[0], "again"; got != want {
		t.Errorf("followup msg: got %v want %v", got, want)
	}
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
	if fc.startCalls != 2 {
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
