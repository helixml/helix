// Package lifecycle owns the cross-cutting orchestration that
// composes store + runtime when a Worker is created or destroyed.
//
// This package owns the two halves of the Worker lifecycle: Hire (the
// create cascade — Worker row, reporting line, topology reconcile,
// hire-activation dispatch) and Fire (the destroy cascade — Helix
// project/app teardown, store cleanup, topology reconcile), plus the
// DeleteRole cascade. Both REST and the MCP
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
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// HireDispatcher fires the per-hire activation for a new AI Worker. A
// narrow interface so the lifecycle service doesn't import the tools
// package. The composition root's dispatcher satisfies it.
type HireDispatcher interface {
	DispatchHire(ctx context.Context, orgID string, workerID orgchart.WorkerID, activationID activation.ID)
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

// The lifecycle runs two kinds of reconciler after a structural change, and
// they are deliberately separate types because their CONTRACTS differ — not
// just a comment, the compiler enforces which list a reconciler can join.
//
// WorkerReconciler is scoped to the Worker(s) that changed: callers pass the
// affected ids and it converges only their neighbourhood (cheap, and it
// no-ops on an empty set). It is structural — Hire treats a failure as FATAL
// (the new Worker's channels weren't set up), Fire as best-effort.
// *reconcile.Reconciler (activation/team/DM Topics) is the implementer.
type WorkerReconciler interface {
	Reconcile(ctx context.Context, orgID string, affected ...orgchart.WorkerID) error
}

// OrgReconciler converges some derived state for the WHOLE org — it takes no
// affected set. Contract: idempotent and always best-effort (a failure is
// logged and the mutation proceeds; it re-runs on the next mutation / at
// startup). *slackrouting.Reconciler (Slack auto-router routes) is the first
// implementer; new whole-org reconcilers just satisfy this and append.
//
// Both are declared here (the consumer) so lifecycle stays decoupled from
// each reconciler's package.
type OrgReconciler interface {
	Reconcile(ctx context.Context, orgID string) error
}

// Service composes the worker-lifecycle operations the REST layer
// drives. All fields are required; pass nil HelixRuntime only in
// tests that don't need the Helix-side teardown.
type Service struct {
	Store  *store.Store
	Helix  HelixRuntime
	Logger *slog.Logger

	// WorkerReconcilers are the Worker-scoped reconcilers (see the contract
	// on WorkerReconciler) run on hire (FATAL) and fire (best-effort) with the
	// affected Worker ids. Today just the activation/team/DM topology
	// reconciler. Empty/nil is a no-op.
	WorkerReconcilers []WorkerReconciler

	// Mirror is the transcript mirror; Fire stops the fired Worker's
	// subscription so it doesn't leak. nil is a no-op.
	Mirror *helix.Mirror

	// OrgReconcilers are the whole-org, best-effort reconcilers (see the
	// contract on OrgReconciler) run after every hire/fire — Slack auto-router
	// routes today; future ones just append. Empty/nil is a no-op.
	OrgReconcilers []OrgReconciler

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
// per-hire IdentityContent, the initial reporting line, a topology
// reconcile, the hiring-user bookkeeping, and — for AI Workers — a
// pre-allocated hire activation dispatched through the Spawner. This is
// the single implementation the MCP hire_worker tool and the REST POST
// /workers handler both call (no synthetic Invocation).
//
// State lives in the domain (DB) + the per-Worker repo's helix-specs
// git branch (role.md / identity.md / agent.md), which the agent pulls
// inside its sandbox — there is no API-host workspace directory. A
// Worker's MCP tool surface is derived live from Role.Tools — there is
// no per-Worker tool record and no tools param.
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
	// The id becomes a path segment in the helix-specs git layout
	// (workers/<id>/.context/…) and in topic ids — reject any traversal
	// or separator before it propagates.
	if err := orgchart.ValidID(string(id)); err != nil {
		return HireResult{}, fmt.Errorf("worker id: %w", err)
	}

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

	// Reconcile the activation/team Topics implied by the new Worker and its
	// reporting line (mints the hire's transcript + the manager's team Topic
	// from one declarative pass). FATAL: a Worker without its channels is a
	// broken hire. Worker-scoped to the new id.
	for _, rec := range s.WorkerReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID, id); err != nil {
			return HireResult{}, fmt.Errorf("reconcile topology for hire %q: %w", id, err)
		}
	}

	// Run the whole-org reconcilers (Slack auto-router routes, …) for the new
	// Worker. Best-effort: a failure must not abort the hire — each is
	// idempotent and re-runs on the next hire/fire and at startup.
	s.runOrgReconcilers(ctx, orgID, "hire", id)

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
		s.Dispatcher.DispatchHire(ctx, orgID, id, hireActID)
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
//     team Topics and collapse any ex-manager's team Topic that just
//     lost its last report. topology is the single owner of
//     activation/team Topic lifecycle — there is no inline
//     Topics.Delete here any more.
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
// audit trail; only the Topic row is dropped.
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

	// Capture the fired Worker's managers AND reports BEFORE deletion: the
	// reporting lines cascade-drop with the Worker row, so the List* calls
	// return nothing afterward. We feed both sets to the topology reconcile
	// below. The ex-managers let a manager's team Topic collapse when its
	// last report leaves. The ex-reports are needed for DM teardown: the
	// reconciler's DM-channel cleanup is an all-pairs-of-affected scan, so
	// to tear down `s-dm-<fired>-<report>` BOTH endpoints must be in the
	// affected set — without the reports, firing a manager orphans every
	// `s-dm-<manager>-<report>` channel (the report stays subscribed to a
	// DM with a now-deleted worker).
	var exManagers, exReports []orgchart.WorkerID
	if s.Store.ReportingLines != nil {
		exManagers, _ = s.Store.ReportingLines.ListManagers(ctx, orgID, id)
		exReports, _ = s.Store.ReportingLines.ListReports(ctx, orgID, id)
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

	if err := s.Store.Workers.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete worker row %q: %w", id, err)
	}

	// Settle the activation/team Topics now that the row (and its reporting
	// lines) are gone — the Worker-scoped reconcilers tear down the fired
	// Worker's own Topics and collapse an ex-manager's team Topic when its
	// last report just left. Best-effort here (unlike hire): a failure leaves
	// a dangling Topic row, not a half-deleted worker, so we log and continue.
	affected := append([]orgchart.WorkerID{id}, exManagers...)
	affected = append(affected, exReports...)
	for _, rec := range s.WorkerReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
			s.logger().Warn("fire: reconcile topology", "worker", id, "err", err)
		}
	}

	// Run the whole-org reconcilers so the fired Worker's Slack auto-route is
	// GC'd (its subscription already cascaded with the row; the reconciler
	// drops the route + owned Topic). Best-effort, like topology above.
	s.runOrgReconcilers(ctx, orgID, "fire", id)
	return nil
}

// runOrgReconcilers runs every whole-org reconciler best-effort, logging (not
// propagating) failures so one reconciler can't abort the lifecycle mutation
// or block the others. phase/worker are for the log line only.
func (s *Service) runOrgReconcilers(ctx context.Context, orgID, phase string, worker orgchart.WorkerID) {
	for _, rec := range s.OrgReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID); err != nil {
			s.logger().Warn(phase+": reconcile", "worker", worker, "err", err)
		}
	}
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
