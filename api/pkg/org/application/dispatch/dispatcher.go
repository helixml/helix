// Package dispatch turns a publish on a Topic into one activation per
// subscribed AI Worker. The server is the event bus; Workers are
// reactors. Each activation is a single fresh run of the Spawner — no
// long-running agent loops, no in-process state per worker beyond a
// per-Worker queue that serialises overlapping events.
//
// Lifecycle:
//   - hire_worker calls DispatchHire to fire a TriggerHire activation
//     (the new Worker's first run).
//   - publish calls Dispatch with the freshly-appended Event to fan it
//     out to every subscribed AI Worker as a TriggerEvent activation.
//
// Both calls return immediately; activations run on goroutines. Each
// Worker has a single runner goroutine that drains a per-Worker
// queue: new triggers arriving while an activation is in flight wait
// in the queue and are processed one at a time, in arrival order, as
// the current activation finishes. Triggers are not coalesced — each
// activation carries exactly one trigger so that a busy Topic (e.g. a
// GitHub Topic firing an event per commit, CI run and issue) can't
// fold its backlog into one oversized activation that exhausts the
// Worker's context window. The trade-off is more (sequential)
// activations under burst traffic.
package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// Dispatcher routes Events to subscribed AI Workers and runs the
// configured Spawner for each one. External delivery is deliberately not
// part of dispatch; legacy Topic delivery is isolated in publishing.
//
// The per-Worker serialisation logic (one in-flight Spawn per Worker,
// queued triggers drained one at a time in arrival order) moved out to
// activation.Queue in B5.10; Dispatcher delegates Enqueue to its
// embedded Queue and focuses on the event-side fan-out.
type Dispatcher struct {
	store           *store.Store
	queue           ActivationQueue
	logger          *slog.Logger
	processorRunner ProcessorRunner
}

type ActivationQueue interface {
	Enqueue(orgID string, agentID orgchart.NodeID, trigger activation.Trigger)
}

// ProcessorRunner is the late-bound execution arm that turns an Event
// into the Processor outputs its Topic feeds. application/processing.Runner
// satisfies it; declared here (not imported) so dispatch does not depend
// on processing — the wiring is Dispatcher.New → build publishing →
// build Runner → RegisterProcessorRunner.
type ProcessorRunner interface {
	Run(ctx context.Context, e streaming.Event, msg streaming.Message)
}

// New returns a Dispatcher. spawner may be nil to disable activation
// (useful for tests). logger must be non-nil.
func New(s *store.Store, spawner runtime.Spawner, logger *slog.Logger) *Dispatcher {
	var spawn activation.Spawn
	if spawner != nil {
		spawn = activation.Spawn(spawner)
	}
	return &Dispatcher{
		store:  s,
		queue:  activation.NewQueue(spawn, logger),
		logger: logger,
	}
}

// RegisterProcessorRunner wires the execution arm that fans an Event
// out to the Processors reading its Topic. Late-bound for the same
// reason: the Runner depends on the publishing
// service, which is built after the Dispatcher. nil runner → Dispatch's
// processor fan-out no-ops.
func (d *Dispatcher) RegisterProcessorRunner(r ProcessorRunner) {
	d.processorRunner = r
}

// RegisterActivationQueue replaces the in-memory activation queue. Production
// uses this to install restart-safe delivery without changing dispatch callers.
func (d *Dispatcher) RegisterActivationQueue(q ActivationQueue) {
	if q != nil {
		d.queue = q
	}
}

// DispatchHire fires a hire-time activation for a freshly-created AI
// Worker. Returns immediately; the activation runs on a goroutine with
// its own background context — independent of the HTTP request that
// triggered it, so the spawned process is not killed when the request
// completes.
//
// activationID is the pre-allocated audit-row ID hire_worker created
// alongside the Worker. It's threaded through the trigger so the
// Spawner reuses the same row (StartedAt=now, EndedAt=nil) rather
// than writing a sibling. Empty activationID is allowed for callers
// that don't pre-allocate — the Spawner mints its own ID in that
// case.
//
// No-op if the Spawner is nil.
func (d *Dispatcher) DispatchHire(_ context.Context, orgID string, workerID orgchart.NodeID, activationID activation.ID) {
	d.queue.Enqueue(orgID, workerID, activation.Trigger{
		Kind:         activation.TriggerHire,
		ActivationID: activationID,
	})
}

