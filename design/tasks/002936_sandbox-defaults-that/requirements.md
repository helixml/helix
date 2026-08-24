# Requirements: Right-Size Spec-Task Sandbox Defaults and Replay Stream Init on Reconnect

## Background

One outage on 2026-08-24 (meta / node01, task `spt_01m0s0vktb0twdtwz7cmk6wgtg`,
session `ses_01m0s0vkvq89563fdkxza4cry6`) produced the user-visible symptom
"video streaming is broken". It was not a video bug. It was two independent
defects that combined:

**Part A — the default sandbox is too small.** The desktop container sat at
99.999% of its 8 GiB `memory.max` with `memory.pressure full avg60=55.6%` and
`cpu.stat nr_throttled 20319/25366`. Under that starvation GStreamer's
`set_state(PLAYING)` took 43–100 s instead of <1 s. `desktop-bridge` sends
`StreamInit` only *after* `streamer.Start()` returns
(`api/pkg/desktop/ws_stream.go`, `handleStreamWebSocketInternal`), so the browser
timed out, closed, and every later write hit `broken pipe`. Each retry sent a
fresh `init`, tripping `[SHARED_VIDEO] Evicted dead source in GetOrCreate` and
leaking pipelines ("active pipelines: 2, 3, 4"). Proof of causation:
`docker update --memory 24g --memory-swap 48g --cpus 12` on the live container
took CPU from 397% (CFS-throttled at the 4-vCPU cap) to 1213%, the pipeline
started in the same second, and `helix spectask stream --duration 30` returned
1638 frames at 54.6 avg FPS. Nothing else changed.

**Part B — the stream proxy never replays `init` on reconnect.**
`proxy.ResilientProxy` transparently re-dials the backend and its
`CreateWebSocketUpgradeFunc` (`api/pkg/proxy/resilient.go:686`) redoes only the
HTTP upgrade. The `init` text frame the browser sent once, on the original
socket, is never replayed. desktop-bridge blocks on a mandatory `init` with a
30 s read deadline, so every reconnect yields a backend socket that waits 30 s,
dies, and reconnects — eight cycles in the outage log. The client socket stays
open the whole time, so the browser shows a live-but-frozen stream and no error
is surfaced anywhere.

Part A caused the drops in this outage. Part B is a latent bug that will fire on
*any* future backend drop (API restart, RevDial blip, sandbox recreate) and is
escapable only by a manual page reload.

## User Stories

### US-1 — A spec task gets enough memory and CPU to run its own workload
**As a** Helix user creating a spec task
**I want** the sandbox to be sized for the work a real spec task does
**So that** the desktop, the agent, and video streaming all function without OOM
kills or CFS throttling.

Acceptance criteria:
- **Decided (Luke):** the global default is raised from 4 vCPU / 8192 MB to
  **12 vCPU / 24576 MB**. `DefaultSpecTaskSandboxVCPUs = 12`,
  `DefaultSpecTaskSandboxMemoryMB = 24576`. 24 GB is the number because a
  Helix-in-Helix task's real steady state exceeds 8 GB for the inner stack alone,
  and the largest uncapped desktop on node01 runs 29.75 GiB.
- This also satisfies the brief's rule that the p90 observed task sits at **under
  90%** of its memory ceiling (see the censoring analysis in `design.md` A1).
- A newly created spec task's container really carries the new limits, verified
  from the host with
  `docker exec helix-sandbox-nvidia-1 docker inspect -f '{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}}' <container>`.
- Out-of-the-box behaviour is correct with **no** environment variables set.

### US-2 — A user can select a sandbox large enough for a Helix-in-Helix task
**As a** user running a task that builds Helix inside the sandbox (inner Postgres
cluster, `go build -tags ORT`, gopls at 1.9 GB, Zed, gnome-shell)
**I want** to select a sandbox larger than 16 GB
**So that** the task is not capped below its known ~24 GB working set.

Acceptance criteria:
- **Decided (Luke):** the ladder becomes exactly these five rungs, keeping the
  2 GB-per-vCPU ratio throughout:

  | vCPU | memory MB | |
  |---|---|---|
  | 1 | 2048 | unchanged |
  | 4 | 8192 | unchanged |
  | 8 | 16384 | unchanged |
  | 12 | 24576 | **new** — the new default |
  | 16 | 32768 | **new** — the new ceiling |

