package topology

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Reconciler converges the persisted Streams/Subscriptions onto the
// channels the reporting graph requires. It depends only on the four
// narrow repositories it actually touches — Workers, ReportingLines,
// Streams, Subscriptions — never the whole *store.Store (CLAUDE.md
// helix-org philosophy: small interfaces, ≤4 collaborators). That is what
// keeps it table-testable and lets every structural mutation depend on it
// without pulling in the heavyweight lifecycle service.
type Reconciler struct {
	workers store.Workers
	lines   store.ReportingLines
	streams store.Streams
	subs    store.Subscriptions
	now     func() time.Time
}

// Deps are the constructor-injected collaborators for NewReconciler.
// ReportingLines is optional: a store that doesn't wire it yields a graph
// with no reporting edges (activation streams only).
type Deps struct {
	Workers        store.Workers
	ReportingLines store.ReportingLines
	Streams        store.Streams
	Subscriptions  store.Subscriptions
	// Now seams the clock for tests. Falls back to time.Now().UTC().
	Now func() time.Time
}

// NewReconciler builds a Reconciler from its narrow repositories. A nil
// Workers repo (the "not wired" case) yields a Reconciler whose methods
// no-op, so runtimes/tests that don't wire topology degrade gracefully.
func NewReconciler(deps Deps) *Reconciler {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Reconciler{
		workers: deps.Workers,
		lines:   deps.ReportingLines,
		streams: deps.Streams,
		subs:    deps.Subscriptions,
		now:     now,
	}
}

