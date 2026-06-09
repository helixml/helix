package activation

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// Spawn is the callback Queue fires per coalesced batch. The shape
// is identical to runtime.Spawner; the activation package doesn't
// import runtime to avoid the cycle (runtime already imports
// activation for Trigger). Callers convert their runtime.Spawner via
// `activation.Spawn(rs)`.
//
// A nil Spawn turns Enqueue into a no-op — useful for tests / event-
// side wirings that exercise transport fan-out without running real
// activations.
type Spawn func(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, triggers []Trigger) error

// Queue holds the per-Worker pending-trigger lists and the lifecycle
// state that turns bursts of arrivals into a single Spawn call. The
// invariants it owns (05 §3):
//
//   - at most one in-flight Spawn per Worker;
//   - every trigger that arrives while a Worker's spawn is running
//     waits in pending and is delivered as part of the next batch;
//   - distinct Workers run independently.
//
// Lifted out of helix-org/dispatch.Dispatcher in B5.10 because the
// queueing logic isn't specific to Event/transport fan-out — every
// activation-emitter needs the same per-Worker serialisation, not
// just the Stream-event dispatcher. Dispatcher now holds a *Queue
// and delegates Enqueue.
type Queue struct {
	spawn  Spawn
	logger *slog.Logger
	lanes  sync.Map // map[laneKey]*workerLane
}

// laneKey identifies a serialisation lane. It MUST include orgID:
// Worker IDs are only unique within an org (the store keys workers by
// the composite (org_id, id)), so the owner of every org shares the id
// "w-owner". Keying lanes by workerID alone collapses every org's
// w-owner into one lane — their triggers coalesce into a single batch
// and lane.orgID becomes last-writer-wins, so one org's activation runs
// under another org's context (wrong project, MCP URL, secrets). The
// composite key keeps each tenant's lane separate.
type laneKey struct {
	orgID    string
	workerID orgchart.WorkerID
}

// NewQueue returns a Queue that calls spawn per coalesced batch. spawn
// may be nil — Enqueue then no-ops, which keeps tests that don't
// exercise the runtime green. logger may be nil; falls back to
// slog.Default.
func NewQueue(spawn Spawn, logger *slog.Logger) *Queue {
	if logger == nil {
		logger = slog.Default()
	}
	return &Queue{spawn: spawn, logger: logger}
}

// workerLane is the per-Worker state. New triggers arriving while
// running == true are appended to pending; the runner picks them up
// at the top of its next loop iteration and feeds them to spawn as
// one coalesced batch. envPath is captured from the most recent
// Enqueue — stable in practice (a Worker's environment doesn't
// move) but last-writer-wins if it ever does.
type workerLane struct {
	mu      sync.Mutex
	pending []Trigger
	orgID   string
	envPath string
	running bool
}

// Enqueue appends a trigger to the Worker's lane and starts the
// runner goroutine if one isn't already draining the lane. Returns
// immediately. The runner uses context.Background internally so it
// outlives the HTTP request that triggered Enqueue.
func (q *Queue) Enqueue(orgID string, workerID orgchart.WorkerID, envPath string, trigger Trigger) {
	if q.spawn == nil {
		return
	}
	lane := q.laneFor(orgID, workerID)
	lane.mu.Lock()
	lane.pending = append(lane.pending, trigger)
	lane.envPath = envPath // last writer wins; stable in practice
	lane.orgID = orgID
	if lane.running {
		lane.mu.Unlock()
		return
	}
	lane.running = true
	lane.mu.Unlock()
	go q.run(workerID, lane)
}

// run drains the Worker's lane, calling spawn once per drain with
// however many triggers accumulated. Exits when an iteration finds
// the lane empty under the lock — at which point any later Enqueue
// will see running == false and start a fresh runner.
func (q *Queue) run(workerID orgchart.WorkerID, lane *workerLane) {
	for {
		lane.mu.Lock()
		if len(lane.pending) == 0 {
			lane.running = false
			lane.mu.Unlock()
			return
		}
		batch := lane.pending
		lane.pending = nil
		envPath := lane.envPath
		orgID := lane.orgID
		lane.mu.Unlock()

		q.activate(context.Background(), orgID, workerID, envPath, batch)
	}
}

// activate is one synchronous spawn call. The runner serialises
// these per-Worker so spawn is never invoked concurrently for the
// same Worker.
func (q *Queue) activate(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, batch []Trigger) {
	q.logger.Info("activation.start",
		"worker", workerID,
		"trigger", batch[0].Kind,
		"triggers", len(batch),
		"event", batch[0].EventID,
	)
	err := q.spawn(ctx, orgID, workerID, envPath, batch)
	if err != nil && !errors.Is(err, context.Canceled) {
		q.logger.Warn("activation.fail",
			"worker", workerID,
			"trigger", batch[0].Kind,
			"triggers", len(batch),
			"err", err,
		)
		return
	}
	q.logger.Info("activation.done",
		"worker", workerID,
		"trigger", batch[0].Kind,
		"triggers", len(batch),
	)
}

func (q *Queue) laneFor(orgID string, workerID orgchart.WorkerID) *workerLane {
	got, _ := q.lanes.LoadOrStore(laneKey{orgID: orgID, workerID: workerID}, &workerLane{})
	return got.(*workerLane)
}
