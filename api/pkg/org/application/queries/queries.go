// Package queries is the read facade for the org graph. It is the
// single application-layer home for the projection reads that the REST
// read handlers and the per-Bot MCP server used to make directly
// against the store repositories.
//
// Unlike the per-aggregate mutation services (triggers/bots/…), this is
// intentionally ONE service spanning several repos: reads carry no
// invariants to keep honest, so there is nothing to split on, and the
// design (§5.3/§8) explicitly sanctions "a thin query service for
// consistency." Methods return domain aggregates — DTO mapping stays in
// the adapter. Each method is one repo call; no business logic lives
// here.
package queries

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

// Queries reads the org graph. Constructed once at the composition root
// from the narrow read repositories.
type Queries struct {
	bots        store.Nodes
	lines       store.ReportingLines
	triggers    store.Triggers
	attachments store.WorkerAttachments
	processors  store.Processors
	events      store.Events
	activations activation.Repository
}

// Deps are the constructor-injected read repositories. Any may be nil if
// a deployment doesn't wire that aggregate; the matching method then
// returns an error from the nil repo (callers already tolerate read
// failures by degrading the projection).
type Deps struct {
	Nodes          store.Nodes
	ReportingLines store.ReportingLines
	Triggers       store.Triggers
	Attachments    store.WorkerAttachments
	Processors     store.Processors
	Events         store.Events
	Activations    activation.Repository
}

// New constructs the read facade.
func New(deps Deps) *Queries {
	return &Queries{
		bots:        deps.Nodes,
		lines:       deps.ReportingLines,
		triggers:    deps.Triggers,
		attachments: deps.Attachments,
		processors:  deps.Processors,
		events:      deps.Events,
		activations: deps.Activations,
	}
}

func (q *Queries) ListBots(ctx context.Context, orgID string) ([]orgchart.Node, error) {
	return q.bots.List(ctx, orgID)
}

func (q *Queries) GetBot(ctx context.Context, orgID string, id orgchart.NodeID) (orgchart.Node, error) {
	return q.bots.Get(ctx, orgID, id)
}

// ReportingLinesWired reports whether the reporting-lines repo is
// available — some handlers 501 / degrade when it isn't.
func (q *Queries) ReportingLinesWired() bool { return q.lines != nil }

func (q *Queries) ListReportingLines(ctx context.Context, orgID string) ([]orgchart.ReportingLine, error) {
	return q.lines.List(ctx, orgID)
}

func (q *Queries) ListManagers(ctx context.Context, orgID string, reportID orgchart.NodeID) ([]orgchart.NodeID, error) {
	return q.lines.ListManagers(ctx, orgID, reportID)
}

func (q *Queries) ListTriggers(ctx context.Context, orgID string) ([]trigger.Trigger, error) {
	return q.triggers.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("created_at"), store.WithOrderAsc("id"))
}

// GetTrigger returns one Trigger, or store.ErrNotFound when the
// (org, id) pair does not exist.
func (q *Queries) GetTrigger(ctx context.Context, orgID, id string) (trigger.Trigger, error) {
	rows, err := q.triggers.Find(ctx, store.WithOrg(orgID), store.WithID(id), store.WithLimit(1))
	if err != nil {
		return trigger.Trigger{}, err
	}
	if len(rows) == 0 {
		return trigger.Trigger{}, store.ErrNotFound
	}
	return rows[0], nil
}

// TriggerMembers returns the Workers attached to a Trigger.
func (q *Queries) TriggerMembers(ctx context.Context, orgID, triggerID string) ([]attachment.Attachment, error) {
	return q.attachments.Find(ctx, store.WithOrg(orgID), store.WithTriggerID(triggerID), store.WithOrderAsc("created_at"), store.WithOrderAsc("id"))
}

// WorkerAttachments returns every source a Worker is attached to.
func (q *Queries) WorkerAttachments(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]attachment.Attachment, error) {
	return q.attachments.Find(ctx, store.WithOrg(orgID), store.WithWorkerID(workerID), store.WithOrderAsc("created_at"), store.WithOrderAsc("id"))
}

func (q *Queries) StreamEvents(ctx context.Context, orgID string, streamID streaming.StreamID, limit int) ([]streaming.Event, error) {
	return q.events.ListForStream(ctx, orgID, streamID, limit)
}