// DispatchManual fires an operator-driven activation. Used by the
// worker UI's "Start Desktop" button to put the per-Worker project
// through the full activation pipeline (ensureProject -> ensureSession).
//
// Returns immediately; the activation runs on the per-Worker queue
// goroutine. activationID semantics match DispatchHire — callers that
// pre-allocate the audit row pass its ID through; empty means the
// Spawner mints its own. No-op if the Spawner is nil.
func (d *Dispatcher) DispatchManual(_ context.Context, orgID string, workerID orgchart.NodeID, activationID activation.ID) {
	d.queue.Enqueue(orgID, workerID, activation.Trigger{
		Kind:         activation.TriggerManual,
		ActivationID: activationID,
	})
}

// Dispatch fans an Event out to every AI Worker subscribed to its
// Topic (skipping the Worker that sourced the event), then traverses
// processors. It never performs external delivery.
//
// Returns immediately. A per-Worker queue serialises overlapping
// subscriber activations within a Worker, draining them one trigger at
// a time in arrival order; outbound POSTs have no such ordering
// guarantee.
func (d *Dispatcher) Dispatch(ctx context.Context, e streaming.Event) {
	orgID := e.OrganizationID
	// Parse the canonical Message envelope. Every production write
	// goes through Message.Encode via streaming.NewMessageEvent, so a
	// parse failure here is a programming bug — a hand-poked DB row,
	// or a regression in a future write path. Skip fan-out so a bad
	// event doesn't render a half-formed activation prompt; the error
	// is logged so the bug is visible.
	msg, err := e.Message()
	if err != nil {
		d.logger.Error("dispatch: parse message — skipping fan-out", "event", e.ID, "err", err)
		return
	}
	subs, err := d.store.Subscriptions.ListForTopic(ctx, orgID, e.TopicID)
	if err != nil {
		d.logger.Error("dispatch: list subscriptions", "topic", e.TopicID, "err", err)
		return
	}
	// Subscriptions are bot-anchored: each subscription names the bot to
	// activate directly. A subscription pointing at a fired bot silently
	// dispatches to nobody (the row is dropped on fire — see
	// lifecycle.Fire).
	targets := make([]orgchart.NodeID, 0, len(subs))
	for _, sub := range subs {
		targets = append(targets, orgchart.NodeID(sub.NodeID))
	}
	d.deliver(ctx, orgID, targets, orgchart.NodeID(e.Source), activation.Trigger{
		Kind:      activation.TriggerEvent,
		EventID:   e.ID,
		TopicID:   e.TopicID,
		Source:    e.Source,
		Message:   msg, // full canonical envelope; rendered by the spawner into the activation prompt
		CreatedAt: e.CreatedAt,
	})
	// Processor fan-out: hand the event + parsed message to the
	// execution arm, which publishes each processor's output back
	// through the same publish→dispatch path (so output topics dispatch
	// to their own subscribers, and processor chains just recurse).
	// Late-bound; no-op until RegisterProcessorRunner is called.
	if d.processorRunner != nil {
		d.processorRunner.Run(ctx, e, msg)
	}
}

// DispatchSource routes a Trigger or exact Processor-output event through
// Worker attachments. Production transports remain on Dispatch until PR 3.
func (d *Dispatcher) DispatchSource(ctx context.Context, e eventsource.Event) error {
	if d.store.WorkerAttachments == nil {
		return errors.New("dispatch source: attachment repository is not configured")
	}
	opts := []store.Option{store.WithOrg(e.OrganizationID)}
	if e.Source.Kind == eventsource.KindTrigger {
		opts = append(opts, store.WithTriggerID(e.Source.TriggerID))
	} else {
		opts = append(opts, store.WithProcessorID(e.Source.ProcessorID), store.WithOutputID(e.Source.OutputID))
	}
	rows, err := d.store.WorkerAttachments.Find(ctx, opts...)
	if err != nil {
		return fmt.Errorf("dispatch source: find attachments: %w", err)
	}
	targets := make([]orgchart.NodeID, 0, len(rows))
	for _, a := range rows {
		targets = append(targets, a.WorkerID)
	}
	d.deliver(ctx, e.OrganizationID, targets, orgchart.NodeID(e.OriginatingWorkerID), activation.Trigger{Kind: activation.TriggerEvent, EventID: streaming.EventID(e.ID), EventSource: e.Source, Source: orgchart.NodeID(e.OriginatingWorkerID), Message: e.Message, CreatedAt: e.CreatedAt})
	return nil
}

func (d *Dispatcher) deliver(ctx context.Context, orgID string, targets []orgchart.NodeID, origin orgchart.NodeID, trigger activation.Trigger) {
	seen := map[orgchart.NodeID]struct{}{}
	for _, id := range targets {
		if id == origin {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		node, err := d.store.Nodes.Get(ctx, orgID, id)
		if err != nil {
			d.logger.Warn("dispatch: get bot", "bot", id, "err", err)
			continue
		}
		if node.IsHuman() {
			continue
		}
		d.queue.Enqueue(orgID, node.ID, trigger)
	}
}
