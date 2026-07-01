// Tests for the org-scoped (multi-tenant) store. The key invariants:
//
//   - Short human-readable IDs (`b-owner`, `p-root`) repeat across orgs
//     — the composite PK (id, org_id) keeps writes unambiguous.
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

func TestBots_SameIDAcrossOrgs(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	for _, org := range []string{orgA, orgB} {
		b, err := orgchart.NewBot("b-owner", "# Owner", nil, time.Now().UTC(), org)
		if err != nil {
			t.Fatalf("orgchart.NewBot(%s): %v", org, err)
		}
		if err := s.Bots.Create(ctx, b); err != nil {
			t.Fatalf("Bots.Create(%s): %v", org, err)
		}
	}

	// Same id, different orgs: both Get calls succeed and return the
	// org-scoped row.
	for _, org := range []string{orgA, orgB} {
		got, err := s.Bots.Get(ctx, org, "b-owner")
		if err != nil {
			t.Fatalf("Bots.Get(%s, b-owner): %v", org, err)
		}
		if got.OrganizationID != org {
			t.Errorf("Bots.Get(%s, b-owner).OrganizationID = %q, want %q", org, got.OrganizationID, org)
		}
	}
}

func TestBots_GetWrongOrgReturnsNotFound(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	mustSeedOwner(t, s, orgA)

	if _, err := s.Bots.Get(ctx, orgB, "b-owner"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Bots.Get(%s, b-owner) wantErr ErrNotFound, got %v", orgB, err)
	}
}

func TestBots_ListFiltersByOrg(t *testing.T) {
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
		got, err := s.Bots.List(ctx, tc.org)
		if err != nil {
			t.Fatalf("Bots.List(%s): %v", tc.org, err)
		}
		if len(got) != tc.want {
			t.Errorf("Bots.List(%s) = %d rows, want %d", tc.org, len(got), tc.want)
		}
		for _, b := range got {
			if b.OrganizationID != tc.org {
				t.Errorf("Bots.List(%s) returned bot in org %q", tc.org, b.OrganizationID)
			}
		}
	}
}

func TestBots_ListFiltersByOrg_Owner(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	mustSeedOwner(t, s, orgA)
	mustSeedOwner(t, s, orgB)

	got, err := s.Bots.List(ctx, orgA)
	if err != nil {
		t.Fatalf("Bots.List(%s): %v", orgA, err)
	}
	if len(got) != 1 || got[0].OrganizationID != orgA {
		t.Errorf("Bots.List(%s) returned %+v, want one bot in %s", orgA, got, orgA)
	}
}

// mustSeedOwner installs the canonical owner Bot for the given org.
// Mirrors what bootstrap.Run would create at the JSON-API entry point.
func mustSeedOwner(t *testing.T, s *store.Store, orgID string) {
	t.Helper()
	ctx := context.Background()

	b, err := orgchart.NewBot("b-owner", "# Owner", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("orgchart.NewBot: %v", err)
	}
	if err := s.Bots.Create(ctx, b); err != nil {
		t.Fatalf("Bots.Create(%s): %v", orgID, err)
	}
}