- `SpecTaskSandboxPresetForVCPUs` and `ValidPreset`
  (`api/pkg/types/simple_spec_task.go:117-144`) accept all five.
- Memory remains **not** independently selectable — vCPU picks the rung.
- All existing rungs remain valid, so no stored value is retroactively rejected
  by `ValidPreset`.
- Every UI and prose copy of the ladder moves with it:
  `SpecTaskExecutionControls.tsx`, `CodeAgentExecutionControls.tsx`,
  `CreateSandboxDialog.tsx`, and the org MCP tool description in
  `api/pkg/org/interfaces/mcptools/spec_tasks.go` (currently says "1, 4 or 8").

### US-3 — An operator can change the default without rebuilding
**As an** operator with a 500 GB box (or a 16 GB laptop)
**I want** to set the default sandbox size from configuration
**So that** I do not need a source rebuild to right-size my install.

Acceptance criteria:
- New env vars alongside the other `Default*` values in
  `api/pkg/config/config.go` (the `Sandboxes` struct at line 166, which already
  hosts `HELIX_SANDBOX_DEFAULT_RUNTIME`).
- Unset env → the new compile-time default. Set env → the configured value.
- An invalid configured pair (one that fails `ValidPreset`) is rejected loudly at
  startup, not silently ignored.

### US-4 — Tasks created before the change pick up the new default
**As a** user whose task was created after 2026-08-10 (commit `1eff4e801`, which
made `CreateTaskFromPrompt` materialize the default onto the row)
**I want** my task to run at the new default
**So that** I am not permanently pinned to `{"vcpus": 4, "memory_mb": 8192}` by an
accident of when the row was written.

**Actual scale of the problem** (Luke, counted on meta today — this corrects the
brief, which implied *every* task created since 2026-08-10 was affected):

| rows | `sandbox_resource_overrides` | meaning | action |
|---|---|---|---|
| 4102 | `NULL` | inherits the const | nothing to do — these get the new default for free |
| 178 | `{"vcpus": 8, "memory_mb": 16384}` | a real user choice | **DO NOT TOUCH** |
| 31 | `{"vcpus": 4, "memory_mb": 8192}` | the materialized old default | the only stale rows |

Only 31 rows were ever stale. Luke has **already backfilled those by hand on
meta** to `{"vcpus": 12, "memory_mb": 24576}`, keeping a backup of the old values.
The migration is still needed for every other deployment.

Acceptance criteria:
- New tasks that did not specify a size and had no project default store **no**
  sandbox override; the default is resolved at container-create time. A stored
  override means "the user chose this", not "this was the default the day the row
  was written".
- The migration matches **only** rows holding exactly `{"vcpus": 4,
  "memory_mb": 8192}`. It must never match `{"vcpus": 8, "memory_mb": 16384}` —
  that is 178 deliberate user choices on meta alone.
- A task where the user deliberately chose a size, or where a project default
  applied, keeps that value.
- The migration is idempotent and a no-op on meta, whose 31 rows now hold
  `12/24576` and no longer match the old pair.
- Verified live: a task row created *before* the change starts a container with
  the new limits.

### US-4a — A stored 12-vCPU value renders correctly in the UI
**As a** user opening one of the backfilled tasks
**I want** the sandbox selector to show my task's actual size
**So that** the task does not appear to have no sandbox configured.

A stored `12/24576` is already safe with today's *backend* code: every
`ValidPreset()` call site validates an **incoming request**, none validate a value
read from the DB, and `sandboxResourceLimits` just multiplies. The gap is purely
in the frontend — until the new ladder lands, `SpecTaskExecutionControls.tsx` has
no 12-vCPU rung, so meta's 31 backfilled tasks render with **no preset selected**.

Acceptance criteria:
- After the frontend ladder change, a task storing `{"vcpus": 12,
  "memory_mb": 24576}` renders with the "12 CPU / 24 GB RAM" rung selected and
  marked as the default.
- Verified by opening one of the 31 backfilled tasks on meta, not by unit test
  alone.
- A stored value matching **no** rung (possible for any row hand-edited in future)
  degrades gracefully — it shows the raw size rather than rendering blank.

### US-5 — Every duplicated copy of the ladder moves together
**As a** future maintainer
**I want** one source of truth for the preset ladder and the default
**So that** the bug cannot be reintroduced through a different door.

