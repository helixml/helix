// Package workers is the application service that owns the structural
// Worker-mutation use cases that the MCP tools and REST handlers used
// to implement independently: UpdateIdentity (per-Worker persona) and
// UpdateRole (rewrite the content of the Role a Worker holds).
//
// Hire is intentionally NOT here yet. Unlike the other mutations, hire
// already has a single implementation (tools.HireWorker; the REST POST
// /workers handler delegates to it via a synthetic invocation), so it
// has no drift to fix — and relocating its ~9 collaborators churns the
// composition root. That relocation rides with the composition-root
// phase (design §5.4 / Phase G), where the wiring is being reworked
// anyway.
//
// The service depends on the narrow store.Workers repository plus the
// roles application service (UpdateRole rewrites the held Role's content
// through the same tested read-modify-write the roles service owns —
// no duplicated RMW, tools/streams preserved). CLAUDE.md §5.0: small
// interfaces, ≤4 collaborators.
package workers

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// ErrWorkerHasNoRole is returned by UpdateRole when the target Worker
// carries no Role to rewrite. Adapters map it to 409 Conflict.
var ErrWorkerHasNoRole = errors.New("worker has no role")

// Workers owns the worker-mutation use cases.
type Workers struct {
	workers store.Workers
	roles   *roles.Roles
}

// Deps are the constructor-injected collaborators for New.
type Deps struct {
	Workers store.Workers
	// Roles is the role-mutation service UpdateRole delegates to so the
	// "a Worker's capability is its Role" content rewrite reuses the
	// roles RMW (tools/streams preserved) instead of duplicating it.
	Roles *roles.Roles
}

// New constructs the Workers service.
func New(deps Deps) *Workers {
	return &Workers{workers: deps.Workers, roles: deps.Roles}
}

// UpdateIdentity rewrites a Worker's IdentityContent via the domain's
// WithIdentityContent builder and persists it, returning the updated
// Worker. Returns store.ErrNotFound (wrapped) when the (orgID, id) row
// is absent, including cross-tenant id guesses.
func (s *Workers) UpdateIdentity(ctx context.Context, orgID string, id orgchart.WorkerID, content string) (orgchart.Worker, error) {
	existing, err := s.workers.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	updated := existing.WithIdentityContent(content)
	if err := s.workers.Update(ctx, updated); err != nil {
		return nil, fmt.Errorf("update worker: %w", err)
	}
	return updated, nil
}

// UpdateRole rewrites the content of the Role the given Worker holds.
// The Worker's MCP surface is derived from its Role, so "edit this
// worker's role" maps to a content update on the held Role — delegated
// to the roles service so Tools/Streams are preserved. Returns
// store.ErrNotFound when the Worker (or its Role) is absent.
func (s *Workers) UpdateRole(ctx context.Context, orgID string, id orgchart.WorkerID, content string) (orgchart.Role, error) {
	wk, err := s.workers.Get(ctx, orgID, id)
	if err != nil {
		return orgchart.Role{}, err
	}
	roleID := wk.RoleID()
	if roleID == "" {
		return orgchart.Role{}, ErrWorkerHasNoRole
	}
	return s.roles.Update(ctx, orgID, roleID, roles.UpdateParams{Content: &content})
}
