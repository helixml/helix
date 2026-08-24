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
- The global default sandbox size is raised from 4 vCPU / 8192 MB to a value
  justified by the node01 usage sample in `design.md`.
- The chosen default leaves the p90 observed task at **under 90%** of its memory
  ceiling.
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
- `SpecTaskSandboxPresetForVCPUs` and `ValidPreset`
  (`api/pkg/types/simple_spec_task.go:117-144`) accept at least one rung above
  8 vCPU / 16384 MB.
- Memory remains **not** independently selectable — vCPU stays the preset key and
  memory is derived from it.
- All existing rungs (1/2048, 4/8192, 8/16384) remain valid, so no stored value
  is retroactively rejected by `ValidPreset`.
- The new rungs are selectable in the UI (`SpecTaskExecutionControls`,
  `CodeAgentExecutionControls`) and via the org MCP `sandbox_vcpus` argument.

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

Acceptance criteria:
- New tasks that did not specify a size and had no project default store **no**
  sandbox override; the default is resolved at container-create time.
- Existing rows holding exactly the old default are backfilled so they resolve to
  the live default.
- A task where the user *deliberately* chose a size, or where a project default
  applied, keeps that value. A stored override means "someone chose this".
- Verified live: a task row created *before* the change starts a container with
  the new limits.

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
  `cli/sandbox`) picks its **default** size. Its ladder is extended for
  consistency; its default is a separate product decision — see `design.md`.
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

## Open Questions

1. **Is 12 vCPU / 24 GB the right default, or is 8 vCPU / 16 GB preferred despite
   the p90 rule?** `design.md` recommends 12/24576 because it is literally the
   configuration proven live to fix the outage, and because 16 GB leaves the p90
   task at ~131% of its ceiling. The cost is density: a 12-vCPU default is the
   entire CPU allocation of this 12-core box, and on a smaller host it must be
   clamped (see Q2). If density matters more than headroom, say so and we ship
   8/16384 with 12/24576 and 16/32768 as selectable rungs instead.
2. **Should the resolved default be clamped to host CPU count?** Docker rejects
   `--cpus` greater than the host's CPU count outright ("Range of CPUs is from
   0.01 to N.00"), so a 12-vCPU default would fail container creation on any host
   with fewer than 12 cores — breaking the "works out of the box" goal this task
   exists to serve. `design.md` proposes clamping vCPUs to `runtime.NumCPU()` in
   `api/pkg/hydra/devcontainer.go`'s `sandboxResourceLimits` with a warning log.
   Memory needs no clamp (Docker permits limits above host RAM). Confirm this is
   wanted; it is the difference between a safe default and one that breaks small
   installs.
3. **Is the backfill migration acceptable as a blunt instrument?** The proposed
   migration NULLs `spec_tasks.sandbox_resource_overrides` where it equals exactly
   `{"vcpus":4,"memory_mb":8192}`. Between 2026-08-10 and today a materialized
   default and a deliberate 4/8192 choice are **indistinguishable in the data**, so
   a user who genuinely wanted 4/8192 would be reset to the default. The argument
   for doing it anyway: getting more resources is not harmful, and the user can
   re-select 4 CPU at any time. Project rows are deliberately **not** touched
   (projects only store an override when an admin explicitly set one).
4. **One PR or two?** `design.md` recommends two — Part A (defaults + config +
   DB migration + ~12 call sites) and Part B (wire-protocol replay) have
   different blast radii, different reviewers, and should be independently
   revertable. PR2 branches off PR1 so the joint end-to-end verification runs on
   the combined tree. Say if a single PR is preferred.
5. **Does the shared pipeline actually survive the drop?** When the backend socket
   dies, `handleStreamWebSocketInternal`'s `defer streamer.Stop()` runs. If that
   client was the last subscriber, the shared source may be torn down, in which
   case the replayed init legitimately creates a new pipeline rather than
   subscribing to an existing one. That is correct behaviour but is a pipeline
   restart, and under Part A's starvation it took 43 s. This must be **observed in
   the log during verification**, not assumed. If teardown is refcount-immediate,
   a short linger on the shared source may be a follow-up.
6. **Should the sandboxes-API ladder (`CreateSandboxDialog`,
   `controller_provision.go`, `cli/sandbox`) move too?** `design.md` extends its
   rungs to match but leaves its default alone. Confirm that split is what you
   want, or say whether the sandboxes surface should be left entirely untouched.