Acceptance criteria:
- Every site listed in `design.md`'s call-site table is updated or explicitly
  justified as unchanged, including the ones **not** named in the brief
  (`api/pkg/sandbox/controller_provision.go:42-43`,
  `api/pkg/cli/sandbox/sandbox.go:264-266`,
  `frontend/src/components/tasks/NewSpecTaskForm.tsx:152,339`,
  `frontend/src/components/agent/CodeAgentExecutionControls.tsx:54-55`,
  `frontend/src/components/session/projectChatItemDetails.ts:65`).
- The org MCP tool description string
  (`api/pkg/org/interfaces/mcptools/spec_tasks.go:56`) and the error message in
  `api/pkg/org/infrastructure/runtime/helix/spectasks.go:207` state the new
  ladder — otherwise org Workers keep passing the old one.
- A grep for `8192` / `16384` / `DefaultSpecTaskSandbox` / `ValidPreset` after the
  change shows no stale sandbox literal outside the shared modules and fixtures.

### US-6 — A backend reconnect resumes the video stream by itself
**As a** user watching a spec task desktop
**I want** the stream to recover on its own when the backend socket drops
**So that** an API restart or RevDial blip does not leave me with a frozen
picture and no error until I reload the page.

Acceptance criteria:
- The proxy captures the client's first text frame and re-sends it to the backend
  after each successful reconnect upgrade.
- Forcing a real mid-stream drop (restart `desktop-bridge` in the desktop
  container) with a browser watching results in video resuming unattended, with
  **no** `failed to read init message` in the desktop-bridge log.
- The mechanism reads as the sibling of `CreateWebSocketUpgradeFunc`'s
  `extraHeaders` handling, whose doc comment already argues this exact principle
  for the other piece of per-connection state.

### US-7 — Replay does not grant privilege or defeat the circuit breaker
**As a** Helix operator
**I want** replay to carry only what is safe to carry
**So that** an embed-key viewer does not gain input, and the shared-video circuit
breaker can still latch.

Acceptance criteria:
- The read-only path (`desktopReadOnlyHeader` / `X-Helix-Readonly`) keeps working
  across replay; a replayed init never re-grants input to an embed-key viewer.
- A replayed init has `user_retry` **cleared**. Only a genuine user-initiated
  retry (the Restart button, on a fresh client socket) resets the breaker via
  `GetSharedVideoRegistry().ResetCircuitBreaker`.
- The replayed init subscribes to the existing shared source rather than tripping
  `[SHARED_VIDEO] Evicted dead source in GetOrCreate`; confirmed by log, not
  assumed.
- Go unit tests in `api/pkg/proxy/` cover "reconnect replays init" and "replayed
  init has `user_retry` cleared".

### US-8 — The work is verified end-to-end, not by unit test alone
Acceptance criteria (from the brief; `CLAUDE.md` rules apply):
- Tested in the inner Helix at `http://localhost:8080`: register
  `test@helix.ml` / `helixtest`, complete onboarding, create a spec task, open its
  detail page, confirm video actually plays.
- `helix spectask stream <session> --duration 30` returns ~1600 frames at
  50–60 FPS at 1920x1080. Zero frames means still broken.
- Part B is exercised through a **real** reconnect, not the happy path — test the
  next operation after the drop, not just the state change.
- `cd frontend && yarn build` and `go build ./pkg/...` pass before committing.
- CI checked after pushing (`gh pr checks`) and failures fixed unasked.
- A design doc lands at
  `design/2026-08-24-sandbox-defaults-and-stream-init-replay.md` in the helix
  repo recording the chosen default, the migration strategy, and the
  `user_retry`-on-replay decision.
- PR/issue links given as full URLs (`https://github.com/helixml/helix/pull/NNN`).

## Non-Goals

- Changing how the sandboxes API (`CreateSandboxDialog`, `controller_provision.go`,
  `cli/sandbox`) picks its **default** size. Luke confirmed its *ladder* moves
  with the rest; its default is a separate product decision — see `design.md` A6.
- Making memory independently selectable from vCPU.
- Restructuring `ResilientProxy` into a frame-aware WebSocket proxy. Only the
  client's first text frame is parsed.
