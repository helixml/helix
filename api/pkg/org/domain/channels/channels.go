// Package channels is the pure domain rule that turns the org's
// reporting graph into the communication channels it requires: the
// per-Bot transcript (the append-only log of a Bot's activations), the
// per-manager team Topic (the downward broadcast channel a manager
// briefs their reports on), and the per-edge DM channel (the 1:1 a
// manager and report message on).
//
// Everything here is a PURE function over the reporting graph — no I/O,
// fully table-tested. It answers "what Topics and Subscriptions does
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

// TeamTopicID returns the deterministic Topic ID for a manager's team
// broadcast channel — the downward channel a manager publishes to so
// every direct report receives the post in one shot. Mirrors the
// `s-transcript-<id>` convention in domain/activation.
func TeamTopicID(managerID orgchart.BotID) streaming.TopicID {
	return streaming.TopicID("s-team-" + string(managerID))
}

// DMTopicID returns the deterministic Topic ID for a 1:1 channel
// between two Bots, ordered by string compare so A→B and B→A share one
// Topic. DM channels follow the reporting graph — the reconciler
// provisions one per reporting edge (manager ↔ report), so a Bot can
// assume the channel to a manager or direct report already exists. The
// managers / reports read paths hand this id back so a Bot can escalate
// up / message a report 1:1 along an existing channel.
func DMTopicID(a, b orgchart.BotID) streaming.TopicID {
	pair := sortedPair(a, b)
	return streaming.TopicID("s-dm-" + pair[0] + "-" + pair[1])
}

func sortedPair(a, b orgchart.BotID) [2]string {
	p := [2]string{string(a), string(b)}
	if p[0] > p[1] {
		p[0], p[1] = p[1], p[0]
	}
	return p
}

// Channel is a communication channel the reporting structure requires.
// The reconciler builds a streaming.Topic from it when the row is
// missing; the fields are immutable once created, so an existing row is
// never rewritten. Its ID is the id of the Topic that realises it.
type Channel struct {
	ID          streaming.TopicID
	Name        string
	Description string
	// CreatedBy owns the channel. Activation channels are owned by the
	// Bot they transcribe; team channels by the manager.
	CreatedBy orgchart.BotID
}

// Membership identifies one required (Bot is a member of Channel) edge.
type Membership struct {
	BotID   orgchart.BotID
	TopicID streaming.TopicID
}

// Set is the complete collection of channels and memberships a reporting
// graph requires. It is the output of the pure Required function.
type Set struct {
	Channels map[streaming.TopicID]Channel
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
//   - Team Topic `s-team-M` exists for Bot M iff M has ≥1 direct
//     report. Members are M plus all of M's direct reports. A Bot with
//     two managers is therefore a member of two team Topics — correct:
//     either manager can brief it. No reports → no team Topic (lazy).
//
//   - DM Topic `s-dm-<pair>` exists for every reporting edge (M, R),
//     with members exactly {M, R}. This is the 1:1 channel a Bot
//     escalates up / messages a report down on. DMs are deliberately
//     tied to the reporting graph: a Bot can only DM the bots it shares a
//     reporting line with (its managers and its direct reports), not
//     arbitrary peers. Peer-to-peer or skip-level reach is a deliberate,
//     explicitly-created Topic, not an implicit DM.
func Required(bots []orgchart.Bot, lines []orgchart.ReportingLine) Set {
	set := Set{
		Channels: map[streaming.TopicID]Channel{},
		Members:  map[Membership]struct{}{},
	}

	exists := make(map[orgchart.BotID]struct{}, len(bots))
	for _, b := range bots {
		exists[b.ID] = struct{}{}
	}

	managersByReport := map[orgchart.BotID][]orgchart.BotID{}
	reportsByManager := map[orgchart.BotID][]orgchart.BotID{}
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
		dmID := DMTopicID(l.ManagerID, l.ReportID)
		pair := sortedPair(l.ManagerID, l.ReportID)
		set.Channels[dmID] = Channel{
			ID:          dmID,
			Name:        "dm: " + pair[0] + " ↔ " + pair[1],
			Description: dmChannelDescription(pair[0], pair[1]),
			CreatedBy:   orgchart.BotID(pair[0]),
		}
		set.Members[Membership{BotID: l.ManagerID, TopicID: dmID}] = struct{}{}
		set.Members[Membership{BotID: l.ReportID, TopicID: dmID}] = struct{}{}
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
			set.Members[Membership{BotID: m, TopicID: sid}] = struct{}{}
		}

		// Team Topic: only when the Bot has at least one report.
		reports := reportsByManager[wid]
		if len(reports) > 0 {
			sid := TeamTopicID(wid)
			set.Channels[sid] = Channel{
				ID:          sid,
				Name:        "Team: " + string(wid),
				Description: teamChannelDescription(wid),
				CreatedBy:   wid,
			}
			set.Members[Membership{BotID: wid, TopicID: sid}] = struct{}{}
			for _, r := range reports {
				set.Members[Membership{BotID: r, TopicID: sid}] = struct{}{}
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

func transcriptChannelDescription(botID orgchart.BotID) string {
	return "Per-message activation transcript for " + string(botID) +
		" — assistant text, tool calls, tool results, and chat turns, in " +
		"order. The Bot's managers observe it; it is how the org audits " +
		"and tails what the Bot did."
}

func teamChannelDescription(managerID orgchart.BotID) string {
	return "Team broadcast channel for " + string(managerID) +
		" and their direct reports. The manager publishes once here to " +
		"brief the whole team; every direct report receives it."
}

func dmChannelDescription(a, b string) string {
	return "Direct 1:1 channel between " + a + " and " + b +
		", provisioned because they share a reporting line — used to " +
		"escalate up or direct a report down. Reporting pairs only: a " +
		"Bot may DM its managers and direct reports, never an arbitrary " +
		"peer. Peer or skip-level reach is a deliberately, explicitly " +
		"created Topic, not an implicit DM."
}
