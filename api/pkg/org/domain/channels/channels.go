// Package channels is the pure domain rule that turns the org's
// reporting graph into the communication channels it requires: the
// per-Bot transcript (the append-only log of a Bot's activations), the
// per-manager team chat (the downward broadcast channel a manager
// briefs their reports on), and the per-edge DM channel (the 1:1 a
// manager and report message on).
//
// Each channel is realised as a system-managed local Trigger, and each
// membership as a Worker attachment to it. They are ordinary Triggers —
// same event stream, same attachment dispatch, same history — created
// and torn down only by this rule and its reconciler.
//
// Everything here is a PURE function over the reporting graph — no I/O,
// fully table-tested. It answers "what Triggers and attachments does
// this graph require?" and nothing else. The application-layer
// Reconciler (application/reconcile) loads the graph from the store,
// calls Required, diffs the required set against what's persisted, and
// applies create/attach/detach/delete idempotently.
package channels

import (
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// TeamTriggerID returns the deterministic Trigger ID for a manager's
// team chat — the downward channel a manager sends to so every direct
// report receives the message in one shot. Mirrors the
// `s-transcript-<id>` convention in domain/activation.
func TeamTriggerID(managerID orgchart.NodeID) string {
	return "s-team-" + string(managerID)
}

// DMTriggerID returns the deterministic Trigger ID for a 1:1 channel
// between two Nodes, ordered by string compare so A→B and B→A share one
// Trigger. DM channels follow the reporting graph — the reconciler
// provisions one per reporting edge (manager ↔ report), so a Bot can
// assume the channel to a manager or direct report already exists. The
// managers / reports read paths hand this id back so a Bot can escalate
// up / message a report 1:1 along an existing channel.
func DMTriggerID(a, b orgchart.NodeID) string {
	pair := sortedPair(a, b)
	return "s-dm-" + pair[0] + "-" + pair[1]
}

func sortedPair(a, b orgchart.NodeID) [2]string {
	p := [2]string{string(a), string(b)}
	if p[0] > p[1] {
		p[0], p[1] = p[1], p[0]
	}
	return p
}

// Channel is a communication channel the reporting structure requires.
// The reconciler builds a trigger.Trigger from it when the row is
// missing; the fields are immutable once created, so an existing row is
// never rewritten. Its ID is the id of the Trigger that realises it,
// which is also the id of the event stream carrying its history.
type Channel struct {
	ID          string
	Name        string
	Description string
	// CreatedBy owns the channel. Activation channels are owned by the
	// Bot they transcribe; team channels by the manager.
	CreatedBy orgchart.NodeID
}

// Membership identifies one required (Bot is attached to Channel) edge.
type Membership struct {
	NodeID    orgchart.NodeID
	TriggerID string
}

// Set is the complete collection of channels and memberships a reporting
// graph requires. It is the output of the pure Required function.
type Set struct {
	Channels map[string]Channel
	Members  map[Membership]struct{}
}

// Required computes the channels and memberships the given reporting
// graph requires. Pure: no I/O, deterministic, table-tested.
//
// Rules (reporting is many-to-many):
//
//   - Transcript `s-transcript-B` exists for every Bot B (it is the
//     append-only home for B's activations). Members are B's managers, so
//     every manager observes the transcript of each Bot reporting to
//     them. A manager-less bot gets a member-less topic — no bot is ever
//     self-subscribed (that would re-trigger it indefinitely, and a
//     transcript is observe-only regardless).
//
//   - Team chat `s-team-M` exists for Bot M iff M has ≥1 direct
//     report. Members are M plus all of M's direct reports. A Bot with
//     two managers is therefore a member of two team chats — correct:
//     either manager can brief it. No reports → no team chat (lazy).
//
//   - DM channel `s-dm-<pair>` exists for every reporting edge (M, R),
//     with members exactly {M, R}. This is the 1:1 channel a Bot
//     escalates up / messages a report down on. DMs are deliberately
//     tied to the reporting graph: a Bot can only DM the bots it shares a
//     reporting line with (its managers and its direct reports), not
//     arbitrary peers. Peer-to-peer or skip-level reach is a deliberate,
//     explicitly-created channel, not an implicit DM.
func Required(bots []orgchart.Node, lines []orgchart.ReportingLine) Set {
	set := Set{
		Channels: map[string]Channel{},
		Members:  map[Membership]struct{}{},
	}

	exists := make(map[orgchart.NodeID]struct{}, len(bots))
	for _, b := range bots {
		exists[b.ID] = struct{}{}
	}

	managersByReport := map[orgchart.NodeID][]orgchart.NodeID{}
	reportsByManager := map[orgchart.NodeID][]orgchart.NodeID{}
	for _, l := range lines {
		// Defend against lines that reference a Bot that no longer
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
		dmID := DMTriggerID(l.ManagerID, l.ReportID)
		pair := sortedPair(l.ManagerID, l.ReportID)
		set.Channels[dmID] = Channel{
			ID:          dmID,
			Name:        "dm: " + pair[0] + " ↔ " + pair[1],
			Description: dmChannelDescription(pair[0], pair[1]),
			CreatedBy:   orgchart.NodeID(pair[0]),
		}
		set.Members[Membership{NodeID: l.ManagerID, TriggerID: dmID}] = struct{}{}
		set.Members[Membership{NodeID: l.ReportID, TriggerID: dmID}] = struct{}{}
	}

	for _, b := range bots {
		wid := b.ID
		managers := managersByReport[wid]

		// Transcript: every Bot gets one (the append-only home for its
		// activations). Observers are the bot's managers; a manager-less
		// bot has none. No bot is ever self-subscribed — that would
		// re-trigger it forever, and a transcript is observe-only anyway.
		sid := activation.TranscriptID(wid)
		set.Channels[sid] = Channel{
			ID:          sid,
			Name:        "Transcript: " + string(wid),
			Description: transcriptChannelDescription(wid),
			CreatedBy:   wid,
		}
		for _, m := range managers {
			set.Members[Membership{NodeID: m, TriggerID: sid}] = struct{}{}
		}

		// Team chat: only when the Bot has at least one report.
		reports := reportsByManager[wid]
		if len(reports) > 0 {
			sid := TeamTriggerID(wid)
			set.Channels[sid] = Channel{
				ID:          sid,
				Name:        "Team: " + string(wid),
				Description: teamChannelDescription(wid),
				CreatedBy:   wid,
			}
			set.Members[Membership{NodeID: wid, TriggerID: sid}] = struct{}{}
			for _, r := range reports {
				set.Members[Membership{NodeID: r, TriggerID: sid}] = struct{}{}
			}
		}
	}
	return set
}

// The channel descriptions below describe what each channel IS, in the
// org's own terms (who observes it, who publishes, who may use it). They
// deliberately name no tools: how a Bot reads or writes a channel is the
// interface layer's concern (the MCP tool descriptions own that), and
// the domain must not pin itself to that surface.

func transcriptChannelDescription(botID orgchart.NodeID) string {
	return "Per-message activation transcript for " + string(botID) +
		" — assistant text, tool calls, tool results, and chat turns, in " +
		"order. The Bot's managers observe it; it is how the org audits " +
		"and tails what the Bot did."
}

func teamChannelDescription(managerID orgchart.NodeID) string {
	return "Team chat for " + string(managerID) +
		" and their direct reports. The manager sends once here to " +
		"brief the whole team; every direct report receives it."
}

func dmChannelDescription(a, b string) string {
	return "Direct 1:1 channel between " + a + " and " + b +
		", provisioned because they share a reporting line — used to " +
		"escalate up or direct a report down. Reporting pairs only: a " +
		"Bot may DM its managers and direct reports, never an arbitrary " +
		"peer. Peer or skip-level reach is a deliberately, explicitly " +
		"created channel, not an implicit DM."
}
