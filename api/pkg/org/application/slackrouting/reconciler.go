// Package slackrouting keeps each Slack auto-router's per-Worker routes in
// sync with the org's Workers. It is a second, independent reconciler that
// *composes* the existing processor machinery (it drives the processors
// application service to add/remove route Outputs) rather than extending
// reconcile.Reconciler — exactly the composition the feature calls for.
//
// What it owns: for every Automated Slack router Processor in an org, one
// "managed route" per AI Worker — a filter Output whose predicate is
// `mentions "<name>" .Message.body` (word-boundary, case-insensitive) and
// whose auto-provisioned output Topic the Worker is subscribed to. The
// route carries the Worker id in Output.ManagedFor, which is the only thing
// the reconciler keys on:
//
//   - AI Worker exists, no managed route → add the route + subscribe.
//   - Managed route whose Worker no longer exists (or is no longer AI) →
//     remove the route (cascading its owned Topic + subscription).
//   - Manual routes (empty ManagedFor) and the default route → never touched.
//   - Existing managed route for a live Worker → left as-is (so user edits
//     to the predicate survive), but its subscription is self-healed.
//
// It never *creates* the router — that is bound to the workspace-connect
// event — so a deleted router stays deleted (Reconcile is a no-op when no
// Automated router exists). Worker names are immutable ids, so the
// reconciler only ever reconciles route presence/absence, never content;
// there is no fingerprint and no rename-sync.
package slackrouting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// ProcessorService is the slice of the processors application service the
// reconciler drives. *processors.Processors satisfies it.
type ProcessorService interface {
	List(ctx context.Context, orgID string) ([]processor.Processor, error)
	AddOutput(ctx context.Context, orgID string, id processor.ProcessorID, spec processors.OutputSpec) (processor.Output, error)
	RemoveOutput(ctx context.Context, orgID string, id processor.ProcessorID, outputID string) error
}

// Attacher is the slice of the attachment service the reconciler drives.
// *attachments.Service satisfies it.
type Attacher interface {
	AttachAll(ctx context.Context, orgID string, workerID orgchart.NodeID, sources []eventsource.SourceRef, createdBy string) error
}

// Reconciler converges Automated Slack routers' managed routes onto the
// org's Nodes. Construct with New.
type Reconciler struct {
	bots     store.Nodes
	attacher Attacher
	procs    ProcessorService
	now      func() time.Time
	logger   *slog.Logger
}

// Deps are the constructor-injected collaborators.
type Deps struct {
	Nodes       store.Nodes
	Attachments Attacher
	Processors  ProcessorService
	Now         func() time.Time
	Logger      *slog.Logger
}

// New builds a Reconciler. A nil Nodes or Processors repo yields a
// Reconciler whose Reconcile no-ops, so runtimes/tests that don't wire
// Slack routing degrade gracefully.
func New(deps Deps) *Reconciler {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{
		bots:     deps.Nodes,
		attacher: deps.Attachments,
		procs:    deps.Processors,
		now:      now,
		logger:   logger,
	}
}

// Reconcile converges every Automated Slack router in the org. Safe to call
// on every hire/fire and at startup; idempotent. A nil/unwired Reconciler,
// or an org with no Automated router, is a no-op.
func (r *Reconciler) Reconcile(ctx context.Context, orgID string) error {
	if r == nil || r.bots == nil || r.procs == nil {
		return nil
	}

	procs, err := r.procs.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("slackrouting: list processors: %w", err)
	}
	var routers []processor.Processor
	for _, p := range procs {
		if p.Automated() && p.Kind == processor.KindFilter {
			routers = append(routers, p)
		}
	}
	if len(routers) == 0 {
		return nil // nothing to maintain — router never created or deleted
	}

	bots, err := r.bots.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("slackrouting: list bots: %w", err)
	}
	// The Nodes the router should route to: every bot activates. Keyed by
	// id for diffing against ManagedFor.
	botIDs := map[orgchart.NodeID]struct{}{}
	for _, b := range bots {
		botIDs[b.ID] = struct{}{}
	}

	for _, router := range routers {
		if err := r.reconcileRouter(ctx, orgID, router, botIDs); err != nil {
			return err
		}
	}
	return nil
}

// reconcileRouter brings one router's managed routes to match botIDs.
func (r *Reconciler) reconcileRouter(ctx context.Context, orgID string, router processor.Processor, botIDs map[orgchart.NodeID]struct{}) error {
	managed := map[orgchart.NodeID]processor.Output{} // ManagedFor → route
	for _, o := range router.Outputs {
		if o.ManagedFor != "" {
			managed[orgchart.NodeID(o.ManagedFor)] = o
		}
	}

	// Remove managed routes whose Bot is gone.
	for botID, out := range managed {
		if _, ok := botIDs[botID]; ok {
			continue
		}
		if err := r.procs.RemoveOutput(ctx, orgID, router.ID, out.ID); err != nil {
			return fmt.Errorf("slackrouting: remove route for %q: %w", botID, err)
		}
		r.logger.Info("slackrouting: removed route for departed bot", "router", router.ID, "bot", botID)
	}

	// Add a managed route for every Bot that lacks one; self-heal the
	// attachment for those that already have one.
	for botID := range botIDs {
		if out, ok := managed[botID]; ok {
			if err := r.ensureAttached(ctx, orgID, botID, router.ID, out.ID); err != nil {
				return err
			}
			continue
		}
		out, err := r.procs.AddOutput(ctx, orgID, router.ID, processors.OutputSpec{
			Label:      string(botID),
			Match:      matchPredicate(botID),
			ManagedFor: string(botID),
		})
		if err != nil {
			return fmt.Errorf("slackrouting: add route for %q: %w", botID, err)
		}
		if err := r.ensureAttached(ctx, orgID, botID, router.ID, out.ID); err != nil {
			return err
		}
		r.logger.Info("slackrouting: added route for bot", "router", router.ID, "bot", botID, "output", out.ID)
	}
	return nil
}

// ensureAttached idempotently attaches the Bot to the route's output
// branch, so a managed route always delivers even if the attachment was
// dropped out-of-band.
func (r *Reconciler) ensureAttached(ctx context.Context, orgID string, botID orgchart.NodeID, routerID processor.ProcessorID, outputID string) error {
	if r.attacher == nil {
		return nil
	}
	src := eventsource.ProcessorOutput(routerID, outputID)
	if err := r.attacher.AttachAll(ctx, orgID, botID, []eventsource.SourceRef{src}, processor.SystemActor); err != nil {
		return fmt.Errorf("slackrouting: attach %q→%s: %w", botID, src.Key(), err)
	}
	return nil
}

// matchPredicate builds the filter predicate for a managed route — a
// word-boundary, case-insensitive mention of the Bot's FULL id in the body
// (`b-jokebot`, not `jokebot`). The id is the Bot's canonical name and is
// what the org refers to it by; matching the bare slug would over-trigger on
// common words. `mentions`'s `\b…\b` handles the internal hyphen fine.
func matchPredicate(botID orgchart.NodeID) string {
	return fmt.Sprintf(`{{ mentions %q .Message.body }}`, string(botID))
}
