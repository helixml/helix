package helix

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/store/sqlite"
)

// fakeGitWriter is the minimum WorkspaceGit fake — captures the
// last write so tests can assert on path, branch, and content.
type fakeGitWriter struct {
	mu         sync.Mutex
	lastRepoID string
	lastPath   string
	lastBranch string
	lastBody   []byte
	lastMsg    string
	err        error
}

func (f *fakeGitWriter) CreateOrUpdateFileContents(_ context.Context, repoID, path, branch string, content []byte, commitMessage, _, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastRepoID = repoID
	f.lastPath = path
	f.lastBranch = branch
	f.lastBody = content
	f.lastMsg = commitMessage
	return "sha-test", f.err
}

func (f *fakeGitWriter) CreateBranch(_ context.Context, _, _, _ string) error { return nil }

func newSeededStore(t *testing.T, repoID string) (*store.Store, worker.ID) {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	r, _ := role.New("r-eng", "# Role", nil, nil, time.Now().UTC())
	_ = s.Roles.Create(ctx, r)
	pos, _ := domain.NewPosition("p-eng", "r-eng", nil)
	_ = s.Positions.Create(ctx, pos)
	w, _ := domain.NewAIWorker("w-eng", "p-eng", "# Persona")
	_ = s.Workers.Create(ctx, w)
	if repoID != "" {
		_ = SaveProject(ctx, s, w.ID(), "prj_x", "app_x", repoID)
	}
	return s, w.ID()
}

func TestWorkspaceWritesToWorkerRepo(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeGitWriter{}
	w := NewWorkspace(fc, s, "helix-specs", "helix-org", "ho@example.com")
	if err := w.MirrorFile(context.Background(), wid, "role.md", "# Role", "update_role: r-eng"); err != nil {
		t.Fatalf("MirrorFile: %v", err)
	}
	if fc.lastRepoID != "repo-1" {
		t.Errorf("repo: %q", fc.lastRepoID)
	}
	wantPath := "workers/" + string(wid) + "/.context/role.md"
	if fc.lastBranch != "helix-specs" || fc.lastPath != wantPath || string(fc.lastBody) != "# Role" {
		t.Errorf("req: repo=%q path=%q branch=%q body=%q (want path=%q)",
			fc.lastRepoID, fc.lastPath, fc.lastBranch, fc.lastBody, wantPath)
	}
}

func TestWorkspaceUnboundWorkerIsNoop(t *testing.T) {
	t.Parallel()
	// Worker without a Helix project — repoID empty.
	s, wid := newSeededStore(t, "")
	fc := &fakeGitWriter{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "role.md", "# Role", ""); err != nil {
		t.Fatalf("MirrorFile: %v", err)
	}
	if fc.lastRepoID != "" {
		t.Errorf("expected no write when worker has no repo, got %q", fc.lastRepoID)
	}
}

func TestWorkspaceSurfacesErrors(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeGitWriter{err: errors.New("boom")}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "role.md", "x", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkspaceRejectsBadName(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeGitWriter{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	for _, bad := range []string{"", "/role.md", "../role.md", "a/../b"} {
		if err := w.MirrorFile(context.Background(), wid, bad, "x", ""); err == nil {
			t.Errorf("name %q: expected error", bad)
		}
	}
	if fc.lastRepoID != "" {
		t.Errorf("expected no write on bad names, got %q", fc.lastRepoID)
	}
}

// TestWorkspaceEmptyWorkerIDError pins the input-validation contract.
func TestWorkspaceEmptyWorkerIDError(t *testing.T) {
	t.Parallel()
	s, _ := newSeededStore(t, "repo-1")
	fc := &fakeGitWriter{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), "", "role.md", "x", ""); err == nil {
		t.Fatal("expected error for empty workerID")
	}
}

// TestWorkspaceInvalidatesSessionOnRoleEdit verifies the warm-session
// invalidation: editing role.md clears the persisted SessionID so the
// next activation gets a fresh Claude context that re-reads role.md
// from scratch.
func TestWorkspaceInvalidatesSessionOnRoleEdit(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	if err := SaveSession(context.Background(), s, wid, "ses_warm"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeGitWriter{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "role.md", "# Role v2", ""); err != nil {
		t.Fatalf("MirrorFile: %v", err)
	}
	state, _ := LoadState(context.Background(), s, wid)
	if state.SessionID != "" {
		t.Errorf("session must be cleared after role edit; got %q", state.SessionID)
	}
}

// TestWorkspaceInvalidatesSessionOnIdentityEdit — same for identity.md.
func TestWorkspaceInvalidatesSessionOnIdentityEdit(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	if err := SaveSession(context.Background(), s, wid, "ses_warm"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeGitWriter{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "identity.md", "# Identity v2", ""); err != nil {
		t.Fatalf("MirrorFile: %v", err)
	}
	state, _ := LoadState(context.Background(), s, wid)
	if state.SessionID != "" {
		t.Errorf("session must be cleared after identity edit; got %q", state.SessionID)
	}
}

// TestWorkspaceDoesNotInvalidateOnOtherFiles — checkpoint pushes leave
// the warm session alone; only role.md and identity.md invalidate.
func TestWorkspaceDoesNotInvalidateOnOtherFiles(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	if err := SaveSession(context.Background(), s, wid, "ses_warm"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeGitWriter{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	if err := w.MirrorFile(context.Background(), wid, "notes.md", "# Notes", ""); err != nil {
		t.Fatalf("MirrorFile: %v", err)
	}
	state, _ := LoadState(context.Background(), s, wid)
	if state.SessionID != "ses_warm" {
		t.Errorf("warm session must be preserved for notes.md; got %q", state.SessionID)
	}
}

// TestWorkspaceSerialisesPerRepo verifies the per-repo lock: two
// concurrent MirrorFile calls against the same repo execute serially.
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
		t.Errorf("peak concurrent writes per repo = %d, want 1", got)
	}
}

type serialisingFake struct {
	gate     chan struct{}
	inflight *int32
	peak     *int32
}

func (f *serialisingFake) CreateBranch(_ context.Context, _, _, _ string) error { return nil }

func (f *serialisingFake) CreateOrUpdateFileContents(_ context.Context, _, _, _ string, _ []byte, _, _, _ string) (string, error) {
	cur := atomic.AddInt32(f.inflight, 1)
	defer atomic.AddInt32(f.inflight, -1)
	for {
		p := atomic.LoadInt32(f.peak)
		if cur <= p || atomic.CompareAndSwapInt32(f.peak, p, cur) {
			break
		}
	}
	<-f.gate
	return "sha", nil
}
