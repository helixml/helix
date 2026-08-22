// Package reconcile is the application-layer reconciler that converges
// the persisted Triggers/attachments onto the channels the reporting
// graph requires. The pure derivation — "what channels does this graph
// require?" — lives in domain/channels; this package loads the graph
// from the store, calls channels.Required, diffs the required set
// against what's persisted, and applies create/attach/detach/delete
// idempotently.
//
// The Reconciler is the single owner of transcript/team/DM channel
// lifecycle. Every structural mutation (hire, add/remove reporting line,
// fire) announces *what changed* by calling Reconcile; the reconciler
// decides the consequences. Event-specific deltas drift; a declarative
// diff can't.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

// Reconciler converges the persisted Triggers/attachments onto the
// channels the reporting graph requires. It depends only on the narrow
// repositories it actually touches — Nodes, ReportingLines, Triggers,
// WorkerAttachments — never the whole *store.Store (CLAUDE.md helix-org
// philosophy: small interfaces). That is what keeps it table-testable and
// lets every structural mutation depend on it without pulling in the
// heavyweight lifecycle service.
type Reconciler struct {
	bots        store.Nodes
	lines       store.ReportingLines
	triggers    store.Triggers
	attachments store.WorkerAttachments
	now         func() time.Time
}

// Deps are the constructor-injected collaborators for New.
// ReportingLines is optional: a store that doesn't wire it yields a graph
// with no reporting edges (transcripts only).
type Deps struct {
	Nodes          store.Nodes
	ReportingLines store.ReportingLines
	Triggers       store.Triggers
	Attachments    store.WorkerAttachments
	// Now seams the clock for tests. Falls back to time.Now().UTC().
	Now func() time.Time
}

// New builds a Reconciler from its narrow repositories. A nil Nodes
// repo (the "not wired" case) yields a Reconciler whose methods no-op, so
// runtimes/tests that don't wire topology degrade gracefully.
func New(deps Deps) *Reconciler {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Reconciler{
		bots:        deps.Nodes,
		lines:       deps.ReportingLines,
		triggers:    deps.Triggers,
		attachments: deps.Attachments,
		now:         now,
	}
}