func (q *Queries) AllEvents(ctx context.Context, orgID string, limit int) ([]streaming.Event, error) {
	return q.events.ListAll(ctx, orgID, limit)
}

// PageStreamEvents returns a page of events on one stream, newest first,
// for the paginated REST events endpoint.
func (q *Queries) PageStreamEvents(ctx context.Context, orgID string, streamID streaming.StreamID, limit, offset int) ([]streaming.Event, error) {
	return q.events.PageForStream(ctx, orgID, streamID, limit, offset)
}

// CountStreamEvents returns the total number of events on one stream —
// the total-count meta the paginated events endpoint surfaces.
func (q *Queries) CountStreamEvents(ctx context.Context, orgID string, streamID streaming.StreamID) (int, error) {
	return q.events.CountForStream(ctx, orgID, streamID)
}

// WorkerStreams resolves the event streams a Worker currently receives:
// one per attachment. A Trigger's stream is its own id; a Processor
// branch records its stream on the branch, so those are looked up.
// Attachments to a source that no longer exists are skipped — the row is
// inert until the cascade removes it, and a Worker's inbox must not fail
// to load because of one.
//
// Order is stable (attachment order), and duplicates are dropped: two
// branches of one Processor can, after the cutover conversion, share a
// stream.
func (q *Queries) WorkerStreams(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]streaming.StreamID, error) {
	rows, err := q.WorkerAttachments(ctx, orgID, workerID)
	if err != nil {
		return nil, fmt.Errorf("list attachments for %q: %w", workerID, err)
	}
	streams := make([]streaming.StreamID, 0, len(rows))
	seen := make(map[streaming.StreamID]struct{}, len(rows))
	cache := map[string]processor.Processor{}
	for _, a := range rows {
		var streamID streaming.StreamID
		switch a.Source.Kind {
		case eventsource.KindTrigger:
			streamID = a.Source.TriggerID
		case eventsource.KindProcessorOutput:
			p, ok := cache[a.Source.ProcessorID]
			if !ok {
				p, err = q.processors.Get(ctx, orgID, a.Source.ProcessorID)
				if err != nil {
					if errors.Is(err, store.ErrNotFound) {
						continue
					}
					return nil, fmt.Errorf("resolve attachment %q: %w", a.ID, err)
				}
				cache[a.Source.ProcessorID] = p
			}
			out, ok := p.Output(a.Source.OutputID)
			if !ok {
				continue
			}
			streamID = out.StreamID
		}
		if streamID == "" {
			continue
		}
		if _, dup := seen[streamID]; dup {
			continue
		}
		seen[streamID] = struct{}{}
		streams = append(streams, streamID)
	}
	return streams, nil
}

// BotEvents returns the newest events across every stream the Worker is
// attached to — its inbox.
func (q *Queries) BotEvents(ctx context.Context, orgID string, botID orgchart.NodeID, limit int) ([]streaming.Event, error) {
	streams, err := q.WorkerStreams(ctx, orgID, botID)
	if err != nil {
		return nil, err
	}
	return q.events.ListForStreams(ctx, orgID, streams, limit)
}

// ListReports returns the direct reports of the given manager.
func (q *Queries) ListReports(ctx context.Context, orgID string, managerID orgchart.NodeID) ([]orgchart.NodeID, error) {
	return q.lines.ListReports(ctx, orgID, managerID)
}

// FindAttachment returns the Worker's attachment to a Trigger, or
// store.ErrNotFound when the Worker is not attached to it.
func (q *Queries) FindAttachment(ctx context.Context, orgID string, workerID orgchart.NodeID, triggerID string) (attachment.Attachment, error) {
	rows, err := q.attachments.Find(ctx, store.WithOrg(orgID), store.WithWorkerID(workerID), store.WithTriggerID(triggerID), store.WithLimit(1))
	if err != nil {
		return attachment.Attachment{}, err
	}
	if len(rows) == 0 {
		return attachment.Attachment{}, store.ErrNotFound
	}
	return rows[0], nil
}

// GetActivation returns one activation audit row by id.
func (q *Queries) GetActivation(ctx context.Context, orgID string, id activation.ID) (*activation.Activation, error) {
	return q.activations.Get(ctx, orgID, id)
}
