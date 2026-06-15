// Package lifecycle owns the cross-cutting orchestration that
// composes store + runtime + on-disk state when a Worker is created
// or destroyed.
//
// This package owns the two halves of the Worker lifecycle: Hire (the
// create cascade — Worker row, env dir, reporting line, topology
// reconcile, hire-activation dispatch) and Fire (the destroy cascade —
// Helix project/app teardown, store cleanup, env-dir removal, topology
// reconcile), plus the DeleteRole cascade. Both REST and the MCP
// hire_worker tool drive Hire here, so the hire semantics cannot drift
// between callers. Fire has no MCP counterpart by design (the LLM
// should not be able to delete workers from chat), so it is a plain Go
// service callable from REST handlers only.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// HireDispatcher fires the per-hire activation for a new AI Worker. A
// narrow interface so the lifecycle service doesn't import the tools
// package. The composition root's dispatcher satisfies it.
type HireDispatcher interface {
	DispatchHire(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, activationID activation.ID)
}

// HelixRuntime is the slice of runtime/helix.ProjectService that the
// Fire cascade needs to tear down a Worker's Helix-side project and
// agent app. Production wiring satisfies this with the in-process
// adapter used everywhere else; the interface exists so tests can
// stub.
type HelixRuntime interface {
	DeleteProject(ctx context.Context, id string) error
	DeleteApp(ctx context.Context, id string) error
}

