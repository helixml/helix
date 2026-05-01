package helixspecs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools/helixclient"
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
		_ = s.Workers.SetHelixProject(ctx, w.ID(), "prj_x", "app_x", repoID)
	}
	return s, w.ID()
}

func TestPublisherWritesToWorkerRepo(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeClient{}
	p := NewPerWorker(fc, s, "helix-specs", "helix-org", "ho@example.com")
	if err := p.PublishFile(context.Background(), wid, "job/role.md", "# Role", "update_role: r-eng"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if fc.lastRepoID != "repo-1" {
		t.Errorf("repo: %q", fc.lastRepoID)
	}
	if fc.lastReq.Branch != "helix-specs" || fc.lastReq.Path != "job/role.md" || fc.lastReq.Content != "# Role" {
		t.Errorf("req: %+v", fc.lastReq)
	}
}

func TestPublisherUnboundWorkerIsNoop(t *testing.T) {
	t.Parallel()
	// Worker without a Helix project — repoID empty.
	s, wid := newSeededStore(t, "")
	fc := &fakeClient{}
	p := NewPerWorker(fc, s, "helix-specs", "", "")
	if err := p.PublishFile(context.Background(), wid, "job/role.md", "# Role", ""); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if fc.lastRepoID != "" {
		t.Errorf("expected no PutFile when worker has no repo, got %q", fc.lastRepoID)
	}
}

func TestPublisherSurfacesErrors(t *testing.T) {
	t.Parallel()
	s, wid := newSeededStore(t, "repo-1")
	fc := &fakeClient{err: errors.New("boom")}
	p := NewPerWorker(fc, s, "helix-specs", "", "")
	if err := p.PublishFile(context.Background(), wid, "job/role.md", "x", ""); err == nil {
		t.Fatal("expected error")
	}
}
