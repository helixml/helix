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
	"github.com/helixml/helix/api/pkg/org/application/topology"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// ErrWorkerHasNoRole is returned by UpdateRole when the target Worker
// carries no Role to rewrite. Adapters map it to 409 Conflict.
var ErrWorkerHasNoRole = errors.New("worker has no role")

// ErrReportingCycle is returned by AddParent when the proposed edge
// would close a loop in the reporting DAG. Adapters map it to 409.
var ErrReportingCycle = errors.New("reporting cycle")

// ErrReportingLinesUnavailable is returned when the reporting-lines
// repository is not wired. Adapters map it to 501.
var ErrReportingLinesUnavailable = errors.New("reporting lines not wired")

// Workers owns the worker-mutation use cases.
type Workers struct {
	workers  store.Workers
	roles    *roles.Roles
	lines    store.ReportingLines
	topology *topology.Reconciler
}

// Deps are the constructor-injected collaborators for New.
type Deps struct {
	Workers store.Workers
	// Roles is the role-mutation service UpdateRole delegates to so the
	// "a Worker's capability is its Role" content rewrite reuses the
	// roles RMW (tools/streams preserved) instead of duplicating it.
	Roles *roles.Roles
	// Lines + Topology back AddParent/RemoveParent. Lines may be nil
	// (the service then returns ErrReportingLinesUnavailable); Topology
	// may be nil (a no-op reconcile, handled by the Reconciler itself).
	Lines    store.ReportingLines
	Topology *topology.Reconciler
}

// New constructs the Workers service.
func New(deps Deps) *Workers {
	return &Workers{workers: deps.Workers, roles: deps.Roles, lines: deps.Lines, topology: deps.Topology}
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

// AddParent wires a reporting line (reportID reports to managerID),
// guarding the DAG against cycles, then reconciles the activation/team
// Streams the new edge implies. Both endpoints must exist. Idempotent:
// re-adding an existing line is a no-op (the repo's Add is idempotent).
// Returns ErrReportingCycle (→409), ErrReportingLinesUnavailable (→501),
// or store.ErrNotFound (→404) for the adapter to map.
func (s *Workers) AddParent(ctx context.Context, orgID string, reportID, managerID orgchart.WorkerID) error {
	if s.lines == nil {
		return ErrReportingLinesUnavailable
	}
	if _, err := s.workers.Get(ctx, orgID, reportID); err != nil {
		return fmt.Errorf("get worker %s: %w", reportID, err)
	}
	if _, err := s.workers.Get(ctx, orgID, managerID); err != nil {
		return fmt.Errorf("get manager %s: %w", managerID, err)
	}
	line, err := orgchart.NewReportingLine(orgID, managerID, reportID)
	if err != nil {
		return err
	}
	if err := s.guardCycle(ctx, orgID, reportID, managerID); err != nil {
		return err
	}
	if err := s.lines.Add(ctx, line); err != nil {
		return fmt.Errorf("add reporting line: %w", err)
	}
	// Pass both endpoints so the manager's team stream is in scope.
	if err := s.topology.Reconcile(ctx, orgID, reportID, managerID); err != nil {
		return fmt.Errorf("reconcile topology: %w", err)
	}
	return nil
}

// guardCycle walks up the DAG from managerID; if reportID is reachable,
// adding (manager → report) would close a loop.
func (s *Workers) guardCycle(ctx context.Context, orgID string, reportID, managerID orgchart.WorkerID) error {
	lines, err := s.lines.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list reporting lines: %w", err)
	}
	managersOf := map[orgchart.WorkerID][]orgchart.WorkerID{}
	for _, l := range lines {
		managersOf[l.ReportID] = append(managersOf[l.ReportID], l.ManagerID)
	}
	seen := map[orgchart.WorkerID]bool{}
	queue := []orgchart.WorkerID{managerID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == reportID {
			return fmt.Errorf("making %s report to %s would create a reporting cycle: %w", reportID, managerID, ErrReportingCycle)
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		queue = append(queue, managersOf[cur]...)
	}
	return nil
}

// RemoveParent drops the (reportID → managerID) reporting line, then
// reconciles the Streams the dropped edge implies. Returns
// ErrReportingLinesUnavailable (→501) or store.ErrNotFound (→404).
func (s *Workers) RemoveParent(ctx context.Context, orgID string, reportID, managerID orgchart.WorkerID) error {
	if s.lines == nil {
		return ErrReportingLinesUnavailable
	}
	if err := s.lines.Remove(ctx, orgID, reportID, managerID); err != nil {
		return fmt.Errorf("remove reporting line %s→%s: %w", reportID, managerID, err)
	}
	// Both endpoints named — the ex-manager is no longer in
	// ListManagers(report), so it must be explicit to fall in scope.
	if err := s.topology.Reconcile(ctx, orgID, reportID, managerID); err != nil {
		return fmt.Errorf("reconcile topology: %w", err)
	}
	return nil
}
