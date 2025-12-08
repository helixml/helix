# Keyframes-Only Mode Debugging

**Date:** 2025-12-08
**Status:** DISABLED - Pending investigation
**Feature:** Low-bandwidth fallback (~1fps) streaming mode

## Overview

The keyframes-only mode was intended to provide a low-bandwidth fallback for users with poor network connections. When enabled, the streaming system would:

1. Set source FPS to 15 (indicating low-quality mode to the encoder)
2. Drop all non-keyframe (P-frame) packets on the backend
3. Send only IDR (keyframe) frames to the client
4. At GOP=60 with 60fps source, this should result in ~1fps of corruption-free video

## Expected Behavior

With GOP (Group of Pictures) configured at 60 frames:
- Source: 60fps capture
- Keyframe interval: Every 60 frames = 1 keyframe per second
- Expected output: 1 corruption-free frame per second
- Client receives only self-contained IDR frames (no P-frame dependencies)

## Observed Behavior

**Both H.264 and H.265 exhibit the same issue:**

1. Stream connects successfully
2. Stream reports 15fps (LQ mode active)
3. Only ONE keyframe is ever received (typically frame ~121)
4. No subsequent frames are delivered to `submit_decode_unit`
5. WebSocket connection stays open for ~14 seconds
6. Connection closes with "Failed to send audio frame: Closed"

### Key Log Observations

```
[WebSocket] Stream setup: 1920x1080x15 and H264
[WebSocket] Keyframe received (frame 121), sending
[WebSocket] Failed to send audio frame: Closed
```

The `submit_decode_unit` function is called exactly ONCE and then never again, despite 14 seconds of stream runtime.

## Implementation Details

### Backend (Rust - streamer/src/video.rs)

```rust
impl VideoDecoder for WebSocketVideoDecoder {
    fn submit_decode_unit(&mut self, unit: VideoDecodeUnit<'_>) -> DecodeResult {
        // ...
        let is_keyframe = matches!(unit.frame_type, FrameType::Idr);

        if self.keyframes_only {
            if is_keyframe {
                info!("[WebSocket] Keyframe received (frame {}), sending", unit.frame_number);
            } else {
                // Drop non-keyframes
                return DecodeResult::Ok;
            }
        }
        // ... send frame via IPC
        DecodeResult::Ok
    }
}
```

### Frontend Configuration (MoonlightStreamViewer.tsx)

```typescript
if (qualityMode === 'low') {
    streamSettings.fps = 15;  // Tells server to use keyframes-only mode
    streamSettings.bitrate = 2000;  // 2 Mbps for keyframes-only
    qualitySessionId = sessionId ? `${sessionId}-lq` : undefined;
}
```

### Backend rate limiting (moonlight_proxy.go)

Added 3-second rate limiting to prevent Wolf deadlock from rapid reconnects:

```go
const streamingRateLimitDuration = 3 * time.Second

func (apiServer *HelixAPIServer) checkStreamingRateLimit(sessionID string) bool {
    // Returns false if same session tried to connect within 3 seconds
}
```

## Investigation Progress

### What we checked:

1. **Codec type**: Tested both H.264 and H.265 - same issue with both
2. **GOP configuration**: Verified GOP=60 in encoder settings is correct
3. **Debug logging**: Added frame logging to `submit_decode_unit` - confirmed only one call
4. **Rate limiting**: Implemented backend rate limiting to prevent rapid reconnects
5. **Frame type detection**: `FrameType::Idr` detection works (the one keyframe IS sent)

### What we ruled out:

- **Codec-specific**: Issue occurs with both H.264 and H.265
- **Frontend timing**: WebSocket stays open for 14 seconds
- **IPC channel**: First frame IS sent successfully

### Likely root causes (not yet confirmed):

1. **Moonlight protocol layer stops delivering frames after first keyframe**
   - The Moonlight common library may have flow control that expects ACKs
   - Dropping P-frames silently may confuse the protocol

2. **Encoder stops producing frames**
   - The Wolf/GStreamer encoder may stop when it detects frames aren't being consumed

3. **IPC backpressure from dropped frames**
   - The unbounded channel may have some hidden backpressure behavior

## Files Involved

- `frontend/src/components/external-agent/MoonlightStreamViewer.tsx` - Quality mode toggle (DISABLED)
- `frontend/src/lib/moonlight-web-ts/stream/video.ts` - WebCodecs configuration
- `moonlight-web-stream/moonlight-web/streamer/src/video.rs` - WebSocketVideoDecoder
- `moonlight-web-stream/moonlight-web/common/src/ipc.rs` - IPC messages
- `helix/api/pkg/server/moonlight_proxy.go` - Rate limiting

## Next Steps

To properly debug this issue:

1. **Add detailed frame logging in the Moonlight common library**
   - Log every frame received from Wolf before it reaches the decoder
   - Identify if frames stop being delivered or stop being generated

2. **Check Wolf encoder logs**
   - Verify encoder is still producing frames after first keyframe

3. **Test with frame acknowledgment**
   - Instead of silently dropping P-frames, send a "skip" acknowledgment

4. **Test without keyframes-only mode but with very low FPS**
   - Set source to 1fps (if supported) instead of filtering frames

5. **Check Moonlight protocol spec**
   - Look for flow control or acknowledgment requirements

## Current Status

The entire quality mode toggle has been **disabled** in the frontend:
- Both `adaptive` and `low` modes are disabled
- Quality mode is permanently locked to `high` (60fps)
- The quality toggle button is commented out in MoonlightStreamViewer.tsx
- All code is preserved and can be re-enabled by uncommenting

### To re-enable:
1. Uncomment the quality toggle button section in `MoonlightStreamViewer.tsx` (search for "DISABLED: Quality mode toggle")
2. The original cycle logic (adaptive -> high -> low -> adaptive) is preserved in the commented code

## Related Backend Changes

The rate limiting feature added during this debugging IS kept active:
- Protects against DOS from rapid reconnection attempts
- Returns HTTP 429 for connections within 3 seconds of previous connection
- Prevents Wolf deadlock from rapid stream restart loops

## References

- Original implementation: WebSocket streaming mode for low-latency fallback
- Moonlight protocol: Based on NVIDIA GameStream protocol
- WebCodecs API: Used for hardware-accelerated decoding in browser
