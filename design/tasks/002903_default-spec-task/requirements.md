# Requirements: Raise Default Spec-Task Sandbox to 8 vCPU / 16 GB

## Background

Spec-task desktop containers default to **4 vCPU / 8 GB**. That is below the real
steady-state footprint of a desktop task, so Chrome renderers are OOM-killed
continuously and the user sees "Aw, Snap!" every few minutes.

Measured on meta (node01), container `ubuntu-external-01m0cm7thsmwf3aydezsntg2rg`,
21 hours old:

```
memory.current 8181452800   memory.max 8589934592     (95.2%, pinned for hours)
memory.events: max 432991   oom 113   oom_kill 37     swap 0
```

Every OOM victim is a Chrome renderer, because Chrome tags renderers with
`oom_score_adj=300` so they are sacrificed first. Chrome is the victim, not the cause.
The 8 GB is consumed by `zed` (2.86 GB), `chrome-devtools-mcp` (2.54 GB), the desktop
stack (gnome-shell / Xwayland / mutter / desktop-bridge / 2x ghostty, ~1.5 GB), the
`claude` agent SDK (0.44 GB), and a duplicate `chrome-devtools-mcp` plus watchdogs
(~0.57 GB) — roughly 7.9 GB *before* Chrome's own browser/GPU/network processes
(~1 GB) and any renderer (0.1–0.6 GB each). The host has no swap, so the cap is hard.

Desktops on the same box still running **uncapped** (`limit=0`, created before the cap
was applied) sit at 6.3 / 8.7 / 9.3 / 15.0 / 24.8 GiB with `oom_kill 0`. Both containers
carrying the 8 GiB cap have OOM kills. The host has ~257 GB free.

