# Design: Right-Size Spec-Task Sandbox Defaults and Replay Stream Init on Reconnect

Two halves of one outage. Part A is a data-driven default change plus config
plumbing plus a de-materialization migration. Part B is ~120 lines of new proxy
code with unit tests. They ship as two stacked PRs.

---

# Part A — sandbox defaults

> **Decided by Luke, 2026-08-24 — fixed, not suggestions.**
> Ladder: 1/2048, 4/8192, 8/16384, **12/24576**, **16/32768** (2 GB per vCPU
> throughout). Default: **12 vCPU / 24576 MB**, still operator-configurable, with
> 12/24576 as the no-env fallback. Migration targets **only** rows holding exactly
> `{"vcpus": 4, "memory_mb": 8192}` — never the 178 rows holding
> `{"vcpus": 8, "memory_mb": 16384}`. `CreateSandboxDialog.tsx` ladder moves too.
> The analysis below is retained as the *why*, not as a live proposal.

## A1. The chosen default: 12 vCPU / 24576 MB

### Luke's reasoning

24 GB is the number because a Helix-in-Helix task's real steady state exceeds 8 GB
for the inner stack alone, and the largest uncapped desktop on node01 runs
29.75 GiB. The supporting arithmetic below arrives at the same figure
independently.

### The data

Steady-state memory of desktop containers on node01, 2026-08-24
(`docker exec helix-sandbox-nvidia-1 docker stats`):

| container | usage | limit | censored? |
|---|---|---|---|
| `01m0sbr51axfh4gmm30xct6f0k` | 7.29 GiB | 8 GiB | yes — pinned at cap |
| `01m0s0vkvq89563fdkxza4cry6` | 7.89 GiB | 8 GiB | yes — the outage |
| `01m0cm7thsmwf3aydezsntg2rg` | 8.32 GiB | 16 GiB | no |
| `01kzzym0gtzhg57e891mj1466x` | 6.57 GiB | 16 GiB | no |
| `01kz6r8hvd8b2eav84r5cryfs3` | 5.56 GiB | 16 GiB | no |
| `01kzdznnqrvwfwf0ppgf098tmk` | 9.67 GiB | uncapped | no |
| `01kz6r9czzj9tzs1twxn0ff1qd` | **29.75 GiB** | uncapped | no |

Only the uncapped and under-limit rows show true demand. The two at 8 GiB are
**censored** — their real demand is `≥` what is shown, not equal to it.

Sorted lower bounds: 5.56, 6.57, ≥7.29, ≥7.89, 8.32, 9.67, 29.75.
p90 of seven points falls between the 6th and 7th value → **≈21 GiB**, and that
is a lower bound because of the censoring.

The brief's rule is *"do not pick a number that leaves the p90 task inside 90% of
its ceiling"*. 21 / 0.9 = **23.3 GiB → the ceiling must be at least 24 GiB.**
16 GiB puts the p90 at ~131% of its ceiling — i.e. an OOM, not a margin. The
prior-art spec's 8/16 GB is therefore the floor the brief names, not the answer
the data gives.

### The vCPU figure

`cpu.stat` showed `nr_throttled 20319 / 25366 periods`, `throttled_usec 76,850 s`
at the 4-vCPU cap. Raising to 12 took observed utilisation from 397% to **1213%**
instantly — the workload wanted every core it was given. 12 is the number that
was measured, not extrapolated.

### The pick

**Default = 12 vCPU / 24576 MB.** It is exactly
`docker update --memory 24g --memory-swap 48g --cpus 12`, the configuration
proven live to take the outage from 0 frames to 1638 frames at 54.6 FPS. Picking
anything smaller means shipping a default we have direct evidence is insufficient.

The 29.75 GiB container is one long-lived uncapped outlier; it is served by
selecting the new 16/32768 rung, or by an operator raising the config default
(A3). Chasing the max rather than the p90 would halve node density for one
observation.

**Settled.** Luke fixed the default at 12/24576. No longer open.

### A1a. Clamp vCPUs to host capacity — required for "works out of the box"

Docker **rejects** `--cpus` above the host CPU count:
`Range of CPUs is from 0.01 to N.00, as there are only N CPUs available`.
A 12-vCPU default therefore fails container creation outright on any host with
fewer than 12 cores. That directly defeats the point of this task. (This box has
`nproc` = 12 and 499 GB RAM — exactly at the line.)

`api/pkg/hydra/devcontainer.go:1104`:

```go
func sandboxResourceLimits(vcpus, memoryMB int) (nanoCPUs, memory, memorySwap int64) {
	if vcpus > 0 {
		nanoCPUs = int64(vcpus) * 1_000_000_000
	}
	...
}
```

Clamp there — it is the single funnel for both create (`:1048`) and update
(`:2181`) — to `runtime.NumCPU()`, logging once when a clamp happens. Memory
needs no clamp: Docker permits a memory limit above host RAM (it warns), and an
unreachable ceiling is strictly better than a guaranteed-OOM one.

