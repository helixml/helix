# Stream Reconnect Storm Root-Cause Investigation

**Date:** 2026-05-21
**Status:** Investigation — hypotheses narrowed; needs browser logs to disambiguate H1 vs H2
**Reporter:** Luke
**Subject session:** `spt_01ks63ax30bfk91t9faec1cp3w` (URL) → `ses_01ks63b0btvj0ax8kzsy43xr4r` (external-agent session) → desktop container `ubuntu-external-01ks63b0btvj0ax8kzsy43xr`
**Subject client_id:** `7307a64c-7165-4a7c-aa4b-4906e2e592f2` (single browser tab)

Prior context: `design/2026-04-12-frontend-reconnect-loop-investigation.md` (different mechanism — fixed in `f8c926cad`).

---

## Symptom

The "Stats for Nerds" panel shows a perfect 5-7s reconnect cycle while video is otherwise flowing at 61fps:

```
21:38:43 WebSocket opened
21:38:43 Connection complete
21:38:47 Error: WebSocket error
21:38:47 Disconnected unexpectedly (code: 1006)
21:38:48 WebSocket opened
21:38:48 Connection complete
21:38:54 Error: WebSocket error
21:38:54 Disconnected unexpectedly (code: 1006)
21:38:55 WebSocket opened
21:38:56 Connection complete
```

Despite the visible reconnects, the GPU pipeline never sees a real outage — `pipeline_received` increases monotonically in `VIDEO LATENCY STATS` while `ws_frames` resets ~every 5s.

---

## Evidence

### Server (API) — `pkg/server/external_agent_handlers.go`

Each cycle on the API:

```
INF Stream WebSocket connection established        proxy_session_id=A
INF Superseding previous stream proxy for same client  client_id=7307a64c…  new_proxy=B  old_proxy=A
INF Proxying stream WebSocket to screenshot-server via RevDial  proxy_session_id=B
WRN Resilient proxy ended with error               proxy_session_id=A  error="context canceled"
INF Stream WebSocket connection closed             proxy_session_id=A  reconnect_count=0
INF Stream WebSocket connection established        proxy_session_id=B
```

The dedup code (`external_agent_handlers.go:1379-1418`) registers `activeStreamProxies["stream:<sess>:<client_id>"] = {proxy_session_id, cancel}`. When a new WS arrives with the same key it cancels the prior proxy's context. The old `resilientProxy.Run()` returns with `context canceled`, `defer clientConn.Close()` does a raw TCP close — *no WebSocket Close frame is sent* — so the browser sees `code: 1006 "abnormal closure"` and reconnects.

### Sandbox (desktop-bridge) — same window

```
INF stream init received   width=1920 height=1080 fps=60 …
INF websocket closed       close_code=1006 close_text="unexpected EOF" duration=14s frames_sent=900
INF stream init received   …
INF websocket closed       close_code=1006 close_text="unexpected EOF" duration=1s   frames_sent=111
WRN ping failed, closing dead connection   err="write tcp [::1]:9876->[::1]:51972: write: connection reset by peer"
```

The `[::1]:9876 → [::1]:51972` write is on the internal localhost socket between hydra (inside `helix-sandbox-nvidia-1`) and desktop-bridge (inside the ubuntu-external container). RST comes from the hydra side because the API tore its end of the resilient proxy down on supersede.

### Per-connection lifetime (one observation window, ~85s)

Extracted from the API log timeline above (each row = one proxy_session_id from establishment to close):

| Time     | proxy_session_id | lifetime | trigger                              |
|----------|------------------|----------|--------------------------------------|
| 20:39:24 | 5995…            | 4 s      | superseded                           |
| 20:39:28 | cde0…            | 1 s      | superseded                           |
| 20:39:29 | 5878…            | 8 s      | superseded                           |
| 20:39:37 | 9ce0…            | 1 s      | superseded                           |
| 20:39:38 | fc90…            | 14 s     | superseded                           |
| 20:39:52 | fe7e…            | 1 s      | superseded                           |
| 20:39:53 | 4644…            | 7 s      | superseded                           |
| 20:40:00 | 1293…            | 1 s      | superseded                           |
| 20:40:01 | 5558…            | 2 s      | superseded                           |
| 20:40:03 | eb00…            | 10 s     | superseded                           |
| 20:40:13 | c5a7…            | 1 s      | superseded                           |
| 20:40:14 | 909f…            | 2 s      | superseded                           |
| 20:40:16 | 2492…            | 15 s     | superseded                           |
| 20:40:31 | 74e6…            | 14 s     | superseded                           |
| 20:40:45 | 92c1…            | 1 s      | superseded                           |
| 20:40:46 | 056d…            | <1s      | client clean close (reconnect_count=1) |

