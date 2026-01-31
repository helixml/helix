# How We Built Adaptive Remote Desktop Streaming That Actually Works on Bad Connections

**TL;DR:** We built a remote desktop streaming system that automatically detects when your connection sucks and switches to a clever screenshot-based fallback — without losing keyboard/mouse input. Here's how we did it with WebSockets, WebCodecs, and a 15-line protocol hack.

---

## The Problem: WebRTC Fails in Enterprise Networks

We're building [Helix](https://github.com/helixml/helix), an open-source AI platform that lets you run AI coding agents in cloud sandboxes. These sandboxes need remote desktop streaming — you need to see what the AI is doing.

Our first approach: WebRTC. It's the standard for low-latency video. It works great... until it doesn't.

**Enterprise networks hate WebRTC.**

- TURN servers get blocked by firewalls
- UDP traffic is deprioritized or dropped
- NAT traversal fails in complex network topologies
- IT departments configure aggressive timeouts

So we built a pure WebSocket fallback. HTTP/HTTPS works everywhere. L7 load balancers love it. IT departments don't block it.

But WebSocket streaming has its own problem: **when the connection gets slow, the video freezes and input becomes unresponsive.**

## The Observation: Screenshots Are Cheap

While debugging the frozen video issue, we noticed something interesting. When we opened the sandbox screenshot endpoint for debugging:

```
GET /api/v1/external-agents/{session}/screenshot?format=jpeg&quality=70
```

The screenshots loaded **fast**. Like, really fast. Even on bad connections.

Why? Because a single JPEG screenshot is:
- **100-200KB** — way smaller than a video keyframe
- **No codec overhead** — just raw pixels compressed
- **Stateless** — no decoder state to corrupt
- **Self-contained** — arrives complete or not at all

Meanwhile, the 60fps H.264 stream was:
- **20-40 Mbps** sustained bitrate
- **GOP dependencies** — miss one frame, everything corrupts
- **Decoder state** — needs continuous frames to stay synchronized
- **Buffering hell** — WebSocket backpressure causes jitter

## The Hack: Screenshot Mode with Live Input

Here's the key insight: **we don't need video for the visual feedback. We just need screenshots. But we absolutely need the WebSocket for input.**

The WebSocket stream carries two things:
1. Video frames (H.264 via WebCodecs)
2. Input events (keyboard, mouse, touch)

What if we could **pause the video but keep the input flowing?**

### The Protocol Extension

We added exactly one new message type to our WebSocket protocol:

```rust
// In ws_protocol.rs
t if t == WsMessageType::ControlMessage as u8 => {
    if let Ok(ctrl) = ControlMessage::decode(Bytes::copy_from_slice(&data)) {
        if let Some(enabled) = ctrl.get_set_video_enabled() {
            video_enabled.store(enabled, Ordering::Relaxed);
        }
    }
    None
}
```

And one check before sending video frames:

```rust
StreamerIpcMessage::VideoFrame { ... } => {
    // Skip video frames when video is paused (screenshot mode)
    if !video_enabled_for_ipc.load(Ordering::Relaxed) {
        continue;
    }
    // ... rest of frame handling
}
```

That's it. 15 lines of Rust.

On the client side, one method:

```typescript
setVideoEnabled(enabled: boolean) {
  const json = JSON.stringify({ set_video_enabled: enabled })
  const message = new Uint8Array(1 + jsonBytes.length)
  message[0] = WsMessageType.ControlMessage
  message.set(new TextEncoder().encode(json), 1)
  this.ws.send(message.buffer)
}
```

### The Result

When we detect high latency (RTT > 150ms), we:

1. Send `{"set_video_enabled": false}` to the server
2. Start polling screenshots at 2-10 FPS (adaptive quality)
3. Keep the WebSocket open for input events

**Bandwidth drops from 40 Mbps to ~500 Kbps.** Input stays responsive. The user sees updated frames.

## The Oscillation Problem

But there's a catch. When you stop sending video frames, the WebSocket becomes almost empty. Just tiny input events and pings.

**The latency drops.**

Now the adaptive mode sees low latency and thinks: "Great! Let's switch back to video!"

