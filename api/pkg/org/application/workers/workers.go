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
	"os"
	"path/filepath"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/application/topology"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// HireDispatcher fires the per-hire activation for a new AI Worker. A
// narrow interface (not the full tools.EventDispatcher) so the workers
// service doesn't import the tools package (which imports workers).
type HireDispatcher interface {
	DispatchHire(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, activationID activation.ID)
}

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
	// Hire collaborators (nil-tolerant where the design allows).
	envs       store.Environments
	acts       activation.Repository
	dispatcher HireDispatcher
	hireHook   runtime.HireHook
	envsDir    string
	now        func() time.Time
	newID      func() string
}

// Deps are the constructor-injected collaborators for New. The set is
// larger than the other services' because Hire is the heaviest org
// use case (design §5.1) — grouped here in one options struct.
type Deps struct {
	Workers store.Workers
	// Roles is the role-mutation service UpdateRole delegates to so the
	// "a Worker's capability is its Role" content rewrite reuses the
	// roles RMW (tools/streams preserved) instead of duplicating it.
	Roles *roles.Roles
	// Lines + Topology back AddParent/RemoveParent + Hire's reconcile.
	// Lines may be nil (AddParent/RemoveParent then return
	// ErrReportingLinesUnavailable); Topology may be nil (no-op
	// reconcile, handled by the Reconciler itself).
	Lines    store.ReportingLines
	Topology *topology.Reconciler
	// Hire collaborators. Environments + a clock/id-generator are
	// required for Hire; Dispatcher fires the AI hire activation;
	// Activations pre-allocates its audit row; HireHook runs runtime
	// bookkeeping (persist the hiring user). EnvsDir is the root under
	// which each Worker's env directory is created.
	Environments store.Environments
	Activations  activation.Repository
	Dispatcher   HireDispatcher
	HireHook     runtime.HireHook
	EnvsDir      string
	Now          func() time.Time
	NewID        func() string
}

