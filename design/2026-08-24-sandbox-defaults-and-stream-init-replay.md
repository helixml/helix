# Sandbox defaults that work out of the box, and stream `init` replay on reconnect

2026-08-24. Two defects from one outage (meta / node01, task
`spt_01m0s0vktb0twdtwz7cmk6wgtg`, session `ses_01m0s0vkvq89563fdkxza4cry6`). The
symptom was "video streaming is broken". It was not a video bug.

## The outage

The desktop container sat at 99.999% of its 8 GiB `memory.max`, with
`memory.pressure full avg60=55.6%` and `cpu.stat nr_throttled 20319/25366`. Under
that starvation GStreamer's `set_state(PLAYING)` took 43–100 s instead of under a
second. `desktop-bridge` sends `StreamInit` only *after* `streamer.Start()`
returns, so the browser timed out, closed, and every later write hit
`broken pipe`. Each retry sent a fresh `init`, tripping
`[SHARED_VIDEO] Evicted dead source in GetOrCreate` and leaking pipelines. It
never converged.

`docker update --memory 24g --memory-swap 48g --cpus 12` on the live container
took CPU from 397% (CFS-throttled at the 4-vCPU cap) to 1213%, the pipeline
started in the same second, and `helix spectask stream --duration 30` returned
1638 frames at 54.6 FPS. Nothing else changed.

Underneath that, a second bug: the stream proxy never replays `init` when it
re-dials the backend, so *any* backend drop leaves the stream permanently dead
until the user reloads the page.

---

## Part A — the chosen default: 12 vCPU / 24576 MB

### Why 24 GB

Steady-state memory of desktop containers on node01, sampled 2026-08-24:

| container | usage | limit |
|---|---|---|
| `01m0sbr51axfh4gmm30xct6f0k` | 7.29 GiB | 8 GiB |
| `01m0s0vkvq89563fdkxza4cry6` | 7.89 GiB | 8 GiB ← the outage |
| `01m0cm7thsmwf3aydezsntg2rg` | 8.32 GiB | 16 GiB |
| `01kzzym0gtzhg57e891mj1466x` | 6.57 GiB | 16 GiB |
| `01kz6r8hvd8b2eav84r5cryfs3` | 5.56 GiB | 16 GiB |
| `01kzdznnqrvwfwf0ppgf098tmk` | 9.67 GiB | uncapped |
| `01kz6r9czzj9tzs1twxn0ff1qd` | **29.75 GiB** | uncapped |

A container pinned at its limit is a **censored** observation, not a
measurement — its real demand is `≥` what is shown. Treating the two 8 GiB rows
that way, sorted lower bounds are 5.56, 6.57, ≥7.29, ≥7.89, 8.32, 9.67, 29.75,
putting p90 at roughly **21 GiB** — itself a lower bound.

The rule we set was that the p90 task must not sit inside 90% of its ceiling.
21 / 0.9 = 23.3 GiB, so the ceiling has to be at least 24 GiB. 16 GiB would put
the p90 at ~131% of its ceiling, i.e. an OOM rather than a margin.

12 vCPU comes from the measurement, not extrapolation: at the 4-vCPU cap the
container was throttled on 80% of periods, and raising it to 12 took observed
utilisation to 1213%.

So the default is exactly the configuration proven live to fix this outage.

### The ladder

`1/2048`, `4/8192`, `8/16384`, **`12/24576`**, **`16/32768`** — 2 GB per vCPU
throughout. `SpecTaskSandboxPresetForVCPUs` and `ValidPreset` now derive from one
`SpecTaskSandboxPresets` table so they cannot drift.

All pre-existing rungs stay valid, deliberately. Re-keying 8 from 16384 to 24576
(so the default could stay at 8 vCPU) would make every stored
`{"vcpus": 8, "memory_mb": 16384}` fail `ValidPreset()` and be rejected on the
next update of that row — 178 such rows on meta, all deliberate user choices.

### Docker rejects a CPU limit above the host

`--cpus` greater than the host CPU count is a hard error
(`Range of CPUs is from 0.01 to N.00`), so a 12-vCPU default would fail container
creation outright on any smaller host — defeating the point of this change.
`sandboxResourceLimits` now clamps vCPUs to `runtime.NumCPU()` and logs when it
does. Memory is deliberately **not** clamped: Docker accepts a limit above host
RAM, and an unreachable ceiling beats one that OOM-kills the desktop.

### Operator configuration

`HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS` / `_MEMORY_MB` on `config.Sandboxes`,
defaulting to 12/24576 so out-of-the-box behaviour is correct with no env set.
An invalid pair fails startup rather than being ignored:

```
failed to load server config: HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS/_MEMORY_MB:
default spec task sandbox 6 vCPU / 12288 MB is not a valid preset
(vCPUs must be one of 1, 4, 8, 12, 16, with memory following)
```

Installed as a process-wide value from `LoadServerConfig` rather than threaded
through constructors: `DefaultSpecTaskSandboxResources()` is read from packages
with no config handle at all (`desktopBillingResources` is a free function,
`HydraExecutor` holds no `ServerConfig`), and the value is immutable after
startup.

### Migration strategy for materialized rows

Commit `1eff4e801` (2026-08-10) made `CreateTaskFromPrompt` write the default
onto the row. The brief assumed every task since was affected; the real counts on
meta were:

| rows | value | action |
|---|---|---|
| 4102 | `NULL` | nothing to do |
| 178 | `{"vcpus": 8, "memory_mb": 16384}` | **never touch** — real user choices |
| 31 | `{"vcpus": 4, "memory_mb": 8192}` | the only stale rows |

**Both halves of the fix, because either alone is insufficient:**

1. **Stop materializing.** Both create paths now store `nil` when neither the
   request nor the project chose a size. `resolveSpecTaskLaunchConfig` already
   resolves `nil` to the live default at container-create time. A stored override
   now means "someone chose this", not "this was the default the day the row was
   written".
2. **Backfill** (`0009_unmaterialize_spec_task_sandbox_default`) NULLs rows
   holding *exactly* the old pair. Writing `NULL` rather than the new pair is the
   point: a NULLed row tracks every future default change, whereas an explicit
   `12/24576` would be frozen exactly the way `4/8192` was, recreating this
   ticket the next time the default moves.

The predicate matches the exact object and nothing else. Verified on seeded data:
both 4/8192 rows (including a differently-key-ordered one, which `jsonb`
normalises) became `NULL`; `8/16384` and a near-miss `4/4096` were untouched;
re-running changed nothing.

Project rows are deliberately not touched — a project only stores an override
when an admin explicitly set one.

### Two gaps that only end-to-end testing found

**The frontend was re-materializing the default.** With the server fixed, new
rows *still* came back with `{"vcpus": 12, "memory_mb": 24576}` written on them,
because both create forms initialised their state to the default and
`buildNewChatTaskRequest` sends the field whenever it is truthy. Same freeze,
new value. Both forms now hold `undefined` until the user picks a size; the
selectors already render the default for an undefined value, so the UI is
unchanged.

**The UI's "· Default" marker lied to operators.** The marker came from a
hardcoded frontend constant, so with `HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS=8`
set the selector marked 12 vCPU as Default while containers came up at 8/16384.
`/api/v1/config` now carries `default_spec_task_sandbox` and
`useDefaultSandboxPreset` reads it.

Generalising: making a value operator-configurable on the server is only half the
job if a client renders its own copy of it, and removing a materialized default
on the server is only half the job if a client keeps sending it.

---

## Part B — replay `init` on reconnect

### The bug

`proxy.ResilientProxy` re-dials the backend transparently, but
`CreateWebSocketUpgradeFunc` redoes only the HTTP upgrade. `desktop-bridge`
blocks on a mandatory `{"type":"init"}` text frame with a 30 s read deadline
before it will start a streamer. The browser sent that frame once, on the
original socket. So every reconnect produced a backend socket that waited 30 s,
died and reconnected — eight cycles in the outage log — while the *client* socket
stayed open, showing a live-but-frozen picture with no error surfaced anywhere.

### The mechanism

`SessionReplay` is the frame-level sibling of `CreateWebSocketUpgradeFunc`'s
`extraHeaders`. That doc comment already argues the principle — headers belong on
the upgrade func *because it also runs on reconnect*, and dropping them "would
silently restore the privilege mid-session". `init` has the identical
requirement and was missed. The header path re-sends what *we* know about the
caller; this path re-sends what the *client* told the backend.

`StreamInitReplay` carries a minimal RFC 6455 client-frame parser — enough to
find the first text frame in the client→server direction and no more.
`ResilientProxy` is otherwise a raw byte proxy and deliberately stays that way.
Client frames are masked, so the replay is re-encoded with a fresh mask key
(required anyway, since the payload changes).

The replay is written to the new backend **before** the buffered input: the
backend is blocked in its init read and processes nothing else first.

### Decision: a replayed init has `user_retry` cleared

`user_retry` marks an explicit user-initiated retry (the Restart button) and is
the only thing that clears a latched shared-video circuit breaker. A proxy
reconnect is by definition automatic. If replay preserved the flag, one press of
Restart would re-assert it on every subsequent backend drop, the breaker would
reset each time, and **it could never latch** — which is exactly the retry storm
the breaker exists to stop, and exactly what the outage log shows. The Restart
button is unaffected: it opens a new client socket whose init is not a replay.