// Reconcile settles the channels touched by a change to the given
// affected Worker(s). It loads the whole graph, computes the required
// channels, then — only for the channels owned by the affected Workers
// and their one-hop managers/reports — diffs required vs actual and
// applies create-trigger / attach / detach / delete-trigger idempotently.
//
// Scoping to the affected Workers' channels (rather than every Trigger in
// the org) is what keeps Reconcile from touching operator-created
// Triggers: the only ids it ever considers are `s-transcript-<id>`,
// `s-team-<id>` and `s-dm-<pair>` for the affected Workers and their
// immediate neighbours.
//
// Callers announce what changed:
//   - hire W              → Reconcile(org, W)
//   - add/remove W→M line → Reconcile(org, W, M)
//   - fire W (managers M…)→ Reconcile(org, W, M…)  (capture M… first)
//
// A nil or unwired Reconciler is a no-op, so runtimes/tests that don't
// wire topology degrade gracefully.
func (r *Reconciler) Reconcile(ctx context.Context, orgID string, affected ...orgchart.NodeID) error {
	if r == nil || r.bots == nil {
		return nil
	}
	if len(affected) == 0 {
		return nil
	}

	bots, err := r.bots.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("reconcile: list bots: %w", err)
	}
	var lines []orgchart.ReportingLine
	if r.lines != nil {
		lines, err = r.lines.List(ctx, orgID)
		if err != nil {
			return fmt.Errorf("reconcile: list reporting lines: %w", err)
		}
	}

	required := channels.Required(bots, lines)

	// Bucket required members by channel so each converge is O(members).
	requiredMembers := map[string][]orgchart.NodeID{}
	for k := range required.Members {
		requiredMembers[k.TriggerID] = append(requiredMembers[k.TriggerID], k.NodeID)
	}

	// Index the (current) graph to find each affected Worker's one-hop
	// neighbours — their team/transcripts can move too.
	managersByReport := map[orgchart.NodeID][]orgchart.NodeID{}
	reportsByManager := map[orgchart.NodeID][]orgchart.NodeID{}
	for _, l := range lines {
		managersByReport[l.ReportID] = append(managersByReport[l.ReportID], l.ManagerID)
		reportsByManager[l.ManagerID] = append(reportsByManager[l.ManagerID], l.ReportID)
	}

	// Collect the channel ids in scope. Only ever transcript / team / DM
	// ids derived from the affected Workers and their one-hop neighbours
	// — never an operator-created Trigger.
	relevant := map[string]struct{}{}
	for _, a := range affected {
		relevant[activation.TranscriptID(a)] = struct{}{}
		relevant[channels.TeamTriggerID(a)] = struct{}{}
		// A manager's team chat gains/loses this Worker as a member,
		// and the manager↔this-Worker DM channel is created/kept.
		for _, m := range managersByReport[a] {
			relevant[channels.TeamTriggerID(m)] = struct{}{}
			relevant[channels.DMTriggerID(a, m)] = struct{}{}
		}
		// A report's transcript gains/loses this Worker as an
		// observer, and the this-Worker↔report DM channel is
		// created/kept.
		for _, rep := range reportsByManager[a] {
			relevant[activation.TranscriptID(rep)] = struct{}{}
			relevant[channels.DMTriggerID(a, rep)] = struct{}{}
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
			relevant[channels.DMTriggerID(affected[i], affected[j])] = struct{}{}
		}
	}

	ids := make([]string, 0, len(relevant))
	for id := range relevant {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	now := r.clock()
	for _, id := range ids {
		ch, want := required.Channels[id]
		if !want {
			// The channel should not exist. Delete the Trigger;
			// attachments cascade with the row. Absent already → fine.
			if err := r.triggers.Delete(ctx, orgID, id); err != nil && !errors.Is(err, store.ErrNotFound) {
				return fmt.Errorf("reconcile: delete channel %q: %w", id, err)
			}
			continue
		}
		if err := r.convergeChannel(ctx, orgID, ch, requiredMembers[id], now); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileAll converges the full topology for every Worker in the org.
// Call at server startup so Workers hired before the reconciler was
// wired (or before a new channel rule was added) get their transcript,
// team, and DM channels created or corrected idempotently. Internally
// loads every Worker ID and delegates to Reconcile so the scoping and
// create/delete/attach logic stays in one place.
func (r *Reconciler) ReconcileAll(ctx context.Context, orgID string) error {
	if r == nil || r.bots == nil {
		return nil
	}
	bots, err := r.bots.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("reconcile: ReconcileAll list bots: %w", err)
	}
	if len(bots) == 0 {
		return nil
	}
	ids := make([]orgchart.NodeID, len(bots))
	for i, b := range bots {
		ids[i] = b.ID
	}
	return r.Reconcile(ctx, orgID, ids...)
}

func (r *Reconciler) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now().UTC()
}

