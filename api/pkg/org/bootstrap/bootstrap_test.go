package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	orggorm "github.com/helixml/helix/api/pkg/org/store/gorm"
	"github.com/helixml/helix/api/pkg/org/bootstrap"
)

// TestRunStampsOwnerWorkerWithOrganizationID pins H5.2: when the
// caller passes Params.OrganizationID, the owner Worker bootstrap
// creates is stamped with that OrgID. This is the first concrete
// multi-tenant scaffolding — H5.3 will make the (org_id,
// worker_id) lookup composite so two helix.Organizations can
// bootstrap independently without colliding on "w-owner".
func TestRunStampsOwnerWorkerWithOrganizationID(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	envDir := filepath.Join(t.TempDir(), "w-owner")
	if err := mkdirAll(envDir); err != nil {
		t.Fatalf("mkdir env: %v", err)
	}

	ctx := context.Background()
	result, err := bootstrap.Run(ctx, s, bootstrap.Params{
		EnvironmentPath: envDir,
		OrganizationID:  "org-acme",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.WorkerID != "w-owner" {
		t.Errorf("WorkerID = %q, want w-owner", result.WorkerID)
	}
	owner, err := s.Workers.Get(ctx, result.WorkerID)
	if err != nil {
		t.Fatalf("Get owner: %v", err)
	}
	if owner.OrganizationID() != "org-acme" {
		t.Errorf("owner.OrganizationID() = %q, want org-acme", owner.OrganizationID())
	}
}

// TestRunOmitsOrgIDWhenUnspecified pins the back-compat path: a
// caller that doesn't set OrganizationID gets today's behaviour —
// the owner Worker has no OrgID, single-tenant alpha-mode.
func TestRunOmitsOrgIDWhenUnspecified(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	envDir := filepath.Join(t.TempDir(), "w-owner")
	if err := mkdirAll(envDir); err != nil {
		t.Fatalf("mkdir env: %v", err)
	}

	ctx := context.Background()
	result, err := bootstrap.Run(ctx, s, bootstrap.Params{EnvironmentPath: envDir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	owner, _ := s.Workers.Get(ctx, result.WorkerID)
	if owner.OrganizationID() != "" {
		t.Errorf("owner.OrganizationID() = %q, want empty (single-tenant default)", owner.OrganizationID())
	}
}

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