Two distinct populations: **long-lived (4-15 s)** and **short-lived (1-2 s)**. The pattern is overwhelmingly **long → short → long → short → …**. Every long-lived session is followed by a short-lived one before the next long-lived one starts.

That alternation is the key — it means the client opens **two** new WS connections in rapid succession after each long-lived stream dies. Exactly one of them gets to live for any meaningful time.

---

## Architecture refresher

```
Browser tab (DesktopStreamViewer + WebSocketStream)
   │ WebSocket (sessionId + client_id query param)
   ▼
helix-api  /api/v1/external-agents/{sessionId}/ws/stream
   │  hijack + activeStreamProxies dedup keyed by stream:<sess>:<client_id>
   │  RevDial bridge → resilient proxy
   ▼
hydra (in helix-sandbox-nvidia-1)
   │ localhost socket → :9876 (desktop-bridge HTTP server inside ubuntu-external)
   ▼
desktop-bridge (in ubuntu-external container)
   │  GStreamer pipeline → /ws/stream
   ▼
Browser canvas
```

Stable identifiers:
- `componentInstanceIdRef` = `getOrCreateStreamUUID(sessionId)` — stored in `sessionStorage`, survives remounts within a tab.
- One `DesktopStreamViewer` per page (`SpecTaskDetailContent.tsx:2350` and `:2842` are if/else on layout). `ScreenshotViewer` uses HTTP polling, not WebSocket.

The 2026-04-12 fix in `f8c926cad` removed the component-level `ws.readyState === CLOSED` check that previously fought `WebSocketStream`'s internal backoff. **That fix is still in place** — the current `checkFrameHealth` only acts on `decoderState === "closed"` or `lastWsMessageTime > 0 && timeSinceWsData > 5000ms`, both with a `DECODER_CRASH_RECONNECT_COOLDOWN_MS = 10s` rate limit.

---

## Hypotheses

I've eliminated a long list (component-level CLOSED check — gone; dual viewer mount — only one renders; ScreenshotViewer — HTTP not WS; bandwidth/quality changes — user-driven only; `getOrCreateStreamUUID` instability — sessionStorage-backed). Three remain plausible:

### H1: WSStream internal reconnect + component-level reconnect both fire (the duplicate-connect race)

Each long-lived stream dies for *some* reason (H4 below), then **two** code paths each try to re-establish it:

1. **`WebSocketStream.onClose`** (`websocket-stream.ts:465`) sees `this.closed === false`, schedules `this.connect()` with exponential-backoff jittered delay (`reconnectDelay * 2^(attempt-1) * (0.5..1)`, first attempt ≈ 0.5–2 s). This reuses the **same WSStream instance** and `streamRef.current` remains the same object.
2. **Component** receives `disconnected` event → just sets `Reconnecting…` UI, doesn't itself reconnect. BUT if a `reconnectAborted` (line 938) or `error` (line 856) is dispatched, the component calls `reconnectRef.current(1000, …)` (lines 877 / 966), which immediately `disconnect(true)`s and re-`connect()`s the WHOLE WSStream after 1 s.

The component-level `reconnect()` calls `streamRef.current.close()` which cancels the internal backoff (`WSStream.close()` at line 2251 clears `reconnectTimeoutId`) — *if* it runs before the internal timer fires.

