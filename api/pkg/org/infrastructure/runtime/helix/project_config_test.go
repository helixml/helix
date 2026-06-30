package helix

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/types"
)

// newOrgTestStoreForProjectConfig returns a fresh gorm-backed
// org store for the project-config tests, without the role +
// position + worker scaffolding the spawner tests need.
// (newHelixTestStore in spawner_test.go seeds those — we don't
// need them because ProjectConfig touches only WorkerRuntimeState,
// not the full chart.)
func newOrgTestStoreForProjectConfig(t *testing.T) *storeWrapper {
	t.Helper()
	return &storeWrapper{Store: *orggorm.GetOrgTestDB(t)}
}

// storeWrapper is a tiny shim so the project-config tests can take
// `&wrap.Store` without leaking a `*Store` pointer to callers that
// only need the struct value. Mirrors how `store.Store` is
// commonly embedded in tests across this package.
type storeWrapper struct{ Store store.Store }

// saveAllPointers is the project-config tests' shorthand for the
// per-Worker pointer set the spawner persists on hire +
// project-apply. We bypass the runtime's normal SaveProject /
// SaveSession path because tests don't need the full apply flow —
// they just need the project_id key in WorkerRuntimeState so
// ProjectConfig's worker→project lookup succeeds.
func saveAllPointers(t *testing.T, st *store.Store, orgID string, workerID orgchart.BotID, projectID, agentAppID, repoID, sessionID string) {
	t.Helper()
	if err := SaveProject(context.Background(), st, orgID, workerID, projectID, agentAppID, repoID); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if err := SaveSession(context.Background(), st, orgID, workerID, sessionID); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
}

// TestNewProjectConfig_RejectsNilDeps pins the construction
// contract — neither half is optional. Catching this at construction
// time saves a confusing nil-deref the first time the tool is called.
func TestNewProjectConfig_RejectsNilDeps(t *testing.T) {
	t.Parallel()
	if _, err := NewProjectConfig(nil, &fakeProjectService{}); err == nil {
		t.Error("expected error on nil store")
	}
	if _, err := NewProjectConfig(&newOrgTestStoreForProjectConfig(t).Store, nil); err == nil {
		t.Error("expected error on nil ProjectService")
	}
}

// TestGetWorkerProjectConfig_RoundTrips pins the worker→project
// resolution + read path. Seed a worker's runtime state with a
// project ID, configure the fake helix service with that project's
// startup script, and assert the snapshot returns the same.
func TestGetWorkerProjectConfig_RoundTrips(t *testing.T) {
	t.Parallel()
	wrap := newOrgTestStoreForProjectConfig(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_01abc", "app_x", "repo_y", "ses_z")

	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{ID: "prj_01abc", StartupScript: "echo hello"}

	pc, err := NewProjectConfig(&wrap.Store, svc)
	if err != nil {
		t.Fatalf("NewProjectConfig: %v", err)
	}
	snap, err := pc.GetWorkerProjectConfig(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("GetWorkerProjectConfig: %v", err)
	}
	if snap.ProjectID != "prj_01abc" {
		t.Errorf("ProjectID = %q", snap.ProjectID)
	}
	if snap.StartupScript != "echo hello" {
		t.Errorf("StartupScript = %q", snap.StartupScript)
	}
}

// TestGetWorkerProjectConfig_NoProjectIDReturnsUnsupported pins the
// "worker is hired against a different runtime / hasn't activated
// yet" failure mode. The tool surfaces this as ErrProjectConfigUnsupported
// so the LLM gets a clear "this worker has no helix project to
// configure" message instead of a generic store error.
func TestGetWorkerProjectConfig_NoProjectIDReturnsUnsupported(t *testing.T) {
	t.Parallel()
	wrap := newOrgTestStoreForProjectConfig(t)
	// Worker exists in runtime state with NO project_id key set.
	pc, err := NewProjectConfig(&wrap.Store, newFakeProjectService())
	if err != nil {
		t.Fatalf("NewProjectConfig: %v", err)
	}
	_, err = pc.GetWorkerProjectConfig(context.Background(), "org-test", orgchart.BotID("w-noproject"))
	if !errors.Is(err, runtime.ErrProjectConfigUnsupported) {
		t.Errorf("err = %v, want ErrProjectConfigUnsupported", err)
	}
}

// TestUpdateWorkerProjectConfig_PatchFlowsToHelix pins that a
// pointer-set StartupScript on the patch reaches Helix's
// ProjectUpdateRequest unchanged, and that the post-update
// snapshot reflects what landed.
func TestUpdateWorkerProjectConfig_PatchFlowsToHelix(t *testing.T) {
	t.Parallel()
	wrap := newOrgTestStoreForProjectConfig(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_01abc", "app_x", "repo_y", "ses_z")

	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{ID: "prj_01abc", StartupScript: "old"}

	pc, err := NewProjectConfig(&wrap.Store, svc)
	if err != nil {
		t.Fatalf("NewProjectConfig: %v", err)
	}
	newScript := "#!/bin/bash\napt install gh"
	snap, err := pc.UpdateWorkerProjectConfig(context.Background(), "org-test", wid, runtime.ProjectConfigPatch{
		StartupScript: &newScript,
	})
	if err != nil {
		t.Fatalf("UpdateWorkerProjectConfig: %v", err)
	}
	if svc.updateProjectCalls != 1 {
		t.Errorf("UpdateProject called %d times, want 1", svc.updateProjectCalls)
	}
	if svc.updateProjectPatchLast.StartupScript == nil || *svc.updateProjectPatchLast.StartupScript != newScript {
		t.Errorf("patch sent to helix = %+v, want StartupScript pointer to %q", svc.updateProjectPatchLast, newScript)
	}
	if snap.StartupScript != newScript {
		t.Errorf("returned StartupScript = %q, want %q (post-update reflected)", snap.StartupScript, newScript)
	}
}

// TestUpdateWorkerProjectConfig_NilPatchFieldsLeaveAloneInHelix pins
// the partial-update contract: a nil pointer on the runtime patch
// MUST stay nil on the underlying ProjectUpdateRequest. Otherwise
// Helix would over-write whatever's there with the zero value.
func TestUpdateWorkerProjectConfig_NilPatchFieldsLeaveAloneInHelix(t *testing.T) {
	t.Parallel()
	wrap := newOrgTestStoreForProjectConfig(t)
	wid := orgchart.BotID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, "prj_01abc", "app_x", "repo_y", "ses_z")

	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{ID: "prj_01abc", StartupScript: "preserved"}

	pc, err := NewProjectConfig(&wrap.Store, svc)
	if err != nil {
		t.Fatalf("NewProjectConfig: %v", err)
	}
	// Empty patch — no fields set.
	_, err = pc.UpdateWorkerProjectConfig(context.Background(), "org-test", wid, runtime.ProjectConfigPatch{})
	if err != nil {
		t.Fatalf("UpdateWorkerProjectConfig: %v", err)
	}
	if svc.updateProjectPatchLast.StartupScript != nil {
		t.Errorf("StartupScript pointer should be nil; got %+v", svc.updateProjectPatchLast.StartupScript)
	}
}
