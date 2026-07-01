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

func TestBotsRoundTripAndUpdate(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	created := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	b, err := orgchart.NewBot("b-ceo", "# CEO\nTop of the org.", nil, created, "org-test")
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	if err := s.Bots.Create(ctx, b); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Bots.Get(ctx, "org-test", "b-ceo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "# CEO\nTop of the org." {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not persisted: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}

	updated := got.WithContent("# CEO\nNow with more verve.").WithUpdatedAt(created.Add(time.Hour))
	if err := s.Bots.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = s.Bots.Get(ctx, "org-test", "b-ceo")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Content != "# CEO\nNow with more verve." {
		t.Fatalf("post-update content = %q", got.Content)
	}
	if !got.UpdatedAt.Equal(created.Add(time.Hour)) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, created.Add(time.Hour))
	}

	list, err := s.Bots.List(ctx, "org-test")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List length = %d, want 1", len(list))
	}
}

func TestBotsNotFound(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_, err := s.Bots.Get(context.Background(), "org-test", "missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

// TestBotReportingHierarchy round-trips a reporting line through the
// store and confirms deleting the manager cascades the line away.
func TestBotReportingHierarchy(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	owner, err := orgchart.NewBot("b-owner", "# Owner", nil, now, "org-test")
	if err != nil {
		t.Fatalf("NewBot owner: %v", err)
	}
	if err := s.Bots.Create(ctx, owner); err != nil {
		t.Fatalf("Create owner: %v", err)
	}
	child, err := orgchart.NewBot("b-ceo", "# CEO", nil, now, "org-test")
	if err != nil {
		t.Fatalf("NewBot child: %v", err)
	}
	if err := s.Bots.Create(ctx, child); err != nil {
		t.Fatalf("Create child: %v", err)
	}
	line, err := orgchart.NewReportingLine("org-test", "b-owner", "b-ceo")
	if err != nil {
		t.Fatalf("NewReportingLine: %v", err)
	}
	if err := s.ReportingLines.Add(ctx, line); err != nil {
		t.Fatalf("Add reporting line: %v", err)
	}

	managers, err := s.ReportingLines.ListManagers(ctx, "org-test", "b-ceo")
	if err != nil {
		t.Fatalf("ListManagers: %v", err)
	}
	if len(managers) != 1 || managers[0] != "b-owner" {
		t.Fatalf("managers = %v, want [b-owner]", managers)
	}

	// Deleting the manager cascades the line away.
	if err := s.Bots.Delete(ctx, "org-test", "b-owner"); err != nil {
		t.Fatalf("Delete owner: %v", err)
	}
	managers, err = s.ReportingLines.ListManagers(ctx, "org-test", "b-ceo")
	if err != nil {
		t.Fatalf("ListManagers after delete: %v", err)
	}
	if len(managers) != 0 {
		t.Fatalf("managers = %v after deleting manager, want none (cascade)", managers)
	}
}

// TestBotOrganizationIDRoundTrip: a Bot created with an OrgID
// round-trips through Create → Get with the OrgID preserved (composite
// (id, org_id) PK means lookups are scoped to the creating org).
func TestBotOrganizationIDRoundTrip(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	scoped, err := orgchart.NewBot("b-acme-bot", "# bot", nil, time.Now().UTC(), "org-acme")
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	if err := s.Bots.Create(ctx, scoped); err != nil {
		t.Fatalf("Create scoped: %v", err)
	}
	got, err := s.Bots.Get(ctx, "org-acme", "b-acme-bot")
	if err != nil {
		t.Fatalf("Get scoped: %v", err)
	}
	if got.OrganizationID != "org-acme" {
		t.Errorf("round-tripped OrgID = %q, want org-acme", got.OrganizationID)
	}
}
