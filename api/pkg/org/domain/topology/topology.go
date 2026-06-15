// Package topology is the pure domain rule that turns the org's
// reporting graph into communication channels: the per-Worker
// activation Stream (who observes a Worker's transcript), the
// per-manager team Stream (the downward broadcast channel a manager
// briefs their reports on), and the per-edge DM channel (the 1:1 a
// manager and report message on).
//
// Everything here is a PURE function over the reporting graph — no I/O,
// fully table-tested. It answers "what Streams and Subscriptions does
// this graph imply?" and nothing else. The application-layer
// Reconciler (application/topology) loads the graph from the store,
// calls DesiredTopology, diffs the desired set against what's
// persisted, and applies create/subscribe/unsubscribe/delete
// idempotently.
package topology

import (
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TeamStreamID returns the deterministic Stream ID for a manager's team
// broadcast channel — the downward channel a manager publishes to so
// every direct report receives the post in one shot. Mirrors the
// `s-activations-<id>` convention in domain/activation.
func TeamStreamID(managerID orgchart.WorkerID) streaming.StreamID {
	return streaming.StreamID("s-team-" + string(managerID))
}

// DMStreamID returns the deterministic Stream ID for a 1:1 channel
// between two Workers, ordered by string compare so A→B and B→A share
// one Stream. DM channels are a topology concern — the reconciler
// provisions one per reporting edge (manager ↔ report), so the `dm`
// tool can assume the channel exists rather than creating it. The
// managers / reports read tools hand this id back so a Worker can
// escalate up / message a report 1:1 along an existing channel.
func DMStreamID(a, b orgchart.WorkerID) streaming.StreamID {
	pair := sortedPair(a, b)
	return streaming.StreamID("s-dm-" + pair[0] + "-" + pair[1])
}

func sortedPair(a, b orgchart.WorkerID) [2]string {
	p := [2]string{string(a), string(b)}
	if p[0] > p[1] {
		p[0], p[1] = p[1], p[0]
	}
	return p
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
//
//   - DM Stream `s-dm-<pair>` exists for every reporting edge (M, R),
//     with members exactly {M, R}. This is the 1:1 channel a Worker
//     escalates up / messages a report down on. DMs are deliberately
//     tied to the reporting graph: a Worker can only DM the people it
//     shares a reporting line with (its managers and its direct
//     reports), not arbitrary peers — the `dm` tool assumes the channel
//     exists and refuses when it doesn't. Peer-to-peer or skip-level
//     reach is a deliberate, explicitly-created stream, not an implicit
//     DM.
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

		// DM channel for this reporting edge: members exactly {M, R}.
		dmID := DMStreamID(l.ManagerID, l.ReportID)
		pair := sortedPair(l.ManagerID, l.ReportID)
		spec.Streams[dmID] = DesiredStream{
			ID:          dmID,
			Name:        "dm: " + pair[0] + " ↔ " + pair[1],
			Description: dmStreamDescription(pair[0], pair[1]),
			CreatedBy:   orgchart.WorkerID(pair[0]),
		}
		spec.Subs[SubKey{WorkerID: l.ManagerID, StreamID: dmID}] = struct{}{}
		spec.Subs[SubKey{WorkerID: l.ReportID, StreamID: dmID}] = struct{}{}
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

func dmStreamDescription(a, b string) string {
	return "Direct 1:1 channel between " + a + " and " + b +
		" — provisioned because they share a reporting line. Either " +
		"escalates up / messages down here via the `dm` tool; both read " +
		"it with read_events. Reporting pairs only — there is no implicit " +
		"DM channel between arbitrary Workers."
}