Regression window: commit `1eff4e801` (2026-08-10, "fix(specs): apply default sandbox
resources"), which changed a nil `SandboxResourceOverrides` from *uncapped* to *the
4 vCPU / 8 GB default*.

## Hard constraint

Memory is **not** independently selectable. `SandboxResourceOverrides.ValidPreset()`
accepts exactly three pairs — 1/2048, 4/8192, 8/16384 — and
`SpecDrivenTaskService.CreateTaskFromPrompt` rejects anything else with
`invalid sandbox resource preset`. "Default to 16 GB" therefore necessarily means
defaulting to the 8 vCPU preset. A 4 vCPU / 16384 pairing must not be introduced.

## User Stories

### US-1 — New spec tasks get enough memory
**As a** user running a spec task in a desktop sandbox
**I want** the default container to be 8 vCPU / 16 GB
**So that** Chrome renderers stop being OOM-killed mid-session.

Acceptance criteria:
- [ ] `types.DefaultSpecTaskSandboxVCPUs == 8` and `types.DefaultSpecTaskSandboxMemoryMB == 16384`.
- [ ] A spec task created with no explicit override and no project default materializes
      `{"vcpus": 8, "memory_mb": 16384}`.
- [ ] The resulting container has `HostConfig.Memory == 17179869184` and
      `HostConfig.NanoCpus == 8000000000`, verified with `docker inspect` — not merely
      asserted by a unit test.
- [ ] A project-level `default_sandbox_resource_overrides`, where set, still wins over
      the global default. Behaviour unchanged.

### US-2 — The UI shows the new default
**As a** user creating a spec task
**I want** the size selector to preselect 8 vCPU and mark it "· Default"
**So that** the UI agrees with what the server will actually provision.

Acceptance criteria:
- [ ] `SpecTaskExecutionControls` and `CodeAgentExecutionControls` both preselect 8 vCPU
      and render "· Default" against the 8 vCPU row.
- [ ] `Home.tsx` and `NewSpecTaskForm.tsx` initialise their sandbox state to the new
      default, and `NewSpecTaskForm`'s project-default fallbacks resolve to it.
- [ ] The preset *table* is unchanged: the three offered sizes remain 1/2, 4/8, 8/16.

### US-3 — The default lives in one place
**As a** maintainer
**I want** a single shared source for the preset table and the default
**So that** the four-way drift that produced this bug cannot recur.

Acceptance criteria:
- [ ] One module (`frontend/src/constants/sandboxPresets.ts`) exports `SANDBOX_PRESETS`
      and `DEFAULT_SANDBOX_PRESET`.
- [ ] Entries keep **both** `label` and `description` so neither consuming component
      loses what it renders.
- [ ] No `4` / `8192` sandbox-default literal remains in `frontend/src` outside that
      module and test fixtures. Verified by grep, not by inspection.

### US-4 — Docs match reality
Acceptance criteria:
- [ ] The `Nil values from legacy projects resolve to the standard 4 vCPU / 8 GB preset`
      comment is updated, and the generated OpenAPI artifacts are regenerated with
      `./stack update_openapi`.
- [ ] The three-preset validation error strings in `spec_driven_task_handlers.go:170`
      and `spec_task_execution_config_handlers.go:110` are **not** touched — they
      enumerate all three sizes and stay correct.

## Corrections to the brief (verified against the tree)

1. **The swagger comment is in `api/pkg/types/project.go:246`, not `api/pkg/types/types.go`.**
   It is regenerated into seven artifacts, not one: `api/pkg/server/docs.go`,
   `api/pkg/server/swagger.json`, `api/pkg/server/swagger.yaml`, `swagger.json`,
   `openapi.json`, `frontend/swagger/swagger.yaml`, and `frontend/src/api/api.ts:5028`.

2. **There is a fifth hardcoded frontend site the brief did not list:**
   `frontend/src/components/session/ProjectChatItemTooltip.tsx:38-39` uses
   `?.vcpus || 4` and `?.memory_mb || 8192` to describe a task's compute in the project
   chat tooltip. Its test at `ProjectChatItemTooltip.test.tsx:99` asserts
   `'4 vCPU · 8 GB RAM'` for a task with no stored override. This is exactly the class of
   drift US-3 exists to prevent, so it is in scope. See Open Questions — the semantics
   here differ slightly from the other four sites.

## Out of Scope

- **No migration.** Since `1eff4e801`, `CreateTaskFromPrompt` *materializes* the default
  into the task row, so every task created on or after 2026-08-10 has
  `{"vcpus": 4, "memory_mb": 8192}` stored explicitly and will **not** pick up the new
  default — only newly created tasks benefit. An explicitly stored value is
  indistinguishable from a deliberate user choice, so rewriting rows is a separate
  decision. State this consequence clearly in the PR body.
- The stale `hydra` binary in `helix-sandbox-nvidia-1` on meta (dated Aug 4, predates
  `PATCH /dev-containers/{session_id}/resources`, so live resizes there return
  `hydra API error (status 404)`). Deployment staleness on one host, not a code bug.
- Changing the preset table itself.
- `frontend/src/components/sandboxes/CreateSandboxDialog.tsx` — separate Sandboxes-API
  feature with its own size list.

## Open Questions

1. **`ProjectChatItemTooltip` semantics.** This tooltip renders *historical* tasks. A task
   with a nil override was created before `1eff4e801` and actually ran **uncapped** —
   neither 4/8 nor 8/16 is literally true for it. Plan of record is to point it at
   `DEFAULT_SANDBOX_PRESET` so the tooltip matches how the server resolves nil today, and
   update the test's expectation to `'8 vCPU · 16 GB RAM'`. Confirm you'd rather have
   server-consistency than historical accuracy here; the alternative is leaving the
   tooltip's literals alone with a comment explaining why they intentionally differ.

2. **Scope of the shared module.** The brief names four call sites and this spec adds a
   fifth. Should `frontend/src/components/project/ProjectTaskDefaults.tsx` (the
   project-level default picker) also consume the shared table? It has no preset table of
   its own today, so it is left untouched unless you say otherwise.

3. **Capacity.** Doubling the default doubles per-task reservation. The evidence cites
   ~257 GB free on the affected host, which comfortably absorbs it there, but no check was
   made of other hosts or of any org-level quota that counts reserved vCPU/memory. Assumed
   to be a non-issue; flag if there is a quota path that needs adjusting alongside.
