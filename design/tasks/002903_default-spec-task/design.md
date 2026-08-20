# Design: Raise Default Spec-Task Sandbox to 8 vCPU / 16 GB

## Approach

Two constants change in Go; the frontend's four-way-duplicated copy of the same default
is collapsed into one shared module before the value is changed there. The value itself
is a two-line edit — the de-duplication is the part that stops this recurring.

## Go: change two constants, verify nothing else resolves a default

`api/pkg/types/simple_spec_task.go:86-87`:

```go
const (
	DefaultSpecTaskSandboxVCPUs    = 8     // was 4
	DefaultSpecTaskSandboxMemoryMB = 16384 // was 8192
)
```

Everything server-side that resolves a default already routes through
`DefaultSpecTaskSandboxResources()` / `EffectiveSpecTaskSandboxResources()`. Confirmed by
grep — the only callers are:

| Site | Role |
|---|---|
| `api/pkg/services/spec_driven_task_service.go:163` | materializes the default at task creation |
| `api/pkg/org/infrastructure/runtime/helix/spectasks.go:194` | same, via the org MCP create path |
| `api/pkg/server/spec_task_execution_config_handlers.go:130` | rollback target on a failed resize |
| `api/pkg/external-agent/task_overrides_test.go:84-85` | references the constants symbolically |

No other Go logic needs editing. **Re-run the grep after the change rather than trusting
this table** — it is a snapshot, and a new caller could have landed.

Deliberately unchanged: `api/pkg/sandbox/controller_provision.go:57` and
`api/pkg/cli/sandbox/sandbox.go:252` enumerate all three presets and remain correct.
`SpecTaskSandboxPresetForVCPUs` maps vCPUs → memory for the whole table and is
default-agnostic.

## Frontend: extract, then change

New `frontend/src/constants/sandboxPresets.ts`:

```ts
export interface SandboxPreset {
  vcpus: number
  memory_mb: number
  label: string        // SpecTaskExecutionControls renders this
  description: string  // both components render this
}

export const SANDBOX_PRESETS: SandboxPreset[] = [
  { vcpus: 1, memory_mb: 2048,  label: '1 CPU', description: '2 GB RAM' },
  { vcpus: 4, memory_mb: 8192,  label: '4 CPU', description: '8 GB RAM' },
  { vcpus: 8, memory_mb: 16384, label: '8 CPU', description: '16 GB RAM' },
]

export const DEFAULT_SANDBOX_PRESET = SANDBOX_PRESETS[2]
```

Keeping **both** `label` and `description` matters: `SpecTaskExecutionControls` entries
carry both, `CodeAgentExecutionControls` entries carry only `description`. Dropping either
silently blanks one component's menu.

`DEFAULT_SANDBOX_PRESET` moves from index `[1]` to `[2]`. Both components already derive
the "· Default" marker from `DEFAULT_SANDBOX_PRESET.vcpus`, so the marker follows for free.

Call sites, all switching to the shared module:

| File | What changes |
|---|---|
| `components/tasks/SpecTaskExecutionControls.tsx:56-62` | delete local table + `DEFAULT_SANDBOX_PRESET`, import instead. `selectSandbox`'s `typeof SANDBOX_PRESETS[number]` param type becomes `SandboxPreset`. |
| `components/agent/CodeAgentExecutionControls.tsx:49-55` | same |
| `pages/Home.tsx:133-134, 164` | two `{ vcpus: 4, memory_mb: 8192 }` literals → derive from `DEFAULT_SANDBOX_PRESET` |
| `components/tasks/NewSpecTaskForm.tsx:149-150, 335-337` | useState initial value, and the `project?.default_sandbox_resource_overrides?.vcpus \|\| 4` / `?.memory_mb \|\| 8192` fallbacks |
| `components/session/ProjectChatItemTooltip.tsx:38-39` | **not in the original brief** — same `\|\| 4` / `\|\| 8192` pattern |

Home.tsx and NewSpecTaskForm store `{vcpus, memory_mb}` API shapes, not presets, so spread
only the two numeric fields:
`{ vcpus: DEFAULT_SANDBOX_PRESET.vcpus, memory_mb: DEFAULT_SANDBOX_PRESET.memory_mb }`.
Pushing a `label`/`description` into an API payload would be a behaviour change.

### Decision: `ProjectChatItemTooltip` follows the shared default

This tooltip describes tasks that already exist. A nil override there means the task
predates `1eff4e801` and genuinely ran uncapped, so no preset label is strictly accurate.
Pointing it at `DEFAULT_SANDBOX_PRESET` keeps the UI consistent with how the server
resolves nil *today*, which is the property a reader of the tooltip actually needs. The
alternative — freezing the literals with an explanatory comment — preserves history at the
cost of reintroducing exactly the drift US-3 removes. Flagged as Open Question 1.