Flagged as **Open Question 2**.

## A2. The preset ladder

Current (`api/pkg/types/simple_spec_task.go:117-144`):
`1/2048`, `4/8192`, `8/16384`. New:

| vCPU | memory MB | note |
|---|---|---|
| 1 | 2048 | unchanged |
| 4 | 8192 | unchanged |
| 8 | 16384 | unchanged |
| **12** | **24576** | new — the new default |
| **16** | **32768** | new ceiling — covers the 29.75 GiB observation |

Three properties this preserves, all deliberate:

1. **vCPU stays the preset key.** Memory is derived from it everywhere in this
   codebase (`SpecTaskSandboxPresetForVCPUs`, the `preset.vcpus === …` comparisons
   in both frontend menus, the org MCP `sandbox_vcpus` argument). Never treat
   memory as independently selectable.
2. **The 2 GB-per-vCPU ratio holds across all five rungs.** No special case.
3. **Every existing rung stays valid.** The tempting alternative — re-key 8 from
   16384 to 24576 so the default can stay at 8 vCPU — would make every stored
   `{"vcpus":8,"memory_mb":16384}` row fail `ValidPreset()` and be rejected by
   `project_handlers.go:409,755` and `spec_task_execution_config_handlers.go:109`
   on the next update. Rejected for that reason.

## A3. Operator configuration

New fields on the existing `Sandboxes` struct (`api/pkg/config/config.go:166`),
which already hosts `DefaultRuntime` / `HELIX_SANDBOX_DEFAULT_RUNTIME`:

```go
// DefaultSpecTaskVCPUs / DefaultSpecTaskMemoryMB size the desktop container a
// spec task gets when neither the task nor its project specifies one. They must
// form a valid preset (see types.SandboxResourceOverrides.ValidPreset) — memory
// is keyed off vCPUs everywhere else in the system, so an arbitrary pairing here
// would produce a container the UI cannot represent and a resize cannot restore.
DefaultSpecTaskVCPUs    int `envconfig:"HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS" default:"12"`
DefaultSpecTaskMemoryMB int `envconfig:"HELIX_SPEC_TASK_SANDBOX_DEFAULT_MEMORY_MB" default:"24576"`
```

The `default:` tags carry the new value, so **out-of-the-box behaviour with no
env set is the correct behaviour** — which is the whole point of the task.

### Plumbing: a configured package default in `types`

`DefaultSpecTaskSandboxResources()` is called from `types`, `services`,
`org/infrastructure/runtime/helix`, `server`, and `external-agent` — including
from `desktopBillingResources()` (`hydra_executor.go:749`), a free function with
no receiver and no context. `HydraExecutor` does not hold a `*config.ServerConfig`
at all.

Threading config through every one of those constructors is the textbook answer
and is roughly ten constructor signatures plus their tests, for a value that is
immutable after startup. Instead:

```go
// api/pkg/types/simple_spec_task.go

// Compile-time fallback. Config overrides it at startup; this is what a binary
// with no configuration does, and it must be a correct answer on its own.
const (
	DefaultSpecTaskSandboxVCPUs    = 12
	DefaultSpecTaskSandboxMemoryMB = 24576
)

var configuredSpecTaskSandboxDefault atomic.Pointer[SandboxResourceOverrides]

// SetDefaultSpecTaskSandboxResources installs the operator-configured default.
// Called exactly once, from API startup, before anything serves. Rejects a pair
// that is not a valid preset rather than silently accepting a size the UI cannot
// display and a resize cannot roll back to.
func SetDefaultSpecTaskSandboxResources(r SandboxResourceOverrides) error { … }

func DefaultSpecTaskSandboxResources() *SandboxResourceOverrides { … }
```

`DefaultSpecTaskSandboxResources()` and `EffectiveSpecTaskSandboxResources()`
keep their exact signatures, so **no call site changes shape**. The tradeoff —
package-level mutable state in `types` — is accepted because the alternative is
churn across ten constructors for a startup-immutable value, and because tests can
set and restore it in one line. Document the "set once, before serving" contract
in the doc comment.

Invalid configured pair → startup error, not a silent fallback. An operator who
sets `VCPUS=6` should be told, not quietly given 12.

## A4. Materialized rows — strategy (c), both

Commit `1eff4e801` (2026-08-10) made `CreateTaskFromPrompt` materialize the
default onto the row.

### The real scale — corrects the brief

The brief said "**every** task created since 2026-08-10 carries an explicit
`{"vcpus": 4, "memory_mb": 8192}`". Luke counted meta's `spec_tasks` today and
that is not what the data shows:

| rows | value | meaning | action |
|---|---|---|---|
| 4102 | `NULL` | inherits the const | **nothing to do** — these pick up the new default for free |
| 178 | `{"vcpus": 8, "memory_mb": 16384}` | a real user choice | **DO NOT TOUCH** |
| 31 | `{"vcpus": 4, "memory_mb": 8192}` | the materialized old default | the only stale rows |

