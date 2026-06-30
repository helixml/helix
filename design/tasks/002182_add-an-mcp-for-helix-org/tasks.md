# Implementation Tasks: Add Helix-Org MCP Tools for Workers to Manage Spec Tasks

**TDD is mandatory.** Every step below is a red→green cycle: write the failing
test(s) first, watch them fail (red), then write the minimum code to pass
(green), then refactor under green. No production code — new or changed — lands
without a preceding red test. Existing helix/Optimus tests are never edited;
they must stay green throughout because their code is not touched.

## Pre-work (no code)

- [x] Confirm (read-only) the org port can reuse the canonical `services` layer as-is: `SpecDrivenTaskService.CreateTaskFromPrompt`, `StartSpecGeneration`/`StartJustDoItMode`, `ApproveSpecs`, `services.GenerateDesignDocPath`, `store.Store` reads. No edits to `api/pkg/agent/skill/project/*` or existing `services`/`store`. **Decision: in-proc impl depends on a narrow `SpecTaskService` interface (ProjectService precedent), server wires the adapter.**
- [x] Decide and document the approver identity for Worker approvals. **Decision: use `WorkerState.HiringUserID` (from `LoadState`), fall back to task creator. No new state needed.**

## Layer 1 — Infrastructure port (`runtime.SpecTasks`)

- [x] RED: tests asserting `NoopSpecTasks` returns `ErrSpecTasksUnsupported` for every verb.
- [x] GREEN: add the `SpecTasks` port + reviewer-shaped verbs (`Create`, `List`, `Get`, `StartPlanning`, `ReviewSpec`, `ApproveSpec`, `RequestChanges`, `CreatePullRequests`), input/view structs, `NoopSpecTasks`, `ErrSpecTasksUnsupported` to `runtime.go`.

## Layer 2 — In-proc impl (`runtimehelix.SpecTasks`)

- [x] RED: tests (following `project_config_test.go`) for no-project error, task-ownership enforcement, each verb's status transition, and created-task shape parity with the REST path — all failing against an empty impl.
- [x] GREEN: implement `api/pkg/org/infrastructure/runtime/helix/spectasks.go` via narrow `SpecTaskStore` (satisfied by `*helixstore.Store`) + `SpecTaskWorkflow` (`ApproveSpecs` + `EnsurePullRequests`), resolving worker→projectID via `LoadState`. **Note:** `RequestChanges` makes the `spec_revision` status transition (full design-review-comment thread is the REST path, deferred).

## Layer 3 — Application service (`spectasks.Service`)

- [x] RED: tests with a fake `runtime.SpecTasks` port covering caller→orgID/workerID extraction, project scoping, infra-error mapping, and view shaping.
- [x] GREEN: implement `api/pkg/org/application/spectasks/spectasks.go` depending only on the port.

## Layer 4 — Deps wiring

- [x] RED: test that `Config.Build()` constructs `SpecTasks` over a non-nil port and `DefaultDeps` defaults to `NoopSpecTasks{}` (no nil-interface panic).
- [x] GREEN: add `SpecTasks *spectasks.Service` to `mcptools.Deps` and build it in `Config.Build()`.

## Layer 5 — MCP tools

- [ ] RED: per-tool tests (fake application service, following `configure_worker_project_test.go`) for name, input schema, and happy/error `Invoke` — for each of `create_spectask`, `list_spectasks`, `get_spectask`, `start_spectask_planning`, `review_spectask_spec`, `approve_spectask_spec`, `request_spectask_changes`, `create_spectask_prs` (one PR per attached repo; result lists all PRs).
- [ ] GREEN: implement the tool files in `api/pkg/org/interfaces/mcptools/`, delegating to `deps.SpecTasks`, scoped to `inv.Caller`.
- [ ] RED: test that all new tools are registered by `RegisterBuiltins` and that the mutating/approving ones are absent from `BaseReadTools`.
- [ ] GREEN: register the tools in `RegisterBuiltins`.

## Layer 6 — Composition

- [ ] RED: an integration-level test that a Worker whose Role lists the tools can drive a task end-to-end (create → plan → review → approve → request changes → create PRs) in its own project, including a multi-repo project opening one PR per repo.
- [ ] GREEN: wire `helix_org.go` — build `runtimehelix.NewSpecTasks(...)`, inject into `spectasks.New(...)`, set `deps.SpecTasks` before `RegisterBuiltins`.

## Layer 7 — Eventing: transport + AttentionService trigger

- [ ] RED: tests for `transport.KindSpecTask` (strategy lookup + `kindOrder` membership + project-scoped config parse).
- [ ] GREEN: add `transport.KindSpecTask = "spectask"` and its strategy/`kindOrder`/config to `api/pkg/org/domain/transport/`.
- [ ] RED: a test on `services.AttentionService` (fake `Publisher`) asserting `EmitEvent` publishes a new `AttentionEvent`, skips publish on the idempotency-dedup path, and is nil-safe when no publisher is wired. (This is the only change to existing code — gated behind a red test first.)
- [ ] GREEN: add the optional nil-guarded `Publisher` sink to `AttentionService`, published after the dedup check beside `notifySlack`.
- [ ] RED: tests for the spectask transport infra — `AttentionEvent` → `streaming.Message` mapping and project→`KindSpecTask` topic resolution (auto-create like the Slack workspace topic).
- [ ] GREEN: implement `api/pkg/org/infrastructure/transports/spectask/`.
- [ ] RED: an integration test that an emitted `AttentionEvent` produces exactly one activation per subscribed Worker (and none for non-subscribers), with a payload (task id, event type, new status) sufficient to act.
- [ ] GREEN: wire the `Publishing`-backed publisher into `AttentionService` in `server.go` (~line 608).

## Final verification

- [ ] Run `go build ./...` and the full test suite. Confirm the **unmodified** Optimus skill (`api/pkg/agent/skill/project/`) and existing `services`/`store` tests still pass, all new red tests are now green, and no production code landed without a preceding test.