// convergeChannel brings one managed channel to exactly its required
// state: get-or-create the Trigger, attach every required member, AND
// detach anyone the required set no longer includes. The removal is the
// load-bearing half — it's what fixes the reparent desync where an old
// manager stayed attached.
func (r *Reconciler) convergeChannel(ctx context.Context, orgID string, ch channels.Channel, members []orgchart.NodeID, now time.Time) error {
	t, err := triggerForChannel(ch, now, orgID)
	if err != nil {
		return fmt.Errorf("reconcile: build channel %q: %w", ch.ID, err)
	}
	if err := r.ensureTrigger(ctx, t); err != nil {
		return err
	}

	actual, err := r.attachments.Find(ctx, store.WithOrg(orgID), store.WithTriggerID(ch.ID))
	if err != nil {
		return fmt.Errorf("reconcile: list members of %q: %w", ch.ID, err)
	}
	attached := make(map[orgchart.NodeID]struct{}, len(actual))
	for _, a := range actual {
		attached[a.WorkerID] = struct{}{}
	}

	requiredSet := make(map[orgchart.NodeID]struct{}, len(members))
	for _, m := range members {
		requiredSet[m] = struct{}{}
	}

	sorted := append([]orgchart.NodeID(nil), members...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, m := range sorted {
		if _, ok := attached[m]; ok {
			continue
		}
		if err := r.attach(ctx, orgID, ch, m, now); err != nil {
			return err
		}
	}

	for _, a := range actual {
		if _, ok := requiredSet[a.WorkerID]; ok {
			continue
		}
		if err := r.attachments.Delete(ctx, orgID, a.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("reconcile: detach %q from %q: %w", a.WorkerID, ch.ID, err)
		}
	}
	return nil
}

// ensureTrigger is a get-or-create on a deterministic channel id.
//
// Concurrency-safe by design. The id is deterministic (s-dm-<pair>,
// s-team-<id>, s-transcript-<id>), so two callers can race on the same id
// — two simultaneous DMs between the same pair, two reconciles touching
// one manager's team chat. A plain check-then-act would let the loser of
// the race hit the row's unique constraint and return a spurious error.
// Instead, on a Create failure we re-read: if the row is now present,
// another caller won the race and the outcome we wanted holds. Only a
// still-absent row is a genuine failure worth surfacing.
func (r *Reconciler) ensureTrigger(ctx context.Context, t trigger.Trigger) error {
	rows, err := r.triggers.Find(ctx, store.WithOrg(t.OrganizationID), store.WithID(t.ID), store.WithLimit(1))
	if err != nil {
		return fmt.Errorf("reconcile: lookup channel %q: %w", t.ID, err)
	}
	if len(rows) > 0 {
		return nil
	}
	if createErr := r.triggers.Create(ctx, t); createErr != nil {
		rows, getErr := r.triggers.Find(ctx, store.WithOrg(t.OrganizationID), store.WithID(t.ID), store.WithLimit(1))
		if getErr != nil || len(rows) == 0 {
			return fmt.Errorf("reconcile: create channel %q: %w", t.ID, createErr)
		}
	}
	return nil
}

// attach adds one member, tolerating the same create race as
// ensureTrigger — the unique index on (org, worker, trigger) means a
// concurrent winner leaves exactly the row we wanted.
func (r *Reconciler) attach(ctx context.Context, orgID string, ch channels.Channel, member orgchart.NodeID, now time.Time) error {
	a, err := attachment.New(managedAttachmentID(member, ch.ID), orgID, member, eventsource.Trigger(ch.ID), string(ch.CreatedBy), now)
	if err != nil {
		return fmt.Errorf("reconcile: build attachment %q→%q: %w", member, ch.ID, err)
	}
	if createErr := r.attachments.Create(ctx, a); createErr != nil {
		rows, findErr := r.attachments.Find(ctx, store.WithOrg(orgID), store.WithWorkerID(member), store.WithTriggerID(ch.ID), store.WithLimit(1))
		if findErr != nil || len(rows) == 0 {
			return fmt.Errorf("reconcile: attach %q→%q: %w", member, ch.ID, createErr)
		}
	}
	return nil
}

// managedAttachmentID is the row id for a managed channel membership.
// It is derived from the pair rather than minted, so a repeat reconcile
// after a partial failure converges on the same row instead of
// accumulating duplicates, and so the row is recognisable as
// reconciler-owned. Operator-created attachments use minted `wa-<id>`s.
func managedAttachmentID(member orgchart.NodeID, channelID string) string {
	return "wa-" + string(member) + "-" + channelID
}

// triggerForChannel builds the Trigger the reconciler persists for a
// required Channel. Managed channels are always local transport: they
// carry internal messages only and have no provider-side hook.
func triggerForChannel(ch channels.Channel, now time.Time, orgID string) (trigger.Trigger, error) {
	return trigger.New(ch.ID, orgID, ch.Name, ch.Description, transport.KindLocal, nil, string(ch.CreatedBy), now)
}
