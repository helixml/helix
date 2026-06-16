# Implementation Tasks: Process Worker Activation Triggers One at a Time

- [x] In `api/pkg/org/domain/activation/queue.go`, change `Queue.run()` to drain a single trigger per iteration (`pending[0]`, then `pending = pending[1:]`) and call `activate` with a one-element `[]Trigger` slice instead of the whole pending list.
- [x] Update the doc comments on `Queue`, `workerLane`, and `run()` in `queue.go` to describe sequential one-at-a-time drain instead of coalescing.
- [x] Update the package doc and `Dispatcher` doc comment in `api/pkg/org/application/dispatch/dispatcher.go` to remove the "coalesced batch / collapses webhook cascades" wording and describe one-at-a-time delivery.
- [~] Rewrite `TestQueueCoalescesBurstIntoOneBatch` in `api/pkg/org/domain/activation/queue_test.go` as `TestQueueDrainsTriggersOneAtATime`: hold the first activation, enqueue a burst, then assert each Spawner call gets exactly one trigger and triggers arrive in FIFO order.
- [ ] Run `go test ./api/pkg/org/domain/activation/... ./api/pkg/org/application/dispatch/...` and confirm all queue/dispatcher tests pass (serialization, parallel-workers, and cross-org isolation invariants still hold).
