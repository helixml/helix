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
// registered streaming.Outbound emitter for the Topic's Transport Kind
// (webhook, email, …) — but it knows nothing about how each transport
// delivers; that lives behind the streaming.Outbound port.
//
// The per-Worker serialisation logic (one in-flight Spawn per Worker,
// queued triggers drained one at a time in arrival order) moved out to
// activation.Queue in B5.10; Dispatcher delegates Enqueue to its
// embedded Queue and focuses on the event-side fan-out.
type Dispatcher struct {
	store           *store.Store
	queue           *activation.Queue
	logger          *slog.Logger
	outbound        map[transport.Kind]streaming.Outbound
	processorRunner ProcessorRunner
}

// ProcessorRunner is the late-bound execution arm that turns an Event
// into the Processor outputs its Topic feeds. application/processing.Runner
// satisfies it; declared here (not imported) so dispatch does not depend
// on processing — the wiring is Dispatcher.New → build publishing →
// build Runner → RegisterProcessorRunner, exactly like RegisterOutbound.
type ProcessorRunner interface {
	Run(ctx context.Context, e streaming.Event, msg streaming.Message)
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

// RegisterProcessorRunner wires the execution arm that fans an Event
// out to the Processors reading its Topic. Late-bound for the same
// reason as RegisterOutbound: the Runner depends on the publishing
// service, which is built after the Dispatcher. nil runner → Dispatch's
// processor fan-out no-ops.
func (d *Dispatcher) RegisterProcessorRunner(r ProcessorRunner) {
	d.processorRunner = r
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
func (d *Dispatcher) DispatchHire(_ context.Context, orgID string, workerID orgchart.BotID, activationID activation.ID) {
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
func (d *Dispatcher) DispatchManual(_ context.Context, orgID string, workerID orgchart.BotID, activationID activation.ID) {
	d.queue.Enqueue(orgID, workerID, activation.Trigger{
		Kind:         activation.TriggerManual,
		ActivationID: activationID,
	})
}

// Dispatch fans an Event out to every AI Worker subscribed to its
// Topic (skipping the Worker that sourced the event) and emits an
// outbound webhook POST if the Topic's Transport is configured for
// it. Each fan-out target — subscriber activation, outbound POST —
// runs on its own goroutine with its own background context, so a
// slow target never stalls the publish that triggered Dispatch.
//
// Returns immediately. A per-Worker queue serialises overlapping
// subscriber activations within a Worker, draining them one trigger at
// a time in arrival order; outbound POSTs have no such ordering
// guarantee.
func (d *Dispatcher) Dispatch(ctx context.Context, e streaming.Event) {
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
	subs, err := d.store.Subscriptions.ListForTopic(ctx, orgID, e.TopicID)
	if err != nil {
		d.logger.Error("dispatch: list subscriptions", "topic", e.TopicID, "err", err)
		return
	}
	// Subscriptions are bot-anchored: each subscription names the bot to
	// activate directly. A subscription pointing at a fired bot silently
	// dispatches to nobody (the row is dropped on fire — see
	// lifecycle.Fire).
	for _, sub := range subs {
		botID := orgchart.BotID(sub.BotID)
		if string(botID) == string(e.Source) {
			continue // do not deliver the event back to its publisher
		}
		b, err := d.store.Bots.Get(ctx, orgID, botID)
		if err != nil {
			d.logger.Warn("dispatch: get bot", "bot", botID, "err", err)
			continue
		}
		// A human node is a person placeholder — it never runs. Never spawn
		// it. Delivery-to-human (the in-app ask + external channels) is
		// Stage 2; for now a human subscriber is simply not activated.
		// See design/2026-07-07-humans-in-the-org.md.
		if b.IsHuman() {
			continue
		}
		trigger := activation.Trigger{
			Kind:      activation.TriggerEvent,
			EventID:   e.ID,
			TopicID:   e.TopicID,
			Source:    e.Source,
			Message:   msg, // full canonical envelope; rendered by the spawner into the activation prompt
			CreatedAt: e.CreatedAt,
		}
		d.queue.Enqueue(orgID, b.ID, trigger)
	}
	// Processor fan-out: hand the event + parsed message to the
	// execution arm, which publishes each processor's output back
	// through the same publish→dispatch path (so output topics dispatch
	// to their own subscribers, and processor chains just recurse).
	// Late-bound; no-op until RegisterProcessorRunner is called.
	if d.processorRunner != nil {
		d.processorRunner.Run(ctx, e, msg)
	}
}

// emitOutbound dispatches Event-level outbound traffic for Topics
// whose Transport is configured for it: webhook (HTTP POST) or email
// (Postmark API). No-op for local Topics or for transports without
// the necessary config. Failures are logged and dropped — the
// underlying append has already succeeded.
//
// Events with empty Source ("system-emitted", typically inbound
// events from this transport's own webhook handler) are not
// re-emitted. Otherwise a bidirectional Topic (one that's both
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
	topic, err := d.store.Topics.Get(ctx, e.OrganizationID, e.TopicID)
	if err != nil {
		// Topic was deleted, or store error. Either way nothing to emit;
		// the append-side code path has already logged anything material.
		return
	}
	emitter, ok := d.outbound[topic.Transport.Kind]
	if !ok {
		return // local topic, or a transport with no outbound emitter
	}
	// Fire on a goroutine with a background context: the delivery must
	// outlive the request that triggered Dispatch. The emitter owns its
	// own timeout, config parsing, and failure logging.
	go func() { //nolint:gosec // intentional: the send outlives the triggering request
		if err := emitter.Emit(context.Background(), topic, e); err != nil {
			d.logger.Warn("dispatch.emit", "topic", e.TopicID, "event", e.ID, "kind", topic.Transport.Kind, "err", err)
		}
	}()
}
