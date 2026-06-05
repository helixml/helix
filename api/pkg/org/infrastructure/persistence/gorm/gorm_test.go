package gorm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)
	return s
}

func TestRolesRoundTripAndUpdate(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	created := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	r, err := orgchart.NewRole("r-ceo", "# CEO\nTop of the org.", nil, nil, created, "org-test")
	if err != nil {
		t.Fatalf("NewRole: %v", err)
	}
	if err := s.Roles.Create(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Roles.Get(ctx, "org-test", "r-ceo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "# CEO\nTop of the org." {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not persisted: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}

	updated := orgchart.Role{
		ID:             got.ID,
		OrganizationID: "org-test",
		Content:        "# CEO\nNow with more verve.",
		CreatedAt:      got.CreatedAt,
		UpdatedAt:      created.Add(time.Hour),
	}
	if err := s.Roles.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = s.Roles.Get(ctx, "org-test", "r-ceo")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Content != "# CEO\nNow with more verve." {
		t.Fatalf("post-update content = %q", got.Content)
	}
	if !got.UpdatedAt.Equal(created.Add(time.Hour)) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, created.Add(time.Hour))
	}

	list, err := s.Roles.List(ctx, "org-test")
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
	_, err := s.Roles.Get(context.Background(), "org-test", "missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestPositionsRoundTripAndChildren(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	root, _ := orgchart.NewPosition("p-root", "r-owner", nil, "org-test")
	if err := s.Positions.Create(ctx, root); err != nil {
		t.Fatalf("Create root: %v", err)
	}
	rootID := root.ID
	child, _ := orgchart.NewPosition("p-ceo", "r-ceo", &rootID, "org-test")
	if err := s.Positions.Create(ctx, child); err != nil {
		t.Fatalf("Create child: %v", err)
	}

	got, err := s.Positions.Get(ctx, "org-test", "p-ceo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != "p-root" {
		t.Fatalf("parent = %v, want p-root", got.ParentID)
	}

	kids, err := s.Positions.ListChildren(ctx, "org-test", "p-root")
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

	human, err := orgchart.NewHumanWorker("w-owner", "p-root", "i am the owner", "org-test")
	if err != nil {
		t.Fatalf("NewHumanWorker: %v", err)
	}
	if err := s.Workers.Create(ctx, human); err != nil {
		t.Fatalf("Create human: %v", err)
	}

	ai, err := orgchart.NewAIWorker("w-ceo", "p-ceo", "you are the ceo", "org-test")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if err := s.Workers.Create(ctx, ai); err != nil {
		t.Fatalf("Create ai: %v", err)
	}

	gotHuman, err := s.Workers.Get(ctx, "org-test", "w-owner")
	if err != nil {
		t.Fatalf("Get human: %v", err)
	}
	if gotHuman.Kind() != orgchart.WorkerKindHuman {
		t.Fatalf("kind = %q, want human", gotHuman.Kind())
	}
	if _, ok := gotHuman.(*orgchart.HumanWorker); !ok {
		t.Fatalf("want *HumanWorker, got %T", gotHuman)
	}

	gotAI, err := s.Workers.Get(ctx, "org-test", "w-ceo")
	if err != nil {
		t.Fatalf("Get ai: %v", err)
	}
	if gotAI.Kind() != orgchart.WorkerKindAI {
		t.Fatalf("kind = %q, want ai", gotAI.Kind())
	}

}

// TestWorkerOrganizationIDRoundTrip: a Worker created with an OrgID
// round-trips through Create → Get with the OrgID preserved (composite
// (id, org_id) PK means lookups are scoped to the creating org).
func TestWorkerOrganizationIDRoundTrip(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	scoped, err := orgchart.NewAIWorker("w-acme-bot", "p-eng", "# bot", "org-acme")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if err := s.Workers.Create(ctx, scoped); err != nil {
		t.Fatalf("Create scoped: %v", err)
	}
	got, err := s.Workers.Get(ctx, "org-acme", "w-acme-bot")
	if err != nil {
		t.Fatalf("Get scoped: %v", err)
	}
	if got.OrganizationID() != "org-acme" {
		t.Errorf("round-tripped OrgID = %q, want org-acme", got.OrganizationID())
	}
}