// Reconcile settles the activation/team Streams touched by a change to
// the given affected Worker(s). It loads the whole graph, computes the
// required channels, then — only for the Streams owned by the affected
// Workers and their one-hop managers/reports — diffs required vs actual
// and applies create-stream / subscribe / unsubscribe / delete-stream
// idempotently.
//
// Scoping to the affected Workers' streams (rather than every stream in
// the org) is what keeps Reconcile from touching DM streams or
// operator-created streams: the only Stream ids it ever considers are
// `s-activations-<id>` and `s-team-<id>` for the affected Workers and
// their immediate neighbours.
//
// Callers announce what changed:
//   - hire W              → Reconcile(org, W)
//   - add/remove W→M line → Reconcile(org, W, M)
//   - fire W (managers M…)→ Reconcile(org, W, M…)  (capture M… first)
//
// A nil or unwired Reconciler is a no-op, so runtimes/tests that don't
// wire topology degrade gracefully.
func (r *Reconciler) Reconcile(ctx context.Context, orgID string, affected ...orgchart.WorkerID) error {
	if r == nil || r.workers == nil {
		return nil
	}
	if len(affected) == 0 {
		return nil
	}

	workers, err := r.workers.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("topology: list workers: %w", err)
	}
	var lines []orgchart.ReportingLine
	if r.lines != nil {
		lines, err = r.lines.List(ctx, orgID)
		if err != nil {
			return fmt.Errorf("topology: list reporting lines: %w", err)
		}
	}

	required := channels.Required(workers, lines)

	// Bucket required members by stream so each ensure is O(members).
	requiredMembers := map[streaming.StreamID][]orgchart.WorkerID{}
	for k := range required.Members {
		requiredMembers[k.StreamID] = append(requiredMembers[k.StreamID], k.WorkerID)
	}

	// Index the (current) graph to find each affected Worker's one-hop
	// neighbours — their team/activation streams can move too.
	managersByReport := map[orgchart.WorkerID][]orgchart.WorkerID{}
	reportsByManager := map[orgchart.WorkerID][]orgchart.WorkerID{}
	for _, l := range lines {
		managersByReport[l.ReportID] = append(managersByReport[l.ReportID], l.ManagerID)
		reportsByManager[l.ManagerID] = append(reportsByManager[l.ManagerID], l.ReportID)
	}

	// Collect the Stream ids in scope. Only ever activation / team / DM
	// stream ids derived from the affected Workers and their one-hop
	// neighbours — never an operator-created stream.
	relevant := map[streaming.StreamID]struct{}{}
	for _, a := range affected {
		relevant[activation.StreamID(a)] = struct{}{}
		relevant[channels.TeamStreamID(a)] = struct{}{}
		// A manager's team stream gains/loses this Worker as a member,
		// and the manager↔this-Worker DM channel is created/kept.
		for _, m := range managersByReport[a] {
			relevant[channels.TeamStreamID(m)] = struct{}{}
			relevant[channels.DMStreamID(a, m)] = struct{}{}
		}
		// A report's activation stream gains/loses this Worker as an
		// observer, and the this-Worker↔report DM channel is
		// created/kept.
		for _, rep := range reportsByManager[a] {
			relevant[activation.StreamID(rep)] = struct{}{}
			relevant[channels.DMStreamID(a, rep)] = struct{}{}
		}
	}
	// All-pairs of the affected set covers DM-channel *teardown*: when a
	// reporting edge is removed the two endpoints are no longer one
	// another's neighbours, so the neighbour walk above wouldn't reach
	// their DM channel. Both endpoints are passed in `affected`
	// (add/remove-parent pass (report, manager); fire passes
	// (firedID, ex-managers…)), so the pair is named here and the diff
	// below deletes the now-unrequired channel.
	for i := 0; i < len(affected); i++ {
		for j := i + 1; j < len(affected); j++ {
			relevant[channels.DMStreamID(affected[i], affected[j])] = struct{}{}
		}
	}

	ids := make([]streaming.StreamID, 0, len(relevant))
	for sid := range relevant {
		ids = append(ids, sid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	now := r.clock()
	for _, sid := range ids {
		ch, want := required.Channels[sid]
		if !want {
			// The Stream should not exist. Delete it (subscriptions
			// cascade with the row). Absent already → fine.
			if err := r.streams.Delete(ctx, orgID, sid); err != nil && !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("topology: delete stream %q: %w", sid, err)
			}
			continue
		}
		if err := r.ensureStream(ctx, orgID, ch, requiredMembers[sid], now); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileAll converges the full topology for every Worker in the org.
// Call at server startup so Workers hired before the reconciler was
// wired (or before a new channel rule was added) get their activation,
// team, and DM Streams created or corrected idempotently. Internally
// loads every Worker ID and delegates to Reconcile so the scoping and
// create/delete/subscribe logic stays in one place.
func (r *Reconciler) ReconcileAll(ctx context.Context, orgID string) error {
	if r == nil || r.workers == nil {
		return nil
	}
	workers, err := r.workers.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("topology: ReconcileAll list workers: %w", err)
	}
	if len(workers) == 0 {
		return nil
	}
	ids := make([]orgchart.WorkerID, len(workers))
	for i, w := range workers {
		ids[i] = w.ID()
	}
	return r.Reconcile(ctx, orgID, ids...)
}

func (r *Reconciler) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now().UTC()
}

// ensureStream converges one managed Stream: create it if missing,
// subscribe every required member, and unsubscribe anyone the required
// set no longer includes (the load-bearing half — this is what fixes
// the reparent desync where an old manager stayed subscribed).
func (r *Reconciler) ensureStream(ctx context.Context, orgID string, ch channels.Channel, members []orgchart.WorkerID, now time.Time) error {
	stream, err := streamForChannel(ch, now, orgID)
	if err != nil {
		return fmt.Errorf("topology: build stream %q: %w", ch.ID, err)
	}
	if err := r.ensureStreamWithMembers(ctx, stream, now, members...); err != nil {
		return fmt.Errorf("topology: ensure stream %q: %w", ch.ID, err)
	}

	requiredSet := make(map[orgchart.WorkerID]struct{}, len(members))
	for _, m := range members {
		requiredSet[m] = struct{}{}
	}
	actual, err := r.subs.ListForStream(ctx, orgID, ch.ID)
	if err != nil {
		return fmt.Errorf("topology: list subscribers of %q: %w", ch.ID, err)
	}
	for _, sub := range actual {
		if _, ok := requiredSet[orgchart.WorkerID(sub.WorkerID)]; ok {
			continue
		}
		if err := r.subs.Delete(ctx, orgID, orgchart.WorkerID(sub.WorkerID), ch.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("topology: unsubscribe %q from %q: %w", sub.WorkerID, ch.ID, err)
		}
	}
	return nil
}
