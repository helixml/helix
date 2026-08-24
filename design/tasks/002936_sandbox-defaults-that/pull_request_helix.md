# Right-size spec-task sandbox defaults and replay stream init on reconnect

## Summary

Two defects from one outage on 2026-08-24 (task `spt_01m0s0vktb0twdtwz7cmk6wgtg`,
session `ses_01m0s0vkvq89563fdkxza4cry6`). The symptom was "video streaming is
broken". It was not a video bug. Part A and Part B are separate commits so either
half can be reverted alone. Full reasoning:
`design/2026-08-24-sandbox-defaults-and-stream-init-replay.md`.

### ⚠️ Contains an irreversible data migration

`0009_unmaterialize_spec_task_sandbox_default` NULLs
`spec_tasks.sandbox_resource_overrides` on rows holding **exactly**
`{"vcpus": 4, "memory_mb": 8192}`. It deliberately never matches
`{"vcpus": 8, "memory_mb": 16384}` — 178 such rows on meta are real user choices,
not materialized defaults. The down migration is a documented no-op: which NULLs
previously held 4/8192 is not recorded, and re-materializing all NULLs would
write an override onto the 4102 rows that never had one.

Run this before and after, and confirm the 8/16384 total is unchanged:

```sql
SELECT sandbox_resource_overrides, count(*) FROM spec_tasks GROUP BY 1;
```

On meta the migration is a **no-op** — those 31 rows were hand-backfilled to
12/24576 on 2026-08-24 and no longer match the old pair.

### Part A — the default sandbox was below the real workload

The desktop container sat at 99.999% of its 8 GiB `memory.max` with CFS
throttling on 80% of periods. Under that starvation GStreamer's
`set_state(PLAYING)` took 43–100 s instead of under a second, so the browser
timed out waiting for `StreamInit` and every retry leaked another pipeline.
`docker update --memory 24g --memory-swap 48g --cpus 12` fixed it instantly with
nothing else changed.

**New default: 12 vCPU / 24576 MB** — exactly that configuration. Treating
containers pinned at their cap as *censored* observations rather than
measurements, p90 real demand is ≈21 GiB, so the "p90 must sit under 90% of its
ceiling" rule forces a ceiling of at least 24 GiB; 16 GiB would put the p90 at
~131% of its ceiling.

- **Ladder:** `1/2048`, `4/8192`, `8/16384`, **`12/24576`**, **`16/32768`**.
  2 GB per vCPU throughout. Every pre-existing rung stays valid, so no stored
  value is retroactively rejected by `ValidPreset`.
  `SpecTaskSandboxPresetForVCPUs` and `ValidPreset` now derive from one table
  instead of two hand-maintained switches.
- **Operator-configurable:** `HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS` /
  `_MEMORY_MB`, defaulting to 12/24576 so out-of-the-box behaviour is correct
  with no env set. An invalid pair fails startup rather than being ignored.
- **CPU clamp:** Docker rejects `--cpus` above the host CPU count outright, so a
  12-vCPU default would break container creation on any smaller host — defeating
  the point of this change. `sandboxResourceLimits` clamps to `runtime.NumCPU()`
  and logs when it does. Memory is deliberately not clamped; Docker accepts a
  limit above host RAM, and an unreachable ceiling beats one that OOM-kills.
- **Materialization:** both create paths now store `nil` when nobody chose a
  size, and the default resolves at container-create time. A stored override now
  means "someone chose this", not "this was the default the day the row was
  written".
- Every duplicated copy of the ladder moved with it, including five sites the
  brief did not list and the org MCP tool description prose (Workers read that
  string literally and would keep passing the old ladder).

Two gaps that only end-to-end testing caught — both would have shipped the
original bug at a new value:

- The **frontend kept re-materializing the default**: both create forms
  initialised their state to it and always sent it, so rows still came back with
  `{"vcpus": 12, "memory_mb": 24576}` written on them. Removing a materialized
  default server-side is only half the job if a client keeps sending it.
- The UI's **"· Default" marker lied to operators**: it came from a hardcoded
  frontend constant, so with the env override set the selector marked 12 vCPU
  while containers came up at 8/16384. `/api/v1/config` now carries
  `default_spec_task_sandbox`.

### Part B — the stream proxy never replayed `init`

`ResilientProxy` re-dials the backend transparently but redid only the HTTP
upgrade. `desktop-bridge` blocks on a mandatory `init` frame with a 30 s read
deadline, so every reconnect produced a backend socket that waited 30 s, died and
reconnected — while the *client* socket stayed open, showing a live-but-frozen
picture with **no error surfaced anywhere**. Only a page reload escaped it.
Part A caused the drops in this outage; this fires on any future drop.

`SessionReplay` is the frame-level sibling of `CreateWebSocketUpgradeFunc`'s
`extraHeaders`, whose doc comment already argues this exact principle for the
other piece of per-connection state. It carries a minimal RFC 6455 client-frame
parser — enough to find the first text frame and no more; `ResilientProxy`
otherwise stays a raw byte proxy.

