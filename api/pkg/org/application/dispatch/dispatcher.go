// Package dispatch turns a publish on a Stream into one activation per
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
// activation carries exactly one trigger so that a busy Stream (e.g. a
// GitHub Stream firing an event per commit, CI run and issue) can't
// fold its backlog into one oversized activation that exhausts the
// Worker's context window. The trade-off is more (sequential)
// activations under burst traffic.
package dispatch

import (
	"context"
	"log/slog"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// Dispatcher routes Events to subscribed AI Workers and runs the
// configured Spawner for each one. It also fans Events out to the
// registered streaming.Outbound emitter for the Stream's Transport Kind
// (webhook, email, …) — but it knows nothing about how each transport
// delivers; that lives behind the streaming.Outbound port.
//
// The per-Worker serialisation logic (one in-flight Spawn per Worker,
// queued triggers drained one at a time in arrival order) moved out to
// activation.Queue in B5.10; Dispatcher delegates Enqueue to its
// embedded Queue and focuses on the event-side fan-out.
type Dispatcher struct {
	store    *store.Store
	queue    *activation.Queue
	logger   *slog.Logger
	outbound map[transport.Kind]streaming.Outbound
}

// New returns a Dispatcher. spawner may be nil to disable activation
// (useful for tests). logger must be non-nil. Outbound emitters are
// registered separately via RegisterOutbound.
func New(s *store.Store, spawner runtime.Spawner, logger *slog.Logger) *Dispatcher {
	var spawn activation.Spawn
	if spawner != nil {
		spawn = activation.Spawn(spawner)
	}
	return &Dispatcher{
		store:    s,
		queue:    activation.NewQueue(spawn, logger),
		logger:   logger,
		outbound: map[transport.Kind]streaming.Outbound{},
	}
}

// RegisterOutbound wires the streaming.Outbound emitter for a transport
// Kind. Late-binding (rather than constructor injection) because some
// transports also take the Dispatcher for inbound activation, so the
// wiring is Dispatcher.New → Transport.New → RegisterOutbound. Kinds
// with no registered emitter no-op on outbound.
func (d *Dispatcher) RegisterOutbound(kind transport.Kind, e streaming.Outbound) {
	d.outbound[kind] = e
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
func (d *Dispatcher) DispatchHire(_ context.Context, orgID string, workerID orgchart.WorkerID, activationID activation.ID) {
	d.queue.Enqueue(orgID, workerID, activation.Trigger{
		Kind:         activation.TriggerHire,
		ActivationID: activationID,
	})
}

// DispatchManual fires an operator-driven activation. Used by the
// worker UI's "Start Desktop" button to put the per-Worker project
// through the full activation pipeline (ensureProject → AttachHelixOrgMCP
// → ensureSession), which re-attaches the helix-org MCP entry that
// applyProject's wholesale Config.Helix replace wipes between
// activations. Without this path, restarting a paused desktop via
// /sessions/{id}/resume alone leaves the desktop without the helix-org
// MCP until the next AI activation or owner-chat call.
//
// Returns immediately; the activation runs on the per-Worker queue
// goroutine. activationID semantics match DispatchHire — callers that
// pre-allocate the audit row pass its ID through; empty means the
// Spawner mints its own. No-op if the Spawner is nil.
func (d *Dispatcher) DispatchManual(_ context.Context, orgID string, workerID orgchart.WorkerID, activationID activation.ID) {
	d.queue.Enqueue(orgID, workerID, activation.Trigger{
		Kind:         activation.TriggerManual,
		ActivationID: activationID,
	})
}

// Dispatch fans an Event out to every AI Worker subscribed to its
// Stream (skipping the Worker that sourced the event) and emits an
// outbound webhook POST if the Stream's Transport is configured for
// it. Each fan-out target — subscriber activation, outbound POST —
// runs on its own goroutine with its own background context, so a
// slow target never stalls the publish that triggered Dispatch.
//
// Returns immediately. A per-Worker queue serialises overlapping
// subscriber activations within a Worker, draining them one trigger at
// a time in arrival order; outbound POSTs have no such ordering
// guarantee.
func (d *Dispatcher) Dispatch(ctx context.Context, e streaming.Event) {
	subs, err := d.store.Subscriptions.ListForStream(ctx, e.OrganizationID, e.StreamID)
	if err != nil {
		// Still attempt outbound emission (DispatchTo does it) even when
		// the subscription list fails — keep the two side effects
		// independent. Pass no targets so fan-out is a no-op.
		d.logger.Error("dispatch: list subscriptions", "stream", e.StreamID, "err", err)
		d.DispatchTo(ctx, e, nil)
		return
	}
	targets := make([]orgchart.WorkerID, 0, len(subs))
	for _, sub := range subs {
		targets = append(targets, orgchart.WorkerID(sub.WorkerID))
	}
	d.DispatchTo(ctx, e, targets)
}

// DispatchTo is the fan-out seam: it emits the Stream's outbound
// traffic (always) and activates exactly the named target Workers
// (subject to the same publisher-skip, AI-only, and source-kind rules
// as the broadcast path). Dispatch computes `targets` as every
// subscriber; the Slack ingest narrows them through a Router first
// (§9.4). A target not subscribed/known is skipped with a warning, so a
// Router can only ever restrict the subscriber set, never invent a
// recipient outside it.
//
// Returns immediately. Each fan-out target runs on the per-Worker queue;
// outbound POSTs have no ordering guarantee.
func (d *Dispatcher) DispatchTo(ctx context.Context, e streaming.Event, targets []orgchart.WorkerID) {
	orgID := e.OrganizationID
	d.emitOutbound(ctx, e)
	// Parse the canonical Message envelope. Every production write
	// goes through Message.Encode via streaming.NewMessageEvent, so a
	// parse failure here is a programming bug — a hand-poked DB row,
	// or a regression in a future write path. Skip fan-out so a bad
	// event doesn't render a half-formed activation prompt; the error
	// is logged so the bug is visible. Outbound emission already
	// fired above and is unaffected.
	msg, err := e.Message()
	if err != nil {
		d.logger.Error("dispatch: parse message — skipping fan-out", "event", e.ID, "err", err)
		return
	}
	// Resolve the publishing Worker's kind once so every fan-out target
	// gets the same source_kind on its Trigger. Empty Source (system or
	// transport inbound) leaves SourceKind empty — agent.md treats that
	// as human-origin by default.
	var sourceKind orgchart.WorkerKind
	if e.Source != "" {
		if sourceWorker, err := d.store.Workers.Get(ctx, orgID, e.Source); err == nil {
			sourceKind = sourceWorker.Kind()
		}
	}
	// Targets are worker-anchored: each names the AI worker to activate
	// directly. A target pointing at a fired worker silently dispatches
	// to nobody (the row is dropped on fire — see lifecycle.Fire).
	for _, workerID := range targets {
		if string(workerID) == string(e.Source) {
			continue // do not deliver the event back to its publisher
		}
		w, err := d.store.Workers.Get(ctx, orgID, workerID)
		if err != nil {
			d.logger.Warn("dispatch: get worker", "worker", workerID, "err", err)
			continue
		}
		if w.Kind() != orgchart.WorkerKindAI {
			continue // human Workers are not activated by the runtime
		}
		trigger := activation.Trigger{
			Kind:       activation.TriggerEvent,
			EventID:    e.ID,
			StreamID:   e.StreamID,
			Source:     e.Source,
			SourceKind: sourceKind,
			Message:    msg, // full canonical envelope; rendered by the spawner into the activation prompt
			CreatedAt:  e.CreatedAt,
		}
		d.queue.Enqueue(orgID, w.ID(), trigger)
	}
}

// emitOutbound dispatches Event-level outbound traffic for Streams
// whose Transport is configured for it: webhook (HTTP POST) or email
// (Postmark API). No-op for local Streams or for transports without
// the necessary config. Failures are logged and dropped — the
// underlying append has already succeeded.
//
// Events with empty Source ("system-emitted", typically inbound
// events from this transport's own webhook handler) are not
// re-emitted. Otherwise a bidirectional Stream (one that's both
// inbound and outbound on the same provider) would echo every
// inbound message straight back out to itself — never useful, often
// catastrophic. Worker-published events (Source != "") still emit.
//
// Runs on a goroutine with its own background context so a slow
// target never stalls the caller.
func (d *Dispatcher) emitOutbound(ctx context.Context, e streaming.Event) {
	if e.Source == "" {
		return
	}
	stream, err := d.store.Streams.Get(ctx, e.OrganizationID, e.StreamID)
	if err != nil {
		// Stream was deleted, or store error. Either way nothing to emit;
		// the append-side code path has already logged anything material.
		return
	}
	emitter, ok := d.outbound[stream.Transport.Kind]
	if !ok {
		return // local stream, or a transport with no outbound emitter
	}
	// Fire on a goroutine with a background context: the delivery must
	// outlive the request that triggered Dispatch. The emitter owns its
	// own timeout, config parsing, and failure logging.
	go func() { //nolint:gosec // intentional: the send outlives the triggering request
		if err := emitter.Emit(context.Background(), stream, e); err != nil {
			d.logger.Warn("dispatch.emit", "stream", e.StreamID, "event", e.ID, "kind", stream.Transport.Kind, "err", err)
		}
	}()
}
