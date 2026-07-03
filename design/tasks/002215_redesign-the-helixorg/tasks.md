# Implementation Tasks: Single Org-Wide Helix Events Topic (Replace Per-Project Spec-Task Topics)

## Transport kind
- [ ] Add `api/pkg/org/domain/transport/helixevents.go`: `KindHelixEvents = "helix_events"`, empty `HelixEventsConfig` (always-valid `Validate`), `helixEvents` strategy, `HelixEventsConfig()` accessor.
- [ ] Register `KindHelixEvents` in the `strategies` map and `kindOrder` in `transport.go`.
- [ ] Add `helixevents_test.go` (kind in `KindValues`, empty config valid).

## Reconciler + deterministic topic
- [ ] Add `helixEventsTopicID()` helper returning `streaming.TopicID("s-helix-events")` (single source of truth, shared with the publisher).
- [ ] Add `api/pkg/org/application/helixevents/reconciler.go` with `Reconcile(ctx, orgID)`: idempotent get-or-create of the single `helix_events` topic (race-safe re-read on create conflict) + delete legacy `spectask` topics; narrow deps (`store.Topics`, `now`, logger); nil-safe no-op.
- [ ] Add reconciler unit tests: creates exactly one topic, idempotent on re-run, deletes legacy `spectask` rows, no-op on nil deps.
- [ ] Build the reconciler in the composition root (`helix_org.go`, near `slackrouting.New`) and wire it into bootstrap in `helix_org_middleware.go` alongside the other reconcilers.

## Publisher
- [ ] Rewrite `attentionTopicPublisher` in `spec_task_attention_publisher.go` to resolve the org's single `helix_events` topic by `helixEventsTopicID()` and publish (defensive idempotent get-or-create; keep the org-empty no-op).
- [ ] Rename `specTaskEventExtra` → `helixEventExtra`; add `domain` (`"spectask"`) + `event_type` and keep `project_id`/`spec_task_id`/names; preserve coerced Message fields (Subject/Body/ThreadID/MessageID).
- [ ] Update `spec_task_attention_publisher_test.go` for single-topic behavior + new envelope.

## Remove per-project path
- [ ] Delete `EnsureSpecTaskTopic` and its per-project logic + tests.
- [ ] Delete `transport/spectask.go` and `transport/spectask_test.go`; remove `KindSpecTask` from `strategies`/`kindOrder`.
- [ ] Update `transport_test.go` kind count/order assertions (drop `KindSpecTask`).
- [ ] Confirm `helix_events` is absent from `TRANSPORT_KINDS` in `frontend/src/pages/HelixOrgTopics.tsx` (user-uncreatable) and not accepted by `create_topic` for user callers.

## Docs + prompt
- [ ] Update `api/pkg/org/application/prompts/templates/pm_bot.md`: subscribe to the single "Helix events" topic and filter by `project_id`/`event_type`/`domain`; remove per-project "Spec tasks: <projectId>" language.
- [ ] Update 002209 `design.md`/`requirements.md` references from per-project `KindSpecTask` topics to the single Helix events topic + filter routing.

## Verify
- [ ] `go build ./...` and `go test ./api/pkg/org/... ./api/pkg/server/...` pass.
- [ ] E2E on inner Helix (`localhost:8080`): exactly one "Helix events" topic per org; PM bot wired via a filter processor on `project_id` is activated for the right project across two projects; no per-project topics created.
