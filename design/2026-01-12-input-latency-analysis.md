# Input Latency Analysis: Key-to-Eyeball Pipeline

**Date:** 2026-01-12
**Status:** Analysis Complete
**Author:** Luke (with Claude)

## Executive Summary

This document traces the complete input latency pipeline from browser keypress to rendered character on screen, identifying all potential latency sources across the GNOME and Sway compositors.

**Estimated Total Key-to-Eyeball Latency Components:**

| Stage | GNOME | Sway | Notes |
|-------|-------|------|-------|
| Browser event capture | <1ms | <1ms | Negligible |
| WebSocket send | <1ms | <1ms | Non-blocking |
| RevDial proxy | 1-5ms | 1-5ms | HTTP tunnel overhead |
| Input parsing | <1ms | <1ms | Binary protocol |
| D-Bus/Wayland injection | 1-5ms | <1ms | D-Bus has IPC overhead |
| Compositor processing | 1-16ms | 0-16ms | Depends on vsync |
| Application rendering | Variable | Variable | App-dependent |
| **Screen capture** | **16-100ms** | **16ms** | **Main suspect for GNOME** |
| Encoding | 1-5ms | 1-5ms | GPU encoder (NVENC) |
| WebSocket transmission | 1-10ms | 1-10ms | Network dependent |
| Decoding + display | 1-5ms | 1-5ms | WebCodecs + canvas |
| **Total** | **~40-150ms** | **~25-55ms** | - |

**Key Findings:**
1. GNOME ScreenCast uses damage-based updates with 100ms keepalive-time, meaning idle screens only update 10 fps
2. Sway/wf-recorder uses `--no-damage` flag and should be 60fps fixed
3. The `keepalive-time=100` in pipewirezerocopysrc limits GNOME's minimum frame rate to 10fps

## Pipeline Deep Dive

### Stage 1: Browser Event Capture

**Files:** `frontend/src/lib/helix-stream/stream/input.ts`

Browser captures DOM events and converts to binary protocol:

```typescript
// input.ts:168-187
private sendKeyEvent(isDown: boolean, event: KeyboardEvent) {
    // Convert to evdev keycode (Linux native) for direct WebSocket mode
    let key = convertToEvdevKey(event)
    let modifiers = convertToEvdevModifiers(event)
    this.sendKey(isDown, key, modifiers)
}

// Binary format: [subType:1][isDown:1][modifiers:1][keycode:2 BE]
sendKey(isDown: boolean, key: number, modifiers: number) {
    this.buffer.putU8(0)
    this.buffer.putBool(isDown)
    this.buffer.putU8(modifiers)
    this.buffer.putU16(key)
    trySendChannel(this.keyboard, this.buffer)
}
```

**Latency:** <1ms (direct evdev codes, no conversion needed on backend)

**Optimization applied:** Uses evdev codes directly instead of Windows VK codes, eliminating backend VK→evdev conversion.

### Stage 2: WebSocket Transmission (Frontend to API)

**Files:** `frontend/src/lib/helix-stream/stream/websocket-stream.ts`

The WebSocketStream class patches input methods to use WebSocket transport:

```typescript
// websocket-stream.ts:1404-1410
sendKey(isDown: boolean, key: number, modifiers: number) {
    // Format: subType(1) + isDown(1) + modifiers(1) + keyCode(2)
    this.inputBuffer[0] = 0
    this.inputBuffer[1] = isDown ? 1 : 0
    this.inputBuffer[2] = modifiers
    this.inputView.setUint16(3, key, false)
    this.sendInputMessage(WsMessageType.KeyboardInput, this.inputBuffer.subarray(0, 5))
}
```

**Latency:** <1ms for `ws.send()` (non-blocking), but buffer congestion tracked:

```typescript
// websocket-stream.ts:1317-1339
private isInputBufferCongested(): boolean {
    const buffered = this.ws.bufferedAmount
    if (buffered === 0) {
        this.lastBufferDrainTime = now
        return false
    }
    // Skip if buffer hasn't drained
    return true
}
```

Mouse throttling matches stream FPS:
```typescript
// websocket-stream.ts:270
this.mouseThrottleMs = Math.floor(1000 / settings.fps)
```

