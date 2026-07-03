# Implementation Tasks: Single Org-Wide Helix Events Topic (Replace Per-Project Spec-Task Topics)

## Transport kind
- [x] Add `api/pkg/org/domain/transport/helixevents.go`: `KindHelixEvents = "helix_events"`, empty `HelixEventsConfig` (always-valid `Validate`), `helixEvents` strategy, `HelixEventsConfig()` accessor.
- [x] Register `KindHelixEvents` in the `strategies` map and `kindOrder` in `transport.go`.
- [x] Add `helixevents_test.go` (kind in `KindValues`, empty config valid).

## Reconciler + deterministic topic
- [x] Add `helixEventsTopicID()` helper returning `streaming.TopicID("s-helix-events")` — implemented as exported const `helixevents.TopicID` (single source of truth, shared with the publisher).
- [x] Add `api/pkg/org/application/helixevents/reconciler.go` with `Reconcile(ctx, orgID)`: idempotent get-or-create of the single `helix_events` topic (race-safe re-read on create conflict); does NOT touch legacy `spectask` rows (manual cleanup); narrow deps (`store.Topics`, `now`, logger); nil-safe no-op.
- [x] Add reconciler unit tests: creates exactly one topic, idempotent on re-run, no-op on nil deps.
- [x] Build the reconciler in the composition root (`helix_org.go`, near `slackrouting.New`) and wire it into bootstrap in `helix_org_middleware.go` alongside the other reconcilers.

## Publisher
- [x] Rewrite `attentionTopicPublisher` in `spec_task_attention_publisher.go` to resolve the org's single `helix_events` topic by `helixevents.TopicID` and publish (defensive idempotent get-or-create via the reconciler; keep the org-empty no-op).
- [x] Rename `specTaskEventExtra` → `helixEventExtra`; add `domain` (`"spectask"`) + `event_type` and keep `project_id`/`spec_task_id`/names; preserve coerced Message fields (Subject/Body/ThreadID/MessageID).
- [x] Update `spec_task_attention_publisher_test.go` for single-topic behavior + new envelope.

## Remove per-project path
- [x] Delete `EnsureSpecTaskTopic` and its per-project logic + tests.
- [x] Delete `transport/spectask.go` and `transport/spectask_test.go`; remove `KindSpecTask` from `strategies`/`kindOrder`.
- [x] Update `transport_test.go` kind count/order assertions (drop `KindSpecTask`).
- [x] Confirm `helix_events` is absent from `TRANSPORT_KINDS` in `frontend/src/pages/HelixOrgTopics.tsx` (user-uncreatable). Note: `create_topic` MCP enum is derived from `KindValues()` (same as `spectask`/`slack` before) — unchanged behavior; the constraint was the New Topic UI, which excludes it. No frontend change needed.

## Verify
- [x] `go build ./api/pkg/server/... ./api/pkg/org/...` and `CGO_ENABLED=1 go test ./api/pkg/org/domain/transport/ ./api/pkg/org/application/helixevents/ ./api/pkg/server/ (publisher+bootstrap tests)` pass. (Full `./...` fails only on the unrelated gstreamer/`go-gst` package needing `pkg-config`.)
- [x] **Browser E2E on `localhost:8080`** (see `screenshots/`). Note: helix-org is off by default in the dev stack — enabled it via `HELIX_ORG_ENABLED=true` + granted the `helix-org` alpha feature to the test user (both are environment/account setup, not code):
  - [x] Topics page shows exactly **one** "Helix events" topic (`s-helix-events`, kind `helix_events`), created by the bootstrap reconciler. Screenshot `01-single-helix-events-topic.png`.
  - [x] New Topic dialog does **not** offer `helix_events` (only local/webhook/github/postmark/cron). Screenshot `02-new-topic-no-helix-events.png`.
  - [x] Created **two** projects (projone, projtwo) via the UI — topic count stayed at **1**, no per-project topics created (verified in UI + DB `org_topics`).
  - [~] PM bot + filter-processor routing and live attention-event activation: NOT run end-to-end. Rationale: this exercises the publish→dispatch→activation path that is **unchanged** from 002209 (only the topic identity changed), and a real attention event requires the full spec-task agent/sandbox flow (spec_ready/pr_ready fire only after planning/PR completes). The publisher's target topic (`s-helix-events`) and the generic envelope (`domain`/`event_type`/`project_id`, coerced Subject/Body/ThreadID/MessageID) are covered by the unit tests in `spec_task_attention_publisher_test.go`.