Two consequences worth internalising before writing the migration:

1. **The overwhelming majority of rows are already `NULL`.** The materialization
   bug was far narrower than the outage report implied — most tasks were always
   going to inherit a raised default. This does not make the fix unnecessary, but
   it does mean the migration is a 31-row cleanup, not a mass rewrite.
2. **The 178-row group is the one that matters.** A migration written against
   "any row holding a value that happens to equal some default" would eat 178
   deliberate user choices. Match the old pair **exactly** and nothing else.

Luke has **already hand-backfilled the 31 rows on meta** to
`{"vcpus": 12, "memory_mb": 24576}`, with a backup of the old values. The
migration must therefore also be a safe no-op on meta — which it is, since those
rows no longer match the old pair.

### (b) Stop materializing — the durable fix

`api/pkg/services/spec_driven_task_service.go:157-164` and
`api/pkg/org/infrastructure/runtime/helix/spectasks.go:203-216` both end with:

```go
if sandboxResources == nil {
	sandboxResources = types.DefaultSpecTaskSandboxResources()   // ← delete this
}
```

Delete both. When neither the request nor the project specified a size, store
`nil`. The resolution already exists downstream:
`HydraExecutor.resolveSpecTaskLaunchConfig` (`hydra_executor.go:697`) calls
`types.EffectiveSpecTaskSandboxResources(task.SandboxResourceOverrides)`, which
handles nil by returning the default. So the default is resolved at
container-create time, live, for every start path including forks and reconciler
resumes.

This is the shape the brief argues for and it is right: **a stored override means
"the user chose this", not "this was the default the day the row was written."**

Note `spec_driven_task_service.go:1075,1082` (`VCPUs`/`MemoryMB` accessors for a
task) already route through `EffectiveSpecTaskSandboxResources` and keep working
with nil. Check the API response shape — if the frontend renders
`task.sandbox_resource_overrides` directly, a now-nil field must fall back to the
shared default constant rather than rendering blank.

### (a) Backfill — for every deployment other than meta

New migration `api/pkg/store/migrations/0009_unmaterialize_spec_task_sandbox_default.up.sql`:

```sql
-- Between 1eff4e801 (2026-08-10) and this migration, CreateTaskFromPrompt wrote
-- the global default onto rows that had no explicit size, freezing those tasks at
-- 4 vCPU / 8 GB forever. NULL means "no override, use the live default" — which is
-- what these rows meant all along.
--
-- Match the old default pair EXACTLY. On meta this is 31 rows. The 178 rows
-- holding {"vcpus": 8, "memory_mb": 16384} are deliberate user choices and must
-- never be caught by this. Do not generalise this predicate to "any row equal to
-- some default" — 8/16384 was a valid selectable preset the whole time.
--
-- No-op on meta, whose 31 rows were hand-backfilled to 12/24576 on 2026-08-24.
UPDATE spec_tasks
   SET sandbox_resource_overrides = NULL
 WHERE sandbox_resource_overrides = '{"vcpus": 4, "memory_mb": 8192}'::jsonb;
```

`sandbox_resource_overrides` is `gorm:"type:jsonb;serializer:json"`, so
`= '…'::jsonb` compares semantically (key-order independent). Confirm with
`\d+ spec_tasks` that the column really is `jsonb` and not `text` before relying
on that; if it is `text`, cast both sides.

**Sanity-check the predicate against production counts before and after.** Run the
`SELECT sandbox_resource_overrides, count(*) FROM spec_tasks GROUP BY 1` that
produced Luke's table, confirm the 8/16384 count is identical after the migration,
and put both numbers in the PR body. That check is the whole safety story for this
migration.

Down migration: a no-op with a comment. The information needed to reverse it
(which NULLs were previously 4/8192) is gone by construction, and re-materializing
all NULLs would recreate the bug for the 4102 rows that never had the value.

### `NULL` vs the explicit new pair — Open Question 1

Luke's hand backfill wrote `{"vcpus": 12, "memory_mb": 24576}`. The shipped
migration above writes **`NULL`** instead, deliberately: NULL is the only value
consistent with the "a stored override means the user chose this" principle Luke
re-affirmed in the same decision. A NULLed row tracks every future default change;
an explicit `12/24576` row is frozen exactly the way `4/8192` was — it would
recreate this ticket the next time the default moves.

It is a one-word difference and either is defensible. Flagged rather than decided
silently because it diverges from what meta now holds. Meta's 31 rows can be
NULLed separately if that shape is preferred; the migration is a no-op there
either way.

**Residual caveat.** A user who deliberately selected 4 vCPU / 8 GB is
indistinguishable in the data from a materialized default and gets reset. Luke's
counts bound this to at most 31 rows on meta. Getting more resources is not
harmful and 4 CPU can be re-selected.

**Projects are deliberately not touched.** `project_handlers.go:503` sets
`DefaultSandboxResourceOverrides` straight from the request — a project only
carries a value when an admin explicitly configured one. Nulling those would
destroy real intent.

