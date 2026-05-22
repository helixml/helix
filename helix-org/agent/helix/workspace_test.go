package helix

import (
	"context"
	"errors"
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

type fakeClient struct {
	helixclient.Client
	lastRepoID string
	lastReq    helixclient.PutFileRequest
	err        error
}

func (f *fakeClient) PutFile(_ context.Context, repoID string, req helixclient.PutFileRequest) error {
	f.lastRepoID = repoID
	f.lastReq = req
	return f.err
}

func newSeededStore(t *testing.T, repoID string) (*store.Store, worker.ID) {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	role, _ := role.New("r-eng", "# Role", nil, nil, time.Now().UTC())
	_ = s.Roles.Create(ctx, role)
	pos, _ := domain.NewPosition("p-eng", "r-eng", nil)
	_ = s.Positions.Create(ctx, pos)
	w, _ := domain.NewAIWorker("w-eng", []position.ID{"p-eng"}, "# Persona")
	_ = s.Workers.Create(ctx, w)
	if repoID != "" {
		_ = runtimehelix.SaveProject(ctx, s, w.ID(), "prj_x", "app_x", repoID)
	}
	return s, w.ID()
}

func TestWorkspaceWritesToWorkerRepo(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "helix-org", "ho@example.com")
	if err := w.MirrorFile(context.Background(), wid, "role.md", "# Role", "update_role: r-eng"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if fc.lastRepoID != "repo-1" {
		t.Errorf("repo: %q", fc.lastRepoID)
	}
	wantPath := "workers/" + string(wid) + "/.context/role.md"
	if fc.lastReq.Branch != "helix-specs" || fc.lastReq.Path != wantPath || fc.lastReq.Content != "# Role" {
		t.Errorf("req: %+v (want path=%q)", fc.lastReq, wantPath)
	}
}

func TestWorkspaceUnboundWorkerIsNoop(t *testing.T) {
	t.Parallel()
	// Worker without a Helix project — repoID empty.
	s, wid := newSeededStore(t, "")
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "role.md", "# Role", ""); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if fc.lastRepoID != "" {
		t.Errorf("expected no PutFile when worker has no repo, got %q", fc.lastRepoID)
	}
}

func TestWorkspaceSurfacesErrors(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeClient{err: errors.New("boom")}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "role.md", "x", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkspaceRejectsBadName(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	for _, bad := range []string{"", "/role.md", "../role.md", "a/../b"} {
		if err := w.MirrorFile(context.Background(), wid, bad, "x", ""); err == nil {
			t.Errorf("name %q: expected error", bad)
		}
	}
	if fc.lastRepoID != "" {
		t.Errorf("expected no PutFile on bad names, got %q", fc.lastRepoID)
	}
}

// TestWorkspaceEmptyWorkerIDError pins the input-validation contract.
func TestWorkspaceEmptyWorkerIDError(t *testing.T) {
	t.Parallel()
	s, _ := newSeededStore(t, "repo-1")
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), "", "role.md", "x", ""); err == nil {
		t.Fatal("expected error for empty workerID")
	}
}

// TestWorkspaceInvalidatesSessionOnRoleEdit verifies the warm-session
// invalidation: editing role.md clears the persisted SessionID so the
// next activation gets a fresh Claude context that re-reads role.md
// from scratch. Critical — without this, update_role hot edits land
// in the helix-specs branch but the agent keeps using its cached
// in-memory version from the first activation.
func TestWorkspaceInvalidatesSessionOnRoleEdit(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	if err := runtimehelix.SaveSession(context.Background(), s, wid, "ses_warm"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "role.md", "# Role v2", ""); err != nil {
		t.Fatalf("MirrorFile: %v", err)
	}
	state, _ := runtimehelix.LoadState(context.Background(), s, wid)
	if state.SessionID != "" {
		t.Errorf("session must be cleared after role edit; got %q", state.SessionID)
	}
}

// TestWorkspaceInvalidatesSessionOnIdentityEdit — same as above for
// identity.md.
func TestWorkspaceInvalidatesSessionOnIdentityEdit(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	if err := runtimehelix.SaveSession(context.Background(), s, wid, "ses_warm"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "identity.md", "# Identity v2", ""); err != nil {
		t.Fatalf("MirrorFile: %v", err)
	}
	state, _ := runtimehelix.LoadState(context.Background(), s, wid)
	if state.SessionID != "" {
		t.Errorf("session must be cleared after identity edit; got %q", state.SessionID)
	}
}

// TestWorkspaceDoesNotInvalidateOnOtherFiles checkpoint pushes /
// arbitrary other files leave the warm session alone — only role.md
// and identity.md trigger invalidation.
func TestWorkspaceDoesNotInvalidateOnOtherFiles(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	if err := runtimehelix.SaveSession(context.Background(), s, wid, "ses_warm"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "notes.md", "# Notes", ""); err != nil {
		t.Fatalf("MirrorFile: %v", err)
	}
	state, _ := runtimehelix.LoadState(context.Background(), s, wid)
	if state.SessionID != "ses_warm" {
		t.Errorf("warm session must be preserved for notes.md; got %q", state.SessionID)
	}
}

// TestWorkspaceSerialisesPerRepo verifies the per-repo lock: two
// concurrent MirrorFile calls against the same repo execute
// serially — Helix's git write path is not concurrency-safe per repo.
// We measure peak concurrency seen inside PutFile.
func TestWorkspaceSerialisesPerRepo(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	gate := make(chan struct{})
	var inflight, peak int32
	fc := &serialisingFake{
		gate:     gate,
		inflight: &inflight,
		peak:     &peak,
	}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.MirrorFile(context.Background(), wid, "notes.md", "x", "")
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if got := atomic.LoadInt32(&peak); got > 1 {
		t.Errorf("peak concurrent PutFile calls per repo = %d, want 1", got)
	}
}

type serialisingFake struct {
	helixclient.Client
	gate     chan struct{}
	inflight *int32
	peak     *int32
}

func (f *serialisingFake) PutFile(_ context.Context, _ string, _ helixclient.PutFileRequest) error {
	cur := atomic.AddInt32(f.inflight, 1)
	defer atomic.AddInt32(f.inflight, -1)
	for {
		p := atomic.LoadInt32(f.peak)
		if cur <= p || atomic.CompareAndSwapInt32(f.peak, p, cur) {
			break
		}
	}
	<-f.gate
	return nil
}
