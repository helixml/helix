// Package dispatch turns a publish on a Stream into one activation per
// subscribed AI Worker. The server is the event bus; Workers are
// reactors. Each activation is a single fresh run of the Spawner — no
// long-running agent loops, no in-process state per worker beyond a
// per-Worker queue that coalesces overlapping events.
//
// Lifecycle:
//   - hire_worker calls DispatchHire to fire a TriggerHire activation
//     (the new Worker's first run).
//   - publish calls Dispatch with the freshly-appended Event to fan it
//     out to every subscribed AI Worker as a TriggerEvent activation.
//
// Both calls return immediately; activations run on goroutines. Each
// Worker has a single runner goroutine that drains a per-Worker
// queue: new triggers arriving while an activation is in flight are
// appended and processed as one coalesced batch when the current
// activation finishes. This collapses webhook cascades (e.g. five
// GitHub events fired by the worker's own action against a shared
// auth token) into a single follow-up activation, which keeps cost
// bounded under burst traffic.
package dispatch

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/runtime"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/store"
)

// outboundTimeout caps how long an outbound webhook POST may take. A
// hung target must not stall the dispatcher. 5 seconds is generous for
// HTTP and short enough that local listeners (nc, requestbin) which
// don't speak HTTP back fail fast and the next event isn't blocked.
const outboundTimeout = 5 * time.Second

// EmailEmitter is the subset of an email transport the dispatcher
// invokes for outbound emit on email-kind Streams. Defining it here
// keeps the dispatcher decoupled from any specific provider package.
type EmailEmitter interface {
	Emit(ctx context.Context, event domain.Event) error
}

// Dispatcher routes Events to subscribed AI Workers and runs the
// configured Spawner for each one. It also emits outbound webhook
// POSTs and outbound email sends for Streams whose Transport is
// configured for them.
type Dispatcher struct {
	store        *store.Store
	spawner      runtime.Spawner
	logger       *slog.Logger
	httpClient   *http.Client
	emailEmitter EmailEmitter

	// per-worker queues coalesce activations. Each is created on first
	// use via sync.Map.LoadOrStore.
	queues sync.Map // map[worker.ID]*workerQueue
}

// workerQueue holds the pending triggers for one Worker plus the
// state needed to coordinate the single runner goroutine that drains
// them. New triggers arriving while running == true are appended to
// pending; the runner picks them up at the top of its next loop
// iteration and feeds them to the Spawner as a single batched
// activation. envPath is captured from the most recent enqueue —
// stable in practice (a Worker's environment doesn't move) but the
// last writer wins if it ever does.
type workerQueue struct {
	mu      sync.Mutex
	pending []activation.Trigger
	envPath string
	running bool
}

// New returns a Dispatcher. spawner may be nil to disable activation
// (useful for tests). logger must be non-nil. The internal HTTP client
// uses a fixed timeout suitable for outbound webhook POSTs; tests that
// need to substitute a fake transport can replace it via SetHTTPClient.
func New(s *store.Store, spawner runtime.Spawner, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		store:      s,
		spawner:    spawner,
		logger:     logger,
		httpClient: &http.Client{Timeout: outboundTimeout},
	}
}

// SetHTTPClient replaces the HTTP client used for outbound webhook
// POSTs. Intended for tests only.
func (d *Dispatcher) SetHTTPClient(c *http.Client) { d.httpClient = c }

