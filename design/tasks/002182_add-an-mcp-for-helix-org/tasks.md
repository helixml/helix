# Implementation Tasks: Add Helix-Org MCP Tools for Workers to Manage Spec Tasks

- [ ] Extract shared spec-task logic into `api/pkg/spectask/service.go`: create/list/get/update/start, `generateDesignDocPath`, task-number assignment, status transitions, project-ownership check (operating on `store.Store` + `types.SpecTask`).
- [ ] Refactor the five Optimus skill tools in `api/pkg/agent/skill/project/` to delegate to the new shared service (behaviour-preserving); confirm existing tests in that package still pass.
- [ ] Add the `SpecTasks` port to `api/pkg/org/infrastructure/runtime/runtime.go` with `Create/List/Get/Update/Start` methods, input/view structs, `NoopSpecTasks`, and `ErrSpecTasksUnsupported`.
- [ ] Implement `runtimehelix.SpecTasks` in `api/pkg/org/infrastructure/runtime/helix/spectasks.go`: resolve worker→projectID via `LoadState`, enforce task ownership, delegate to the shared service.
- [ ] Add `SpecTasks runtime.SpecTasks` to `mcptools.Deps`; default it to `NoopSpecTasks{}` in `Config.Build()` / `DefaultDeps`.
- [ ] Create the five org MCP tools in `api/pkg/org/interfaces/mcptools/` (`create_spectask.go`, `list_spectasks.go`, `get_spectask.go`, `update_spectask.go`, `start_spectask.go`) implementing `tool.Tool` and delegating to `deps.SpecTasks`, scoped to `inv.Caller`.
- [ ] Register the five tools in `RegisterBuiltins`; keep mutating tools out of `BaseReadTools`.
- [ ] Wire `runtimehelix.NewSpecTasks(st, inProcClient)` into `deps.SpecTasks` in `helix_org.go` before `RegisterBuiltins`.
- [ ] Add unit tests for each new tool (fake `SpecTasks` port) following `configure_worker_project_test.go`.
- [ ] Add a unit test for `runtimehelix.SpecTasks` following `project_config_test.go` (no-project error, ownership enforcement, parity of created task shape).
- [ ] Run `go build ./...` and the org + skill package test suites; verify a Worker whose Role lists the tools can create/list/get/update/start a task in its own project.
