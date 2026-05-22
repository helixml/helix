package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/store/sqlite"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return s
}

func TestRolesRoundTripAndUpdate(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	created := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	r, err := role.New("r-ceo", "# CEO\nTop of the org.", nil, nil, created)
	if err != nil {
		t.Fatalf("NewRole: %v", err)
	}
	if err := s.Roles.Create(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Roles.Get(ctx, "r-ceo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "# CEO\nTop of the org." {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not persisted: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}

	updated := role.Role{
		ID:        got.ID,
		Content:   "# CEO\nNow with more verve.",
		CreatedAt: got.CreatedAt,
		UpdatedAt: created.Add(time.Hour),
	}
	if err := s.Roles.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = s.Roles.Get(ctx, "r-ceo")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Content != "# CEO\nNow with more verve." {
		t.Fatalf("post-update content = %q", got.Content)
	}
	if !got.UpdatedAt.Equal(created.Add(time.Hour)) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, created.Add(time.Hour))
	}

	list, err := s.Roles.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List length = %d, want 1", len(list))
	}
}

func TestRolesNotFound(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_, err := s.Roles.Get(context.Background(), "missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestPositionsRoundTripAndChildren(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	root, _ := domain.NewPosition("p-root", "r-owner", nil)
	if err := s.Positions.Create(ctx, root); err != nil {
		t.Fatalf("Create root: %v", err)
	}
	rootID := root.ID
	child, _ := domain.NewPosition("p-ceo", "r-ceo", &rootID)
	if err := s.Positions.Create(ctx, child); err != nil {
		t.Fatalf("Create child: %v", err)
	}

	got, err := s.Positions.Get(ctx, "p-ceo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != "p-root" {
		t.Fatalf("parent = %v, want p-root", got.ParentID)
	}

	kids, err := s.Positions.ListChildren(ctx, "p-root")
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(kids) != 1 || kids[0].ID != "p-ceo" {
		t.Fatalf("children = %+v, want [p-ceo]", kids)
	}
}

func TestWorkersHumanAndAI(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	human, err := domain.NewHumanWorker("w-owner", "p-root", "i am the owner")
	if err != nil {
		t.Fatalf("NewHumanWorker: %v", err)
	}
	if err := s.Workers.Create(ctx, human); err != nil {
		t.Fatalf("Create human: %v", err)
	}

	ai, err := domain.NewAIWorker("w-ceo", "p-ceo", "you are the ceo")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if err := s.Workers.Create(ctx, ai); err != nil {
		t.Fatalf("Create ai: %v", err)
	}

	gotHuman, err := s.Workers.Get(ctx, "w-owner")
	if err != nil {
		t.Fatalf("Get human: %v", err)
	}
	if gotHuman.Kind() != worker.KindHuman {
		t.Fatalf("kind = %q, want human", gotHuman.Kind())
	}
	if _, ok := gotHuman.(*domain.HumanWorker); !ok {
		t.Fatalf("want *HumanWorker, got %T", gotHuman)
	}

	gotAI, err := s.Workers.Get(ctx, "w-ceo")
	if err != nil {
		t.Fatalf("Get ai: %v", err)
	}
	if gotAI.Kind() != worker.KindAI {
		t.Fatalf("kind = %q, want ai", gotAI.Kind())
	}

}

// TestWorkerOrganizationIDRoundTrip pins H5.1 at the storage seam:
// a Worker stamped with OrgID via WithOrgID round-trips through
// Create → Get with the OrgID preserved. Workers without OrgID
// (today's single-tenant default) read back with empty OrgID.
func TestWorkerOrganizationIDRoundTrip(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	scoped, err := domain.NewAIWorker("w-acme-bot", "p-eng", "# bot")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if err := s.Workers.Create(ctx, scoped.WithOrgID("org-acme")); err != nil {
		t.Fatalf("Create scoped: %v", err)
	}
	got, err := s.Workers.Get(ctx, "w-acme-bot")
	if err != nil {
		t.Fatalf("Get scoped: %v", err)
	}
	if got.OrganizationID() != "org-acme" {
		t.Errorf("round-tripped OrgID = %q, want org-acme", got.OrganizationID())
	}

	// Sanity: a Worker created without WithOrgID reads back empty
	// (single-tenant alpha — no migration of legacy rows needed).
	unscoped, _ := domain.NewAIWorker("w-global", "p-eng", "# global")
	if err := s.Workers.Create(ctx, unscoped); err != nil {
		t.Fatalf("Create unscoped: %v", err)
	}
	g, _ := s.Workers.Get(ctx, "w-global")
	if g.OrganizationID() != "" {
		t.Errorf("unscoped OrgID = %q, want empty", g.OrganizationID())
	}
}

func TestGrants(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	g, err := domain.NewToolGrant("g-1", "w-ceo", "hire_worker")
	if err != nil {
		t.Fatalf("NewToolGrant: %v", err)
	}
	if err := s.Grants.Create(ctx, g); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Grants.Get(ctx, "g-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ToolName != "hire_worker" {
		t.Fatalf("tool = %q", got.ToolName)
	}

	list, err := s.Grants.ListByWorker(ctx, "w-ceo")
	if err != nil {
		t.Fatalf("ListByWorker: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}

	if err := s.Grants.Delete(ctx, "g-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Grants.Get(ctx, "g-1")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after delete err = %v, want ErrNotFound", err)
	}
}
