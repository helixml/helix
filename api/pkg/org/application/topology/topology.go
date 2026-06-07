// Package topology owns the one place where the org's reporting graph
// is turned into communication channels: the per-Worker activation
// Stream (who observes a Worker's transcript) and the per-manager team
// Stream (the downward broadcast channel a manager briefs their reports
// on).
//
// The split mirrors the codebase's existing layering:
//
//   - DesiredTopology is a PURE function over the reporting graph — no
//     I/O, fully table-tested. It answers "what Streams and
//     Subscriptions does this graph imply?" and nothing else.
//   - Reconciler (reconciler.go) loads the graph from the store, calls
//     DesiredTopology, diffs the desired set against what's persisted,
//     and applies create/subscribe/unsubscribe/delete idempotently.
//
// This is the single owner of activation/team Stream lifecycle. Every
// structural mutation (hire, add/remove reporting line, fire) announces
// *what changed* by calling Reconcile; the reconciler decides the
// stream consequences. Event-specific deltas drift; a declarative diff
// can't.
//
// DM Streams are deliberately NOT here: they are pairwise and on-demand,
// not a function of the graph, so there is nothing to reconcile. But the
// *mechanism* — "ensure Stream X exists with member set Y" — is the
// shared EnsureStreamWithMembers helper that both the dm tool and this
// reconciler call. One implementation of the mechanism; triggers where
// they belong.
package topology

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TeamStreamID returns the deterministic Stream ID for a manager's team
// broadcast channel — the downward channel a manager publishes to so
// every direct report receives the post in one shot. Mirrors the
// `s-activations-<id>` convention in domain/activation.
func TeamStreamID(managerID orgchart.WorkerID) streaming.StreamID {
	return streaming.StreamID("s-team-" + string(managerID))
}

// DesiredStream is a Stream the topology wants to exist. The reconciler
// builds a streaming.Stream from it when the row is missing; the fields
// are immutable once created, so an existing row is never rewritten.
type DesiredStream struct {
	ID          streaming.StreamID
	Name        string
	Description string
	// CreatedBy owns the Stream. Activation Streams are owned by the
	// Worker they transcribe; team Streams by the manager.
	CreatedBy orgchart.WorkerID
}

// SubKey identifies one desired (Worker subscribes to Stream) edge.
type SubKey struct {
	WorkerID orgchart.WorkerID
	StreamID streaming.StreamID
}

// Spec is the complete set of Streams and Subscriptions a reporting
// graph implies. It is the output of the pure DesiredTopology function.
type Spec struct {
	Streams map[streaming.StreamID]DesiredStream
	Subs    map[SubKey]struct{}
}

// DesiredTopology computes the Streams and Subscriptions the given
// reporting graph implies. Pure: no I/O, deterministic, table-tested.
//
// Rules (reporting is many-to-many):
//
//   - Activation Stream `s-activations-W` exists for Worker W iff W is
//     an AI Worker OR W has no managers (the root — typically the
//     human owner). Subscribers are W's managers, so every manager
//     observes the transcript of each Worker reporting to them. A
//     manager-less *human* (the owner) observes its own transcript so
//     its chat turns still surface on the Streams page — no
//     special-casing by id, it falls out of "no managers". A
//     manager-less AI gets a subscriber-less stream (never self-
//     subscribed: that would re-trigger it indefinitely).
//
//   - Team Stream `s-team-M` exists for Worker M iff M has ≥1 direct
//     report. Subscribers are M plus all of M's direct reports. A
//     Worker with two managers is therefore a member of two team
//     Streams — correct: either manager can brief it. No reports → no
//     team Stream (lazy).
func DesiredTopology(workers []orgchart.Worker, lines []orgchart.ReportingLine) Spec {
	spec := Spec{
		Streams: map[streaming.StreamID]DesiredStream{},
		Subs:    map[SubKey]struct{}{},
	}

	exists := make(map[orgchart.WorkerID]struct{}, len(workers))
	for _, w := range workers {
		exists[w.ID()] = struct{}{}
	}

	managersByReport := map[orgchart.WorkerID][]orgchart.WorkerID{}
	reportsByManager := map[orgchart.WorkerID][]orgchart.WorkerID{}
	for _, l := range lines {
		// Defend against lines that reference a Worker that no longer
		// exists (a dangling row the cascade hasn't reached yet).
		if _, ok := exists[l.ReportID]; !ok {
			continue
		}
		if _, ok := exists[l.ManagerID]; !ok {
			continue
		}
		managersByReport[l.ReportID] = append(managersByReport[l.ReportID], l.ManagerID)
		reportsByManager[l.ManagerID] = append(reportsByManager[l.ManagerID], l.ReportID)
	}

	for _, w := range workers {
		wid := w.ID()
		managers := managersByReport[wid]

		// Activation Stream: AI Workers always, plus the manager-less
		// root (the human owner).
		if w.Kind() == orgchart.WorkerKindAI || len(managers) == 0 {
			sid := activation.StreamID(wid)
			spec.Streams[sid] = DesiredStream{
				ID:          sid,
				Name:        "Activations: " + string(wid),
				Description: activationStreamDescription(wid),
				CreatedBy:   wid,
			}
			observers := managers
			if len(observers) == 0 && w.Kind() == orgchart.WorkerKindHuman {
				// The owner observes its own transcript. AI Workers are
				// never self-subscribed (re-trigger loop), so a
				// manager-less AI simply has no observer.
				observers = []orgchart.WorkerID{wid}
			}
			for _, m := range observers {
				spec.Subs[SubKey{WorkerID: m, StreamID: sid}] = struct{}{}
			}
		}

		// Team Stream: only when the Worker has at least one report.
		reports := reportsByManager[wid]
		if len(reports) > 0 {
			sid := TeamStreamID(wid)
			spec.Streams[sid] = DesiredStream{
				ID:          sid,
				Name:        "Team: " + string(wid),
				Description: teamStreamDescription(wid),
				CreatedBy:   wid,
			}
			spec.Subs[SubKey{WorkerID: wid, StreamID: sid}] = struct{}{}
			for _, r := range reports {
				spec.Subs[SubKey{WorkerID: r, StreamID: sid}] = struct{}{}
			}
		}
	}
	return spec
}

