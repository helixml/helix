// Tests for the org-scoped (multi-tenant) store. The key invariants:
//
//   - Short human-readable IDs (`w-owner`, `p-root`, `r-owner`) repeat
//     across orgs — the composite PK (id, org_id) keeps writes
//     unambiguous.
//   - Every List takes orgID and returns only that org's rows.
//   - Every Get takes orgID and returns store.ErrNotFound when the
//     pair doesn't exist (even if the same id exists in another org).
//
// The cascade-on-org-delete behaviour is tested at the integration
// layer (see api/pkg/server/helix_org_cascade_test.go) where a real
// helix `organizations` row exists; the gorm test schemas don't have
// that table so the FK isn't installed here.
package gorm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

const (
	orgA = "org-acme"
	orgB = "org-globex"
)

func TestWorkers_SameIDAcrossOrgs(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	for _, org := range []string{orgA, orgB} {
		r, err := orgchart.NewRole("r-owner", "# Owner", nil, nil, time.Now().UTC(), org)
		if err != nil {
			t.Fatalf("orgchart.NewRole(%s): %v", org, err)
		}
		if err := s.Roles.Create(ctx, r); err != nil {
			t.Fatalf("Roles.Create(%s): %v", org, err)
		}
		pos, err := orgchart.NewPosition("p-root", "r-owner", nil, org)
		if err != nil {
			t.Fatalf("NewPosition(%s): %v", org, err)
		}
		if err := s.Positions.Create(ctx, pos); err != nil {
			t.Fatalf("Positions.Create(%s): %v", org, err)
		}
		w, err := orgchart.NewHumanWorker("w-owner", "p-root", "# Owner identity", org)
		if err != nil {
			t.Fatalf("NewHumanWorker(%s): %v", org, err)
		}
		if err := s.Workers.Create(ctx, w); err != nil {
			t.Fatalf("Workers.Create(%s): %v", org, err)
		}
	}

	// Same id, different orgs: both Get calls succeed and return the
	// org-scoped row.
	for _, org := range []string{orgA, orgB} {
		got, err := s.Workers.Get(ctx, org, "w-owner")
		if err != nil {
			t.Fatalf("Workers.Get(%s, w-owner): %v", org, err)
		}
		if got.OrganizationID() != org {
			t.Errorf("Workers.Get(%s, w-owner).OrganizationID = %q, want %q", org, got.OrganizationID(), org)
		}
	}
}

func TestWorkers_GetWrongOrgReturnsNotFound(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	mustSeedOwner(t, s, orgA)

	if _, err := s.Workers.Get(ctx, orgB, "w-owner"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Workers.Get(%s, w-owner) wantErr ErrNotFound, got %v", orgB, err)
	}
}

func TestWorkers_ListFiltersByOrg(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	mustSeedOwner(t, s, orgA)
	mustSeedOwner(t, s, orgB)

	for _, tc := range []struct {
		org  string
		want int
	}{
		{orgA, 1},
		{orgB, 1},
		{"org-missing", 0},
	} {
		got, err := s.Workers.List(ctx, tc.org)
		if err != nil {
			t.Fatalf("Workers.List(%s): %v", tc.org, err)
		}
		if len(got) != tc.want {
			t.Errorf("Workers.List(%s) = %d rows, want %d", tc.org, len(got), tc.want)
		}
		for _, w := range got {
			if w.OrganizationID() != tc.org {
				t.Errorf("Workers.List(%s) returned worker in org %q", tc.org, w.OrganizationID())
			}
		}
	}
}

func TestPositions_ListFiltersByOrg(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	mustSeedOwner(t, s, orgA)
	mustSeedOwner(t, s, orgB)

	got, err := s.Positions.List(ctx, orgA)
	if err != nil {
		t.Fatalf("Positions.List(%s): %v", orgA, err)
	}
	if len(got) != 1 || got[0].OrganizationID != orgA {
		t.Errorf("Positions.List(%s) = %+v, want one position in %s", orgA, got, orgA)
	}
}

func TestRoles_SameIDAcrossOrgs(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	mustSeedOwner(t, s, orgA)
	mustSeedOwner(t, s, orgB)

	for _, org := range []string{orgA, orgB} {
		got, err := s.Roles.Get(ctx, org, "r-owner")
		if err != nil {
			t.Fatalf("Roles.Get(%s, r-owner): %v", org, err)
		}
		if got.OrganizationID != org {
			t.Errorf("Roles.Get(%s) returned OrganizationID = %q", org, got.OrganizationID)
		}
	}
}

// mustSeedOwner installs the canonical owner trio (Role, Position,
// Worker) for the given org. Mirrors what bootstrap.Run would create
// at the JSON-API entry point.
func mustSeedOwner(t *testing.T, s *store.Store, orgID string) {
	t.Helper()
	ctx := context.Background()

	r, err := orgchart.NewRole("r-owner", "# Owner", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("orgchart.NewRole: %v", err)
	}
	if err := s.Roles.Create(ctx, r); err != nil {
		t.Fatalf("Roles.Create(%s): %v", orgID, err)
	}
	pos, err := orgchart.NewPosition("p-root", "r-owner", nil, orgID)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	if err := s.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("Positions.Create(%s): %v", orgID, err)
	}
	w, err := orgchart.NewHumanWorker("w-owner", "p-root", "# Owner", orgID)
	if err != nil {
		t.Fatalf("NewHumanWorker: %v", err)
	}
	if err := s.Workers.Create(ctx, w); err != nil {
		t.Fatalf("Workers.Create(%s): %v", orgID, err)
	}
}