**A replayed init has `user_retry` cleared.** It is the only thing that clears a
latched shared-video circuit breaker, and a proxy reconnect is by definition
automatic. Preserving it would mean one press of Restart re-asserts it on every
later drop, so the breaker could never latch — the retry storm it exists to stop.
The Restart button is unaffected: it opens a new client socket whose init is not
a replay.

Read-only is unaffected: `StreamConfig` has no privilege field, so replay has no
lever to pull; privilege rides on `X-Helix-Readonly`, which the upgrade func
already re-sends on every reconnect. Both halves are now covered by tests,
including that header-on-reconnect invariant, which the doc comment asserted but
nothing tested.

**Known limitation, documented not fixed:** `ResilientProxy` buffers bytes, not
frames, so a drop mid-frame can desync the resumed stream. Both causes
(`copyClientToServer` discarding `Write`'s byte count; 32KB reads not aligning
with frame boundaries) predate this change, and fixing it properly means making
the byte proxy frame-aware in the hot path of every proxied byte.

## Testing

Tested end-to-end in the inner Helix at `localhost:8080` against a real spec task,
not by unit test alone.

**Part A**

- Fresh task row: `sandbox_resource_overrides` is `NULL` — the default is no
  longer materialized.
- Its container:
  `Memory=25769803776 NanoCpus=12000000000 MemorySwap=51539607552` — 24 GiB /
  12 vCPU / 48 GiB swap, exactly the configuration that fixed the outage.
- `helix spectask stream --duration 30` → **1894 frames, 60 instant / 63.1 avg
  FPS** at 1920x1080.
- Selector renders all five rungs with 12 vCPU / 24 GB marked "· Default".
- Config override to 8/16384: `/api/v1/config` reports it and the Default marker
  moves to the 8 vCPU rung. Invalid pair 6/12288 refuses startup with
  `default spec task sandbox 6 vCPU / 12288 MB is not a valid preset (vCPUs must
  be one of 1, 4, 8, 12, 16, with memory following)`. Env removed → back to
  12/24576.
- Migration against seeded data covering all three real-world cases: both 4/8192
  rows became `NULL` (including a differently-key-ordered one, which `jsonb`
  normalises), `8/16384` and a near-miss `4/4096` were untouched, and a second
  run changed nothing.

**Part B — a real reconnect, not the happy path.**

Killing `desktop-bridge` does **not** exercise the replay path: it hosts the
RevDial client, so the proxy fails to re-dial and the browser simply opens a
fresh socket. The give-away is `reconnect_count=0`, and believing that first run
would have been a false positive. Dropping only the backend TCP socket
(`ss -K state established '( sport = :9876 )'`) gives:

```
API    Server connection error … close 1006 (abnormal closure): unexpected EOF
API    Replayed session state to reconnected backend  bytes=297
API    Reconnected successfully, resuming proxy  reconnect_count=1
bridge stream WebSocket connected               11:44:24.975
bridge stream init received … user_retry=false  11:44:24.976   ← 1 ms, not 30 s
bridge [SHARED_VIDEO] Client subscribed (grace period reconnection, starting catchup)
```

- `failed to read init message` across the entire bridge log: **0**.
- Subscribed to the **existing** shared source — no `Evicted dead source`, so no
  pipeline restart and no leak. The registry's 60 s grace period covers exactly
  this window.
- Video resumed unattended with no page reload; a post-reconnect stream check
  returned 1844 frames at 61.5 avg FPS.

**Not live-tested:** a read-only embed-key viewer across a reconnect. Covered
structurally instead — `StreamConfig` has no privilege field, plus new tests that
the replayed payload carries none and that the upgrade func re-emits
`X-Helix-Readonly` on every upgrade.

**Build and suites:** `go build ./pkg/...` and the frontend build both pass. All
affected Go packages and the full vitest suite pass. Two failures on this box —
`TestDiskPressureSuite` (no `zpool` binary) and `MonacoEditorClipboard` (a
suite-ordering flake) — were confirmed identical on `origin/main` via a temporary
worktree, so they are pre-existing and unrelated.

## Screenshots

![Ladder with 12 vCPU / 24 GB marked Default](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002936_sandbox-defaults-that/screenshots/01-sandbox-ladder-12-default.png)
![Video playing on the task detail page](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002936_sandbox-defaults-that/screenshots/02-task-desktop-video.png)
![Config override moves the Default marker to 8 vCPU](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002936_sandbox-defaults-that/screenshots/03-config-override-8vcpu-default.png)
![Video live after a real proxy reconnect](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002936_sandbox-defaults-that/screenshots/05-video-live-after-proxy-replay.png)

## Supersedes

`spt_01m0evm3dpanc1sfktywbxhes4` ("Raise Default Spec-Task Sandbox to 8 vCPU /
16 GB", stuck in `spec_review` since 2026-08-20). Its frontend de-duplication
plan and OpenAPI fan-out warning are lifted here; it should be closed as
superseded rather than implemented separately.