## A4b. A stored 12/24576 is backend-safe today, frontend-blank today

Luke's note, verified against the code and worth recording because it is
counter-intuitive:

- **Backend: already safe.** Every `ValidPreset()` call site validates an
  **incoming request** (`spec_driven_task_handlers.go:169`,
  `spec_task_execution_config_handlers.go:109`, `project_handlers.go:409,755`).
  **None** validates a value read from the DB. `sandboxResourceLimits`
  (`hydra/devcontainer.go:1104`) just multiplies. So meta's 31 hand-backfilled
  rows already start containers at 12/24576 with today's binary.
- **Frontend: blank.** `SpecTaskExecutionControls.tsx` renders the selection by
  matching the stored `vcpus` against its local rung table. With no 12-vCPU rung,
  those 31 tasks render with **no preset selected**.

The ladder change in A5 fixes this by construction. Two things follow:

1. Verification must include **opening one of the 31 backfilled tasks** and
   confirming the "12 CPU / 24 GB RAM" rung is selected — not just creating a new
   task.
2. While there, make the match degrade gracefully: a stored value matching no rung
   should show the raw size rather than rendering blank. That is what turned a
   data change into an invisible UI bug here, and it will happen again the next
   time someone hand-edits a row.

## A5. Every call site

Grepped `8192|16384|DefaultSpecTaskSandbox|ValidPreset` across `api` and
`frontend/src`. This table includes **five sites the brief did not list**.

### Go — change

| Site | What |
|---|---|
| `api/pkg/types/simple_spec_task.go:86-87` | new consts + configured-default accessor (A3) |
| `api/pkg/types/simple_spec_task.go:119-144` | `SpecTaskSandboxPresetForVCPUs` + `ValidPreset` gain 12 and 16 |
| `api/pkg/config/config.go:166` (`Sandboxes`) | two new env-backed fields (A3) |
| API startup wiring | call `types.SetDefaultSpecTaskSandboxResources`, fail loudly on invalid |
| `api/pkg/services/spec_driven_task_service.go:163` | delete the materialization (A4b) |
| `api/pkg/org/infrastructure/runtime/helix/spectasks.go:215` | delete the materialization (A4b) |
| `api/pkg/org/infrastructure/runtime/helix/spectasks.go:207` | error text `"sandbox_vcpus must be 1, 4 or 8"` → new ladder |
| `api/pkg/org/interfaces/mcptools/spec_tasks.go:56` | tool description prose `"sandbox_vcpus (1, 4 or 8 …)"` → new ladder. **Org Workers read this string; leaving it means they keep passing the old ladder and get a validation error.** |
| `api/pkg/org/infrastructure/runtime/runtime.go:263` | doc comment `"(1, 4 or 8; memory follows)"` |
| `api/pkg/types/project.go:246` | comment `"resolve to the standard 4 vCPU / 8 GB preset"` |
| `api/pkg/hydra/devcontainer.go:1104` | clamp vCPUs to `runtime.NumCPU()` (A1a) |
| **`api/pkg/sandbox/controller_provision.go:42-43`** | *not in brief* — sandboxes-API rungs; extend to 12/16, see A6 |
| **`api/pkg/cli/sandbox/sandbox.go:264-266`** | *not in brief* — same ladder in the CLI size parser; extend |

New migration: `api/pkg/store/migrations/0009_…` (A4a).

### Go — deliberately unchanged

`spec_driven_task_handlers.go:169`, `spec_task_execution_config_handlers.go:109`,
`project_handlers.go:409,755` all call `ValidPreset()` symbolically — they follow
the ladder for free. `spec_task_execution_config_handlers.go:130` uses
`DefaultSpecTaskSandboxResources()` as a resize-rollback target and follows the
configured default for free.

### Frontend — extract first, then change

Lifted wholesale from the prior-art spec (002903), which had this part right.
New `frontend/src/constants/sandboxPresets.ts`:

```ts
export interface SandboxPreset {
  vcpus: number
  memory_mb: number
  label: string        // SpecTaskExecutionControls renders this
  description: string  // both components render this
}

export const SANDBOX_PRESETS: SandboxPreset[] = [
  { vcpus: 1,  memory_mb: 2048,  label: '1 CPU',  description: '2 GB RAM' },
  { vcpus: 4,  memory_mb: 8192,  label: '4 CPU',  description: '8 GB RAM' },
  { vcpus: 8,  memory_mb: 16384, label: '8 CPU',  description: '16 GB RAM' },
  { vcpus: 12, memory_mb: 24576, label: '12 CPU', description: '24 GB RAM' },
  { vcpus: 16, memory_mb: 32768, label: '16 CPU', description: '32 GB RAM' },
]

export const DEFAULT_SANDBOX_PRESET = SANDBOX_PRESETS[3]   // 12 / 24576
```

Keeping **both** `label` and `description` matters: `SpecTaskExecutionControls`
entries carry both, `CodeAgentExecutionControls` entries carry only
`description`. Dropping either silently blanks one menu.

