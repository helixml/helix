# Add helix-org MCP tools for Workers to manage spec tasks

## Summary

Gives helix-org **Workers** an MCP surface to manage the spec tasks in their
own Helix project — create, plan, review, approve, request changes, and open
PRs — plus an event trigger so a Worker is woken when a spec task changes
state, reusing the existing Helix UI notification system.

It reuses the **canonical `services` layer** the REST UI already drives. The
existing Optimus skill (`api/pkg/agent/skill/project/`) and the existing
`services`/`store` behaviour are **untouched** — the only additive edit to
existing code is an optional, nil-guarded event sink on `AttentionService`.

Built strictly test-first (red→green) at every layer.

## What a Worker can do (new MCP tools)

Named for the human-reviewer action they perform; granted per-Role (kept out
of `BaseReadTools`). Each is scoped to the calling Worker's own project — no
project ID is accepted from the LLM.

- `create_spectask`, `list_spectasks`, `get_spectask`
- `start_spectask_planning`
- `review_spectask_spec`, `approve_spectask_spec`, `request_spectask_changes`
- `create_spectask_prs` — opens one PR per repo attached to the project (does
  **not** merge/approve on GitHub)

## Architecture (mirrors the `ProjectConfig` precedent)

```
interfaces/mcptools (8 tools)
   → application/spectasks.Service          (front-of-house; caller scoping)
   → runtime.SpecTasks port                 (infrastructure interface)
   → runtimehelix.SpecTasks                 (reuses SpecDrivenTaskService + store)
```

- Worker→project resolved server-side from `WorkerRuntimeState` (`LoadState`);
  approvals use the Worker's `HiringUserID`.
- Approve/PR verbs delegate to `SpecDrivenTaskService.ApproveSpecs` and the
  server's `ensurePullRequestsForAllRepos` — no status-machine reimplementation.

## Eventing (the Worker's "ears")

- New inbound transport `transport.KindSpecTask` (project-scoped topic).
- `AttentionService` gains an optional `AttentionEventSink`; each emitted
  `AttentionEvent` (the same curated set that drives the Helix UI
  notifications: `specs_pushed`, `pr_ready`, `spec_failed`,
  `implementation_failed`, `ci_passed`, `ci_failed`,
  `agent_interaction_completed`) is also published onto the project's
  spec-task topic, so subscribed Workers are triggered via the normal dispatch
  path. Skipped on the idempotency-dedup path; nil-safe when unwired.

## Changes

- `api/pkg/org/infrastructure/runtime/runtime.go` — `SpecTasks` port, DTOs, `NoopSpecTasks`.
- `api/pkg/org/infrastructure/runtime/helix/spectasks.go` — in-proc impl over narrow ports.
- `api/pkg/org/application/spectasks/` — application service.
- `api/pkg/org/interfaces/mcptools/spec_tasks.go` + `builtins.go` — 8 tools, Deps wiring, registration.
- `api/pkg/org/domain/transport/spectask.go` — `KindSpecTask`.
- `api/pkg/services/attention_service.go` — optional `AttentionEventSink` (additive).
- `api/pkg/server/spec_task_attention_publisher.go`, `spec_tasks_org_wiring.go`, `helix_org.go` — bridge + composition.

## Testing

Unit/integration tests at every layer (Noop port, in-proc impl, application
service, each tool, registration, transport kind, AttentionService sink,
attention→topic publisher, compile-time wiring assertions). All
`api/pkg/org/...` packages, the unmodified Optimus skill, the `services`
AttentionService tests, and the `server` publisher tests pass. Not run:
inner-Helix browser end-to-end and gstreamer/Postgres-only packages.

## Notes / deviations

- Eight small tools live in one file for cohesion.
- The attention→topic bridge lives in `server` (not org infra) because org
  transports import only the org domain; this is the helix↔org composition seam.
- `request_spectask_changes` makes the `spec_revision` transition; the full
  design-review-comment thread stays the REST/UI path.
- Follow-on (noted, out of scope): unify the two existing spec-task
  notification paths (`SubscribeForTasks` Slack updates vs `AttentionService`).
