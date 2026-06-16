# Design: Process Worker Activation Triggers One at a Time

## Where the change lives

`api/pkg/org/domain/activation/queue.go` — specifically `Queue.run()`.

The Queue is the single place that turns bursts of arrivals into Spawner calls.
`Dispatcher` (`api/pkg/org/application/dispatch/dispatcher.go`) just delegates
`Enqueue` to it. So the entire behavioural change is contained in one method.

## Current behaviour

`run()` drains the **whole** pending slice per iteration:

```go
batch := lane.pending
lane.pending = nil
orgID := lane.orgID
lane.mu.Unlock()
q.activate(context.Background(), orgID, workerID, batch)
```

So all triggers that piled up while the previous `activate` was running are
handed to the Spawner as one coalesced `[]Trigger`.

## New behaviour

Drain a single trigger per iteration, leaving the rest in `pending` for the next
loop pass:

```go
trigger := lane.pending[0]
lane.pending = lane.pending[1:]
orgID := lane.orgID
lane.mu.Unlock()
q.activate(context.Background(), orgID, workerID, []activation.Trigger{trigger})
```

The runner loop already re-checks `pending` at the top of each iteration and
exits when empty, so the remaining triggers drain sequentially — one activation
each, in FIFO order — until the lane is empty. The "running" flag still ensures
only one activation per lane at a time.

### Key decision: keep the `[]Trigger` Spawner signature

`Spawn`/`runtime.Spawner` keeps taking `[]Trigger`; we simply always pass a
one-element slice. This is the minimal, lowest-risk change:

- `briefing.BuildPrompt` already special-cases `len(triggers) == 1` (no
  "N triggers queued" preamble) — single-trigger output is unchanged.
- `briefing.DescribeTriggers` already reuses `DescribeTrigger` verbatim for the
  single case — transcript markers unchanged.
- The helix Spawner, audit rows, and all downstream consumers are untouched.

The multi-trigger rendering branches in `briefing` become dead code but are left
in place — removing them is a separate, optional cleanup.

### Why not change the lane drain to a channel / one-at-a-time queue elsewhere?

The lane struct, `Enqueue`, lane keying, and serialization are all correct as-is.
The only thing wrong for our use case is the "take everything" drain. Changing
just that line keeps the proven concurrency invariants (per-worker serialization,
per-(org,worker) isolation) intact.

## Trade-off (accepted)

A busy Stream now produces one activation per event instead of one coalesced
activation. That means **more activations** (higher aggregate cost / more Spawner
invocations) in exchange for **bounded context per activation**. This is the
explicit goal: context blow-up is the production problem; per-activation
serialization already prevents runaway concurrency.

## Test impact

`api/pkg/org/domain/activation/queue_test.go`:

- `TestQueueCoalescesBurstIntoOneBatch` asserts the *opposite* of the new
  behaviour and must be rewritten as `TestQueueDrainsTriggersOneAtATime`: enqueue
  a burst while the first activation is held, then assert each Spawner call
  receives exactly one trigger and that all triggers arrive in order.
- `TestQueueSerializesPerWorker` — still valid (peak in-flight ≤ 1); unchanged.
- `TestQueueDifferentWorkersRunInParallel` — unchanged.
- `TestQueueNilSpawnerIsNoop` — unchanged.
- `TestQueueIsolatesSameWorkerIDAcrossOrgs` — unchanged.

## Docs / comments to update

- The package doc and `Dispatcher` doc comment in `dispatcher.go` describe
  "coalesced batch" / "collapses webhook cascades into a single follow-up
  activation". Update these to describe one-at-a-time sequential drain.
- The doc comments on `Queue`, `workerLane`, and `run()` in `queue.go` reference
  coalescing; update to match.
