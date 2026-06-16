// Package channels is the pure domain rule that turns the org's
// reporting graph into the communication channels it requires: the
// per-Worker transcript (the append-only log of a Worker's activations), the
// per-manager team Stream (the downward broadcast channel a manager
// briefs their reports on), and the per-edge DM channel (the 1:1 a
// manager and report message on).
//
// Everything here is a PURE function over the reporting graph — no I/O,
// fully table-tested. It answers "what Streams and Subscriptions does
// this graph require?" and nothing else. The application-layer
// Reconciler (application/reconcile) loads the graph from the store,
// calls Required, diffs the required set against what's persisted, and
// applies create/subscribe/unsubscribe/delete idempotently.
package channels

import (
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TeamStreamID returns the deterministic Stream ID for a manager's team
// broadcast channel — the downward channel a manager publishes to so
// every direct report receives the post in one shot. Mirrors the
// `s-transcript-<id>` convention in domain/activation.
func TeamStreamID(managerID orgchart.WorkerID) streaming.StreamID {
	return streaming.StreamID("s-team-" + string(managerID))
}

// DMStreamID returns the deterministic Stream ID for a 1:1 channel
// between two Workers, ordered by string compare so A→B and B→A share
// one Stream. DM channels follow the reporting graph — the reconciler
// provisions one per reporting edge (manager ↔ report), so a Worker can
// assume the channel to a manager or direct report already exists. The
// managers / reports read paths hand this id back so a Worker can
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

// Channel is a communication channel the reporting structure requires.
// The reconciler builds a streaming.Stream from it when the row is
// missing; the fields are immutable once created, so an existing row is
// never rewritten. Its ID is the id of the Stream that realises it.
type Channel struct {
	ID          streaming.StreamID
	Name        string
	Description string
	// CreatedBy owns the channel. Activation channels are owned by the
	// Worker they transcribe; team channels by the manager.
	CreatedBy orgchart.WorkerID
}

// Membership identifies one required (Worker is a member of Channel) edge.
type Membership struct {
	WorkerID orgchart.WorkerID
	StreamID streaming.StreamID
}

// Set is the complete collection of channels and memberships a reporting
// graph requires. It is the output of the pure Required function.
type Set struct {
	Channels map[streaming.StreamID]Channel
	Members  map[Membership]struct{}
}

// Required computes the channels and memberships the given reporting
// graph requires. Pure: no I/O, deterministic, table-tested.
//
// Rules (reporting is many-to-many):
//
//   - Transcript `s-transcript-W` exists for Worker W iff W is
//     an AI Worker OR W has no managers (a top-level root worker — so its
//     own chat turns still have a home). Members are W's managers, so
//     every manager observes the transcript of each Worker reporting to
//     them. A manager-less worker gets a member-less stream — no worker
//     is ever self-subscribed (for an AI that would re-trigger it
//     indefinitely, and a transcript is observe-only regardless).
//
//   - Team Stream `s-team-M` exists for Worker M iff M has ≥1 direct
//     report. Members are M plus all of M's direct reports. A Worker
//     with two managers is therefore a member of two team Streams —
//     correct: either manager can brief it. No reports → no team Stream
//     (lazy).
//
//   - DM Stream `s-dm-<pair>` exists for every reporting edge (M, R),
//     with members exactly {M, R}. This is the 1:1 channel a Worker
//     escalates up / messages a report down on. DMs are deliberately
//     tied to the reporting graph: a Worker can only DM the people it
//     shares a reporting line with (its managers and its direct
//     reports), not arbitrary peers. Peer-to-peer or skip-level reach
//     is a deliberate, explicitly-created Stream, not an implicit DM.
func Required(workers []orgchart.Worker, lines []orgchart.ReportingLine) Set {
	set := Set{
		Channels: map[streaming.StreamID]Channel{},
		Members:  map[Membership]struct{}{},
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
		set.Channels[dmID] = Channel{
			ID:          dmID,
			Name:        "dm: " + pair[0] + " ↔ " + pair[1],
			Description: dmChannelDescription(pair[0], pair[1]),
			CreatedBy:   orgchart.WorkerID(pair[0]),
		}
		set.Members[Membership{WorkerID: l.ManagerID, StreamID: dmID}] = struct{}{}
		set.Members[Membership{WorkerID: l.ReportID, StreamID: dmID}] = struct{}{}
	}

	for _, w := range workers {
		wid := w.ID()
		managers := managersByReport[wid]

		// Transcript: AI Workers always, plus any manager-less root
		// worker (so a top-level worker's turns still have a home).
		// Observers are the worker's managers; a manager-less worker has
		// none. No worker is ever self-subscribed — for an AI that would
		// re-trigger it forever, and a transcript is observe-only anyway.
		if w.Kind() == orgchart.WorkerKindAI || len(managers) == 0 {
			sid := activation.TranscriptID(wid)
			set.Channels[sid] = Channel{
				ID:          sid,
				Name:        "Transcript: " + string(wid),
				Description: transcriptChannelDescription(wid),
				CreatedBy:   wid,
			}
			for _, m := range managers {
				set.Members[Membership{WorkerID: m, StreamID: sid}] = struct{}{}
			}
		}

		// Team Stream: only when the Worker has at least one report.
		reports := reportsByManager[wid]
		if len(reports) > 0 {
			sid := TeamStreamID(wid)
			set.Channels[sid] = Channel{
				ID:          sid,
				Name:        "Team: " + string(wid),
				Description: teamChannelDescription(wid),
				CreatedBy:   wid,
			}
			set.Members[Membership{WorkerID: wid, StreamID: sid}] = struct{}{}
			for _, r := range reports {
				set.Members[Membership{WorkerID: r, StreamID: sid}] = struct{}{}
			}
		}
	}
	return set
}

// The channel descriptions below describe what each channel IS, in the
// org's own terms (who observes it, who publishes, who may use it). They
// deliberately name no tools: how a Worker reads or writes a channel is
// the interface layer's concern (the MCP tool descriptions own that), and
// the domain must not pin itself to that surface.

func transcriptChannelDescription(workerID orgchart.WorkerID) string {
	return "Per-message activation transcript for " + string(workerID) +
		" — assistant text, tool calls, tool results, and chat turns, in " +
		"order. The Worker's managers observe it; it is how the org audits " +
		"and tails what the Worker did."
}

func teamChannelDescription(managerID orgchart.WorkerID) string {
	return "Team broadcast channel for " + string(managerID) +
		" and their direct reports. The manager publishes once here to " +
		"brief the whole team; every direct report receives it."
}

func dmChannelDescription(a, b string) string {
	return "Direct 1:1 channel between " + a + " and " + b +
		", provisioned because they share a reporting line — used to " +
		"escalate up or direct a report down. Reporting pairs only: a " +
		"Worker may DM its managers and direct reports, never an arbitrary " +
		"peer. Peer or skip-level reach is a deliberately, explicitly " +
		"created Stream, not an implicit DM."
}