If the internal timer fires *first* (likely, because its minimum is ~0.5 s while the component's path is exactly 1 s), the SAME WSStream opens WS_B. Then 1 s later the component's `reconnect()` setTimeout fires:
- `connect()` at line 600 sees `streamRef.current` (still WSStream A), closes it (closing WS_B!), creates WSStream B (new instance), which opens WS_C.

Net: two new WS connections opened ~1 s apart. Server sees WS_B → registers proxy_B; server sees WS_C → supersedes proxy_B → cancels WS_B → 1006 to client → … but at that point the active stream is WS_C anyway, and the cycle ends *if* nothing kicks it off again.

The trigger for the next cycle is again H4 (something causing WS_C to die after another ~5-15 s).

**Predicts in browser logs:**
- `[WebSocketStream] Reconnecting (attempt N)...` immediately followed (within ~500 ms) by `[DesktopStreamViewer] Closing existing stream before creating new one`.
- Either `[DesktopStreamViewer] Reconnect aborted by stream` or `[DesktopStreamViewer] Ignoring error during explicit close` shortly before.
- Two `[WebSocketStream] Connecting to: ws://…` log lines per disconnect cycle.

### H2: Visibility / focus / ResizeObserver re-render trips `useEffect(connect)` despite the `hasConnectedRef` guard

The auto-connect effect (line 1866-1901) has deps `[sandboxId, sessionId, isVisible, width, height, account.initialized]`. The guard is:

```ts
if (hasConnectedRef.current) return;
```

This means *after* the first connection, `connect()` is never called from this effect again. Good. BUT every dep change re-runs the effect body; if anything makes `hasConnectedRef.current` false again, it reconnects. Nothing in the code currently flips it back to false, but it's worth verifying. Also: `width` and `height` come from props and *change with the layout*, e.g. a `containerSize` change can flow back to props. A nearby effect (line 2625, `[containerSize, isConnected]`) also touches canvas geometry — worth checking that nothing in that chain causes the parent to remount the viewer.

Mid-investigation finding that argues against H2 in this specific session: the *same* `client_id` and the *same* WSStream instance ref logs ("Closing existing stream before creating new one") would or wouldn't appear — browser logs will tell us.

**Predicts in browser logs:**
- `[DesktopStreamViewer] Component unmounting, calling disconnect()` paired with `Component mounted, setting up cleanup handler`.
- Or `[DesktopStreamViewer] Auto-connecting at … Mbps for WxH` lines firing more than once.

### H3: Server-side supersede semantics turn one legitimate reconnect into many

This is the *amplifier*, not the root cause, but worth fixing anyway.

When the dedup cancels `prev`, `defer clientConn.Close()` performs a raw TCP close. The browser sees `code: 1006`. The client (correctly, for a real network failure) treats 1006 as transient and reconnects with backoff. That reconnect arrives at the server, finds the previous *new* proxy still in `activeStreamProxies` (because the cleanup defer hasn't yet removed it), supersedes it, and the loop maintains itself.

If the supersede sent a clean WebSocket close frame with a recognizable application code (e.g. `4000 "superseded"`), the client could distinguish "the server intentionally replaced you" from "the network died" and *not* reconnect. That alone breaks the loop regardless of H1 vs H2.

**Predicts in browser logs:**
- `[WebSocketStream] Disconnected: { code: 1006, reason: "(empty)", wasClean: false, … }` for every disconnect (vs. `code: 4000` after the fix).

### H4: The trigger that kills each long-lived connection in the first place

H1/H2 explain why *one* trigger fans out to many connections, but not what fires the original close every 4-15 s on a connection that's otherwise streaming at 60fps. Candidates:

- **WSStream heartbeat false positive** (`websocket-stream.ts:2124-2148`): fires `ws.close()` when `Date.now() - lastMessageTime > 10_000ms`. `lastMessageTime` is updated on *every* `onMessage`. With server keepalive at 500 ms, this should never fire under healthy streaming. *But* the irregular long lifetimes (4, 8, 14, 7, 10, 15, 14 s) don't match any single timer cadence, so this is unlikely as a primary cause.
- **Component health check stale detect** (`DesktopStreamViewer.tsx:2010`): fires `reconnect(500, "Reconnecting (connection stalled)…")` when `timeSinceWsData > 5000ms`, with 10 s cooldown. Same argument: keepalive should prevent this.
- **VideoDecoder crash** (line 1982): forces `reconnect(500, "Reconnecting (decoder crashed)…")`. Plausible on iPad / Safari; less so on a regular tab. Browser logs will confirm/refute via `Decoder crashed (state=closed)` lines.
- **Tab visibility / focus / network handover**: each transient `visibilitychange` event resets `lastMessageTime` (line 2119) but if the network actually drops a packet during the hidden→visible transition, the stream may briefly stall.
- **Server-side `ping failed` cascade**: the `WRN ping failed` line was observed once. If write backpressure intermittently makes the server's 5 s heartbeat-ping fail (`api/pkg/desktop/ws_stream.go:1362-1384`), the server unconditionally calls `v.ws.UnderlyingConn().Close()` — RST — and the client sees 1006. This would manifest as the first 1006 of a cycle being initiated server-side rather than client-side.
- **A genuine network blip** on the LAN / loopback path.

Without browser logs we cannot tell which fires first. The fact that the same trigger fires irregularly (4 / 8 / 14 / 7 / 10 / 15) makes it look more like an *event-driven* cause (user interaction, decoder hiccup, transient backpressure) than a *timer*.

---

## What I think is most likely

**H3 is in play for every observed disconnect** (the raw-TCP-close turning supersede into 1006 is verified in code). **H1 most likely explains the long-short-long-short alternation.** **H4** is the trigger — and is probably a mix of (a) occasional server-side ping write failure / write backpressure and (b) the iPad/Safari-tier decoder/visibility quirks the component health-check exists to cover.

The clean way to break the storm is **H3 alone**: emit a WS close frame with code `4000 "superseded"` on the cancel path; have `WebSocketStream.onClose` recognise the code and skip its own reconnect *and* dispatch `reconnectAborted` to the component. That converts the cycle into a single bounded event ("server replaced your connection, we honor it") and makes H1's race irrelevant because the supersede no longer triggers a client retry.

H1 should still be fixed properly afterwards (the dual-path reconnect coordination is the same shape of bug that bit us in April).

---

## Raw evidence

### Server API stream events for `ses_01ks63b0btvj0ax8kzsy43xr4r`

(Excerpt — 80 lines saved to `/tmp/api-events.txt` during investigation.)

```
20:39:24Z INF  Stream WebSocket connection closed       proxy_session_id=9242f197a05987a5  reconnect_count=0
20:39:24Z INF  Stream WebSocket connection established  proxy_session_id=5995…  (initial)
20:39:28Z INF  Superseding previous stream proxy for same client  client_id=7307a64c-7165-4a7c-aa4b-4906e2e592f2  new_proxy=cde02798c4f59cf8  old_proxy=59952b574f6b1d79
20:39:28Z INF  Proxying stream WebSocket to screenshot-server via RevDial  proxy_session_id=cde02798c4f59cf8
20:39:28Z WRN  Resilient proxy ended with error  error="context canceled"  proxy_session_id=59952b574f6b1d79
20:39:28Z INF  Stream WebSocket connection closed       proxy_session_id=59952b574f6b1d79  reconnect_count=0
20:39:28Z INF  Stream WebSocket connection established  proxy_session_id=cde02798c4f59cf8
20:39:29Z INF  Superseding previous stream proxy for same client  client_id=7307a64c-7165-4a7c-aa4b-4906e2e592f2  new_proxy=587876ff143952dc  old_proxy=cde02798c4f59cf8
20:39:29Z INF  Proxying stream WebSocket to screenshot-server via RevDial  proxy_session_id=587876ff143952dc
20:39:29Z WRN  Resilient proxy ended with error  error="context canceled"  proxy_session_id=cde02798c4f59cf8
20:39:29Z INF  Stream WebSocket connection closed       proxy_session_id=cde02798c4f59cf8  reconnect_count=0
20:39:29Z INF  Stream WebSocket connection established  proxy_session_id=587876ff143952dc
[…same shape repeats, lifetimes: 8s 1s 14s 1s 7s 1s 2s 10s 1s 2s 15s 14s 1s …]
```

All 80 events share `client_id=7307a64c-7165-4a7c-aa4b-4906e2e592f2` — one browser tab.

### Sandbox / desktop-bridge events

```
21:38:48 stream init received  width=1920 height=1080 fps=60 bitrate=8294  user="Luke Marsden"
21:38:54 websocket closed      close_code=1006 close_text="unexpected EOF" duration=6s  frames_sent=425
21:38:54 stream init received
21:38:55 websocket closed      close_code=1006 close_text="unexpected EOF" duration=1s  frames_sent=134
21:38:55 stream init received
[…same shape repeats…]
21:40:31 WRN ping failed, closing dead connection   err="write tcp [::1]:9876->[::1]:51972: write: connection reset by peer"
21:40:31 websocket closed      close_code=1006 close_text="unexpected EOF" duration=15s frames_sent=869
[…]
21:40:46 websocket closed      close_code=1001 close_text=""               duration=0s  frames_sent=68
21:40:46 ERROR failed to read init message  err="websocket: close 1006 (abnormal closure): unexpected EOF"
```

`close_code=1001` at 21:40:46 = a clean "going away" close (browser tab closed or navigated away). The 1006s before that are all consistent with the server-side raw-TCP-close path.

`pipeline_received` in the periodic `VIDEO LATENCY STATS` lines never drops — the GStreamer pipeline keeps running and feeding frames at 60 Hz. Only the WS outbound transport is being recycled.

### Frontend console logs

**TODO — Luke to paste.** The doc is complete on the server side; the missing piece is which client-side code path fires when. The most informative ~60 s with filter prefix `[DesktopStreamViewer]` / `[WebSocketStream]` will disambiguate H1 vs H2.

Specific log lines that decide the question:

| Log line                                                       | Means                                                       |
|----------------------------------------------------------------|-------------------------------------------------------------|
| `[DesktopStreamViewer] Closing existing stream before creating new one` | `connect()` ran while `streamRef.current` was still set — H1 confirmed |
| `[DesktopStreamViewer] Reconnect aborted by stream`            | `WSStream.close()` was called externally, then onClose saw `closed=true` — H1 path 1 |
| `[DesktopStreamViewer] Decoder crashed (state=closed)`         | H4 candidate (Safari decoder)                                |
| `[DesktopStreamViewer] WS data stall: …ms since last message`  | H4 candidate (stall detect)                                  |
| `[WebSocketStream] Stale connection detected`                  | H4 candidate (heartbeat)                                     |
| `[WebSocketStream] Connection timeout`                         | H4 candidate (initial WS open never completed)               |
| `[WebSocketStream] Disconnected: {code: …}`                    | Confirms it's 1006 on every cycle                            |
| `[WebSocketStream] Will reconnect in Xms (attempt N/M)`        | WSStream internal reconnect about to fire                    |
| `[DesktopStreamViewer] Bitrate changed from … to …`            | If present, blames the bitrate-change effect at line 1717   |
| `[DesktopStreamViewer] Lobby changed from … to …`              | If present, blames the lobby-change effect at line 1761     |
| `[DesktopStreamViewer] Component unmounting`                   | Parent is unmounting the viewer — H2                         |

---

## Recommended fixes

In order of (1) breaks the storm immediately, (2) prevents recurrence, (3) makes future incidents debuggable:

1. **Make supersede a clean WS close, not a TCP RST.** On the cancel path in `external_agent_handlers.go:1408`, write a WebSocket close frame with a custom application code (`4000 superseded`) before `prev.cancel()`. In `WebSocketStream.onClose`, treat `code === 4000` as "do not reconnect"; dispatch `reconnectAborted` to the component; component recognizes the explicit-replacement case and does nothing. **This alone stops the storm.**
2. **Coordinate the dual reconnect paths in the component.** Either:
   - Remove the component-level reconnect on `disconnected` / `error` paths entirely; let `WebSocketStream` own *all* reconnect decisions and have it expose `reconnectAborted` only for the "I really am giving up" case; OR
   - Have `WebSocketStream` expose an `isReconnecting()` predicate so the component's reconnect refuses to fire while internal backoff is pending.
3. **Add a log line on every NEW `WebSocketStream` construction** — `console.log("[WebSocketStream] new instance", { instanceId, sessionId, clientUniqueId })` — so we can count instances per cycle in production telemetry.
4. **Investigate the server-side ping-write failure** observed in `ping failed, closing dead connection`. If write backpressure on the desktop-bridge → hydra socket is real, fixing it removes one trigger for H4.

Tactical: PR (1) is small and self-contained. PR (2)/(3) is the proper coordination cleanup. PR (4) is a separate workstream.