// Service composes the worker-lifecycle operations the REST layer
// drives. All fields are required; pass nil HelixRuntime only in
// tests that don't need the Helix-side teardown.
type Service struct {
	Store   *store.Store
	Helix   HelixRuntime
	Logger  *slog.Logger
	EnvsDir string

	// Reconciler reconciles the activation/team Streams after the Worker
	// row is gone — it tears down the fired Worker's own Streams and
	// collapses an ex-manager's team Stream when its last report just
	// left. nil is a no-op (tests without topology wiring).
	Reconciler *reconcile.Reconciler

	// Mirror is the transcript mirror; Fire stops the fired Worker's
	// subscription so it doesn't leak. nil is a no-op.
	Mirror *helix.Mirror

	// --- Hire collaborators (the create half of the lifecycle) ---

	// Dispatcher fires the per-hire activation for a new AI Worker.
	// nil → no hire activation is dispatched (tests / runtimes without
	// a dispatcher).
	Dispatcher HireDispatcher
	// HireHook runs runtime bookkeeping after the Worker row exists —
	// the helix runtime persists the hiring user. nil is a no-op.
	HireHook runtime.HireHook
	// Now / NewID seam the clock and id-generator. Both are required
	// for Hire; Fire does not use them.
	Now   func() time.Time
	NewID func() string
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
// <EnvsDir>/<workerID>/, the initial reporting line, a topology
// reconcile, the hiring-user bookkeeping, and — for AI Workers — a
// pre-allocated hire activation dispatched through the Spawner. This is
// the single implementation the MCP hire_worker tool and the REST POST
// /workers handler both call (no synthetic Invocation).
//
// State lives in the domain (DB), not on disk: role.md / identity.md /
// agent.md are projected into the Environment by the Spawner at
// activation time. A Worker's MCP tool surface is derived live from
// Role.Tools — there is no per-Worker tool record and no tools param.
func (s *Service) Hire(ctx context.Context, orgID string, p HireParams) (HireResult, error) {
	if err := p.Kind.Validate(); err != nil {
		return HireResult{}, err
	}
	if p.RoleID == "" {
		return HireResult{}, fmt.Errorf("roleId is required")
	}
	if p.IdentityContent == "" {
		return HireResult{}, fmt.Errorf("identityContent is required")
	}
	if s.EnvsDir == "" {
		return HireResult{}, fmt.Errorf("server is not configured with an envs directory")
	}
	if s.NewID == nil || s.Now == nil {
		return HireResult{}, fmt.Errorf("lifecycle: clock/id-generator not wired")
	}
	if s.Store == nil {
		return HireResult{}, errors.New("lifecycle: store is nil")
	}

	if _, err := s.Store.Roles.Get(ctx, orgID, p.RoleID); err != nil {
		return HireResult{}, fmt.Errorf("role %q: %w", p.RoleID, err)
	}

	var parent *orgchart.WorkerID
	if p.ParentID != "" {
		if _, err := s.Store.Workers.Get(ctx, orgID, p.ParentID); err != nil {
			return HireResult{}, fmt.Errorf("parent worker %q: %w", p.ParentID, err)
		}
		parent = &p.ParentID
	}

	id := orgchart.WorkerID(p.ID)
	if id == "" {
		id = orgchart.WorkerID("w-" + s.NewID())
	}
	// The id becomes a path segment under EnvsDir below — reject any
	// traversal ("../…") or separator before it reaches the filesystem.
	if err := orgchart.ValidID(string(id)); err != nil {
		return HireResult{}, fmt.Errorf("worker id: %w", err)
	}
	envPath := filepath.Join(s.EnvsDir, string(id))

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
	if err := s.Store.Workers.Create(ctx, wkr); err != nil {
		return HireResult{}, err
	}

	// Wire the initial reporting line now that both Worker rows exist.
	if parent != nil && s.Store.ReportingLines != nil {
		line, err := orgchart.NewReportingLine(orgID, *parent, id)
		if err != nil {
			return HireResult{}, err
		}
		if err := s.Store.ReportingLines.Add(ctx, line); err != nil {
			return HireResult{}, fmt.Errorf("add reporting line: %w", err)
		}
	}

	env, err := environment.New(string(id), envPath, s.Now(), orgID)
	if err != nil {
		return HireResult{}, err
	}
	if s.Store.Environments != nil {
		if err := s.Store.Environments.Create(ctx, env); err != nil {
			return HireResult{}, fmt.Errorf("create environment: %w", err)
		}
	}

	// Reconcile the activation/team Streams implied by the new Worker and
	// its reporting line (mints the hire's transcript + the
	// manager's team Stream from one declarative pass). A nil Reconciler is
	// a no-op (the Reconciler guards its own nil receiver).
	if err := s.Reconciler.Reconcile(ctx, orgID, id); err != nil {
		return HireResult{}, fmt.Errorf("reconcile topology for hire %q: %w", id, err)
	}

	// Persist the hiring user's identity (if the request carried one)
	// BEFORE dispatch so the Spawner picks it up on its first call.
	if uid := helix.UserIDFromContext(ctx); uid != "" && s.HireHook != nil {
		if err := s.HireHook.OnHire(ctx, orgID, id, uid); err != nil {
			return HireResult{}, fmt.Errorf("hire handler: %w", err)
		}
	}

	// Pre-create the hire-Activation audit row so Hire can return the id
	// synchronously; the Spawner Completes it (matched by
	// Trigger.ActivationID) rather than minting a sibling.
	var hireActID activation.ID
	if p.Kind == orgchart.WorkerKindAI && s.Store.Activations != nil {
		hireActID = activation.ID("a-" + s.NewID())
		hireAct, err := activation.New(hireActID, id, []activation.Trigger{{Kind: activation.TriggerHire}}, s.Now(), orgID)
		if err != nil {
			return HireResult{}, fmt.Errorf("build hire activation: %w", err)
		}
		if err := s.Store.Activations.Create(ctx, hireAct); err != nil {
			return HireResult{}, fmt.Errorf("persist hire activation: %w", err)
		}
	}
	if p.Kind == orgchart.WorkerKindAI && s.Dispatcher != nil {
		s.Dispatcher.DispatchHire(ctx, orgID, id, envPath, hireActID)
	}

	return HireResult{WorkerID: id, ActivationID: hireActID}, nil
}

// Fire tears down a Worker end-to-end:
//
//  1. Read the Helix-runtime state (project + app IDs) before clearing.
//  2. DeleteProject on Helix — stops any active sessions.
//  3. DeleteApp on Helix — removes the auto-provisioned agent app.
//  4. Clear the WorkerRuntimeState sidecar.
//  5. Delete every Subscription for this worker.
//  6. Remove the env directory from disk and delete its row.
//  7. Delete the Worker row.
//  8. Reconcile topology: tear down the fired Worker's own activation +
//     team Streams and collapse any ex-manager's team Stream that just
//     lost its last report. topology is the single owner of
//     activation/team Stream lifecycle — there is no inline
//     Streams.Delete here any more.
//
// Steps 2/3/5/6/8 are best-effort and logged on failure — a half-
// torn worker is better than refusing to clean up partial state.
// Step 7 is the only step whose error propagates (the row is the
// user-visible source of truth).
//
// Subscriptions are worker-anchored, so they die with the worker. A
// new hire into the same Role does not automatically inherit them;
// the hiring playbook explicitly re-subscribes.
//
// Tool capability is derived from Role.Tools, so there is no
// per-Worker tool cascade to clean up.
//
// Activation events themselves are intentionally left behind as an
// audit trail; only the Stream row is dropped.
func (s *Service) Fire(ctx context.Context, orgID string, id orgchart.WorkerID) error {
	if id == "" {
		return errors.New("worker id is empty")
	}
	if s.Store == nil {
		return errors.New("lifecycle: store is nil")
	}
	if _, err := s.Store.Workers.Get(ctx, orgID, id); err != nil {
		return fmt.Errorf("get worker %q: %w", id, err)
	}

	// Capture the fired Worker's managers BEFORE deletion: the reporting
	// lines cascade-drop with the Worker row, so ListManagers returns
	// nothing afterward. We feed these ex-managers to the topology
	// reconcile below so a manager's team Stream collapses when its last
	// report leaves.
	var exManagers []orgchart.WorkerID
	if s.Store.ReportingLines != nil {
		exManagers, _ = s.Store.ReportingLines.ListManagers(ctx, orgID, id)
	}

	state, _ := helix.LoadState(ctx, s.Store, orgID, id)

	if s.Mirror != nil {
		s.Mirror.Stop(orgID, id)
	}

	if s.Helix != nil && state.ProjectID != "" {
		if err := s.Helix.DeleteProject(ctx, state.ProjectID); err != nil && !errors.Is(err, helix.ErrProjectNotFound) {
			s.logger().Warn("fire: delete helix project", "worker", id, "project", state.ProjectID, "err", err)
		}
	}
	if s.Helix != nil && state.AgentAppID != "" {
		if err := s.Helix.DeleteApp(ctx, state.AgentAppID); err != nil && !errors.Is(err, helix.ErrProjectNotFound) {
			s.logger().Warn("fire: delete helix app", "worker", id, "app", state.AgentAppID, "err", err)
		}
	}
	if s.Store.WorkerRuntimeState != nil {
		if err := s.Store.WorkerRuntimeState.Clear(ctx, orgID, id, helix.Backend); err != nil {
			s.logger().Warn("fire: clear runtime state", "worker", id, "err", err)
		}
	}

	// The worker's subscriptions and every reporting line that
	// references it are cascaded structurally when the worker row is
	// deleted below (Workers.Delete drops the subs; the
	// org_reporting_lines ON DELETE CASCADE foreign keys drop the
	// lines). Nothing to drop explicitly here.

	if env, err := s.Store.Environments.Get(ctx, orgID, id); err == nil {
		if env.Path != "" {
			if rmErr := os.RemoveAll(env.Path); rmErr != nil {
				s.logger().Warn("fire: remove env dir", "worker", id, "path", env.Path, "err", rmErr)
			}
		}
	}
	if err := s.Store.Environments.Delete(ctx, orgID, id); err != nil {
		s.logger().Warn("fire: delete environment row", "worker", id, "err", err)
	}

	if err := s.Store.Workers.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete worker row %q: %w", id, err)
	}

	// Settle the activation/team Streams now that the row (and its
	// reporting lines) are gone. topology is the single owner of their
	// lifecycle: reconciling `id` tears down the fired Worker's own
	// activation + team Streams (it has fallen out of the graph), and
	// reconciling the ex-managers collapses a manager's team Stream when
	// its last report just left. Best-effort: a failure here leaves a
	// dangling Stream row, not a half-deleted worker, so we log and
	// continue rather than failing the Fire.
	if s.Reconciler != nil {
		affected := append([]orgchart.WorkerID{id}, exManagers...)
		if err := s.Reconciler.Reconcile(ctx, orgID, affected...); err != nil {
			s.logger().Warn("fire: reconcile topology", "worker", id, "err", err)
		}
	}
	return nil
}

