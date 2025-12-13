# Adaptive Bitrate Networking Analysis

**Date:** 2025-12-13
**Component:** MoonlightStreamViewer.tsx adaptive bitrate algorithm

## Executive Summary

The current implementation is a reasonable first-pass at adaptive bitrate over TCP/WebSocket, but has several gaps that will cause suboptimal behavior on challenging networks. The fundamental issue is that **TCP already does congestion control** - our algorithm is fighting TCP's built-in mechanisms rather than working with them.

## How the Current Algorithm Works

1. **Measure throughput** via rolling 5-second window of observed bitrate
2. **Track max observed** throughput with 5% decay per minute
3. **Detect saturation** if receiving < 90% of requested bitrate
4. **Reduce** to 80% of what we're actually receiving when saturated
5. **Probe before increase** by fetching 3 screenshots and measuring throughput
6. **Increase** if stable 20s and probe shows 125% headroom

## Network Scenario Analysis

### 1. Corporate VPN (Typical: 10-50 Mbps, high latency, packet inspection)

**Characteristics:**
- 50-200ms base RTT (VPN tunnel overhead)
- Packet inspection adds jitter (10-50ms variance)
- Often bandwidth-limited by VPN concentrator, not last-mile
- TCP congestion window slow to grow due to high RTT (BDP issues)

**How current algorithm behaves:**
- ✅ Saturation detection will work - throughput will be lower than requested
- ⚠️ **Problem: Bandwidth probe is too short.** 3 sequential screenshots may not fill the TCP pipe. With 100ms RTT, TCP needs ~10 RTTs to reach steady-state throughput. Our probe completes in maybe 2-3 RTTs.
- ⚠️ **Problem: We're measuring application throughput, not link capacity.** VPN packet inspection causes bursts of delay followed by catch-up. Our 1-second samples may catch a burst or a gap.

**Recommendation:** Increase probe duration or use parallel requests to fill the pipe faster.

### 2. Train/Mobile 4G (Variable: 1-50 Mbps, highly variable, handoffs)

**Characteristics:**
- Bandwidth varies 10x within seconds (cell handoffs, signal strength)
- Latency spikes to 500ms+ during handoffs
- Packet loss during handoffs (TCP retransmits cause stalls)
- Buffer bloat in mobile network causes RTT to spike before throughput drops

**How current algorithm behaves:**
- ✅ 5% decay per minute handles gradual changes
- ⚠️ **Problem: Decay is too slow for cell handoffs.** Bandwidth can drop 10x in 1 second. We need to react in seconds, not minutes.
- ⚠️ **Problem: 10-second reduce cooldown is too slow.** On a bad handoff, user experiences 10 seconds of stutter before we reduce.
- ⚠️ **Problem: We don't detect RTT spikes.** Buffer bloat causes RTT to spike 200-500ms BEFORE throughput drops. By the time throughput drops, the user has already experienced seconds of latency.
- ✅ Probe before increase is good - prevents oscillation after recovery.

**Recommendation:** Add RTT-based early warning. If RTT doubles, preemptively reduce bitrate before throughput drops.

### 3. Satellite Internet (Starlink/GEO: 20-200 Mbps, high latency)

**Characteristics:**
- Starlink: 20-100ms latency, 50-200 Mbps, weather-dependent
- GEO (HughesNet): 600ms+ latency, 10-25 Mbps
- High bandwidth-delay product (BDP) - TCP slow start takes forever
- Jitter on Starlink during satellite handoffs (every 15 seconds)

**How current algorithm behaves:**
- ⚠️ **Problem: Bandwidth probe severely underestimates capacity.** With 600ms RTT, TCP needs 6+ seconds to reach full throughput. Our probe of 3 sequential fetches may complete in 2-3 seconds but never reaches steady state.
- ⚠️ **Problem: Sequential probe requests don't fill the pipe.** We wait for each response before sending next request. Should use parallel requests.
- ✅ High-latency networks are fine once stable - we're measuring application throughput, and if it's steady, we're good.

**Recommendation:** Use parallel probe requests. Fire all 3-5 requests simultaneously, measure total time for all to complete.

### 4. Reliable Corporate Network / Wired Ethernet