| File | What changes |
|---|---|
| `components/tasks/SpecTaskExecutionControls.tsx:57-60` | delete local table, import shared; `selectSandbox`'s `typeof SANDBOX_PRESETS[number]` param becomes `SandboxPreset` |
| **`components/agent/CodeAgentExecutionControls.tsx:54-55`** | *not in brief* — same local table, same treatment |
| `pages/Home.tsx:138,174` | `memory_mb: 8192` and `{ vcpus: 4, memory_mb: 8192 }` → derive from `DEFAULT_SANDBOX_PRESET` |
| **`components/tasks/NewSpecTaskForm.tsx:152,339`** | *not in brief* — useState initial value, and the `?.memory_mb \|\| 8192` project fallback |
| **`components/session/projectChatItemDetails.ts:65`** | *not in brief* — `?.memory_mb \|\| 8192` fallback |
| `components/sandboxes/CreateSandboxDialog.tsx:44-46` | sandboxes-API rungs — see A6 |

`Home.tsx` / `NewSpecTaskForm` store `{vcpus, memory_mb}` API shapes, not presets.
Spread only the two numeric fields —
`{ vcpus: DEFAULT_SANDBOX_PRESET.vcpus, memory_mb: DEFAULT_SANDBOX_PRESET.memory_mb }`.
Pushing `label`/`description` into an API payload would be a behaviour change.

**A5-exit check:** re-run
`grep -rn "8192\|16384\|DefaultSpecTaskSandbox\|ValidPreset" api frontend/src`
after the change and account for every hit. The table above is a snapshot; a new
caller could have landed. Note that many `8192`/`16384` hits are unrelated
(model context lengths in `api/pkg/model/models.go`, WebSocket buffer sizes,
`StreamSupportedVideoFormats` bit flags, `profileBlocks.ts` vLLM args) — do not
touch those.

## A6. Decision: the sandboxes API gets the rungs, not the default

`CreateSandboxDialog.tsx:44-46`, `controller_provision.go:42-43` and
`cli/sandbox/sandbox.go:264-266` are a *different feature* — raw sandboxes,
mostly headless runtimes — that happens to duplicate the same three rungs.

**Extend its ladder to the same five rungs; leave its default alone.**

- Extending costs two lines per file and stops a user being offered a size in one
  place that the other refuses — the ladders diverging is exactly the class of bug
  this task exists to close.
- Not changing its default is deliberate: sandbox size there is an explicit
  user choice at create time, not a silent default applied to work the user did
  not size. No outage evidence exists for that surface, and raising a default
  nobody asked for costs density for no measured benefit.

**Settled.** Luke confirmed `CreateSandboxDialog.tsx` moves with the rest.

## A7. Generated OpenAPI

The `types/project.go:246` doc comment is baked into seven committed generated
files: `api/pkg/server/docs.go`, `api/pkg/server/swagger.json`,
`api/pkg/server/swagger.yaml`, `swagger.json`, `openapi.json`,
`frontend/swagger/swagger.yaml`, `frontend/src/api/api.ts`. Run
`./stack update_openapi` (target at `stack:1536`) and commit the regenerated
output. **Never hand-edit any of them.** Re-grep `standard 4 vCPU` afterwards to
confirm all seven copies moved.

## A8. Test impact

| Test | Why it moves |
|---|---|
| `api/pkg/services/spec_driven_task_service_test.go` (~129) | asserted the materialized default; under A4 (b) the row is now **nil** — this is an inversion, not a value bump |
| `api/pkg/org/infrastructure/runtime/helix/spectasks_sandbox_test.go` (~79) | uses `{4, 8192}` as the contrasting *project* default. Under A4 (b) the no-project case stores nil, so re-point the contrast to `1/2048` (still `ValidPreset`, distinct from the default) or `TestSpecTasks_CreateFallsBackToProjectSandboxDefaults` becomes vacuous |
| new — frontend | a stored `{vcpus: 12, memory_mb: 24576}` selects the 12-CPU rung; a stored value matching no rung does not render blank (A4b) |
| new — migration | the predicate leaves `{"vcpus": 8, "memory_mb": 16384}` rows untouched |
| `api/pkg/external-agent/task_overrides_test.go:84-85` | references the constants symbolically — should keep passing; **confirm by running, don't assume** |
| `api/pkg/hydra/devcontainer_test.go:45` | asserts `MemorySwap == wantMemory*2`; add a clamp case |
| `frontend/.../SpecTaskExecutionControls.test.tsx` (9 sites) | pass `{vcpus: 4, memory_mb: 8192}` as the *current* value — mostly still valid; `:219` clicks the 8-vCPU row and asserts a resize, which still contrasts with a 12-vCPU default. Run before rewriting |
| `frontend/.../ProjectChatItemTooltip.test.tsx`, `projectChatItemDetails` | assert the `\|\| 8192` fallback string |
| `frontend/.../ProjectTaskDefaults.test.tsx` | uses 4/8192 and 16384 as explicit *project* values — still valid, no change expected |
| `frontend/src/pages/newChatLogic.test.ts:69,79` | explicit 8/16384 — still a valid rung, no change |

