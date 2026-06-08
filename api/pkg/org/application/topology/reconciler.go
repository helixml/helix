package topology

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Reconciler converges the persisted Streams/Subscriptions onto the
// desired topology implied by the reporting graph. It needs only the
// store — no runtime, no disk — which is what keeps it table-testable
// and lets every structural mutation depend on it without pulling in
// the heavyweight lifecycle service.
type Reconciler struct {
	Store *store.Store
	// Now seams the clock for tests. Falls back to time.Now().UTC().
	Now func() time.Time
}

func (r *Reconciler) now() time.Time {
	if r != nil && r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

// Reconcile settles the activation/team Streams touched by a change to
// the given affected Worker(s). It loads the whole graph, computes the
// desired topology, then — only for the Streams owned by the affected
// Workers and their one-hop managers/reports — diffs desired vs actual
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
// A nil or store-less Reconciler is a no-op, so runtimes/tests that
// don't wire topology degrade gracefully.
func (r *Reconciler) Reconcile(ctx context.Context, orgID string, affected ...orgchart.WorkerID) error {
	if r == nil || r.Store == nil {
		return nil
	}
	if len(affected) == 0 {
		return nil
	}

	workers, err := r.Store.Workers.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("topology: list workers: %w", err)
	}
	var lines []orgchart.ReportingLine
	if r.Store.ReportingLines != nil {
		lines, err = r.Store.ReportingLines.List(ctx, orgID)
		if err != nil {
			return fmt.Errorf("topology: list reporting lines: %w", err)
		}
	}

	desired := DesiredTopology(workers, lines)

	// Bucket desired subscribers by stream so each ensure is O(members).
	desiredSubs := map[streaming.StreamID][]orgchart.WorkerID{}
	for k := range desired.Subs {
		desiredSubs[k.StreamID] = append(desiredSubs[k.StreamID], k.WorkerID)
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
		relevant[TeamStreamID(a)] = struct{}{}
		// A manager's team stream gains/loses this Worker as a member,
		// and the manager↔this-Worker DM channel is created/kept.
		for _, m := range managersByReport[a] {
			relevant[TeamStreamID(m)] = struct{}{}
			relevant[DMStreamID(a, m)] = struct{}{}
		}
		// A report's activation stream gains/loses this Worker as an
		// observer, and the this-Worker↔report DM channel is
		// created/kept.
		for _, rep := range reportsByManager[a] {
			relevant[activation.StreamID(rep)] = struct{}{}
			relevant[DMStreamID(a, rep)] = struct{}{}
		}
	}
	// All-pairs of the affected set covers DM-channel *teardown*: when a
	// reporting edge is removed the two endpoints are no longer one
	// another's neighbours, so the neighbour walk above wouldn't reach
	// their DM channel. Both endpoints are passed in `affected`
	// (add/remove-parent pass (report, manager); fire passes
	// (firedID, ex-managers…)), so the pair is named here and the diff
	// below deletes the now-undesired channel.
	for i := 0; i < len(affected); i++ {
		for j := i + 1; j < len(affected); j++ {
			relevant[DMStreamID(affected[i], affected[j])] = struct{}{}
		}
	}

	ids := make([]streaming.StreamID, 0, len(relevant))
	for sid := range relevant {
		ids = append(ids, sid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	now := r.now()
	for _, sid := range ids {
		ds, want := desired.Streams[sid]
		if !want {
			// The Stream should not exist. Delete it (subscriptions
			// cascade with the row). Absent already → fine.
			if err := r.Store.Streams.Delete(ctx, orgID, sid); err != nil && !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("topology: delete stream %q: %w", sid, err)
			}
			continue
		}
		if err := r.ensureStream(ctx, orgID, ds, desiredSubs[sid], now); err != nil {
			return err
		}
	}
	return nil
}

// ensureStream converges one managed Stream: create it if missing,
// subscribe every desired member, and unsubscribe anyone the desired
// set no longer includes (the load-bearing half — this is what fixes
// the reparent desync where an old manager stayed subscribed).
func (r *Reconciler) ensureStream(ctx context.Context, orgID string, ds DesiredStream, members []orgchart.WorkerID, now time.Time) error {
	stream, err := newDesiredStream(ds, now, orgID)
	if err != nil {
		return fmt.Errorf("topology: build stream %q: %w", ds.ID, err)
	}
	if err := EnsureStreamWithMembers(ctx, r.Store, stream, now, members...); err != nil {
		return fmt.Errorf("topology: ensure stream %q: %w", ds.ID, err)
	}

	desiredSet := make(map[orgchart.WorkerID]struct{}, len(members))
	for _, m := range members {
		desiredSet[m] = struct{}{}
	}
	actual, err := r.Store.Subscriptions.ListForStream(ctx, orgID, ds.ID)
	if err != nil {
		return fmt.Errorf("topology: list subscribers of %q: %w", ds.ID, err)
	}
	for _, sub := range actual {
		if _, ok := desiredSet[orgchart.WorkerID(sub.WorkerID)]; ok {
			continue
		}
		if err := r.Store.Subscriptions.Delete(ctx, orgID, orgchart.WorkerID(sub.WorkerID), ds.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("topology: unsubscribe %q from %q: %w", sub.WorkerID, ds.ID, err)
		}
	}
	return nil
}