**Characteristics:**
- Consistent 100+ Mbps
- Low latency (1-10ms)
- No packet loss
- Predictable behavior

**How current algorithm behaves:**
- ✅ Will quickly observe high throughput and allow high bitrate
- ✅ Saturation detection works well
- ⚠️ **Minor issue: 20-second wait before increase is conservative.** On a reliable network, could increase faster.
- ⚠️ **Problem: Max observed never gets updated if we're already saturated.** If we start on 4G at 10 Mbps, then switch to ethernet, maxObserved is 10 Mbps. We'll only ever probe to 10 * 0.8 = 8 Mbps. Need the probe to update maxObserved even when checking for increase (this is actually done, good).

**Recommendation:** Consider faster increase on low-latency networks.

### 5. WiFi with Packet Loss (Typical home: variable, interference)

**Characteristics:**
- Bandwidth varies with interference (microwave, neighbors)
- Packet loss causes TCP retransmits → throughput drops but RTT may not spike immediately
- Channel congestion from neighbors causes periodic throughput dips

**How current algorithm behaves:**
- ✅ Throughput-based detection will catch sustained drops
- ⚠️ **Problem: 5-second rolling window may smooth out short drops.** A 2-second WiFi glitch causes stutter but avgThroughput may still look OK.
- ⚠️ **Problem: We don't detect packet loss directly.** TCP handles retransmits transparently, but we could detect it via RTT jitter (retransmit = 1 RTT delay for that data).

**Recommendation:** Consider RTT variance in addition to throughput. High jitter = unhealthy link.

## Identified Gaps and Issues

### Critical Issues

1. **Sequential probe doesn't fill high-BDP pipes**
   - 3 sequential requests with high RTT = never reaches steady-state throughput
   - **Fix:** Use parallel requests: `Promise.all([fetch(), fetch(), fetch()])`

2. **No RTT-based early warning**
   - Buffer bloat causes RTT to spike 200-500ms before throughput drops
   - User experiences latency before we react
   - **Fix:** Track RTT (we already have `wsStats.rttMs`), reduce if RTT > 2x baseline

3. **Reduce cooldown too slow for mobile handoffs**
   - 10-second cooldown = 10 seconds of stutter on cell handoff
   - **Fix:** Faster initial reduce (2-3 seconds), longer cooldown for subsequent reduces

4. **Max decay too slow for rapid changes**
   - 5% per minute won't help with 10x bandwidth drop in 1 second
   - **Fix:** Faster decay when we detect saturation, or instant reset on severe saturation

### Medium Issues

5. **Probe requires active session for screenshots**
   - What if screenshot endpoint is slow for other reasons (server load)?
   - **Fix:** Consider using a dedicated bandwidth test endpoint that just returns random bytes

6. **No variance tracking**
   - We track average but not variance. High variance = unstable link = stay conservative.
   - **Fix:** Track stddev of throughput samples, stay at lower bitrate if variance > 20%

7. **Bitrate jumps are large**
   - Options are [5, 10, 20, 40, 80] - 2x jumps
   - After reduce, we might overshoot on increase
   - **Fix:** Consider intermediate steps: [5, 8, 10, 15, 20, 30, 40, 60, 80]

### Minor Issues

8. **Probe timing includes server-side screenshot generation**
   - Screenshot endpoint has to capture screen, encode JPEG - adds latency unrelated to network
   - This makes probe underestimate network capacity
   - **Fix:** Use a dedicated probe endpoint or subtract estimated server processing time

9. **No distinction between download and upload**
   - We only measure download (stream to client), but video is download-only anyway
   - For input (upload), we send tiny packets - not bandwidth-limited
   - **OK for current use case**

## TCP Behavior Considerations

### Why This Is Hard Over TCP

TCP has its own congestion control (BBR, CUBIC, etc.) that:
1. **Probes for bandwidth** by slowly increasing send rate
2. **Reacts to loss/RTT** by backing off
3. **Fills buffers** before signaling congestion (buffer bloat)

Our algorithm sits on top of TCP and sees only the result. Issues:

- **We can't send faster than TCP allows.** Even if we request 80 Mbps, TCP may only give us 20 Mbps due to its own congestion window.
- **TCP hides packet loss.** We see throughput drop, not the underlying cause.
- **TCP slow start after idle.** After 10 seconds of low bitrate, TCP connection goes idle and congestion window shrinks. When we increase bitrate, there's a ramp-up period.

### What We Can Do

1. **Trust TCP's throughput as ground truth.** If TCP gives us 20 Mbps, that's what the path can sustain.
2. **React to RTT earlier than throughput.** RTT spikes before throughput drops (buffers filling).
3. **Keep the connection warm.** Avoid idle periods that cause TCP slow start.
4. **Use parallel requests for probes.** Multiple concurrent TCP flows fill the pipe faster.

## Recommended Improvements

### Priority 1: Parallel Probe Requests

```typescript
// Current: Sequential (slow on high-RTT)
for (let i = 0; i < probeCount; i++) {
  const response = await fetch(...);
}

// Better: Parallel (fills pipe faster)
const probePromises = Array.from({ length: probeCount }, () =>
  fetch(`/api/v1/external-agents/${sessionId}/screenshot?format=jpeg&quality=90`)
    .then(r => r.blob())
    .then(b => b.size)
    .catch(() => 0)
);
const sizes = await Promise.all(probePromises);
const totalBytes = sizes.reduce((a, b) => a + b, 0);
```

### Priority 2: RTT-Based Early Warning

```typescript
// Track baseline RTT
const baselineRttRef = useRef<number>(0);

// In checkBandwidth:
const currentRtt = wsStats.rttMs;

// Establish baseline during good periods
if (saturation >= SATURATION_THRESHOLD && currentRtt > 0) {
  if (baselineRttRef.current === 0 || currentRtt < baselineRttRef.current) {
    baselineRttRef.current = currentRtt;
  }
}

// Early warning: RTT doubled = buffers filling = reduce soon
if (currentRtt > baselineRttRef.current * 2 && currentRtt > 100) {
  console.log(`[AdaptiveBitrate] RTT spike: ${currentRtt}ms vs baseline ${baselineRttRef.current}ms`);
  // Preemptively reduce before throughput drops
}
```

### Priority 3: Faster Initial Reduce

```typescript
// First reduce after saturation: act fast (2s)
// Subsequent reduces: use normal cooldown (10s)
const INITIAL_REDUCE_COOLDOWN_MS = 2000;
const SUBSEQUENT_REDUCE_COOLDOWN_MS = 10000;

const reduceCooldown = recentlyReduced ? SUBSEQUENT_REDUCE_COOLDOWN_MS : INITIAL_REDUCE_COOLDOWN_MS;
```

### Priority 4: Throughput Variance Tracking

```typescript
// Calculate variance
const mean = avgThroughput;
const variance = observedThroughputRef.current.reduce(
  (sum, val) => sum + Math.pow(val - mean, 2), 0
) / observedThroughputRef.current.length;
const stddev = Math.sqrt(variance);
const coefficientOfVariation = stddev / mean;

// High variance = unstable, stay conservative
if (coefficientOfVariation > 0.3) {
  // Don't increase, maybe reduce proactively
}
```

## Testing Recommendations

1. **Chrome DevTools Network Throttling**
   - Test with "Slow 3G" (400ms RTT, 400 kbps)
   - Test with "Fast 3G" (100ms RTT, 1.5 Mbps)
   - Custom profile: 50ms RTT, 10 Mbps

2. **tc (traffic control) on Linux**
   ```bash
   # Add 200ms latency and 10% packet loss
   tc qdisc add dev eth0 root netem delay 200ms loss 10%
   ```

3. **Real-world testing**
   - Tethered to phone on 4G while walking
   - VPN to remote server
   - Starlink connection (if available)

## Conclusion

The current implementation is a solid foundation but will struggle on high-latency and variable networks. The biggest wins will come from:

1. **Parallel probe requests** - critical for high-RTT networks
2. **RTT-based early warning** - reduces user-perceived latency on congested links
3. **Faster initial reduce** - critical for mobile handoffs

The algorithm correctly identifies that we're measuring TCP's steady-state behavior, but needs to be more reactive to changing conditions and more aggressive at probing capacity on high-BDP links.