// New constructs the Workers service.
func New(deps Deps) *Workers {
	return &Workers{
		workers:    deps.Workers,
		roles:      deps.Roles,
		lines:      deps.Lines,
		topology:   deps.Topology,
		envs:       deps.Environments,
		acts:       deps.Activations,
		dispatcher: deps.Dispatcher,
		hireHook:   deps.HireHook,
		envsDir:    deps.EnvsDir,
		now:        deps.Now,
		newID:      deps.NewID,
	}
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

// HireParams describes a new Worker. ID is optional — when empty a
// fresh `w-<id>` is minted (discouraged; callers should pass a readable
// handle). ParentID is the manager this hire reports to (empty only for
// the org owner).
type HireParams struct {
	ID              string
	RoleID          orgchart.RoleID
	ParentID        orgchart.WorkerID
	Kind            orgchart.WorkerKind
	IdentityContent string
}

// HireResult carries the new Worker id and, for AI hires, the
// pre-allocated hire-activation id.
type HireResult struct {
	WorkerID     orgchart.WorkerID
	ActivationID activation.ID
}

// Hire brings a Worker into existence: a Worker row carrying the
// per-hire IdentityContent, an Environment row pointing at
// <EnvsDir>/<workerID>/, the initial reporting line, a topology reconcile,
// the hiring-user bookkeeping, and — for AI Workers — a pre-allocated
// hire activation dispatched through the Spawner. This is the single
// implementation the MCP hire_worker tool and the REST POST /workers
// handler both call (no synthetic Invocation).
//
// State lives in the domain (DB), not on disk: role.md / identity.md /
// agent.md are projected into the Environment by the Spawner at
// activation time. A Worker's MCP tool surface is derived live from
// Role.Tools — there is no per-Worker tool record and no tools param.
func (s *Workers) Hire(ctx context.Context, orgID string, p HireParams) (HireResult, error) {
	if err := p.Kind.Validate(); err != nil {
		return HireResult{}, err
	}
	if p.RoleID == "" {
		return HireResult{}, fmt.Errorf("roleId is required")
	}
	if p.IdentityContent == "" {
		return HireResult{}, fmt.Errorf("identityContent is required")
	}
	if s.envsDir == "" {
		return HireResult{}, fmt.Errorf("server is not configured with an envs directory")
	}
	if s.newID == nil || s.now == nil {
		return HireResult{}, fmt.Errorf("workers: clock/id-generator not wired")
	}

	if _, err := s.roles.Get(ctx, orgID, p.RoleID); err != nil {
		return HireResult{}, fmt.Errorf("role %q: %w", p.RoleID, err)
	}

	var parent *orgchart.WorkerID
	if p.ParentID != "" {
		if _, err := s.workers.Get(ctx, orgID, p.ParentID); err != nil {
			return HireResult{}, fmt.Errorf("parent worker %q: %w", p.ParentID, err)
		}
		parent = &p.ParentID
	}

	id := orgchart.WorkerID(p.ID)
	if id == "" {
		id = orgchart.WorkerID("w-" + s.newID())
	}
	// The id becomes a path segment under envsDir below — reject any
	// traversal ("../…") or separator before it reaches the filesystem.
	if err := orgchart.ValidID(string(id)); err != nil {
		return HireResult{}, fmt.Errorf("worker id: %w", err)
	}
	envPath := filepath.Join(s.envsDir, string(id))

	var wkr orgchart.Worker
	switch p.Kind {
	case orgchart.WorkerKindHuman:
		w, err := orgchart.NewHumanWorker(id, p.RoleID, p.IdentityContent, orgID)
		if err != nil {
			return HireResult{}, err
		}
		wkr = w
	case orgchart.WorkerKindAI:
		w, err := orgchart.NewAIWorker(id, p.RoleID, p.IdentityContent, orgID)
		if err != nil {
			return HireResult{}, err
		}
		wkr = w
	default:
		return HireResult{}, p.Kind.Validate() // unreachable; Validate above rejected it
	}

	if err := os.MkdirAll(envPath, 0o750); err != nil {
		return HireResult{}, fmt.Errorf("create env dir %q: %w", envPath, err)
	}
	if err := s.workers.Create(ctx, wkr); err != nil {
		return HireResult{}, err
	}

	// Wire the initial reporting line now that both Worker rows exist.
	if parent != nil && s.lines != nil {
		line, err := orgchart.NewReportingLine(orgID, *parent, id)
		if err != nil {
			return HireResult{}, err
		}
		if err := s.lines.Add(ctx, line); err != nil {
			return HireResult{}, fmt.Errorf("add reporting line: %w", err)
		}
	}

	env, err := environment.New(string(id), envPath, s.now(), orgID)
	if err != nil {
		return HireResult{}, err
	}
	if s.envs != nil {
		if err := s.envs.Create(ctx, env); err != nil {
			return HireResult{}, fmt.Errorf("create environment: %w", err)
		}
	}

	// Reconcile the activation/team Streams implied by the new Worker and
	// its reporting line (mints the hire's activation Stream + the
	// manager's team Stream from one declarative pass).
	if err := s.topology.Reconcile(ctx, orgID, id); err != nil {
		return HireResult{}, fmt.Errorf("reconcile topology for hire %q: %w", id, err)
	}

	// Persist the hiring user's identity (if the request carried one)
	// BEFORE dispatch so the Spawner picks it up on its first call.
	if uid := runtimehelix.UserIDFromContext(ctx); uid != "" && s.hireHook != nil {
		if err := s.hireHook.OnHire(ctx, orgID, id, uid); err != nil {
			return HireResult{}, fmt.Errorf("hire handler: %w", err)
		}
	}

	// Pre-create the hire-Activation audit row so Hire can return the id
	// synchronously; the Spawner Completes it (matched by
	// Trigger.ActivationID) rather than minting a sibling.
	var hireActID activation.ID
	if p.Kind == orgchart.WorkerKindAI && s.acts != nil {
		hireActID = activation.ID("a-" + s.newID())
		hireAct, err := activation.New(hireActID, id, []activation.Trigger{{Kind: activation.TriggerHire}}, s.now(), orgID)
		if err != nil {
			return HireResult{}, fmt.Errorf("build hire activation: %w", err)
		}
		if err := s.acts.Create(ctx, hireAct); err != nil {
			return HireResult{}, fmt.Errorf("persist hire activation: %w", err)
		}
	}
	if p.Kind == orgchart.WorkerKindAI && s.dispatcher != nil {
		s.dispatcher.DispatchHire(ctx, orgID, id, envPath, hireActID)
	}

	return HireResult{WorkerID: id, ActivationID: hireActID}, nil
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
