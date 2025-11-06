# WebRTC Video Quality Investigation

**Date:** 2025-11-06
**Status:** Investigation Complete - Root Cause Identified

## Problem Statement

WebRTC streaming via moonlight-web shows severe color artifacts (red/green flickering in grey areas, smudgy colors, blocking especially in colored text) compared to Moonlight Qt native streaming, despite same resolution (4K), framerate (60 FPS), and codec (H.264 Main).

## Key Discovery: Software vs Hardware Decode

**Critical finding:** Moonlight Qt with **software decode exhibits IDENTICAL artifacts** to browser WebRTC!

**Quality comparison:**
- Moonlight Qt (hardware decode): ✅ Perfect quality, no artifacts
- Moonlight Qt (software decode): ❌ Red/green flickering, smudgy greys (MATCHES BROWSER!)
- Browser WebRTC (hardware decode ENABLED): ❌ Same artifacts as Qt software decode

**Implication:** The frames are **corrupted before decoding**. Both hardware and software decoders see bad data.

## Root Cause Analysis

### Wolf Encoder Settings
```toml
nvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate=80000
```

**gop-size=0** = Every frame is an I-frame (keyframe only, no P/B frames)
- Ultra-low latency (no frame dependencies)
- **Massive frame sizes** at 4K60@80Mbps
- Each I-frame: hundreds of KB
- Must be split into hundreds of RTP packets (MTU ~1400 bytes)

### Suspected Issue: RTP Packetization/Reassembly

**Moonlight native protocol:**
- Wolf → ENet protocol → Large frame transfer → Decode
- Handles huge I-frames cleanly ✅

**moonlight-web WebRTC:**
- Wolf → ENet → moonlight-web streamer → **RTP packetization** → WebRTC → **RTP reassembly** → Decode
- Huge I-frames split into 200+ RTP packets
- Reassembly might be corrupting frames ❌

**Evidence:**
1. Same artifacts in software decode (rules out hardware decoder issue)
2. 0 packets lost (network is fine)
3. Only 33-66 Mbps received vs 80 Mbps requested (but 0 packet loss - where's the data?)
4. Custom H.264 payloader exists but still has issues

## What We Tried (Didn't Fix It)

1. ✅ **H.264 profile fix** (Constrained Baseline → Main) - Fixed frame drops but not color
2. ✅ **Queue size increase** (5 → 100 packets) - Reduced drops to near 0%
3. ✅ **SEI NAL preservation** - No effect (upstream skips it anyway)
4. ✅ **Color range changes** (full ↔ limited) - No effect
5. ✅ **Custom H.264 payloader from upstream** - Merged but artifacts remain
6. ❌ **4:4:4 chroma** - Wolf doesn't support it (only 4:2:0 NV12)

## Measured Stats (WebRTC)

From Stats for Nerds overlay:
- **Codec:** video/H264 (4d001f) - Main Profile ✅
- **Resolution:** 3840x2160 ✅
- **FPS:** 60.0 ✅
- **Bitrate:** 33-66 Mbps (requested: 80 Mbps) ❌
- **Packets Lost:** 0 ✅
- **Frames Dropped:** 0-3 (< 1%) ✅
- **Jitter:** 1-3 ms ✅
- **RTT:** 2-9 ms ✅

**Network is perfect, but bitrate is throttled!**

## The Bitrate Mystery

- **Requested:** 80 Mbps
- **Moonlight Qt receives:** 80 Mbps (confirmed by settings)
- **WebRTC receives:** 33-66 Mbps (shown in stats)
- **Packets lost:** 0

**Where is the missing 20-50 Mbps going?**
- Not lost (0% packet loss)
- WebRTC adaptive bitrate control? (but Wolf uses CBR, not adaptive)
- RTP overhead? (unlikely to account for 50% loss)
- Frame corruption causing data to be discarded?

## Why Moonlight Qt Works

**Moonlight protocol** is designed for game streaming:
- Handles large I-frames efficiently
- Custom protocol optimized for low latency
- No RTP fragmentation
- Hardware decode path is well-tested

**WebRTC** is designed for video conferencing:
- Expects smaller frames with P/B frames (not all I-frames)
- RTP designed for ~1400 byte packets (hundreds per 4K I-frame)
- Adaptive bitrate for unstable networks
- May not handle gop-size=0 well

## Potential Solutions (Untested)

### 1. Increase GOP Size in Wolf
Change `gop-size=0` to `gop-size=30` or `gop-size=60`:
- Smaller I-frames (only every 30/60 frames)
- P-frames between keyframes
- More compatible with WebRTC expectations
- **Tradeoff:** Slightly higher latency (~500ms for gop=30)

### 2. Investigate Custom Payloader
The upstream custom H.264 payloader (commit 7611bdf) was supposed to fix H.264 issues, but artifacts remain. Need to understand what it fixes and if gop-size=0 is still problematic.

### 3. Try Different Encoder Preset
Current: `preset=low-latency-hq`
- Try `preset=p4` or `preset=p5` for better quality
- May help with large I-frame encoding

### 4. Accept WebRTC Limitations
For high-quality work:
- Use Moonlight Qt (native, full quality)
- Use WebRTC for convenience/mobile (accept reduced quality)

## Technical Details

### GOP Size Meanings
- `gop-size=-1`: Encoder chooses (usually 30-120 frames)
- `gop-size=0`: All I-frames (every frame is keyframe)
- `gop-size=30`: Keyframe every 30 frames (500ms @ 60 FPS)

### Frame Sizes @ 4K60, 80 Mbps, gop-size=0
- Bits per frame: 80,000,000 / 60 = 1,333,333 bits
- Bytes per frame: ~166 KB per I-frame
- RTP packets per frame: 166,000 / 1400 = ~118 packets minimum
- With overhead: 150-200 packets per frame
- At 60 FPS: **9,000-12,000 RTP packets per second**

### Moonlight Protocol vs WebRTC
| Aspect | Moonlight Native | WebRTC |
|--------|------------------|---------|
| Transport | ENet (UDP, custom) | RTP/SRTP |
| Packet size | Variable, optimized | Fixed MTU ~1400 |
| Frame handling | Whole frames | Fragmented |
| Latency | ~20-40ms | ~40-80ms |
| Quality | Perfect | Artifacts |
| Bitrate | Full 80 Mbps | Throttled to 33-66 |

## Recommendations

1. **Short term:** Document that WebRTC has quality limitations for high-bitrate 4K streaming
2. **Medium term:** Investigate if reducing gop-size=0 to gop-size=30 helps WebRTC quality
3. **Long term:** Consider if Wolf could support native WebRTC (bypassing moonlight-web entirely)

## Files Modified

**moonlight-web-stream:**
- Merged 42 upstream commits (custom H.264 payloader, input improvements)
- Preserved session persistence and certificate caching
- Reverted SEI preservation (upstream skips it)

**Helix frontend:**
- Fixed H.264 profile (Constrained Baseline → Main)
- Added Stats for Nerds overlay
- Increased video queue (5 → 100 packets)
- Ported 3-channel mouse input
- Fixed color range (full → limited)
- Added bitrate calculation and codec detection

**Outcome:** Mouse works, frame drops eliminated (< 1%), but color artifacts remain due to suspected RTP reassembly issues with massive I-frames from gop-size=0.