---

# Part B — replay `init` on reconnect

## B1. The constraint that shapes everything

`ResilientProxy` is a **raw byte** proxy over two hijacked `net.Conn`s
(`external_agent_handlers.go:1385` hijacks the browser side;
`connman.Dial` gives the RevDial side). `copyClientToServer` shovels 32 KB
buffers. It has never parsed a WebSocket frame.

So capturing `init` means introducing exactly as much WebSocket framing awareness
as is needed to find one text frame in the client→server direction, and no more.
Also: browser→server frames are **masked** (RFC 6455 §5.3), so a replayed frame
must be re-masked with a fresh key — a verbatim byte copy is a valid frame too,
but re-encoding is required anyway because the payload changes (B4).

## B2. Mechanism — the sibling of `extraHeaders`

`CreateWebSocketUpgradeFunc`'s doc comment (`resilient.go:678-685`) already
argues the principle:

> They belong here rather than only on the initial handshake because this func
> also runs on RECONNECT, and a reconnect that dropped them would silently
> restore the privilege mid-session.

`init` has the identical requirement at the frame layer. Add the frame-layer
sibling:

```go
// api/pkg/proxy/resilient.go

// SessionReplay carries per-connection state the backend learns from the
// client's first frames and would otherwise lose when the backend reconnects.
//
// It is the frame-level sibling of CreateWebSocketUpgradeFunc's extraHeaders:
// both re-apply state that only the original handshake carried, and both belong
// on the reconnect path rather than only on the initial connection. The header
// path re-sends what WE know about the caller; this path re-sends what the
// CLIENT told the backend. Dropping either leaves a half-configured backend.
type SessionReplay interface {
	// Observe is fed every byte the client sends, in order. Implementations
	// must be cheap once they have what they need.
	Observe(b []byte)
	// Frames returns bytes to write to a freshly-upgraded backend connection
	// before proxying resumes. Nil when there is nothing to replay.
	Frames() []byte
}
```

Wiring, three small edits:

1. `ResilientProxyConfig` gains `Replay SessionReplay` (optional, nil = today's
   behaviour, so every other caller of `NewResilientProxy` is unaffected).
2. `copyClientToServer` calls `p.replay.Observe(buf[:n])` on each successful read,
   before the write/buffer branch.
3. `reconnect()` writes `p.replay.Frames()` to the new `serverConn` **immediately
   after `upgradeFunc` returns nil and before the input buffer is flushed.**
   Ordering matters: the backend is blocked in its init loop and must see init
   first. (It would tolerate the other order — it skips binary frames while
   waiting — but replay-first is what the protocol means.)

## B3. `StreamInitReplay` — new file `api/pkg/proxy/streaminit.go`