- Fixing the underlying ordering issue that `desktop-bridge` sends `StreamInit`
  only after `streamer.Start()` returns. Part A removes the starvation that made
  that ordering fatal; reordering the handshake is a separate change.

## Prior Art

`spt_01m0evm3dpanc1sfktywbxhes4` ("Raise Default Spec-Task Sandbox to 8 vCPU /
16 GB", 2026-08-20) has been stuck in `spec_review` for four days with no branch
and no PR. Its spec lives at
`helix-specs/design/tasks/002903_default-spec-task/`. It is sound and this task
**lifts it**: its frontend de-duplication plan (`sandboxPresets.ts`), its
generated-OpenAPI fan-out warning, and its test-impact table are reused verbatim
in `design.md`. This task supersedes it and is broader — preset ceiling, operator
config, materialized rows, and Part B. Close `spt_01m0evm3dpanc1sfktywbxhes4` as
superseded rather than implementing both.

## Settled by Luke

These were open at first review and are now **fixed, not suggestions**:

- **Ladder** — 1/2048, 4/8192, 8/16384, **12/24576**, **16/32768**. 2 GB per vCPU
  throughout. vCPU picks the rung; memory stays non-independently-selectable.
- **Default** — 12 vCPU / 24576 MB, still operator-configurable via config, with
  12/24576 as the no-env fallback.
- **Migration scope** — target only rows holding exactly `{"vcpus": 4,
  "memory_mb": 8192}` (31 rows on meta, already hand-backfilled there). Never the
  178 rows holding `{"vcpus": 8, "memory_mb": 16384}`. Keep the "stop
  materializing, resolve at container-create time" shape.
- **Sandboxes-API ladder** — `CreateSandboxDialog.tsx` moves with the rest.
- **One PR, not two** — the spec-task system still makes multiple PRs per task
  awkward to drive. Both parts land on one branch, kept in separate commits so
  either half can be reverted alone.

## Open Questions

1. **Should the migration write `NULL` or the explicit new pair?** Luke's hand
   backfill on meta wrote `{"vcpus": 12, "memory_mb": 24576}`. `design.md` A4
   recommends the shipped migration write **`NULL`** instead, because that is the
   only value consistent with the "a stored override means the user chose this"
   principle Luke re-affirmed in the same decision — a NULLed row tracks every
   future default change, an explicit `12/24576` row is frozen exactly the way
   `4/8192` was. This is a one-word difference in the migration. Consequence
   either way is small; flagging it because it diverges from what meta now holds.
   (Meta's 31 rows can be NULLed separately later if that shape is preferred; the
   migration is a no-op there regardless.)
2. **Should the resolved default be clamped to host CPU count?** Docker rejects
   `--cpus` greater than the host's CPU count outright ("Range of CPUs is from
   0.01 to N.00"), so a 12-vCPU default would fail container creation on any host
   with fewer than 12 cores — breaking the "works out of the box" goal this task
   exists to serve. `design.md` proposes clamping vCPUs to `runtime.NumCPU()` in
   `api/pkg/hydra/devcontainer.go`'s `sandboxResourceLimits` with a warning log.
   Memory needs no clamp (Docker permits limits above host RAM). Confirm this is
   wanted; it is the difference between a safe default and one that breaks small
   installs.
3. **Residual, now small: a deliberate 4/8192 choice is still indistinguishable
   from a materialized default.** Luke's counts bound this to at most 31 rows on
   meta (already hand-backfilled), so it is no longer the blunt instrument it
   looked like at first review. Raised only so other deployments running the
   migration know the semantics: a user who genuinely picked 4 CPU is reset to the
   default. Getting more resources is not harmful and they can re-select 4 CPU.
   Project rows are deliberately **not** touched — projects only store an override
   when an admin explicitly set one. No action expected unless you disagree.
4. **Does the shared pipeline actually survive the drop?** When the backend socket
   dies, `handleStreamWebSocketInternal`'s `defer streamer.Stop()` runs. If that
   client was the last subscriber, the shared source may be torn down, in which
   case the replayed init legitimately creates a new pipeline rather than
   subscribing to an existing one. That is correct behaviour but is a pipeline
   restart, and under Part A's starvation it took 43 s. This must be **observed in
   the log during verification**, not assumed. If teardown is refcount-immediate,
   a short linger on the shared source may be a follow-up.