// SetEmailEmitter wires in the email transport's outbound emitter.
// Constructor injection isn't an option because the email transport
// also takes a Dispatcher (for inbound activation), so the wiring
// goes Dispatcher.New → Transport.New → Dispatcher.SetEmailEmitter.
// Nil is allowed (email-kind streams will then no-op on outbound).
func (d *Dispatcher) SetEmailEmitter(e EmailEmitter) { d.emailEmitter = e }

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
func (d *Dispatcher) DispatchHire(_ context.Context, workerID worker.ID, envPath string, activationID activation.ID) {
	if d.spawner == nil {
		return
	}
	d.enqueue(workerID, envPath, activation.Trigger{
		Kind:         activation.TriggerHire,
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
// Returns immediately. A per-Worker queue serialises and coalesces
// overlapping subscriber activations within a Worker; outbound POSTs
// have no such ordering guarantee.
func (d *Dispatcher) Dispatch(ctx context.Context, e domain.Event) {
	d.emitOutbound(ctx, e)
	if d.spawner == nil {
		return
	}
	// Parse the canonical Message envelope once — every appended event
	// stores Message JSON in Body. A parse failure here means a
	// hand-poked or pre-migration event; surface the raw body and warn,
	// don't crash the activation.
	msg, err := e.Message()
	if err != nil {
		d.logger.Warn("dispatch: parse message", "event", e.ID, "err", err)
		msg = message.Message{Body: e.Body}
	}
	subs, err := d.store.Subscriptions.ListForStream(ctx, e.StreamID)
	if err != nil {
		d.logger.Error("dispatch: list subscriptions", "stream", e.StreamID, "err", err)
		return
	}
	// Resolve the publishing Worker's kind once so every fan-out target
	// gets the same source_kind on its Trigger. Empty Source (system or
	// transport inbound) leaves SourceKind empty — agent.md treats that
	// as human-origin by default.
	var sourceKind worker.Kind
	if e.Source != "" {
		if sourceWorker, err := d.store.Workers.Get(ctx, e.Source); err == nil {
			sourceKind = sourceWorker.Kind()
		}
	}
	for _, sub := range subs {
		if sub.WorkerID == e.Source {
			continue // do not deliver the event back to its publisher
		}
		w, err := d.store.Workers.Get(ctx, sub.WorkerID)
		if err != nil {
			d.logger.Warn("dispatch: get worker", "worker", sub.WorkerID, "err", err)
			continue
		}
		if w.Kind() != worker.KindAI {
			continue // human Workers are not activated by the runtime
		}
		env, err := d.store.Environments.Get(ctx, sub.WorkerID)
		if err != nil {
			d.logger.Warn("dispatch: get environment", "worker", sub.WorkerID, "err", err)
			continue
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
		d.enqueue(sub.WorkerID, env.Path, trigger)
	}
}

// enqueue appends a trigger to the Worker's queue and starts the
// runner goroutine if one isn't already draining the queue. Returns
// immediately. The activation goroutine outlives the HTTP request
// that triggered enqueue, so it uses context.Background internally.
func (d *Dispatcher) enqueue(workerID worker.ID, envPath string, trigger activation.Trigger) {
	q := d.queueFor(workerID)
	q.mu.Lock()
	q.pending = append(q.pending, trigger)
	q.envPath = envPath // last writer wins; stable in practice
	if q.running {
		q.mu.Unlock()
		return
	}
	q.running = true
	q.mu.Unlock()
	// Runner outlives the HTTP request that triggered enqueue — it
	// uses context.Background internally for the same reason.
	go d.run(workerID, q)
}

// run drains the Worker's queue, calling the Spawner once per drain
// with however many triggers accumulated. Exits when an iteration
// finds the queue empty under the lock — at which point any later
// enqueue will see running == false and start a fresh runner.
func (d *Dispatcher) run(workerID worker.ID, q *workerQueue) {
	for {
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.running = false
			q.mu.Unlock()
			return
		}
		batch := q.pending
		q.pending = nil
		envPath := q.envPath
		q.mu.Unlock()

		d.activate(context.Background(), workerID, envPath, batch)
	}
}

// activate is one synchronous Spawner call. The runner serialises
// these per-Worker so the Spawner is never invoked concurrently for
// the same Worker.
func (d *Dispatcher) activate(ctx context.Context, workerID worker.ID, envPath string, batch []activation.Trigger) {
	d.logger.Info("dispatch.activate.start",
		"worker", workerID,
		"trigger", batch[0].Kind,
		"triggers", len(batch),
		"event", batch[0].EventID,
	)
	err := d.spawner(ctx, workerID, envPath, batch)
	if err != nil && !errors.Is(err, context.Canceled) {
		d.logger.Warn("dispatch.activate.fail",
			"worker", workerID,
			"trigger", batch[0].Kind,
			"triggers", len(batch),
			"err", err,
		)
		return
	}
	d.logger.Info("dispatch.activate.done",
		"worker", workerID,
		"trigger", batch[0].Kind,
		"triggers", len(batch),
	)
}

func (d *Dispatcher) queueFor(workerID worker.ID) *workerQueue {
	got, _ := d.queues.LoadOrStore(workerID, &workerQueue{})
	return got.(*workerQueue)
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
func (d *Dispatcher) emitOutbound(ctx context.Context, e domain.Event) {
	if e.Source == "" {
		return
	}
	stream, err := d.store.Streams.Get(ctx, e.StreamID)
	if err != nil {
		// Stream was deleted, or store error. Either way nothing to emit;
		// the append-side code path has already logged anything material.
		return
	}
	switch stream.Transport.Kind {
	case transport.KindWebhook:
		cfg, err := stream.Transport.WebhookConfig()
		if err != nil {
			d.logger.Warn("dispatch.emit.config", "stream", e.StreamID, "err", err)
			return
		}
		if cfg.OutboundURL == "" {
			return
		}
		go d.postOutbound(cfg.OutboundURL, e) //nolint:gosec // intentional: the POST outlives the request that triggered Dispatch
	case transport.KindEmail:
		if d.emailEmitter == nil {
			return
		}
		go func() { //nolint:gosec // intentional: the send outlives the request that triggered Dispatch
			if err := d.emailEmitter.Emit(context.Background(), e); err != nil {
				d.logger.Warn("dispatch.emit.email", "stream", e.StreamID, "event", e.ID, "err", err)
			}
		}()
	}
}

// postOutbound is the synchronous body of emitOutbound, split out so
// tests can call it directly and so the goroutine has a clean entry
// point. It uses a fresh background context bounded by outboundTimeout
// (via the http.Client) — the originating request context is
// deliberately not propagated, since the POST must outlive the
// request.
func (d *Dispatcher) postOutbound(targetURL string, e domain.Event) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, targetURL, bytes.NewBufferString(e.Body))
	if err != nil {
		d.logger.Warn("dispatch.emit.build", "stream", e.StreamID, "url", targetURL, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Helix-Stream", string(e.StreamID))
	req.Header.Set("X-Helix-Event", string(e.ID))
	resp, err := d.httpClient.Do(req)
	if err != nil {
		d.logger.Warn("dispatch.emit.do", "stream", e.StreamID, "url", targetURL, "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		d.logger.Warn("dispatch.emit.status", "stream", e.StreamID, "url", targetURL, "status", resp.StatusCode)
		return
	}
	d.logger.Info("dispatch.emit.ok", "stream", e.StreamID, "url", targetURL, "status", resp.StatusCode)
}
