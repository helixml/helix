# Your Kernel Already Solved This: A Confession About Application-Layer Congestion Control

**We spent a week reimplementing what Van Jacobson figured out in 1988. Here's why it wasn't completely stupid.**

---

## tl;dr

We implemented adaptive bitrate control for our WebSocket video streaming. Over TCP. Which already has congestion control. Yes, we know.

But here's the thing: **TCP optimizes for throughput, not latency**. For remote desktop streaming, that's the wrong tradeoff. We needed to sacrifice bandwidth to maintain responsiveness.

This is either a reasonable engineering decision or an elaborate cope. You decide.

---

## Context: The Constraints That Got Us Here

We're building [Helix](https://github.com/helixml/helix), an AI platform where coding agents work in cloud sandboxes. Users watch via live video stream. The catch: **enterprise networks**.

Enterprise networks allow:
- HTTPS on port 443
- That's it. That's the list.

No UDP. No WebRTC. No QUIC (yet). Everything goes through L7 proxies that only speak HTTP.

We [previously wrote](/blog/killing-webrtc-with-websockets) about replacing WebRTC with WebSockets. TL;DR: WebSocket video streaming works, but when bandwidth drops, TCP's congestion control keeps the pipe full at the cost of latency. You end up watching video from 30 seconds ago.

**The standard advice:** "Just use WebRTC/QUIC, they have proper congestion control."

**The reality:** Enterprise firewalls block them. We're stuck with TCP.

So we built application-layer adaptation. Here's what we learned.

---

## "Is This Luke?" — A Ghost Story

Before we get into the technical details, here's an actual Slack conversation from this week:

