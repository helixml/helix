# Desktops under heavy CPU load: false MCP startup failures and stream reconnect churn

2026-10-01. node01 ran at load ~300 on 48 cores with 10 desktops booting at
once. Two bugs turned "slow" into "broken". Both are fixed so that load
degrades to "slow but eventually works".

## Bug 1 — the user's turn failed before the real MCP gate decided

### What happened

| UTC      | event |
|----------|-------|
| 05:37:44 | container start |
| ~05:38:45| settings-sync-daemon boot probe starts (`waitForMCPEndpoints`, 60s) |
| 05:39:43 | GNOME Shell starts (CPU-starved) |
| 05:40:45 | boot probe gives up → `reportAgentStartupError` → interaction `error` |
| 05:41:37 | desktop-bridge `HTTP server starting port=9876` |
| 05:41:38 | `start-zed-core.sh` gate (180s) passes, Zed launches with tools |

The agent was healthy; the user's message was dead (errored interactions are
never retried).

### Root causes

1. **Two decision points.** The daemon's advisory boot probe had a 60s budget,
   the authoritative shell gate 180s, and the advisory one could fail the turn.
2. **Wrong diagnosis.** `helix-desktop` is served by the container's own
   desktop-bridge. desktop-bridge starts its RevDial client immediately but
   only binds :9876 after GNOME session setup, so the API's tunnel dial
   succeeds and the request then reads EOF → 502 "failed to read response from
   desktop". The probe blamed "the Helix API behind the sandbox proxy". The API
   was up throughout.
3. **Wrong clock.** The gate's deadline started at script start, not when the
   bridge it depends on was listening.

### Fix

- **One decision point**: `desktop/shared/start-zed-core.sh`
  `wait_for_mcp_endpoints`. The daemon no longer probes at boot. When the gate
  gives up it POSTs its last verdict to the daemon's
  `/mcp-readiness/gave-up`, which calls `reportAgentStartupError`. Nothing
  else fails a turn over MCP readiness.
- **Pending vs failed**: the daemon's probe for `helix-desktop` first checks
  the local bridge (`127.0.0.1:$SCREENSHOT_PORT/health`). Not listening →
  `/mcp-readiness` answers **425** ("dependency still starting") instead of
  503. A pending bridge never masks a real failure on another server.
- **Two clocks in the gate**: 425 (and daemon-not-listening) accrue against
  `DEP_MAX_WAIT=300s` (the API's own cold-start grace, after which auto-wake
  recreates a container whose agent never connected); anything else against `MCP_MAX_WAIT=180s`, which
  therefore only runs once the desktop is up.
- **Distinct API answer**: `mcp_backend_desktop.go` returns 503 +
  `X-Helix-Desktop-Unavailable: not-connected|not-listening` when it cannot
  reach the bridge, vs the sandbox proxy's own 502 "Helix API unavailable".
  The probe quotes the response body rather than guessing which hop failed.
- **After giving up the gate keeps waiting** (10s cadence) and launches Zed
  if the servers recover, so the next message works instead of the desktop
  staying dead until restarted. It still never launches Zed without tools.

## Bug 2 — "Connection stale" reconnect loop while the pipeline starts

### What happened (`stream-startup-under-load.log`)

- 05:42:23 WS connected, init received, `[SHARED_VIDEO] Starting pipeline`.
- Nothing sent to the client; `[GST_PIPELINE] Starting pipeline` only at
  05:43:38 — **75s inside `NewGstPipelineWithOptions`** (parse + plugin load).
- Client stale timeout (10s of no messages) → close → reconnect.
- 05:42:45 reconnect → `Evicted dead source in GetOrCreate` → `Stopping source`
  on the in-flight one → second pipeline queued behind the first.
- 05:43:51 first conn gets `ConnectionComplete` → broken pipe.
- First keyframe 05:44:18 (~95s after first connect).

### Root causes

1. The handler called `streamer.Start()` synchronously before its read loop:
   for the whole pipeline construction nothing was written to the client and
   nothing was read from it (client RTT pings unanswered, the server's 30s
   read deadline not extended). Server WebSocket pings are invisible to browser
   JS, so the client correctly saw a silent socket.
2. `SharedVideoSourceRegistry` considered any source with `running=false`
   dead, including one whose start was in progress.
3. Pre-existing: `start()` overwrote `startErr` with nil on every no-op call
   after the first, and `stop()` racing an in-flight start could leak the
   pipeline that start went on to create.

### Fix

