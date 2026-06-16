# Process worker activation triggers one at a time

## Summary

In production, busy Streams overwhelm an AI Worker's context window. A real
example: a GitHub Stream emits an event for every commit, CI action, and issue.
While a Worker was mid-activation, the activation Queue **coalesced** the entire
backlog into a single follow-up activation — handing the Worker's next run dozens
of triggers at once, a huge prompt that exhausted the context budget.

This change stops coalescing. The per-Worker queue now drains **one trigger per
activation**, in arrival (FIFO) order. Context per activation is bounded
regardless of how busy the Stream is; the backlog is worked off sequentially. The
existing "at most one in-flight activation per Worker" and per-(org, worker) lane
isolation invariants are unchanged.

Trade-off (intended): a busy Stream now produces one activation per event instead
of one coalesced activation — more sequential activations in exchange for bounded
context each. Per-worker serialization still prevents runaway concurrency.

## Changes

- `api/pkg/org/domain/activation/queue.go` — `Queue.run()` drains a single
  trigger per iteration (`pending[0]`, then `pending = pending[1:]`) and spawns
  with a one-element slice instead of the whole pending list. Doc comments
  updated to describe one-at-a-time draining.
- `api/pkg/org/application/dispatch/dispatcher.go` — package and `Dispatcher` doc
  comments updated: removed the "coalesced batch / collapses webhook cascades"
  wording, describe one-at-a-time FIFO delivery.
- `api/pkg/org/domain/activation/queue_test.go` — `TestQueueCoalescesBurstIntoOneBatch`
  rewritten as `TestQueueDrainsTriggersOneAtATime` (each Spawner call gets exactly
  one trigger, FIFO). Refreshed a stale comment in `TestQueueSerializesPerWorker`.
- `api/pkg/org/application/dispatch/dispatcher_test.go` — dispatch-level
  `TestDispatchCoalescesEvents` rewritten as `TestDispatchDeliversEventsOneAtATime`
  (4 events → 4 activations, one each, FIFO).

## Testing

`go test ./api/pkg/org/domain/activation/... ./api/pkg/org/application/dispatch/...`
— all pass, including serialization, parallel-workers, and cross-org isolation.
