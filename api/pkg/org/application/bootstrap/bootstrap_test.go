package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/bootstrap"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
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
	owner, err := s.Workers.Get(ctx, "org-acme", result.WorkerID)
	if err != nil {
		t.Fatalf("Get owner: %v", err)
	}
	if owner.OrganizationID() != "org-acme" {
		t.Errorf("owner.OrganizationID() = %q, want org-acme", owner.OrganizationID())
	}
}

// TestRunRequiresOrganizationID pins H5.3: bootstrap is multi-tenant
// and the caller MUST pass OrganizationID. The single-tenant
// back-compat that returned a Worker with empty OrgID is gone.
func TestRunRequiresOrganizationID(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	envDir := filepath.Join(t.TempDir(), "w-owner")
	if err := mkdirAll(envDir); err != nil {
		t.Fatalf("mkdir env: %v", err)
	}

	ctx := context.Background()
	if _, err := bootstrap.Run(ctx, s, bootstrap.Params{EnvironmentPath: envDir}); err == nil {
		t.Fatal("Run with empty OrganizationID: expected error, got nil")
	}
}

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