- `ws_stream.go`: heartbeat goroutine starts as soon as init is received; it
  sends WS pings every 5s and a JSON `{"StreamStatus":{"state":"starting_video","elapsed_ms":N}}`
  every 2s until the first frame arrives. `Start()` runs in a goroutine beside
  the read loop, so client pings get pongs throughout. A handler whose client
  left while `Start()` waited bails out (its context is cancelled before the
  deferred `Stop()` waits) instead of registering presence with the same
  `client_unique_id`, which would evict the reconnected live viewer. When a
  viewer's shared source goes away the socket is dropped so the client
  reconnects, rather than being held open by the heartbeat.
- Frontend: `videoStarting` info event → overlay shows "Starting video… (Ns)";
  the 15s video-start timeout is re-armed by each status (it means "no
  progress for 15s", not "no video 15s after ConnectionComplete"). Stale
  detection is unchanged and correct: 10s of no messages at all.
- `shared_video_source.go`: `alive()` = not stopped and (running or start not
  finished). `GetOrCreate`/`GetExisting`/`GetOrCreateWithSource` attach to an
  alive source; a second client's `Subscribe` waits on `startMu` for the
  in-flight start and then catches up. `stop()` waits out an in-flight start
  before tearing down; `start()` refuses after `stop()`; `startErr` is
  recorded once. The grace-period `doStop` spares a source a client joined
  after the stop was scheduled. pipewirezerocopysrc's 10s first-frame
  timeout (`ErrNoFirstFrame`) restarts the pipeline up to 5 times while
  viewers are attached and no frame has ever arrived, instead of ending the
  stream with an error banner.

## Why construction took 75s — findings

Measured on this host (16 cores, 1× RTX 2000 Ada) and in inner-Helix desktops:

| what | condition | time |
|---|---|---|
| `gst-inspect-1.0 nvh264enc`, fresh registry (full plugin scan) | host load ~265 | 61s |
| same, cached registry (in-process nvcodec load only) | host load ~265 | 3.5–5.8s |
| `fakesink` / `pipewirezerocopysrc` inspect, cached registry | host load ~265 | 0.06–0.08s |
| desktop-bridge `InitGStreamer()` at boot | inner desktop, moderate load | 17s |
| same | desktop throttled to 0.1–0.5 CPU | 10m8s |
| nvcodec in-process preload | throttled inner desktop | 12.8s |
| `NewGstPipelineWithOptions` (parse) before fix | 0.1 CPU | 28s |
| same after nvcodec preload | 0.1 CPU | 0.3s |
| pipeline `SetState(PLAYING)` | 0.1 CPU | 13–94s |

- **No registry cache is baked into the image.** `~/.cache/gstreamer-1.0/registry.x86_64.bin`
  lives in the container's writable layer and is rebuilt on every container
  start (outer desktop: written 5m20s after container start). The scan runs
  in desktop-bridge's `InitGStreamer()`, which used to block its HTTP listener
  (and so helix-desktop MCP — Bug 1) until done.
- **CUDA/NVENC init:** the scan happens in `gst-plugin-scanner`, so nvcodec's
  `plugin_init` (CUDA init + NVENC capability probing per GPU) runs *again*
  in-process on first use, i.e. inside the first viewer's pipeline parse.
  Scales with GPU count and contends with every other desktop doing the same.
- **Fix shipped (cheap):** `WarmUpGStreamer` runs init + `gst_plugin_load_by_name("nvcodec")`
  in the background at bridge start (under `pipelineCreateMu`). The HTTP
  listener binds without waiting (8.5s vs 17s+ in a booting desktop) and
  first pipeline parse dropped from 28s to 0.3s at 0.1 CPU.
- **Not done:** baking the registry at image build. The build has no GPU, so
  nvcodec would be cached with zero features; whether GStreamer rescans it on
  a GPU host depends on dependencies nvcodec declares, which I did not
  verify. Worth doing with that check — it removes a 17s–10min CPU-bound scan
  from every boot.
- **Remaining:** under extreme starvation `SetState(PLAYING)` (CUDA context
  + PipeWire negotiation) is still tens of seconds, and the pre-existing 30s
  stall restart can fire before a starved compositor delivers anything. The
  client now rides this out ("Starting video… (Ns)") instead of churning.

## Interplay with auto-wake

`auto_wake_stuck_interactions.go` recreates a container whose agent has not
connected within `coldStartGracePeriod` (5 min). The gate's dependency budget
is sized to that envelope; past it the API's restart, not the gate, is what
acts on a stuck boot.
