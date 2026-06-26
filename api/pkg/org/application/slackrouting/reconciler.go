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
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// ProcessorService is the slice of the processors application service the
// reconciler drives. *processors.Processors satisfies it.
type ProcessorService interface {
	List(ctx context.Context, orgID string) ([]processor.Processor, error)
	AddOutput(ctx context.Context, orgID string, id processor.ProcessorID, spec processors.OutputSpec) (processor.Output, error)
	RemoveOutput(ctx context.Context, orgID string, id processor.ProcessorID, topicID streaming.TopicID) error
}

// Reconciler converges Automated Slack routers' managed routes onto the
// org's AI Workers. Construct with New.
type Reconciler struct {
	workers store.Workers
	subs    store.Subscriptions
	procs   ProcessorService
	now     func() time.Time
	logger  *slog.Logger
}

// Deps are the constructor-injected collaborators.
type Deps struct {
	Workers       store.Workers
	Subscriptions store.Subscriptions
	Processors    ProcessorService
	Now           func() time.Time
	Logger        *slog.Logger
}

// New builds a Reconciler. A nil Workers or Processors repo yields a
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
		workers: deps.Workers,
		subs:    deps.Subscriptions,
		procs:   deps.Processors,
		now:     now,
		logger:  logger,
	}
}

// Reconcile converges every Automated Slack router in the org. Safe to call
// on every hire/fire and at startup; idempotent. A nil/unwired Reconciler,
// or an org with no Automated router, is a no-op.
func (r *Reconciler) Reconcile(ctx context.Context, orgID string) error {
	if r == nil || r.workers == nil || r.procs == nil {
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

	workers, err := r.workers.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("slackrouting: list workers: %w", err)
	}
	// The Workers the router should route to: AI Workers only (only they
	// activate). Keyed by id for diffing against ManagedFor.
	aiWorkers := map[orgchart.WorkerID]struct{}{}
	for _, w := range workers {
		if w.Kind() == orgchart.WorkerKindAI {
			aiWorkers[w.ID()] = struct{}{}
		}
	}

	for _, router := range routers {
		if err := r.reconcileRouter(ctx, orgID, router, aiWorkers); err != nil {
			return err
		}
	}
	return nil
}

// reconcileRouter brings one router's managed routes to match aiWorkers.
func (r *Reconciler) reconcileRouter(ctx context.Context, orgID string, router processor.Processor, aiWorkers map[orgchart.WorkerID]struct{}) error {
	managed := map[orgchart.WorkerID]processor.Output{} // ManagedFor → route
	for _, o := range router.Outputs {
		if o.ManagedFor != "" {
			managed[orgchart.WorkerID(o.ManagedFor)] = o
		}
	}

	// Remove managed routes whose Worker is gone (or no longer AI).
	for workerID, out := range managed {
		if _, ok := aiWorkers[workerID]; ok {
			continue
		}
		if err := r.procs.RemoveOutput(ctx, orgID, router.ID, out.TopicID); err != nil {
			return fmt.Errorf("slackrouting: remove route for %q: %w", workerID, err)
		}
		r.logger.Info("slackrouting: removed route for departed worker", "router", router.ID, "worker", workerID)
	}

	// Add a managed route for every AI Worker that lacks one; self-heal the
	// subscription for those that already have one.
	for workerID := range aiWorkers {
		if out, ok := managed[workerID]; ok {
			if err := r.ensureSubscribed(ctx, orgID, workerID, out.TopicID); err != nil {
				return err
			}
			continue
		}
		name := routeName(workerID)
		out, err := r.procs.AddOutput(ctx, orgID, router.ID, processors.OutputSpec{
			Label:      name,
			Match:      matchPredicate(name),
			ManagedFor: string(workerID),
		})
		if err != nil {
			return fmt.Errorf("slackrouting: add route for %q: %w", workerID, err)
		}
		if err := r.ensureSubscribed(ctx, orgID, workerID, out.TopicID); err != nil {
			return err
		}
		r.logger.Info("slackrouting: added route for worker", "router", router.ID, "worker", workerID, "topic", out.TopicID)
	}
	return nil
}

// ensureSubscribed idempotently subscribes the Worker to the route's output
// Topic, so a managed route always delivers even if the subscription was
// dropped out-of-band.
func (r *Reconciler) ensureSubscribed(ctx context.Context, orgID string, workerID orgchart.WorkerID, topicID streaming.TopicID) error {
	if r.subs == nil {
		return nil
	}
	if _, err := r.subs.Find(ctx, orgID, workerID, topicID); err == nil {
		return nil // already subscribed
	}
	sub, err := streaming.NewSubscription(string(workerID), topicID, r.now(), orgID)
	if err != nil {
		return fmt.Errorf("slackrouting: build subscription %q→%q: %w", workerID, topicID, err)
	}
	if err := r.subs.Create(ctx, sub); err != nil {
		// Lost a create race? A now-present row means success.
		if _, findErr := r.subs.Find(ctx, orgID, workerID, topicID); findErr != nil {
			return fmt.Errorf("slackrouting: subscribe %q→%q: %w", workerID, topicID, err)
		}
	}
	return nil
}

// routeName is the human-facing name a Worker is mentioned by: the Worker id
// with its `w-` prefix stripped (`w-alice` → `alice`).
func routeName(id orgchart.WorkerID) string {
	return strings.TrimPrefix(string(id), "w-")
}

// matchPredicate builds the filter predicate for a managed route — a
// word-boundary, case-insensitive mention of the Worker's name in the body.
func matchPredicate(name string) string {
	return fmt.Sprintf(`{{ mentions %q .Message.body }}`, name)
}