The payload is decoded to `map[string]any`, not `desktop.StreamConfig`.
Round-tripping the struct would drop fields a newer browser sends that this build
does not know about, and re-emit `omitempty` fields the client deliberately
omitted. Everything except `user_retry` passes through unchanged.

Malformed JSON disables replay rather than being re-sent verbatim — replaying
bytes we cannot read risks re-asserting `user_retry`.

### Read-only safety

`StreamConfig` has no privilege field. Read-only is enforced entirely by
`X-Helix-Readonly`, set server-side on the upgrade and already re-applied on
every reconnect. Replay therefore *cannot* re-grant input to an embed-key
viewer — there is no lever in the payload to pull. Both halves are now covered by
tests, including the header-on-reconnect invariant, which the doc comment
asserted but nothing tested.

### The shared pipeline survives

A real backend drop produced
`[SHARED_VIDEO] Client subscribed (grace period reconnection, starting catchup)`
— not `Evicted dead source in GetOrCreate`, not `Created new source`. The
registry's 60 s grace period holds the source alive across the reconnect window,
so the replayed init re-subscribes to the live pipeline rather than restarting
it. No leak, no 43 s stall.

### Known limitation: frame boundaries across a drop

`ResilientProxy` buffers bytes, not frames. `copyClientToServer` discards
`Write`'s returned byte count and re-buffers the whole chunk, and 32KB reads do
not align with frame boundaries, so a drop mid-frame can desync the resumed
stream. Both causes predate this change. Fixing it properly means making the byte
proxy frame-aware in the hot path of every proxied byte; for this stream every
client→server frame is small (13-byte keepalives, input events), so desync needs
a frame split across TCP segments *and* a write failure inside that window.
Documented rather than fixed.

---

## Verification

All in the inner Helix at `localhost:8080`, against a real spec task.

**Part A**

- Fresh task row: `sandbox_resource_overrides` is `NULL`.
- Its container:
  `Memory=25769803776 NanoCpus=12000000000 MemorySwap=51539607552` — 24 GiB,
  12 vCPU, 48 GiB swap. Exactly the configuration that fixed the outage.
- `helix spectask stream --duration 30` → 1894 frames, 60 instant / 63.1 avg FPS
  at 1920x1080.
- Selector shows all five rungs with 12 vCPU / 24 GB marked "· Default".
- Config override to 8/16384: `/api/v1/config` reports it, the "· Default" marker
  moves to the 8 vCPU rung. Invalid pair 6/12288: startup refuses with the
  message quoted above. Env removed: back to 12/24576.
- Migration on seeded data: 4/8192 → NULL (including key-order variant),
  8/16384 and 4/4096 untouched, second run a no-op.

**Part B**

Forcing a real reconnect needs care. **Killing `desktop-bridge` does not exercise
the replay path** — it hosts the RevDial client, so `dialFunc` fails all three
attempts, the proxy gives up, and the *browser* opens a fresh socket with its own
init. The give-away is `reconnect_count=0`. Drop only the backend TCP socket
instead:

```bash
docker exec helix-sandbox-nvidia-1 docker exec <container> \
  ss -K state established '( sport = :9876 )'
```

Observed chain:

```
API    Server connection error, attempting reconnection
         error="websocket: close 1006 (abnormal closure): unexpected EOF"
API    Replayed session state to reconnected backend  bytes=297
API    Reconnected successfully, resuming proxy  reconnect_count=1
bridge stream WebSocket connected            11:44:24.975
bridge stream init received … user_retry=false  11:44:24.976   ← 1ms, not 30s
bridge [SHARED_VIDEO] Client subscribed (grace period reconnection, starting catchup)
```

`failed to read init message` across the entire bridge log: **0**. Video resumed
unattended with no page reload; `helix spectask stream --duration 30` after the
reconnect returned 1844 frames at 61.5 avg FPS.

## Notes for whoever touches this next

- **vCPU is the primary key of a sandbox preset.** Memory is always derived from
  it. Never make memory independently selectable.
- **`ValidPreset()` guards requests, never reads.** Nothing validates a preset
  loaded from the DB, and `sandboxResourceLimits` just multiplies. A row can hold
  a size no rung represents: the backend honours it and the frontend silently
  renders no selection. `sandboxPresetsFor()` now surfaces such a value instead
  of dropping it.
- **Docker rejects `--cpus` above host CPU count but accepts memory above host
  RAM.** That asymmetry is why CPU needs clamping and memory does not.
- **`ResilientProxy` is a raw byte proxy** with no frame awareness. Anything
  needing frame semantics brings its own parser.
- When sizing anything from `docker stats`, a container pinned at its limit is a
  censored observation, not a measurement.
