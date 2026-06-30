// Package lifecycle owns the cross-cutting orchestration that
// composes store + runtime when a Bot is created or destroyed.
//
// This package owns the two halves of the Bot lifecycle: Create (the
// create cascade — bot row, reporting line, topology reconcile,
// create-activation dispatch) and Delete (the destroy cascade — Helix
// project/app teardown, store cleanup, topology reconcile). Both REST
// and the MCP create_bot tool drive Create here, so the semantics
// cannot drift between callers. Delete has no MCP counterpart by design
// (the LLM should not be able to delete bots from chat), so it is a
// plain Go service callable from REST handlers only.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// CreateDispatcher fires the per-create activation for a new Bot. A
// narrow interface so the lifecycle service doesn't import the tools
// package. The composition root's dispatcher satisfies it.
type CreateDispatcher interface {
	DispatchHire(ctx context.Context, orgID string, botID orgchart.BotID, activationID activation.ID)
}

// HelixRuntime is the slice of runtime/helix.ProjectService that the
// Delete cascade needs to tear down a Bot's Helix-side project and
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
// BotReconciler is scoped to the Bot(s) that changed: callers pass the
// affected ids and it converges only their neighbourhood (cheap, and it
// no-ops on an empty set). It is structural — Create treats a failure as
// FATAL (the new Bot's channels weren't set up), Delete as best-effort.
// *reconcile.Reconciler (activation/team/DM Topics) is the implementer.
type BotReconciler interface {
	Reconcile(ctx context.Context, orgID string, affected ...orgchart.BotID) error
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

// Service composes the bot-lifecycle operations the REST/MCP layers
// drive. All fields are required; pass nil HelixRuntime only in tests
// that don't need the Helix-side teardown.
type Service struct {
	Store  *store.Store
	Helix  HelixRuntime
	Logger *slog.Logger

	// Bots is the bot-mutation service Create delegates the row creation
	// to, so the base-tool union and id minting are shared with the
	// REST/MCP update path. Required for Create.
	Bots *bots.Bots

	// BotReconcilers are the Bot-scoped reconcilers (see the contract on
	// BotReconciler) run on create (FATAL) and delete (best-effort) with
	// the affected Bot ids. Today just the activation/team/DM topology
	// reconciler. Empty/nil is a no-op.
	BotReconcilers []BotReconciler

	// Mirror is the transcript mirror; Delete stops the deleted Bot's
	// subscription so it doesn't leak. nil is a no-op.
	Mirror *helix.Mirror

	// OrgReconcilers are the whole-org, best-effort reconcilers (see the
	// contract on OrgReconciler) run after every create/delete — Slack
	// auto-router routes today; future ones just append. Empty/nil is a
	// no-op.
	OrgReconcilers []OrgReconciler

	// --- Create collaborators ---

	// Dispatcher fires the per-create activation for a new Bot.
	// nil → no activation is dispatched (tests / runtimes without a
	// dispatcher).
	Dispatcher CreateDispatcher
	// HireHook runs runtime bookkeeping after the bot row exists — the
	// helix runtime persists the creating user. nil is a no-op.
	HireHook runtime.HireHook
	// Now / NewID seam the clock and id-generator. Both are required for
	// Create; Delete does not use them.
	Now   func() time.Time
	NewID func() string
}

// CreateParams describes a new Bot. ID is optional — when empty the
// bots service mints a fresh `b-<id>` (discouraged; callers should pass
// a readable handle). ParentID is the manager this bot reports to
// (empty only for the org root). Content is the bot's prompt; Tools and
// Topics are its capability/manifest.
type CreateParams struct {
	ID              string
	Content         string
	Tools           []tool.Name
	Topics          []streaming.TopicID
	ParentID        orgchart.BotID
	PreserveContext bool
}

// CreateResult carries the new Bot and the pre-allocated
// create-activation id.
type CreateResult struct {
	Bot          orgchart.Bot
	ActivationID activation.ID
}

// Create brings a Bot into existence: the bot row (content + tools, via
// the bots service so the base-read-tool union is applied), the initial
// reporting line, a topology reconcile, the creating-user bookkeeping,
// and a pre-allocated create activation dispatched through the Spawner.
// This is the single implementation the MCP create_bot tool and the
// REST POST /bots handler both call.
//
// State lives in the domain (DB) + the per-Bot repo's helix-specs git
// branch (role.md / agent.md), which the agent pulls inside its sandbox
// — there is no API-host workspace directory. A Bot's MCP tool surface
// is derived live from Bot.Tools.
func (s *Service) Create(ctx context.Context, orgID string, p CreateParams) (CreateResult, error) {
	if s.NewID == nil || s.Now == nil {
		return CreateResult{}, fmt.Errorf("lifecycle: clock/id-generator not wired")
	}
	if s.Store == nil {
		return CreateResult{}, errors.New("lifecycle: store is nil")
	}
	if s.Bots == nil {
		return CreateResult{}, errors.New("lifecycle: bots service not wired")
	}

	var parent *orgchart.BotID
	if p.ParentID != "" {
		if _, err := s.Store.Bots.Get(ctx, orgID, p.ParentID); err != nil {
			return CreateResult{}, fmt.Errorf("parent bot %q: %w", p.ParentID, err)
		}
		parent = &p.ParentID
	}

	bot, err := s.Bots.Create(ctx, orgID, bots.CreateParams{
		ID:              p.ID,
		Content:         p.Content,
		Tools:           p.Tools,
		Topics:          p.Topics,
		PreserveContext: p.PreserveContext,
	})
	if err != nil {
		return CreateResult{}, err
	}
	id := bot.ID

	// Wire the initial reporting line now that both bot rows exist.
	if parent != nil && s.Store.ReportingLines != nil {
		line, err := orgchart.NewReportingLine(orgID, *parent, id)
		if err != nil {
			return CreateResult{}, err
		}
		if err := s.Store.ReportingLines.Add(ctx, line); err != nil {
			return CreateResult{}, fmt.Errorf("add reporting line: %w", err)
		}
	}

	// Reconcile the activation/team Topics implied by the new Bot and its
	// reporting line (mints the bot's transcript + the manager's team Topic
	// from one declarative pass). FATAL: a Bot without its channels is a
	// broken create. Bot-scoped to the new id.
	for _, rec := range s.BotReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID, id); err != nil {
			return CreateResult{}, fmt.Errorf("reconcile topology for bot %q: %w", id, err)
		}
	}

	// Run the whole-org reconcilers (Slack auto-router routes, …) for the new
	// Bot. Best-effort: a failure must not abort the create — each is
	// idempotent and re-runs on the next create/delete and at startup.
	s.runOrgReconcilers(ctx, orgID, "create", id)

	// Persist the creating user's identity (if the request carried one)
	// BEFORE dispatch so the Spawner picks it up on its first call.
	if uid := helix.UserIDFromContext(ctx); uid != "" && s.HireHook != nil {
		if err := s.HireHook.OnHire(ctx, orgID, id, uid); err != nil {
			return CreateResult{}, fmt.Errorf("create handler: %w", err)
		}
	}

	// Pre-create the create-Activation audit row so Create can return the
	// id synchronously; the Spawner Completes it (matched by
	// Trigger.ActivationID) rather than minting a sibling. Every bot is an
	// agent, so every bot gets a create activation.
	var actID activation.ID
	if s.Store.Activations != nil {
		actID = activation.ID("a-" + s.NewID())
		act, err := activation.New(actID, id, []activation.Trigger{{Kind: activation.TriggerHire}}, s.Now(), orgID)
		if err != nil {
			return CreateResult{}, fmt.Errorf("build create activation: %w", err)
		}
		if err := s.Store.Activations.Create(ctx, act); err != nil {
			return CreateResult{}, fmt.Errorf("persist create activation: %w", err)
		}
	}
	if s.Dispatcher != nil {
		s.Dispatcher.DispatchHire(ctx, orgID, id, actID)
	}

	return CreateResult{Bot: bot, ActivationID: actID}, nil
}

