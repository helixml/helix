package helix

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/types"
)

// ProjectConfig is the helix-runtime implementation of
// runtime.ProjectConfig. It resolves a workerID → Helix projectID
// via the WorkerRuntimeState sidecar (where the spawner persists
// the per-Worker pointers on hire), then reads / writes the
// underlying Helix project through ProjectService.
//
// Construction has explicit pointers so the host can verify it
// wired both halves; nil ProjectService is a hard error at
// construction (rather than at first call) because there's no
// degraded mode that makes sense — without it, neither read nor
// write works.
type ProjectConfig struct {
	store   *store.Store
	helix   ProjectService
	backend string
}

// NewProjectConfig builds a ProjectConfig that uses the given
// helix store + ProjectService. backend overrides the
// WorkerRuntimeState namespace label — default ("") is the
// production `helix` backend. Test fakes can pass an alternate
// label when they share a single store across multiple runtimes.
func NewProjectConfig(st *store.Store, svc ProjectService) (*ProjectConfig, error) {
	if st == nil {
		return nil, errors.New("helix.NewProjectConfig: store is nil")
	}
	if svc == nil {
		return nil, errors.New("helix.NewProjectConfig: ProjectService is nil")
	}
	return &ProjectConfig{store: st, helix: svc, backend: Backend}, nil
}

// GetWorkerProjectConfig resolves the worker's project ID via the
// helix runtime state, then returns the current project config
// snapshot. Failure modes:
//   - worker has no helix state row → ErrProjectConfigUnsupported
//     (the worker is hired against a different runtime / hasn't
//     activated yet).
//   - project lookup fails → wrapped error returned verbatim;
//     callers translate to a friendly message.
func (c *ProjectConfig) GetWorkerProjectConfig(ctx context.Context, orgID string, workerID orgchart.BotID) (runtime.ProjectConfigSnapshot, error) {
	state, err := LoadState(ctx, c.store, orgID, workerID)
	if err != nil {
		return runtime.ProjectConfigSnapshot{}, fmt.Errorf("load worker state: %w", err)
	}
	if state.ProjectID == "" {
		return runtime.ProjectConfigSnapshot{}, fmt.Errorf("worker %s: %w", workerID, runtime.ErrProjectConfigUnsupported)
	}
	proj, err := c.helix.GetProject(ctx, state.ProjectID)
	if err != nil {
		return runtime.ProjectConfigSnapshot{}, fmt.Errorf("get helix project %s: %w", state.ProjectID, err)
	}
	return runtime.ProjectConfigSnapshot{
		ProjectID:     proj.ID,
		StartupScript: proj.StartupScript,
	}, nil
}

// UpdateWorkerProjectConfig applies a partial patch to the
// worker's helix project. Mirrors Helix's pointer convention:
// nil fields on the runtime patch stay nil on the underlying
// `types.ProjectUpdateRequest`, so they don't touch the
// project. Returns the post-update snapshot so callers (and the
// MCP tool's JSON response) can show what landed without an
// extra read.
//
// Failure modes match GetWorkerProjectConfig.
func (c *ProjectConfig) UpdateWorkerProjectConfig(ctx context.Context, orgID string, workerID orgchart.BotID, patch runtime.ProjectConfigPatch) (runtime.ProjectConfigSnapshot, error) {
	state, err := LoadState(ctx, c.store, orgID, workerID)
	if err != nil {
		return runtime.ProjectConfigSnapshot{}, fmt.Errorf("load worker state: %w", err)
	}
	if state.ProjectID == "" {
		return runtime.ProjectConfigSnapshot{}, fmt.Errorf("worker %s: %w", workerID, runtime.ErrProjectConfigUnsupported)
	}
	req := types.ProjectUpdateRequest{
		StartupScript: patch.StartupScript,
	}
	proj, err := c.helix.UpdateProject(ctx, state.ProjectID, req)
	if err != nil {
		return runtime.ProjectConfigSnapshot{}, fmt.Errorf("update helix project %s: %w", state.ProjectID, err)
	}
	return runtime.ProjectConfigSnapshot{
		ProjectID:     proj.ID,
		StartupScript: proj.StartupScript,
	}, nil
}