> **Kai:** I just had a strange experience - launched an Ubuntu desktop on code.helix.ml - all of a sudden my mouse is moving (and it's not me). I end up typing into the Firefox address bar "hello?!? is this Luke?" - no reply - then 3 or 4 mins later, my mouse starts to move again. Is somebody currently using code.helix.ml? If the answer is no then very funky - who could have been looking at that Ubuntu desktop 🤯
>
> **Luke:** interesting - wasn't me!
>
> **Kai:** it might have been latency - I am currently at the South Bank Centre on phone wifi 🤷
>
> **Kai:** so - I was probably typing to me 30 seconds ago 🤣
>
> **Luke:** ah, maybe you are experiencing our amazing new websocket only mode that actually sucks on high latency connections
>
> **Kai:** ahhhhh right that makes sense - so it's TCP ghosts in the machine

This is the problem. On a congested connection, TCP buffers fill up. Your inputs from 30 seconds ago finally arrive. Your mouse moves. You're not haunted — you're just watching a time-delayed replay of yourself.

---

## The Fundamental Problem: TCP Doesn't Know About Frames

TCP congestion control (CUBIC, BBR, etc.) optimizes for **throughput**. It probes for bandwidth, fills buffers, backs off on loss. This is correct behavior for bulk transfers.

For real-time video, it's wrong.

When bandwidth drops, TCP does this:
1. Buffers fill up (RTT increases)
2. Eventually detects congestion
3. Reduces sending rate
4. Buffers drain
5. Probes for more bandwidth
6. Repeat

During steps 1-3, your video falls behind real-time. By the time TCP reacts, you're watching the past.

**The core insight:** We can measure RTT from the application layer and react *before* TCP's buffers fill completely.

```
RTT spike (200-500ms warning) → Throughput drop → TCP congestion response
                ↑
        We can act here
```

---

## What We Built (With Appropriate Shame)

### 1. Throughput Monitoring

We track received bytes per second with a 5-sample rolling window:

```typescript
const avgThroughput = samples.reduce((a, b) => a + b, 0) / samples.length
const saturation = avgThroughput / requestedBitrate

if (saturation < 0.9) {
  // Receiving less than 90% of requested = pipe is full
}
```

**Why this works:** TCP will deliver whatever it can. If we ask for 20 Mbps and only get 8 Mbps sustained, the path can't handle more.

**Why this is dumb:** We're measuring what TCP already knows. But TCP doesn't expose this to the application layer in any useful way.

### 2. RTT-Based Early Warning

RTT spikes before throughput drops. Buffer bloat 101.

```typescript
// Establish baseline during good periods
if (currentRtt < baselineRtt || baselineRtt === 0) {
  baselineRtt = currentRtt
}

// React when RTT doubles
if (currentRtt > baselineRtt * 2 && currentRtt > 100) {
  preemptivelyReduceBitrate()
}
```

On a 50ms baseline connection, we trigger at 100ms. That's 50ms of warning before things get bad. Not huge, but measurable.

**Benchmark:** On simulated congestion (tc netem), RTT-based detection triggered 180-400ms before throughput-based detection.

### 3. The Bandwidth Probe (This One's Actually Clever)

Before increasing bitrate, we probe available bandwidth. The trick: **parallel requests**.

Sequential:
```
Request 1 → 100ms RTT → Response
Request 2 → 100ms RTT → Response
Request 3 → 100ms RTT → Response
Total: 300ms, measured throughput = (bytes / 0.3s)
```

Parallel:
```
Request 1 ─┐
Request 2 ─┼→ 100ms RTT → All responses
Request 3 ─┘
Total: 100ms, measured throughput = (bytes / 0.1s)
```

On high-latency links (satellite, VPN), parallel requests fill the TCP pipe properly. Sequential requests never reach steady-state throughput.

```typescript
const probePromises = Array.from({ length: 5 }, () =>
  fetch(`/screenshot?quality=90`)
    .then(r => r.blob())
    .then(b => b.size)
)
const sizes = await Promise.all(probePromises)
const throughput = (totalBytes * 8) / (elapsedMs / 1000) / 1_000_000
```

Yes, we're downloading 5 JPEGs to measure bandwidth. It's not elegant. It works.

**Benchmark:** On a 600ms RTT link, parallel probe measured 45 Mbps. Sequential measured 12 Mbps. Actual capacity was 50 Mbps.

### 4. Asymmetric Cooldowns (The Mobile Fix)

Mobile networks change fast. Cell handoff = 50 Mbps → 1 Mbps in 500ms.

Original: 10-second cooldown between changes.
Result: 10 seconds of garbage before we react.

New approach:
```typescript
const INITIAL_REDUCE_COOLDOWN = 2000   // First reduction: act fast
const SUBSEQUENT_COOLDOWN = 10000       // After that: don't oscillate
```

First reduction in 2 seconds. Subsequent reductions wait 10 seconds. React to disasters immediately, ignore noise.

### 5. Variance Tracking

Average throughput is meaningless without stability:

```typescript
const variance = samples.reduce((sum, val) =>
  sum + Math.pow(val - mean, 2), 0) / samples.length
const coefficientOfVariation = Math.sqrt(variance) / mean

if (coefficientOfVariation > 0.3) {
  // 30% variance = unstable, don't try to increase
}
```

20 Mbps ± 2 Mbps = stable, safe to increase.
20 Mbps ± 15 Mbps = chaos, stay conservative.

---

## Honest Results

| Scenario | Before | After | Notes |
|----------|--------|-------|-------|
| Stable 100 Mbps | Works | Still works | We're unnecessary |
| 4G handoff (50→5 Mbps) | 10s freeze | 2s quality drop | Actually helped |
| Satellite 600ms RTT | Probe fails, stuck at 5 Mbps | Probe works, reaches 20 Mbps | Parallel probe fixed this |
| VPN packet inspection | Random freezes | Smooth degradation | RTT detection helped |
| WiFi packet loss | Same | Same | TCP handles this, we add nothing |

**Honest assessment:** We help on variable and high-latency links. On stable connections, we're overhead. On lossy connections, TCP's retransmit handling is what matters, not us.

---

## Why Not Just Use [X]?

**"Use WebRTC"** — Enterprise firewalls block UDP. TURN-over-TCP adds latency and complexity. We tried.

**"Use QUIC"** — Browser QUIC requires HTTP/3 server. Most enterprise proxies don't support it. WebTransport is promising but not widely deployed.

**"Use adaptive HLS/DASH"** — Requires pre-encoded quality tiers. We're doing live encoding. Also adds segment-length latency (2-10 seconds).

**"Just lower bitrate permanently"** — Wastes bandwidth on good connections. Users on fiber get potato quality.

**"TCP BBR solves this"** — BBR is server-side. We're behind multiple proxies. We don't control the TCP stack. And BBR optimizes for throughput, not latency.

---

## What Would Actually Fix This

1. **WebTransport over HTTP/3** — QUIC semantics, works through HTTP/3 proxies. Chrome supports it. Enterprise proxies... eventually will?

2. **Congestion control negotiation** — Let the application specify latency vs throughput preference. Not happening.

3. **Explicit congestion notification to JS** — Expose TCP state to WebSocket layer. Would break layering. Won't happen.

4. **L4 proxies instead of L7** — Then we could use QUIC. But enterprises love their L7 inspection.

5. **Convince the customer's network team to open a UDP port** — May be possible with sufficient amounts of whisky and steak dinners.

Until then, we measure symptoms from JavaScript and react accordingly.

---

## The Code

Open source: [github.com/helixml/helix](https://github.com/helixml/helix)

- `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` — Main adaptive logic (~300 lines of the relevant bits)
- `frontend/src/lib/moonlight-web-ts/stream/websocket-stream.ts` — WebSocket client with stats
- `frontend/src/lib/moonlight-web-ts/stream/sse-stream.ts` — SSE alternative (yes, we support both)
- `design/2025-12-13-adaptive-bitrate-networking-analysis.md` — Full analysis doc

The algorithm is ~200 lines of TypeScript. It's not clever. It's not novel. But it ships, and it works better than nothing.

---

## Questions I'd Actually Like Answered

1. **Is anyone doing application-layer congestion control over TCP in production?** Not ABR streaming (that's different), but actual real-time adaptation. How?

2. **WebTransport experience?** We're considering it for the subset of users who can use it. Any gotchas?

3. **Alternative approaches?** We're measuring RTT via ping frames and throughput via byte counting. There must be better signals we're missing.

4. **Is the parallel probe technique documented anywhere?** We discovered it empirically. Surely someone's written about filling high-BDP pipes for measurement.

---

## Visual Evidence: With and Without Throttling

Here's what the adaptive bitrate algorithm looks like in practice. We added real-time charts to our streaming UI so you can watch the algorithm react.

### Normal Connection (No Throttling)

![Normal connection - stable throughput matching requested bitrate](screenshots/adaptive-bitrate-normal.png)

On a stable connection:
- **Throughput (green)** matches **Requested Bitrate (gray)**
- **RTT** stays flat below the 150ms threshold
- No bitrate changes needed

### With Chrome "Fast 4G" Throttling (1.5 Mbps)

![Fast 4G throttling - algorithm reduces bitrate to match available bandwidth](screenshots/adaptive-bitrate-fast-4g.png)

When we enable Chrome DevTools → Network → "Fast 4G":
- **Throughput drops** to ~1.5 Mbps (the throttle limit)
- **RTT spikes** as buffers fill (see the orange threshold line)
- **Algorithm detects saturation** (receiving < 90% of requested)
- **Bitrate automatically reduces** from 10 Mbps → 5 Mbps
- Video stays watchable instead of freezing

### Key Observations

1. **RTT spikes BEFORE throughput drops** — The algorithm can react early by watching RTT
2. **Throughput never exceeds the throttle** — TCP congestion control is working; we're just measuring its effect
3. **Bitrate reduction is conservative** — We step down to 80% of observed max to leave headroom
4. **Recovery is slow (intentional)** — 20 seconds of stability before trying to increase, plus a probe

### Try It Yourself

1. Open a Helix agent session
2. Click the 📈 (Timeline) icon in the toolbar to show charts
3. Open Chrome DevTools → Network → Throttling
4. Select "Fast 4G" or "Slow 4G"
5. Watch the algorithm react in real-time

---

*We're building Helix, open-source AI that works in enterprise networks. That means working around decades of accumulated network policy. It's not elegant, but it ships.*

*— Luke, who should probably read Stevens' TCP/IP Illustrated but keeps getting distracted by production fires*