// DeleteRole tears down a Role end-to-end:
//
//  1. Fire every Worker whose RoleID matches id (each Worker gets
//     the full Fire cascade — project teardown, env removal,
//     subscriptions, the worker row).
//  2. Delete the Role row.
func (s *Service) DeleteRole(ctx context.Context, orgID string, id orgchart.RoleID) error {
	if id == "" {
		return errors.New("role id is empty")
	}
	if s.Store == nil {
		return errors.New("lifecycle: store is nil")
	}
	if _, err := s.Store.Roles.Get(ctx, orgID, id); err != nil {
		return fmt.Errorf("get role %q: %w", id, err)
	}

	if err := s.fireWorkersWithRole(ctx, orgID, id); err != nil {
		return err
	}

	if err := s.Store.Roles.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete role %q: %w", id, err)
	}
	return nil
}

// fireWorkersWithRole fires every Worker holding the given Role.
func (s *Service) fireWorkersWithRole(ctx context.Context, orgID string, roleID orgchart.RoleID) error {
	workers, err := s.Store.Workers.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list workers: %w", err)
	}
	for _, w := range workers {
		if w.RoleID() != roleID {
			continue
		}
		if err := s.Fire(ctx, orgID, w.ID()); err != nil {
			s.logger().Warn("fire worker for role teardown", "role", roleID, "worker", w.ID(), "err", err)
		}
	}
	return nil
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