- Incremental RFC 6455 client-frame parser over the byte stream. Handles the
  7-bit, 16-bit and 64-bit length forms and the 4-byte mask key. Skips non-text
  opcodes (the client's 13-byte binary keepalives, ping/pong, close).
- On the first complete **text** frame (opcode 0x1): unmask the payload, keep it,
  and stop parsing — `Observe` becomes a no-op from then on. One init per
  connection; a later text frame is not session state.
- Transform (B4), then encode a fresh **masked** client text frame with a random
  mask key, cache the bytes, return them from `Frames()`.
- Bound the buffered prefix (a few KB). A client that sends megabytes without a
  text frame is not a stream client; stop buffering rather than grow unbounded.

Wire it in `proxyStreamWebSocket` (`external_agent_handlers.go`) by passing
`Replay: proxy.NewStreamInitReplay()` into `ResilientProxyConfig`, next to the
existing `UpgradeFunc: proxy.CreateWebSocketUpgradeFunc(path, wsKey, readOnlyHeader…)`.
Those two lines sitting together *is* the "obvious sibling" the brief asks for.

## B4. Decision: a replayed init has `user_retry` cleared

`ws_stream.go:205-209` already states the rule:

> UserRetry marks this connection as an explicit user-initiated retry (the
> Restart button), as opposed to an automatic reconnect. It is the only thing
> that clears a latched circuit breaker — automatic reconnects must not, or the
> breaker is back to letting a retry storm through one pipeline at a time.

A proxy reconnect is by definition automatic. If replay preserved
`user_retry: true`, then a user who pressed Restart once would have that flag
re-asserted on every subsequent backend drop, so
`GetSharedVideoRegistry().ResetCircuitBreaker(nodeID)` (`ws_stream.go:1891`) would
fire on each one and **the breaker could never latch** — which is precisely the
runaway the outage log shows ("active pipelines: 2, 3, 4").

Implementation: unmarshal the captured payload into `map[string]any`, `delete` the
`user_retry` key (it is `json:"user_retry,omitempty"`, so absent == false), and
re-marshal. A map, not `StreamConfig` — round-tripping through the struct would
silently drop any field a newer browser sends that this API build does not know
about, and re-emit `omitempty` fields the client deliberately omitted. Everything
except `user_retry` is passed through byte-for-byte in meaning.

The Restart button is untouched: it opens a **new client socket** with
`user_retry: true`, which is the first init on that connection and is not a
replay.

## B5. Read-only safety

`StreamConfig` (`ws_stream.go:192-221`) has **no privilege field**. Read-only is
enforced entirely by `X-Helix-Readonly`, which desktop-bridge reads from the
upgrade request (`ws_stream.go:1828`) and which the API sets server-side via
`CreateWebSocketUpgradeFunc`'s `extraHeaders` — already re-applied on every
reconnect, by design, per its doc comment.

So replay **cannot** re-grant input: the init payload has no lever to pull, and
the header path is independent and already correct. The two mechanisms compose;
neither can undo the other.

Guard it with a test anyway (`replay composes with extraHeaders`) so a future
change that starts carrying privilege in the init payload trips a red test rather
than shipping.

## B6. The shared pipeline on reconnect

The replayed init is byte-identical in meaning to the original (same width,
height, fps, bitrate, session_id), so `GetOrCreate(nodeID, pipelineStr, opts)`
(`shared_video_source.go:239`) should return the **existing live** source.
`[SHARED_VIDEO] Evicted dead source in GetOrCreate` (`:257`) only fires when the
registered source is dead.

**Observed 2026-08-24 — Open Question 5 is answered: the pipeline survives.**
Forcing a real backend drop (`ss -K` on the desktop-bridge socket, leaving RevDial
up) produced:

```
[SHARED_VIDEO] Client subscribed (grace period reconnection, starting catchup)
```

Not `Evicted dead source in GetOrCreate`, and not `Created new source`. The
registry has a 60 s grace period (`grace_period=60000` at init) which holds the
source alive across exactly this window, so the replayed init re-subscribes to
the *live* pipeline. No restart, no 43 s stall, no leak. The linger follow-up
contemplated here is unnecessary — the grace period already is it.

Note the contrast with killing the whole `desktop-bridge` process: that destroys
the registry, so the next connection legitimately logs `Registry initialized` /
`Created new source`. Correct, but it is not the reconnect path — see the
"how to force a REAL proxy reconnect" note in `tasks.md`.

## B7. Known limitation: frame boundaries across a drop

`ResilientProxy` buffers *bytes*, not frames. If the server connection died
partway through a client frame, the bytes already written are lost and the input
buffer resumes mid-frame — after replay, the backend would read a frame fragment
as a header. This hazard predates this change (it applies to keyboard/mouse frames
today) but replay makes it reachable more often.

### Decision at implementation time: documented, not fixed

Re-read of the write path (`copyClientToServer`) confirms the hazard is **wholly
pre-existing** and untouched by this change:

```go
_, err = server.Write(buf[:n])
if err != nil {
    if bufErr := p.bufferInput(buf[:n]); bufErr != nil { ... }   // whole chunk re-buffered
```

Two independent sources of desync, both already there:
1. `Write`'s returned byte count is discarded, so a partially-successful write
   re-buffers bytes the dead connection already consumed.
2. Reads are 32KB chunks that do not align with frame boundaries, so an *earlier*
   successful write can end mid-frame.

The cheap-looking fix in the original sketch does not actually work: `Observe` is
fed bytes *before* the write is attempted, and writes fail partially, so
reconciling "bytes parsed into whole frames" against "bytes actually delivered"
needs accounting the proxy does not currently keep. Doing it properly means
making the byte proxy frame-aware in the hot path — which `requirements.md`
lists as an explicit **non-goal**.

Risk assessment for the stream specifically: every client→server frame here is
small (13-byte keepalives, keyboard/mouse events). Desync needs a frame split
across TCP segments *and* a write failure inside that window. Low probability,
and the consequence is one corrupted frame on a connection that is being
re-established anyway.

**Left as a documented known limitation.** Not worth speculative complexity in
the hot path of every proxied byte, and not made worse by this change — replay
makes reconnects *succeed* where they previously always failed, which is the
only reason the seam is reachable at all.

## B8. Required unit tests — `api/pkg/proxy/`

Named by the brief:
- `reconnect replays init` — fake client/server conns, feed an init frame, kill
  the server conn, assert the freshly-dialled server receives the init before any
  buffered input.
- `replayed init has user_retry cleared` — init with `"user_retry": true` in,
  replayed frame parses to `user_retry` absent/false, and every other field
  unchanged.

Also worth having, all cheap against the parser:
- 7-bit, 16-bit and 64-bit payload length forms.
- Binary keepalives before init are skipped; the first *text* frame wins.
- A later text frame does not overwrite the captured init.
- The replayed frame is a well-formed masked client text frame.
- `Frames()` is nil before any init is seen (reconnect before init replays
  nothing rather than sending garbage).
- Replay composes with `extraHeaders` (B5).

---

# Verification (both parts)

`CLAUDE.md` rules apply. A unit test asserting a struct field is not evidence.

1. `cd api && go build ./pkg/...`; `cd frontend && yarn build`.
2. In the inner Helix at `http://localhost:8080`: register `test@helix.ml` /
   `helixtest`, complete onboarding, create a spec task, open its detail page,
   confirm **video actually plays in the browser**.
3. **Part A, fresh task** — from the host:
   `docker exec helix-sandbox-nvidia-1 docker inspect -f '{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}}' <container>`
   → `25769803776 12000000000` for 12/24576 (or the clamped CPU value on a
   smaller host, which must also be logged).
4. **Part A, pre-existing task** — on a deployment other than meta, pick a row
   holding the old pair, confirm the migration NULLed it and that starting it
   produces the new limits. Confirm the `8/16384` row count is unchanged.
   On meta, open one of the **31 hand-backfilled tasks** and confirm the selector
   shows the "12 CPU / 24 GB RAM" rung selected rather than blank (A4b).
5. **Part A, config** — set `HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS=8` /
   `..._MEMORY_MB=16384`, restart, create a task, confirm the container follows.
   Then set an invalid pair and confirm the API refuses to start.
6. **Part B, real reconnect** — with a browser watching a playing stream, restart
   `desktop-bridge` inside the desktop container. Confirm video **resumes
   unattended**, `reconnect_count` increments, and there is **no**
   `failed to read init message` in the desktop-bridge log. Read the log for which
   `GetOrCreate` branch was taken (B6). Per `CLAUDE.md`, test the *next* operation
   after the drop — click something in the desktop and confirm input still works,
   and confirm an embed-key (read-only) viewer still cannot input after a
   reconnect.
7. `helix spectask stream <session> --duration 30` → ~1600 frames, 50–60 FPS at
   1920x1080. Zero frames means still broken.
8. `gh pr checks` after pushing; fix CI failures without being asked.

# Deliverables

- **One PR** (decided by Luke, 2026-08-24): the spec-task system still makes
  multiple PRs per task awkward to drive, and that operational cost outweighs the
  reviewability gain. Both parts land on
  `feature/002936-right-size-spec-task`.

  The original two-PR argument — different blast radius (a product default plus
  an irreversible DB migration vs. a wire-protocol change), different reviewers,
  independent revertability — is recorded here because it does not go away by
  being overruled. Mitigate it inside the single PR instead: keep Part A and
  Part B in **separate commits** so either can be reverted alone, and give the
  PR body two clearly separated sections. The migration is the one piece that
  cannot be un-run, so it must be called out explicitly at the top of the PR body
  rather than left for a reviewer to find.
- Design doc at `design/2026-08-24-sandbox-defaults-and-stream-init-replay.md` in
  the helix repo recording the chosen default and its reasoning, the migration
  strategy, and the `user_retry`-on-replay decision.
- Full URLs for PRs/issues (`https://github.com/helixml/helix/pull/NNN`), never
  `owner/repo#123`.
- Close `spt_01m0evm3dpanc1sfktywbxhes4` as superseded.

# Notes for future agents

- **vCPU is the primary key of a sandbox preset throughout this codebase.** Memory
  is always derived from it. Never make memory independently selectable.
- **`ValidPreset()` guards requests, never reads.** Nothing validates a preset
  loaded from the DB, and `sandboxResourceLimits` just multiplies. So a row can
  legally hold a size no rung represents: the backend honours it, and the
  **frontend silently renders no selection**. That asymmetry is why a data
  backfill can look correct in `docker inspect` and broken in the UI at the same
  time. Check both.
- When a value like `8/16384` is *both* a plausible default and a real selectable
  preset, a migration predicate written as "equals a default" destroys user
  intent. Match the exact historical pair and diff the group counts before/after.
- The ladder is enforced in **five** independent places: `ValidPreset()` and
  `SpecTaskSandboxPresetForVCPUs()` in `api/pkg/types/simple_spec_task.go`, the
  org runtime's error string, the org MCP tool *description prose* (Workers read
  it), and `api/pkg/sandbox/controller_provision.go`. Adding a rung means all five
  plus the frontend module.
- **Docker rejects `--cpus` above the host CPU count** but accepts a memory limit
  above host RAM. That asymmetry is why the CPU default needs clamping and the
  memory default does not.
- `ResilientProxy` is a **raw byte** proxy. It has no frame awareness and buffers
  bytes across reconnects, so frame boundaries are not preserved. Anything that
  needs frame semantics has to bring its own parser.
- OpenAPI output is committed, generated from Go doc comments, and fans out to
  **seven** files. Always `./stack update_openapi`; never hand-edit.
- When reading `docker stats` to size anything: a container pinned at its limit is
  a **censored** observation, not a measurement. Only uncapped or under-limit rows
  tell you real demand.
- Prior-art spec `helix-specs/design/tasks/002903_default-spec-task/` has a good
  frontend de-duplication plan and OpenAPI fan-out warning; both are lifted here.
