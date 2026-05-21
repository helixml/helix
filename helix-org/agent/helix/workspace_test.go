package helix

import (
	"context"
	"errors"
	"testing"
	"time"

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

func newSeededStore(t *testing.T, repoID string) (*store.Store, domain.WorkerID) {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	role, _ := domain.NewRole("r-eng", "# Role", time.Now().UTC())
	_ = s.Roles.Create(ctx, role)
	pos, _ := domain.NewPosition("p-eng", "r-eng", nil)
	_ = s.Positions.Create(ctx, pos)
	w, _ := domain.NewAIWorker("w-eng", []domain.PositionID{"p-eng"}, "# Persona")
	_ = s.Workers.Create(ctx, w)
	if repoID != "" {
		_ = SaveProject(ctx, s, w.ID(), "prj_x", "app_x", repoID)
	}
	return s, w.ID()
}

func TestWorkspaceWritesToWorkerRepo(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "helix-org", "ho@example.com")
	if err := w.PublishFile(context.Background(), wid, "role.md", "# Role", "update_role: r-eng"); err != nil {
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
	if err := w.PublishFile(context.Background(), wid, "role.md", "# Role", ""); err != nil {
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
	if err := w.PublishFile(context.Background(), wid, "role.md", "x", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestWorkspaceRejectsBadName(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeClient{}
	w := NewWorkspace(fc, s, "helix-specs", "", "")
	for _, bad := range []string{"", "/role.md", "../role.md", "a/../b"} {
		if err := w.PublishFile(context.Background(), wid, bad, "x", ""); err == nil {
			t.Errorf("name %q: expected error", bad)
		}
	}
	if fc.lastRepoID != "" {
		t.Errorf("expected no PutFile on bad names, got %q", fc.lastRepoID)
	}
}
