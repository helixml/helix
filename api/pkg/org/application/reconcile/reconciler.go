// Package reconcile is the application-layer reconciler that converges
// the persisted Streams/Subscriptions onto the channels the reporting
// graph requires. The pure derivation — "what Streams and Subscriptions
// does this graph require?" — lives in domain/channels; this package
// loads the graph from the store, calls channels.Required, diffs the
// required set against what's persisted, and applies create/subscribe/
// unsubscribe/delete idempotently.
//
// The Reconciler is the single owner of activation/team/DM Stream
// lifecycle. Every structural mutation (hire, add/remove reporting line,
// fire) announces *what changed* by calling Reconcile; the reconciler
// decides the stream consequences. Event-specific deltas drift; a
// declarative diff can't.
package reconcile

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
	"github.com/helixml/helix/api/pkg/org/domain/transport"
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

// Deps are the constructor-injected collaborators for New.
// ReportingLines is optional: a store that doesn't wire it yields a graph
// with no reporting edges (transcripts only).
type Deps struct {
	Workers        store.Workers
	ReportingLines store.ReportingLines
	Streams        store.Streams
	Subscriptions  store.Subscriptions
	// Now seams the clock for tests. Falls back to time.Now().UTC().
	Now func() time.Time
}

// New builds a Reconciler from its narrow repositories. A nil Workers
// repo (the "not wired" case) yields a Reconciler whose methods no-op, so
// runtimes/tests that don't wire topology degrade gracefully.
func New(deps Deps) *Reconciler {
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
// `s-transcript-<id>` and `s-team-<id>` for the affected Workers and
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
		return fmt.Errorf("reconcile: list workers: %w", err)
	}
	var lines []orgchart.ReportingLine
	if r.lines != nil {
		lines, err = r.lines.List(ctx, orgID)
		if err != nil {
			return fmt.Errorf("reconcile: list reporting lines: %w", err)
		}
	}

	required := channels.Required(workers, lines)

	// Bucket required members by stream so each converge is O(members).
	requiredMembers := map[streaming.StreamID][]orgchart.WorkerID{}
	for k := range required.Members {
		requiredMembers[k.StreamID] = append(requiredMembers[k.StreamID], k.WorkerID)
	}

	// Index the (current) graph to find each affected Worker's one-hop
	// neighbours — their team/transcripts can move too.
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
		relevant[activation.TranscriptID(a)] = struct{}{}
		relevant[channels.TeamStreamID(a)] = struct{}{}
		// A manager's team stream gains/loses this Worker as a member,
		// and the manager↔this-Worker DM channel is created/kept.
		for _, m := range managersByReport[a] {
			relevant[channels.TeamStreamID(m)] = struct{}{}
			relevant[channels.DMStreamID(a, m)] = struct{}{}
		}
		// A report's transcript gains/loses this Worker as an
		// observer, and the this-Worker↔report DM channel is
		// created/kept.
		for _, rep := range reportsByManager[a] {
			relevant[activation.TranscriptID(rep)] = struct{}{}
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
				return fmt.Errorf("reconcile: delete stream %q: %w", sid, err)
			}
			continue
		}
		if err := r.convergeStream(ctx, orgID, ch, requiredMembers[sid], now); err != nil {
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
		return fmt.Errorf("reconcile: ReconcileAll list workers: %w", err)
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

// convergeStream brings one managed Stream to exactly its required state:
// get-or-create the Stream, subscribe every required member, AND
// unsubscribe anyone the required set no longer includes. The removal is
// the load-bearing half — it's what fixes the reparent desync where an
// old manager stayed subscribed. (The additive half is
// ensureStreamWithMembers; convergeStream adds the diff-and-remove pass
// on top.)
func (r *Reconciler) convergeStream(ctx context.Context, orgID string, ch channels.Channel, members []orgchart.WorkerID, now time.Time) error {
	stream, err := streamForChannel(ch, now, orgID)
	if err != nil {
		return fmt.Errorf("reconcile: build stream %q: %w", ch.ID, err)
	}
	if err := r.ensureStreamWithMembers(ctx, stream, now, members...); err != nil {
		return fmt.Errorf("reconcile: ensure stream %q: %w", ch.ID, err)
	}

	requiredSet := make(map[orgchart.WorkerID]struct{}, len(members))
	for _, m := range members {
		requiredSet[m] = struct{}{}
	}
	actual, err := r.subs.ListForStream(ctx, orgID, ch.ID)
	if err != nil {
		return fmt.Errorf("reconcile: list subscribers of %q: %w", ch.ID, err)
	}
	for _, sub := range actual {
		if _, ok := requiredSet[orgchart.WorkerID(sub.WorkerID)]; ok {
			continue
		}
		if err := r.subs.Delete(ctx, orgID, orgchart.WorkerID(sub.WorkerID), ch.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("reconcile: unsubscribe %q from %q: %w", sub.WorkerID, ch.ID, err)
		}
	}
	return nil
}

// ensureStreamWithMembers is the additive get-or-create-and-subscribe
// primitive convergeStream builds on. It get-or-creates the Stream
// (immutable once it exists, so a present row is left untouched) and
// idempotently subscribes each member. It never *removes* a subscriber —
// that's convergeStream's job — so it is safe to call standalone to
// attach members without disturbing existing ones.
//
// Subscriptions are worker-anchored; members must be existing Workers.
//
// Concurrency-safe by design. The Stream id is deterministic
// (s-dm-<pair>, s-team-<id>, s-transcript-<id>), so two callers can
// race on the same id — two simultaneous DMs between the same pair, two
// reconciles touching one manager's team stream. A plain check-then-act
// would let the loser of the race hit the row's unique constraint and
// return a spurious error. Instead, on a Create failure we re-read the
// store: if the row is now present, another caller won the race and the
// outcome we wanted holds — proceed. Only a still-absent row is a
// genuine failure worth surfacing. This keeps Streams.Create /
// Subscriptions.Create strict for every other caller (createStream,
// hire_worker) while making *this* get-or-create boundary idempotent.
func (r *Reconciler) ensureStreamWithMembers(ctx context.Context, stream streaming.Stream, now time.Time, members ...orgchart.WorkerID) error {
	if _, err := r.streams.Get(ctx, stream.OrganizationID, stream.ID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("lookup stream %q: %w", stream.ID, err)
		}
		if createErr := r.streams.Create(ctx, stream); createErr != nil {
			// Lost the create race? A concurrent caller inserted the same
			// deterministic id between our Get and Create. Benign for a
			// get-or-create — re-check, and only surface the error if the
			// row still isn't there.
			if _, getErr := r.streams.Get(ctx, stream.OrganizationID, stream.ID); getErr != nil {
				return fmt.Errorf("create stream %q: %w", stream.ID, createErr)
			}
		}
	}
	for _, m := range members {
		if _, err := r.subs.Find(ctx, stream.OrganizationID, m, stream.ID); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("find subscription %q→%q: %w", m, stream.ID, err)
		}
		sub, err := streaming.NewSubscription(string(m), stream.ID, now, stream.OrganizationID)
		if err != nil {
			return fmt.Errorf("build subscription %q→%q: %w", m, stream.ID, err)
		}
		if createErr := r.subs.Create(ctx, sub); createErr != nil {
			// Same race on the (worker, stream) subscription key: a
			// concurrent caller subscribed this member first. A
			// now-present row means success.
			if _, findErr := r.subs.Find(ctx, stream.OrganizationID, m, stream.ID); findErr != nil {
				return fmt.Errorf("subscribe %q→%q: %w", m, stream.ID, createErr)
			}
		}
	}
	return nil
}

// streamForChannel builds the streaming.Stream the reconciler persists
// for a required Channel. Activation/team Streams are always local
// transport (the default).
func streamForChannel(ch channels.Channel, now time.Time, orgID string) (streaming.Stream, error) {
	return streaming.NewStream(ch.ID, ch.Name, ch.Description, string(ch.CreatedBy), now, transport.Transport{}, orgID)
}
