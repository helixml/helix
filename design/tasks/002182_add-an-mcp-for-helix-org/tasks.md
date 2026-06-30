# Implementation Tasks: Add Helix-Org MCP Tools for Workers to Manage Spec Tasks

- [ ] Extract shared authoring logic into `api/pkg/spectask/service.go`: create/list/get, `generateDesignDocPath`, task-number assignment, project-ownership check (operating on `store.Store` + `types.SpecTask`).
- [ ] Refactor the Optimus skill create/list/get tools in `api/pkg/agent/skill/project/` to delegate to the shared authoring service (behaviour-preserving); confirm existing tests in that package still pass.
- [ ] Decide and document the approver identity for Worker approvals (hiring user vs task creator) and how GitHub OAuth / commit credentials are sourced, since the human approve handlers validate `ValidateUserGitHubOAuth`.
- [ ] Add the `SpecTasks` port to `api/pkg/org/infrastructure/runtime/runtime.go` with reviewer-shaped verbs (`Create`, `List`, `Get`, `StartPlanning`, `ReviewSpec`, `ApproveSpec`, `RequestChanges`, `ApprovePullRequest`), input/view structs, `NoopSpecTasks`, and `ErrSpecTasksUnsupported`.
- [ ] Implement `runtimehelix.SpecTasks` in `api/pkg/org/infrastructure/runtime/helix/spectasks.go`: resolve worker→projectID via `LoadState`, enforce task ownership, and delegate each verb to the shared authoring service, `services.SpecDrivenTaskService` (StartSpecGeneration/ApproveSpecs), the `submitDesignReview` request_changes path, and the `approveImplementation` logic.
- [ ] Add `SpecTasks runtime.SpecTasks` to `mcptools.Deps`; default it to `NoopSpecTasks{}` in `Config.Build()` / `DefaultDeps`.
- [ ] Create the org MCP tools in `api/pkg/org/interfaces/mcptools/` (`create_spectask`, `list_spectasks`, `get_spectask`, `start_spectask_planning`, `review_spectask_spec`, `approve_spectask_spec`, `request_spectask_changes`, `approve_spectask_pr`) implementing `tool.Tool` and delegating to `deps.SpecTasks`, scoped to `inv.Caller`.
- [ ] Register the tools in `RegisterBuiltins`; keep the mutating/approving tools out of `BaseReadTools`.
- [ ] Wire `runtimehelix.NewSpecTasks(...)` into `deps.SpecTasks` in `helix_org.go` before `RegisterBuiltins`.
- [ ] Add unit tests for each new tool (fake `SpecTasks` port) following `configure_worker_project_test.go`.
- [ ] Add a unit test for `runtimehelix.SpecTasks` following `project_config_test.go` (no-project error, ownership enforcement, each verb's status transition, created-task shape parity).
- [ ] Run `go build ./...` and the org + skill package test suites; verify a Worker whose Role lists the tools can create, plan, review, approve, request changes, and approve a PR for a task in its own project.
