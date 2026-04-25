// Package dispatch turns a publish on a Channel into one activation per
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
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/tools"
)

// Dispatcher routes Events to subscribed AI Workers and runs the
// configured Spawner for each one.
type Dispatcher struct {
	store   *store.Store
	spawner tools.Spawner
	logger  *slog.Logger

	// per-worker mutexes serialise activations. Each is created on first
	// use via sync.Map.LoadOrStore.
	locks sync.Map // map[domain.WorkerID]*sync.Mutex
}

// New returns a Dispatcher. spawner may be nil to disable activation
// (useful for tests). logger must be non-nil.
func New(s *store.Store, spawner tools.Spawner, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{store: s, spawner: spawner, logger: logger}
}

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
// Channel, skipping the Worker that sourced the event. Each subscriber's
// activation runs on its own goroutine with its own background context
// (independent of the HTTP request that triggered the publish). Per-
// Worker mutexes ensure serial processing within a Worker.
//
// Returns immediately. Errors looking up subscribers are logged.
func (d *Dispatcher) Dispatch(ctx context.Context, e domain.Event) {
	if d.spawner == nil {
		return
	}
	streams, err := d.store.Streams.ListForChannel(ctx, e.ChannelID)
	if err != nil {
		d.logger.Error("dispatch: list streams", "channel", e.ChannelID, "err", err)
		return
	}
	for _, s := range streams {
		if s.WorkerID == e.Source {
			continue // do not deliver the event back to its publisher
		}
		w, err := d.store.Workers.Get(ctx, s.WorkerID)
		if err != nil {
			d.logger.Warn("dispatch: get worker", "worker", s.WorkerID, "err", err)
			continue
		}
		if w.Kind() != domain.WorkerKindAI {
			continue // human Workers are not activated by the runtime
		}
		env, err := d.store.Environments.Get(ctx, s.WorkerID)
		if err != nil {
			d.logger.Warn("dispatch: get environment", "worker", s.WorkerID, "err", err)
			continue
		}
		trigger := tools.Trigger{
			Kind:      tools.TriggerEvent,
			EventID:   e.ID,
			ChannelID: e.ChannelID,
			Source:    e.Source,
			Body:      e.Body,
			CreatedAt: e.CreatedAt,
		}
		// Decouple from the request context so the activation isn't
		// cancelled when the HTTP request that triggered publish returns.
		go d.activate(context.Background(), s.WorkerID, env.Path, trigger) //nolint:gosec // intentional: the activation outlives the HTTP request that triggered Dispatch
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
