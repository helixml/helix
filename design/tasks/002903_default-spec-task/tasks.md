# Implementation Tasks: Raise Default Spec-Task Sandbox to 8 vCPU / 16 GB

## Go

- [ ] Set `DefaultSpecTaskSandboxVCPUs = 8` and `DefaultSpecTaskSandboxMemoryMB = 16384` in `api/pkg/types/simple_spec_task.go:86-87`
- [ ] Re-grep `DefaultSpecTaskSandbox` / `EffectiveSpecTaskSandboxResources` to confirm no other Go site resolves a default independently
- [ ] Leave the three-preset validation error strings alone (`spec_driven_task_handlers.go:170`, `spec_task_execution_config_handlers.go:110`, `controller_provision.go:57`)

## Frontend — extract the shared module first

- [ ] Create `frontend/src/constants/sandboxPresets.ts` exporting `SandboxPreset`, `SANDBOX_PRESETS` (1/2048, 4/8192, 8/16384, each with `label` **and** `description`) and `DEFAULT_SANDBOX_PRESET = SANDBOX_PRESETS[2]`
- [ ] Point `components/tasks/SpecTaskExecutionControls.tsx` at the shared module; delete its local table and retype `selectSandbox`'s param as `SandboxPreset`
- [ ] Point `components/agent/CodeAgentExecutionControls.tsx` at the shared module; delete its local table
- [ ] Replace both `{ vcpus: 4, memory_mb: 8192 }` literals in `pages/Home.tsx` (~133, ~164) with the shared default's two numeric fields
- [ ] Replace the useState initial value (~149) and the `|| 4` / `|| 8192` project fallbacks (~335-337) in `components/tasks/NewSpecTaskForm.tsx`
- [ ] Replace the `|| 4` / `|| 8192` fallbacks in `components/session/ProjectChatItemTooltip.tsx:38-39` (site not listed in the brief — see Open Question 1)
- [ ] Grep `frontend/src` to confirm no sandbox-default `4` / `8192` literal survives outside `sandboxPresets.ts` and test fixtures

## Docs

- [ ] Update the "standard 4 vCPU / 8 GB preset" comment in `api/pkg/types/project.go:246`
- [ ] Run `./stack update_openapi` and commit all seven regenerated artifacts
- [ ] Grep for `standard 4 vCPU` to confirm every generated copy moved

## Tests

- [ ] Update the materialized-default assertion in `api/pkg/services/spec_driven_task_service_test.go` (~129)
- [ ] Re-point the contrasting project default in `api/pkg/org/infrastructure/runtime/helix/spectasks_sandbox_test.go` (~79) from 4/8192 to 1/2048 so `TestSpecTasks_CreateFallsBackToProjectSandboxDefaults` still proves project defaults beat the global default
- [ ] Run `api/pkg/external-agent/task_overrides_test.go` to confirm the symbolic assertions still pass
- [ ] Update `frontend/src/components/tasks/SpecTaskExecutionControls.test.tsx` cases that assume the 4/8192 default; run test at ~219 before rewriting it — it may still pass unchanged
- [ ] Update `frontend/src/components/session/ProjectChatItemTooltip.test.tsx:99` to expect `'8 vCPU · 16 GB RAM'`

## Verification

- [ ] `cd api && go build ./...` and run the affected Go packages' tests
- [ ] `cd frontend && yarn build` and run the touched vitest files
- [ ] In the inner Helix: create a spec task and confirm the selector preselects 8 vCPU marked "· Default"
- [ ] Start it and confirm the real cgroup: `docker inspect <container> --format '{{.HostConfig.Memory}}'` is `17179869184` and `{{.HostConfig.NanoCpus}}` is `8000000000`

## PR

- [ ] State in the PR body that tasks created since `1eff4e801` (2026-08-10) have 4/8192 materialized in the row and will not pick up the new default; no migration by design
- [ ] State that project-level `default_sandbox_resource_overrides` still wins over the global default, unchanged