// Delete tears down a Bot end-to-end:
//
//  1. Read the Helix-runtime state (project + app IDs) before clearing.
//  2. DeleteProject on Helix — stops any active sessions.
//  3. DeleteApp on Helix — removes the auto-provisioned agent app.
//  4. Clear the BotRuntimeState sidecar.
//  5. Delete the bot row (its subscriptions cascade with it; the
//     org_reporting_lines ON DELETE CASCADE foreign keys drop its lines).
//  6. Reconcile topology: tear down the deleted Bot's own activation +
//     team Topics and collapse any ex-manager's team Topic that just
//     lost its last report.
//
// Steps 2/3/6 are best-effort and logged on failure — a half-torn bot is
// better than refusing to clean up partial state. Step 5 is the only step
// whose error propagates (the row is the user-visible source of truth).
//
// Subscriptions are bot-anchored, so they die with the bot. Activation
// events themselves are intentionally left behind as an audit trail; only
// the Topic row is dropped.
func (s *Service) Delete(ctx context.Context, orgID string, id orgchart.BotID) error {
	if id == "" {
		return errors.New("bot id is empty")
	}
	if s.Store == nil {
		return errors.New("lifecycle: store is nil")
	}
	if _, err := s.Store.Bots.Get(ctx, orgID, id); err != nil {
		return fmt.Errorf("get bot %q: %w", id, err)
	}

	// Capture the deleted Bot's managers AND reports BEFORE deletion: the
	// reporting lines cascade-drop with the bot row, so the List* calls
	// return nothing afterward. We feed both sets to the topology reconcile
	// below. The ex-managers let a manager's team Topic collapse when its
	// last report leaves. The ex-reports are needed for DM teardown: the
	// reconciler's DM-channel cleanup is an all-pairs-of-affected scan, so
	// to tear down `s-dm-<deleted>-<report>` BOTH endpoints must be in the
	// affected set.
	var exManagers, exReports []orgchart.BotID
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
			s.logger().Warn("delete: delete helix project", "bot", id, "project", state.ProjectID, "err", err)
		}
	}
	if s.Helix != nil && state.AgentAppID != "" {
		if err := s.Helix.DeleteApp(ctx, state.AgentAppID); err != nil && !errors.Is(err, helix.ErrProjectNotFound) {
			s.logger().Warn("delete: delete helix app", "bot", id, "app", state.AgentAppID, "err", err)
		}
	}
	if s.Store.BotRuntimeState != nil {
		if err := s.Store.BotRuntimeState.Clear(ctx, orgID, id, helix.Backend); err != nil {
			s.logger().Warn("delete: clear runtime state", "bot", id, "err", err)
		}
	}

	// The bot's subscriptions and every reporting line that references it
	// are cascaded structurally when the bot row is deleted below
	// (Bots.Delete drops the subs; the org_reporting_lines ON DELETE
	// CASCADE foreign keys drop the lines). Nothing to drop explicitly.

	if err := s.Store.Bots.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete bot row %q: %w", id, err)
	}

	// Settle the activation/team Topics now that the row (and its reporting
	// lines) are gone. Best-effort: a failure leaves a dangling Topic row,
	// not a half-deleted bot, so we log and continue.
	affected := append([]orgchart.BotID{id}, exManagers...)
	affected = append(affected, exReports...)
	for _, rec := range s.BotReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
			s.logger().Warn("delete: reconcile topology", "bot", id, "err", err)
		}
	}

	// Run the whole-org reconcilers so the deleted Bot's Slack auto-route is
	// GC'd. Best-effort, like topology above.
	s.runOrgReconcilers(ctx, orgID, "delete", id)
	return nil
}

// runOrgReconcilers runs every whole-org reconciler best-effort, logging (not
// propagating) failures so one reconciler can't abort the lifecycle mutation
// or block the others. phase/bot are for the log line only.
func (s *Service) runOrgReconcilers(ctx context.Context, orgID, phase string, bot orgchart.BotID) {
	for _, rec := range s.OrgReconcilers {
		if rec == nil {
			continue
		}
		if err := rec.Reconcile(ctx, orgID); err != nil {
			s.logger().Warn(phase+": reconcile", "bot", bot, "err", err)
		}
	}
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