**Potential Issue:** If WebSocket buffer backs up, mouse events are dropped, but keyboard events are always sent.

### Stage 3: RevDial Proxy (API → Sandbox)

**Files:** `api/pkg/server/external_agent_handlers.go`

RevDial creates a reverse tunnel from API server to sandbox:

```
Browser → API Server → RevDial tunnel → Sandbox screenshot-server
```

The WebSocket URL is:
```
/api/v1/external-agents/{sessionId}/ws/stream
```

This is proxied through RevDial to `localhost:9876/ws/stream` inside the sandbox.

**Latency:** 1-5ms (HTTP hijack + tunnel overhead)

**Potential Issue:** RevDial adds a proxy hop that increases latency compared to direct WebSocket.

### Stage 4: Input Parsing (Screenshot Server)

**Files:** `api/pkg/desktop/ws_stream.go`, `api/pkg/desktop/ws_input.go`

Binary message routing:
```go
// ws_stream.go:886-926
if msgType == websocket.BinaryMessage && len(msg) > 0 {
    msgType := msg[0]
    switch msgType {
    case StreamMsgKeyboard: // 0x10
        s.handleWSKeyboard(payload)
    case StreamMsgMouseClick: // 0x11
        s.handleWSMouseButton(payload)
    // ...
    }
}
```

**Latency:** <1ms (direct byte parsing, no JSON)

### Stage 5: Input Injection (D-Bus or Wayland)

**GNOME Path (D-Bus RemoteDesktop):**
```go
// ws_input.go:119-127
if s.conn != nil && s.rdSessionPath != "" {
    rdSession := s.conn.Object(remoteDesktopBus, s.rdSessionPath)
    err := rdSession.Call(remoteDesktopSessionIface+".NotifyKeyboardKeycode",
        0, uint32(evdevCode), isDown).Err
    return
}
```

**Sway Path (Wayland Virtual Keyboard):**
```go
// ws_input.go:130-140
if s.waylandInput != nil {
    if isDown {
        err = s.waylandInput.KeyDownEvdev(evdevCode)
    } else {
        err = s.waylandInput.KeyUpEvdev(evdevCode)
    }
}
```

```go
// wayland_input.go:130-138
func (w *WaylandInput) KeyDownEvdev(evdevCode int) error {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.keyboard.Key(time.Now(), uint32(evdevCode), virtual_keyboard.KeyStatePressed)
}
```

**Latency:**
- GNOME: 1-5ms (D-Bus IPC round-trip)
- Sway: <1ms (direct Wayland protocol)

**Observation:** GNOME's D-Bus path has more IPC overhead than Sway's direct Wayland virtual keyboard protocol.

### Stage 6: Compositor Processing

Both compositors process input and update their internal state.

**GNOME (Mutter):** Processes input synchronously with its main loop.

**Sway:** Processes input via virtual keyboard protocol immediately.

**Latency:** 0-16ms (depends on compositor refresh cycle)

### Stage 7: Application Rendering

The application (e.g., Zed, terminal) receives the input event and renders:
1. Mutter/Sway delivers keyboard event to focused window
2. Application processes keystroke
3. Application queues redraw
4. Application renders new frame

**Latency:** Variable (0-16ms typical for vsync-aligned rendering)

### Stage 8: Screen Capture ⚠️ MAIN LATENCY SOURCE

This is where GNOME and Sway differ significantly:

#### GNOME (pipewirezerocopysrc / pipewiresrc)

**Files:** `api/pkg/desktop/ws_stream.go` lines 361-386

```go
// VideoModeZeroCopy pipeline:
srcPart := fmt.Sprintf("pipewirezerocopysrc pipewire-node-id=%d output-mode=%s keepalive-time=100",
    v.nodeID, outputMode)
```

**Key issue: `keepalive-time=100`** (100ms)

This parameter sets the minimum frame interval when there's no screen damage:
- Screen idle: Frames sent every 100ms = 10fps
- Screen damage: Frame sent immediately on damage... **theoretically**

However, Mutter's ScreenCast uses a **PASSIVE frame clock** which means:
- It only produces frames when the compositor renders
- If Mutter's refresh is 60Hz but no damage occurs, it may batch damage events
- The "damage" signal may not trigger immediately in headless mode