Video resumes. Bandwidth spikes. Latency spikes. Mode switches to screenshots again.

**Oscillation. Every 2 seconds. Forever.**

### The Lock-In Solution

We needed a simple rule: **once you fall back to screenshots due to high latency, stay there until the user explicitly asks to retry.**

```typescript
if (rtt > ENABLE_THRESHOLD_MS && !adaptiveScreenshotEnabled) {
  console.log(`High latency detected, locking to screenshot mode`)
  setAdaptiveScreenshotEnabled(true)
  setAdaptiveLockedToScreenshots(true)  // Don't auto-switch back
}
```

The user sees an amber icon and a message: "Video paused to save bandwidth. Click speed icon to retry video."

Click to unlock. That's it. No oscillation.

## The Screenshot Server

We built a tiny Go server that runs inside the sandbox container:

```go
func captureScreenshot(format string, quality int) ([]byte, string, error) {
    // Build grim arguments (Wayland screenshot tool)
    grimArgs := []string{"-c"}  // Include cursor
    if format == "jpeg" {
        grimArgs = append(grimArgs, "-t", "jpeg", "-q", fmt.Sprintf("%d", quality))
    }
    grimArgs = append(grimArgs, filename)

    cmd := exec.Command("grim", grimArgs...)
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("WAYLAND_DISPLAY=%s", waylandDisplay),
    )
    // ...
}
```

**Note:** Ubuntu's grim package is compiled without libjpeg support. We had to build grim from source with `-Djpeg=enabled`. Fun.

### Adaptive JPEG Quality

The screenshot server supports quality from 10-90. The client adjusts based on fetch time:

```typescript
if (fetchTime > 500) {
  // Too slow - decrease quality aggressively
  newQuality = Math.max(10, currentQuality - 10)
} else if (fetchTime < 300 && currentQuality < 90) {
  // Fast enough - increase quality slightly
  newQuality = Math.min(90, currentQuality + 5)
}
```

Target: **2 FPS minimum** (500ms max per frame). On a bad connection, you get 2 FPS at quality 10. On a good connection, you get 10 FPS at quality 90.

## The Full Picture

Here's what the user experiences:

1. **Good connection** (RTT < 150ms): Full 60fps H.264 video via WebCodecs
2. **Bad connection detected**: System locks to screenshot mode, pauses video stream
3. **Banner appears**: "High latency detected — using screenshots (X FPS)"
4. **User can retry**: Click the speed icon to unlock and try video again
5. **Input works throughout**: Keyboard and mouse never stop working

The speed icon color tells you everything:
- **Green**: Adaptive, using video
- **Amber**: Adaptive, locked to screenshots
- **White**: Forced high quality (60fps only)
- **Orange**: Forced screenshot mode

## Why This Matters

Most remote desktop solutions have one mode. If your connection can't handle it, you're screwed.

We have three modes that smoothly degrade:

| Mode | Bandwidth | Latency | FPS |
|------|-----------|---------|-----|
| Video (60fps) | 20-40 Mbps | <50ms | 60 |
| Screenshots (adaptive) | 100-500 Kbps | 50-500ms | 2-10 |
| Screenshots (forced) | 50-200 Kbps | any | 2-10 |

The video stream stays connected for input. The screenshots provide visual feedback. The WebSocket handles both.

**No UDP. No TURN servers. No WebRTC. Just HTTP/HTTPS that works everywhere.**

## Try It Yourself

Helix is open source: [github.com/helixml/helix](https://github.com/helixml/helix)

The streaming code is in:
- `moonlight-web-stream/` — Rust WebSocket server
- `frontend/src/lib/moonlight-web-ts/stream/` — TypeScript client
- `api/cmd/screenshot-server/` — Go screenshot server

The key files:
- `ws_protocol.rs` — Protocol definition
- `stream.rs` — Server-side video pause handling
- `websocket-stream.ts` — Client with `setVideoEnabled()`
- `MoonlightStreamViewer.tsx` — React component with adaptive logic

---

*Built by the Helix team. We're building open-source AI infrastructure that actually works in enterprise environments. Star us on GitHub if you found this useful!*