## Docs / generated artifacts

`api/pkg/types/project.go:246` (**not** `types.go` as the brief stated) carries:

> Nil values from legacy projects resolve to the standard 4 vCPU / 8 GB preset.

Update the prose, then run `./stack update_openapi` (target defined at `stack:1536`). That
comment is baked into seven generated files — `api/pkg/server/docs.go`,
`api/pkg/server/swagger.json`, `api/pkg/server/swagger.yaml`, `swagger.json`,
`openapi.json`, `frontend/swagger/swagger.yaml`, `frontend/src/api/api.ts:5028`. Commit the
regenerated output; do not hand-edit it. Re-grep for `standard 4 vCPU` afterwards to
confirm every copy moved.

## Test impact

| Test | Why it moves |
|---|---|
| `api/pkg/services/spec_driven_task_service_test.go` (~129) | asserts the materialized default |
| `api/pkg/org/infrastructure/runtime/helix/spectasks_sandbox_test.go` | see below |
| `frontend/.../SpecTaskExecutionControls.test.tsx` | several cases pass `{vcpus: 4, memory_mb: 8192}` as the current value |
| `frontend/.../ProjectChatItemTooltip.test.tsx:99` | asserts `'4 vCPU · 8 GB RAM'` for a nil-override task |
| `api/pkg/external-agent/task_overrides_test.go` | references the constants symbolically — should keep passing; **confirm, don't assume** |

**`spectasks_sandbox_test.go` is the subtle one.** It uses 8/16384 as the explicit case and
`{VCPUs: 4, MemoryMB: 8192}` at line ~79 as the contrasting *project* default, with
`TestSpecTasks_CreateFallsBackToProjectSandboxDefaults` asserting `VCPUs != 4` fails. Once
8/16384 is the global default, that test passes whether or not project defaults are honoured
at all — vacuous. Re-point the contrast case to a different valid preset (1/2048 is the
natural choice: still `ValidPreset()`, and distinct from both the global default and the
explicit case) so it still proves project defaults win.

`SpecTaskExecutionControls.test.tsx:219` clicks the 8 vCPU item and asserts a resize call
to 8/16384. `selectSandbox` (line 202) does **not** short-circuit on an unchanged value, and
the test supplies `{vcpus: 4}` explicitly rather than relying on the default, so it should
still pass — verify by running it rather than pre-emptively rewriting it. If it needs a
contrast that is genuinely not the default, switch the click target to the 1 vCPU row.

`ProjectTaskDefaults.test.tsx` uses 4/8192 as an explicit *project* value; still valid,
no change expected.

## Verification

1. `cd api && go build ./...`, then the affected packages' tests.
2. `cd frontend && yarn build`, plus the touched vitest files.
3. Grep sweep: no `8192` sandbox-default literal left in `frontend/src` outside
   `sandboxPresets.ts` and fixtures; no `standard 4 vCPU` left anywhere.
4. **End-to-end in the inner Helix** — create a spec task, confirm the selector shows
   8 vCPU marked "· Default", start it, then:

```bash
docker inspect <container> --format '{{.HostConfig.Memory}}'   # want 17179869184
docker inspect <container> --format '{{.HostConfig.NanoCpus}}' # want 8000000000
```

A passing unit test is not evidence the container was sized. Check the cgroup.

## PR description must state

- Tasks created since 2026-08-10 (`1eff4e801`) have `{"vcpus": 4, "memory_mb": 8192}`
  materialized in the row and will **not** pick up the new default. Only new tasks benefit.
  No migration in this PR by design — an explicitly stored value is indistinguishable from
  a deliberate user choice, so rewriting rows is a separate decision.
- Project-level `default_sandbox_resource_overrides`, where set, still wins over the global
  default. Unchanged.

## Notes for future agents

- The three presets are enforced in **three** independent places: `ValidPreset()` in
  `api/pkg/types/simple_spec_task.go`, the handler error strings, and
  `api/pkg/sandbox/controller_provision.go:57`. Adding a fourth size means touching all
  three plus `SpecTaskSandboxPresetForVCPUs`.
- vCPU is the primary key of a preset throughout this codebase — memory is always derived
  from it (`SpecTaskSandboxPresetForVCPUs`, the `preset.vcpus === ...` comparisons in both
  menus). Never treat memory as independently selectable.
- OpenAPI output is committed, generated from Go doc comments, and fans out to seven files.
  Always `./stack update_openapi` rather than editing any of them.