func activationStreamDescription(workerID orgchart.WorkerID) string {
	return "Per-message activation transcript for " + string(workerID) +
		" — assistant text, tool calls, tool results, chat turns. " +
		"Read with read_events or worker_log to audit / tail."
}

func teamStreamDescription(managerID orgchart.WorkerID) string {
	return "Team broadcast channel for " + string(managerID) +
		" and their direct reports. The manager publishes here to brief " +
		"the whole team in one post; every report receives it. Discover " +
		"it via the `reports` tool."
}

// EnsureStreamWithMembers is the shared create-stream-and-subscribe
// primitive. It get-or-creates the Stream (immutable once it exists, so
// a present row is left untouched) and idempotently subscribes each
// member. Both the dm tool and the Reconciler call it — one
// implementation of the mechanism, even though the triggers differ.
//
// Subscriptions are worker-anchored; members must be existing Workers.
//
// Concurrency-safe by design. The Stream id is deterministic
// (s-dm-<pair>, s-team-<id>, s-activations-<id>), so two callers can
// race on the same id — two simultaneous DMs between the same pair, two
// reconciles touching one manager's team stream. A plain check-then-act
// would let the loser of the race hit the row's unique constraint and
// return a spurious error. Instead, on a Create failure we re-read the
// store: if the row is now present, another caller won the race and the
// outcome we wanted holds — proceed. Only a still-absent row is a
// genuine failure worth surfacing. This keeps Streams.Create /
// Subscriptions.Create strict for every other caller (createStream,
// hire_worker) while making *this* get-or-create boundary idempotent.
func EnsureStreamWithMembers(ctx context.Context, s *store.Store, stream streaming.Stream, now time.Time, members ...orgchart.WorkerID) error {
	if _, err := s.Streams.Get(ctx, stream.OrganizationID, stream.ID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("lookup stream %q: %w", stream.ID, err)
		}
		if createErr := s.Streams.Create(ctx, stream); createErr != nil {
			// Lost the create race? A concurrent caller inserted the same
			// deterministic id between our Get and Create. Benign for a
			// get-or-create — re-check, and only surface the error if the
			// row still isn't there.
			if _, getErr := s.Streams.Get(ctx, stream.OrganizationID, stream.ID); getErr != nil {
				return fmt.Errorf("create stream %q: %w", stream.ID, createErr)
			}
		}
	}
	for _, m := range members {
		if _, err := s.Subscriptions.Find(ctx, stream.OrganizationID, m, stream.ID); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("find subscription %q→%q: %w", m, stream.ID, err)
		}
		sub, err := streaming.NewSubscription(string(m), stream.ID, now, stream.OrganizationID)
		if err != nil {
			return fmt.Errorf("build subscription %q→%q: %w", m, stream.ID, err)
		}
		if createErr := s.Subscriptions.Create(ctx, sub); createErr != nil {
			// Same race on the (worker, stream) subscription key: a
			// concurrent caller subscribed this member first. A
			// now-present row means success.
			if _, findErr := s.Subscriptions.Find(ctx, stream.OrganizationID, m, stream.ID); findErr != nil {
				return fmt.Errorf("subscribe %q→%q: %w", m, stream.ID, createErr)
			}
		}
	}
	return nil
}

// newDesiredStream builds the streaming.Stream the reconciler persists
// for a DesiredStream. Activation/team Streams are always local
// transport (the default).
func newDesiredStream(ds DesiredStream, now time.Time, orgID string) (streaming.Stream, error) {
	return streaming.NewStream(ds.ID, ds.Name, ds.Description, string(ds.CreatedBy), now, transport.Transport{}, orgID)
}
