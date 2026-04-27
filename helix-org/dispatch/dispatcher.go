// Package dispatch turns a publish on a Stream into one activation per
// subscribed AI Worker. The server is the event bus; Workers are
// reactors. Each activation is a single fresh run of the Spawner — no
// long-running agent loops, no in-process state per worker beyond a
// per-Worker mutex that serialises overlapping events.
//
// Lifecycle:
//   - hire_worker calls DispatchHire to fire a TriggerHire activation
//     (the new Worker's first run).
//   - publish calls Dispatch with the freshly-appended Event to fan it
//     out to every subscribed AI Worker as a TriggerEvent activation.
//
// Both calls return immediately; activations run on goroutines. Per-
// Worker serialisation guarantees only one Spawner at a time per
// Worker, so two events arriving in quick succession are processed in
// order, never in parallel.
package dispatch

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/tools"
)

// outboundTimeout caps how long an outbound webhook POST may take. A
// hung target must not stall the dispatcher. 5 seconds is generous for
// HTTP and short enough that local listeners (nc, requestbin) which
// don't speak HTTP back fail fast and the next event isn't blocked.
const outboundTimeout = 5 * time.Second

// Dispatcher routes Events to subscribed AI Workers and runs the
// configured Spawner for each one. It also emits outbound webhook
// POSTs for Streams whose Transport is configured for them.
type Dispatcher struct {
	store      *store.Store
	spawner    tools.Spawner
	logger     *slog.Logger
	httpClient *http.Client

	// per-worker mutexes serialise activations. Each is created on first
	// use via sync.Map.LoadOrStore.
	locks sync.Map // map[domain.WorkerID]*sync.Mutex
}

// New returns a Dispatcher. spawner may be nil to disable activation
// (useful for tests). logger must be non-nil. The internal HTTP client
// uses a fixed timeout suitable for outbound webhook POSTs; tests that
// need to substitute a fake transport can replace it via SetHTTPClient.
func New(s *store.Store, spawner tools.Spawner, logger *slog.Logger) *Dispatcher {
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

// DispatchHire fires a hire-time activation for a freshly-created AI
// Worker. Returns immediately; the activation runs on a goroutine with
// its own background context — independent of the HTTP request that
// triggered it, so the spawned process is not killed when the request
// completes.
// No-op if the Spawner is nil.
func (d *Dispatcher) DispatchHire(_ context.Context, workerID domain.WorkerID, envPath string) {
	if d.spawner == nil {
		return
	}
	go d.activate(context.Background(), workerID, envPath, tools.Trigger{Kind: tools.TriggerHire}) //nolint:gosec // intentional: the activation outlives the HTTP request that triggered DispatchHire
}

// Dispatch fans an Event out to every AI Worker subscribed to its
// Stream (skipping the Worker that sourced the event) and emits an
// outbound webhook POST if the Stream's Transport is configured for
// it. Each fan-out target — subscriber activation, outbound POST —
// runs on its own goroutine with its own background context, so a
// slow target never stalls the publish that triggered Dispatch.
//
// Returns immediately. Per-Worker mutexes serialise overlapping
// subscriber activations within a Worker; outbound POSTs have no
// such ordering guarantee.
func (d *Dispatcher) Dispatch(ctx context.Context, e domain.Event) {
	d.emitOutbound(ctx, e)
	if d.spawner == nil {
		return
	}
	subs, err := d.store.Subscriptions.ListForStream(ctx, e.StreamID)
	if err != nil {
		d.logger.Error("dispatch: list subscriptions", "stream", e.StreamID, "err", err)
		return
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
		if w.Kind() != domain.WorkerKindAI {
			continue // human Workers are not activated by the runtime
		}
		env, err := d.store.Environments.Get(ctx, sub.WorkerID)
		if err != nil {
			d.logger.Warn("dispatch: get environment", "worker", sub.WorkerID, "err", err)
			continue
		}
		trigger := tools.Trigger{
			Kind:      tools.TriggerEvent,
			EventID:   e.ID,
			StreamID:  e.StreamID,
			Source:    e.Source,
			Body:      e.Body,
			CreatedAt: e.CreatedAt,
		}
		// Decouple from the request context so the activation isn't
		// cancelled when the HTTP request that triggered publish returns.
		go d.activate(context.Background(), sub.WorkerID, env.Path, trigger) //nolint:gosec // intentional: the activation outlives the HTTP request that triggered Dispatch
	}
}

// activate acquires the per-Worker mutex, then invokes the Spawner.
// Spawner is synchronous (returns when claude exits), so the mutex is
// held for the full activation.
func (d *Dispatcher) activate(ctx context.Context, workerID domain.WorkerID, envPath string, trigger tools.Trigger) {
	mu := d.lockFor(workerID)
	mu.Lock()
	defer mu.Unlock()
	d.logger.Info("dispatch.activate.start",
		"worker", workerID,
		"trigger", trigger.Kind,
		"event", trigger.EventID,
	)
	err := d.spawner(ctx, workerID, envPath, trigger)
	if err != nil && !errors.Is(err, context.Canceled) {
		d.logger.Warn("dispatch.activate.fail",
			"worker", workerID,
			"trigger", trigger.Kind,
			"err", err,
		)
		return
	}
	d.logger.Info("dispatch.activate.done",
		"worker", workerID,
		"trigger", trigger.Kind,
	)
}

func (d *Dispatcher) lockFor(workerID domain.WorkerID) *sync.Mutex {
	got, _ := d.locks.LoadOrStore(workerID, &sync.Mutex{})
	return got.(*sync.Mutex)
}

// emitOutbound POSTs the event body to the Stream's outbound URL, if
// the Stream's Transport is webhook with an outbound_url configured.
// No-op for any other Transport, for webhook streams with no
// outbound_url, or for streams that have been deleted between append
// and dispatch. Failures (lookup, HTTP, non-2xx, timeout) are logged
// and dropped — the underlying append has already succeeded.
//
// Runs on a goroutine with its own background context so a slow
// target never stalls the caller.
func (d *Dispatcher) emitOutbound(ctx context.Context, e domain.Event) {
	stream, err := d.store.Streams.Get(ctx, e.StreamID)
	if err != nil {
		// Stream was deleted, or store error. Either way nothing to emit;
		// the append-side code path has already logged anything material.
		return
	}
	if stream.Transport.Kind != domain.TransportWebhook {
		return
	}
	cfg, err := stream.Transport.WebhookConfig()
	if err != nil {
		d.logger.Warn("dispatch.emit.config", "stream", e.StreamID, "err", err)
		return
	}
	if cfg.OutboundURL == "" {
		return
	}
	go d.postOutbound(cfg.OutboundURL, e) //nolint:gosec // intentional: the POST outlives the request that triggered Dispatch
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
