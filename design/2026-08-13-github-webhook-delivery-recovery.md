# GitHub webhook delivery recovery

Date: 2026-08-13
Status: implemented in an uncommitted review worktree

## Incident

GitHub recorded an HTTP 521 for the affected delivery while the Meta API was
restarting. The request never reached Helix. This is the proven root cause for
the incident affecting https://github.com/helixml/helix/pull/3020.

## Evidence correction

An absent row in `org_events` does not by itself prove that GitHub never
delivered the request. The GitHub handlers intentionally return 204 without
appending an event for valid no-op deliveries, including a repository or event
that has no matching Topic. They also return 204 when a deterministic duplicate
event is already present. Delivery evidence must therefore come from GitHub's
hook-delivery history and the Helix handler logs, not from `org_events` alone.

## Structural fix

The API starts a GitHub delivery reconciler once at startup and runs it every
5 minutes. For each configured GitHub Topic it polls the hook's recent
deliveries, groups attempts by GitHub delivery GUID, and requests redelivery of
the latest failed attempt for each GUID. Each scan follows GitHub's cursors
until it reaches the prior delivery checkpoint, the 72-hour retention boundary,
or the end. The checkpoint advances after each successfully processed attempt,
so a later redelivery error leaves that delivery retryable without resubmitting
earlier accepted requests. Redelivery uses the existing signed webhook path.

Event IDs are deterministic from `(topic ID, X-GitHub-Delivery)`. A replay
therefore conflicts with the existing event instead of appending or dispatching
the same delivery twice.

## Explicit ceilings

- Retain and consider deliveries for 72 hours (3 days).
- Delivery checkpoints are in process memory and reset when the API restarts.
- A hook with more than 72 hours of retained deliveries is bounded by the
  retention window rather than by a fixed page count.

## Verification status

Focused tests passed:

    go test ./pkg/github ./pkg/org/infrastructure/transports/github
    go test -race ./pkg/github ./pkg/org/infrastructure/transports/github
    go build ./pkg/server/ ./pkg/store/ ./pkg/types/

The tests cover GitHub delivery listing/redelivery, multi-page checkpoint
recovery, partial-batch retry progress, and duplicate delivery idempotency. A
live GitHub redelivery, the Meta restart, and production deployment behavior
were not verified here.
