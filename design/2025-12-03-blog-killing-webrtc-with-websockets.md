# We Killed WebRTC (And Nobody Noticed)

*How we replaced WebRTC with plain WebSockets for real-time GPU streaming*

---

At [Helix](https://helix.ml), we run AI coding agents in GPU-accelerated containers. Users watch these agents work through a live video stream—think remote desktop, but for AI. The standard solution for browser-based real-time video is WebRTC. After months of TURN server hell, we threw it out and replaced it with plain WebSockets.

The result? Lower latency, simpler infrastructure, and it works everywhere.

## The Problem With WebRTC

WebRTC is designed for peer-to-peer video calls. It handles NAT traversal, codec negotiation, adaptive bitrate, and packet loss recovery. It's an impressive piece of engineering.

But we don't need peer-to-peer. Our architecture is strictly client-server:

```
Browser → Proxy → moonlight-web → Wolf (GPU encoder)
```

For this use case, WebRTC's complexity becomes pure liability.

### TURN Server Hell

Enterprise customers don't allow random UDP ports. They have L7 load balancers that only speak HTTP/HTTPS on port 443. WebRTC requires:

- UDP 3478 (STUN)
- TCP 3478 (TURN)
- UDP 49152-65535 (media relay)

Getting these through a corporate firewall? Good luck. We spent weeks debugging TURN configurations. coturn, Twilio, custom deployments—each had its own failure modes. The "TCP fallback" that TURN promises? In practice, it's unreliable and adds 50-100ms of latency.

We had a customer whose WebRTC connections worked 80% of the time. The other 20%? Black screen. No error message. WebRTC's ICE negotiation would silently fail after 30 seconds of "connecting..."

### The Insight

Here's the thing: **WebSockets work everywhere**. They're just HTTP upgrade. Every L7 proxy handles them. CloudFlare, Akamai, nginx, Kubernetes ingress—all work out of the box.

And for real-time video, WebSockets might actually be *faster* than WebRTC in our architecture:

1. **No jitter buffer** - We can render frames immediately
2. **No TURN relay** - Direct connection through existing proxy
3. **No ICE negotiation** - Connection established in one round-trip

The trade-off is TCP's head-of-line blocking. But on modern networks with low packet loss? Barely matters.

## The Implementation

We stream using the [Moonlight protocol](https://github.com/moonlight-stream)—the same tech that powers NVIDIA GameStream. Wolf (our server) encodes video with NVIDIA's hardware encoder. The browser decodes and displays it.

Previously, our architecture looked like this:

```
Wolf → Moonlight → [RTP packets] → WebRTC → Browser
                                    ↑
                                 TURN server
```

Now it's:

```
Wolf → Moonlight → [NAL units] → WebSocket → Browser
                                    ↑
                              Your existing HTTPS
```

### Binary Protocol

We defined a minimal binary protocol:

```
┌────────────┬────────────────────────────────────┐
│ Type (1B)  │ Payload (variable)                 │
└────────────┴────────────────────────────────────┘

Message Types:
  0x01 - Video Frame
  0x02 - Audio Frame
  0x10 - Keyboard Input
  0x11 - Mouse Click
  0x12 - Mouse Position
  0x13 - Mouse Movement
```

Video frames are raw H264 NAL units—no RTP packetization. Audio is Opus frames. Input goes the other direction.

### WebCodecs for Decoding

The browser-side uses [WebCodecs API](https://developer.mozilla.org/en-US/docs/Web/API/WebCodecs_API), which landed in Chrome 94 and recently in Firefox 130:

```typescript
const decoder = new VideoDecoder({
  output: (frame) => {
    ctx.drawImage(frame, 0, 0)
    frame.close()
  },
  error: console.error,
})

decoder.configure({
  codec: 'avc1.4d0032',  // H264 Main Profile
  hardwareAcceleration: 'prefer-hardware',
  avc: { format: 'annexb' },  // NAL unit format
})

ws.onmessage = (event) => {
  const data = new Uint8Array(event.data)
  if (data[0] === 0x01) {  // Video frame
    decoder.decode(new EncodedVideoChunk({
      type: isKeyframe ? 'key' : 'delta',
      timestamp: parsePTS(data),
      data: data.slice(HEADER_SIZE),
    }))
  }
}
```

Hardware-accelerated H264 decoding, straight to canvas. No MediaSource buffering. No jitter buffer. Frame arrives, frame renders.

### Audio Sync

Audio uses the same approach with `AudioDecoder` and `AudioContext`. We schedule playback based on presentation timestamps:

```typescript
const scheduledTime = audioStartTime + (framePTS - basePTS) / 1_000_000
source.start(Math.max(scheduledTime, audioContext.currentTime))
```

First audio frame establishes the baseline. Subsequent frames are scheduled relative to it. If a frame arrives too late (>100ms behind), we drop it rather than accumulating latency.

### Input Forwarding

Input goes the other direction—same WebSocket, same binary format. We reuse the existing Moonlight input protocol:

```typescript
sendMouseButton(isDown: boolean, button: number) {
  const buf = new Uint8Array([0x02, isDown ? 1 : 0, button])
  ws.send(new Uint8Array([0x11, ...buf]))  // 0x11 = MouseClick
}
```

Server parses and forwards to the Moonlight stream, which injects into the Linux input subsystem. Click in browser → click in remote desktop.

## What We Lost

Nothing is free. Here's what WebRTC gave us that we had to handle ourselves:

### 1. Adaptive Bitrate

WebRTC monitors network conditions and adjusts bitrate automatically. We don't. Our bitrate is fixed at connection time. For enterprise deployments on stable networks, this is fine. For variable mobile connections, it might be a problem.

### 2. Packet Loss Recovery

WebRTC uses NACK and PLI to request retransmission of lost packets. With TCP, we get reliable delivery but head-of-line blocking. A lost packet stalls the stream until retransmitted.

In practice? On datacenter-quality networks, packet loss is rare. When it happens, TCP recovers fast enough that users don't notice.

### 3. Browser Fallbacks

WebCodecs requires Chrome 94+, Safari 16.4+, or Firefox 130+. Older browsers get nothing. We could add MSE-based fallback, but haven't needed it—our users are on modern browsers.

## What We Gained

### Works Everywhere

Literally everywhere. No firewall configuration. No TURN servers. No debugging ICE negotiation. The WebSocket connection just... works.

### Simpler Infrastructure

Before:
- coturn TURN server (or Twilio, $$$)
- STUN server
- ICE configuration management
- Certificate management for TURN-over-TLS
- UDP port ranges

After:
- Your existing HTTPS proxy

### Lower Latency

Without the jitter buffer and TURN relay, we measured 20-30ms lower end-to-end latency. WebRTC's adaptive bitrate sometimes caused quality drops that took seconds to recover. Our fixed bitrate is... fixed.

### Debuggability

WebRTC failures are famously opaque. "ICE connection failed" tells you nothing. WebSocket failures? You get HTTP status codes, error messages, stack traces. When something breaks, you know why.

## Should You Do This?

Probably not, unless:

1. **Your architecture is client-server** - Peer-to-peer genuinely needs WebRTC
2. **Your users are behind restrictive firewalls** - If TURN works for you, keep using it
3. **You control the encoder** - We use Moonlight/Wolf which gives us raw NAL units
4. **Your target browsers support WebCodecs** - No IE11 here

But if you're building real-time video streaming to browsers, and WebRTC's complexity is killing you, know that there's another way.

## The Code

Both repos are open source:

- **[helix](https://github.com/helixml/helix)** - The frontend + API (TypeScript/React/Go)
- **[moonlight-web-stream](https://github.com/helixml/moonlight-web-stream)** - The streaming server (Rust)

The WebSocket streaming code is on the `feature/websocket-only-streaming` branch. Look for `WebSocketStream` in the TypeScript and `run_websocket_only_mode` in the Rust.

---

*We're building AI coding agents that work in GPU-accelerated containers. If you're interested in remote development environments, AI pair programming, or just want to see this streaming tech in action, check out [helix.ml](https://helix.ml).*

*—Luke Marsden, CEO @ Helix*

---

## Discussion Questions for HN

1. Has anyone else replaced WebRTC with WebSockets for real-time video? What was your experience?

2. We're considering adding WebTransport as an alternative to WebSockets. Anyone have experience with it in production?

3. The WebCodecs API is relatively new. Are there edge cases we should watch out for?