**Latency:** 16-100ms depending on damage timing

#### Sway (wf-recorder with --no-damage)

**Files:** `api/pkg/desktop/video_forwarder.go` lines 117-124

```go
v.cmd = exec.CommandContext(ctx, "wf-recorder",
    "-y",
    "-c", "h264_nvenc",
    "-x", "yuv420p",
    "-m", "h264",
    "--no-damage",  // Capture every frame, not just on damage
    "-f", fifoPath,
)
```

The `--no-damage` flag means wf-recorder captures at a fixed rate (typically 60fps based on Sway's output mode).

**Latency:** 0-16ms (one frame time at 60fps)

**Yet Sway is also slow?** This is surprising given `--no-damage`. Possible causes:
1. wf-recorder internal buffering
2. FIFO read latency in GStreamer
3. Encoding latency accumulating

### Stage 9: Video Encoding

**Files:** `api/pkg/desktop/ws_stream.go`, `api/pkg/desktop/gst_pipeline.go`

NVENC encoder settings:
```go
// ws_stream.go:439-441
fmt.Sprintf("nvh264enc preset=low-latency-hq zerolatency=true gop-size=%d rc-mode=cbr-ld-hq bitrate=%d aud=false",
    getGOPSize(), v.config.Bitrate)
```

**Latency:** 1-5ms with NVENC in zerolatency mode

### Stage 10: WebSocket Transmission (Sandbox → Browser)

Frames sent via WebSocket:
```go
// ws_stream.go:706-711
msg := make([]byte, 15+len(data))
copy(msg[:15], header)
copy(msg[15:], data)
return v.writeMessage(websocket.BinaryMessage, msg)
```

**Latency:** 1-10ms (network dependent)

### Stage 11: Decoding and Display

**Files:** `frontend/src/lib/helix-stream/stream/websocket-stream.ts`

WebCodecs hardware-accelerated decoding:
```typescript
// websocket-stream.ts:985
this.videoDecoder.decode(chunk)

// websocket-stream.ts:774-797
private renderVideoFrame(frame: VideoFrame) {
    this.canvasCtx.drawImage(frame, 0, 0)
    frame.close()
}
```

Canvas context is created with low-latency options:
```typescript
// websocket-stream.ts:1614-1617
this.canvasCtx = canvas.getContext("2d", {
    alpha: false,
    desynchronized: true, // Lower latency
})
```

**Latency:** 1-5ms (hardware decoding + canvas draw)

## Identified Latency Sources and Optimizations

### 1. GNOME keepalive-time (100ms → 50ms?)

**Current:** `keepalive-time=100` = 10fps minimum when idle
**Proposed:** `keepalive-time=50` = 20fps minimum when idle

This would only help idle frame rate, not damage-based updates. The real issue is whether damage events are being delivered promptly.

### 2. GNOME ScreenCast PASSIVE Frame Clock

The log showed:
```
"linked_node_id=47, standalone_node_id=50"
"EXPERIMENTAL: using linked session for video (testing damage events)"
```

Linked vs standalone sessions may have different damage event behavior. Previous testing showed standalone sessions only received 2-5 process callbacks in 10-20 seconds.

### 3. Sway wf-recorder Pipeline

Despite `--no-damage`, there may be buffering issues:
1. wf-recorder captures to FIFO
2. GStreamer reads from FIFO via `filesrc`
3. `h264parse` processes the stream

The FIFO read might not be as low-latency as direct PipeWire capture.

### 4. RevDial Proxy Overhead

Every input/video message goes through:
```
Browser → API → RevDial → screenshot-server → D-Bus/Wayland
```

A direct WebSocket to the sandbox (if network allows) would eliminate one hop.

### 5. D-Bus vs Wayland Input

GNOME uses D-Bus RemoteDesktop API which has IPC overhead:
- Each input event = D-Bus method call
- D-Bus message serialization/deserialization

Sway uses direct Wayland virtual keyboard/pointer protocol which is more efficient.

## Measurement Recommendations

### 1. Frame Timing Analysis

The user's insight about 10fps idle frame rate is key:
- When screen is idle (flashing cursor only): 10fps = 100ms between frames
- When typing: frames should arrive faster (damage-based)

**Measurement:** Track actual frame arrival times vs PTS to see if damage events trigger promptly.

The frontend already tracks this:
```typescript
// websocket-stream.ts:883-906
const ptsDeltaMs = (ptsUsNum - this.firstFramePtsUs) / 1000
const expectedArrivalTime = this.firstFrameArrivalTime! + ptsDeltaMs
const latencyMs = arrivalTime - expectedArrivalTime
```

### 2. Input Timestamp Injection

Add timestamps at each stage:
1. Frontend: `performance.now()` when key pressed
2. Backend: `time.Now()` when input received
3. Backend: `time.Now()` after D-Bus/Wayland call
4. Compare frame PTS after input

### 3. Separate Video and Input Latency

The current `rttMs` only measures Ping/Pong RTT, not key-to-frame latency.

A proper measurement would:
1. Record timestamp when key sent
2. Look for frame that shows the character
3. Calculate delta

## Potential Optimizations (Prioritized)

### High Impact

1. **GNOME: Reduce keepalive-time from 100ms to 50ms or 30ms**
   - Location: `ws_stream.go:381`
   - Impact: Higher idle frame rate (may help if damage events are delayed)

2. **GNOME: Investigate damage event delivery**
   - Why aren't damage events triggering frames immediately?
   - Is Mutter batching damage in headless mode?

3. **Sway: Check wf-recorder buffering**
   - Does wf-recorder buffer frames before writing to FIFO?
   - Test direct GStreamer capture vs wf-recorder

### Medium Impact

4. **Remove RevDial hop for video streaming**
   - Direct WebSocket from browser to sandbox (requires network config)

5. **Pre-prime input path**
   - The GNOME keyboard priming (ws_stream.go:185) shows first events can be dropped
   - Ensure input path is warmed up

### Low Impact

6. **Input batching**
   - Batch multiple mouse moves into single message
   - Already implemented via mouse throttling

7. **WebSocket compression**
   - Compress video frames before transmission
   - Already using H.264 which is highly compressed

## Questions to Investigate

1. **Why is Sway slow despite `--no-damage` 60fps capture?**
   - wf-recorder internal latency?
   - FIFO read buffering?
   - Encoding pipeline delay?

2. **Is GNOME damage-based capture actually working?**
   - Log damage events vs frame production
   - Compare linked vs standalone session damage behavior

3. **What is the actual end-to-end key-to-eyeball latency?**
   - Need instrumentation to measure, not just estimate

## CLI Instrumentation Tool

A new CLI command has been created to measure input latency:

```bash
# Run 5 latency tests with automatic terminal setup
helix spectask latency ses_01xxx

# Run 20 tests with verbose output
helix spectask latency ses_01xxx --tests 20 --verbose

# Skip terminal setup if editor already focused
helix spectask latency ses_01xxx --skip-setup
```

The tool works by:
1. Launching a terminal (kitty + cat) to display keystrokes
2. Establishing baseline frame interval (idle screens = ~100ms between frames)
3. Sending keystrokes and detecting out-of-band frames (arriving much faster than baseline)
4. Measuring the delta between keystroke send and frame arrival

**Measurement Strategy:**
- When the screen is idle, frames arrive at the keepalive-time interval (default 100ms = 10fps)
- When you type, damage-based updates trigger an immediate frame capture
- This out-of-band frame arrives much faster than the baseline interval
- The time from sending the key to receiving the out-of-band frame = key-to-eyeball latency

**Note:** Reducing keepalive-time from 100ms to 200ms or higher would make out-of-band detection more accurate, but would also reduce idle frame rate.

## Next Steps

1. ✅ Add instrumentation to measure actual key-to-eyeball latency (DONE - `helix spectask latency`)
2. Test keepalive-time=50 on GNOME (would increase idle FPS but make latency detection harder)
3. Profile wf-recorder pipeline latency on Sway
4. Compare GNOME linked vs standalone ScreenCast sessions for damage responsiveness
5. Consider adding latency markers to frames (embed send timestamp in frame metadata)
