# Video Pipeline Web Worker

## Problem

Main thread contention causes video jitter:
- React renders block frame processing
- Polling (clipboard, session, screenshots) causes 100-300ms pauses
- Garbage collection pauses
- Result: 0ms min jitter (frames batching), 100+ ms max jitter

## Solution

Move entire video pipeline to a dedicated Web Worker using OffscreenCanvas.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         MAIN THREAD                              │
├─────────────────────────────────────────────────────────────────┤
│  React UI, Polling, Event Handlers                              │
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐      │
│  │ Mouse/KB     │───►│ postMessage  │───►│   Worker     │      │
│  │ Events       │    │ (input)      │    │              │      │
│  └──────────────┘    └──────────────┘    └──────┬───────┘      │
│                                                  │               │
│  ┌──────────────┐    ┌──────────────┐           │               │
│  │ Stats UI     │◄───│ postMessage  │◄──────────┘               │
│  │ (1x/sec)     │    │ (stats)      │                           │
│  └──────────────┘    └──────────────┘                           │
│                                                                  │
│  ┌──────────────┐                                               │
│  │ <canvas>     │ ◄── Renders automatically via OffscreenCanvas │
│  └──────────────┘                                               │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                         WEB WORKER                               │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐      │
│  │ WebSocket    │───►│ VideoDecoder │───►│ OffscreenCanvas    │
│  │ (frames)     │    │ (WebCodecs)  │    │ .drawImage() │      │
│  └──────────────┘    └──────────────┘    └──────────────┘      │
│                                                                  │
│  ┌──────────────┐                                               │
│  │ Input Queue  │◄─── postMessage from main thread              │
│  │ → ws.send()  │                                               │
│  └──────────────┘                                               │
└─────────────────────────────────────────────────────────────────┘
```

## Key APIs

```typescript
// Main thread: Transfer canvas to worker
const canvas = document.getElementById('video-canvas');
const offscreen = canvas.transferControlToOffscreen();
worker.postMessage({ type: 'init', canvas: offscreen }, [offscreen]);

// Worker: Create WebSocket and decoder
const ws = new WebSocket(url);  // WebSocket works in workers!
const decoder = new VideoDecoder({
  output: (frame) => {
    ctx.drawImage(frame, 0, 0);
    frame.close();
  },
  error: (e) => console.error(e)
});

// Main thread: Send input events
canvas.onmousemove = (e) => {
  worker.postMessage({ type: 'mouse', x: e.offsetX, y: e.offsetY }, );
};

// Worker: Forward input to WebSocket
onmessage = (e) => {
  if (e.data.type === 'mouse') {
    ws.send(encodeMouseMove(e.data.x, e.data.y));
  }
};
```

## Message Protocol

### Main → Worker
| Type | Data | Transferable |
|------|------|--------------|
| `init` | `{ canvas, wsUrl, config }` | `canvas` |
| `mouse` | `{ x, y, buttons }` | - |
| `keyboard` | `{ key, code, down }` | - |
| `setVideoEnabled` | `{ enabled }` | - |
| `close` | - | - |

### Worker → Main
| Type | Data |
|------|------|
| `stats` | `{ fps, rtt, jitter, ... }` |
| `connected` | `{ width, height }` |
| `disconnected` | `{ reason }` |
| `cursor` | `{ url, hotspotX, hotspotY }` |
| `error` | `{ message }` |

## Files to Create/Modify

### New Files
- `src/lib/helix-stream/workers/video-worker.ts` - Worker entry point
- `src/lib/helix-stream/workers/video-worker-protocol.ts` - Message types

### Modified Files
- `src/lib/helix-stream/stream/websocket-stream.ts` - Refactor to worker orchestrator
- `src/components/external-agent/DesktopStreamViewer.tsx` - Use worker-based stream

## Browser Support

| Browser | OffscreenCanvas | VideoDecoder in Worker | WebSocket in Worker |
|---------|-----------------|------------------------|---------------------|
| Chrome 69+ | ✅ | ✅ | ✅ |
| Firefox 105+ | ✅ | ✅ | ✅ |
| Safari 16.4+ | ✅ | ✅ | ✅ |
| Edge 79+ | ✅ | ✅ | ✅ |

## Implementation Steps

1. Create worker file with WebSocket + VideoDecoder
2. Create message protocol types
3. Create main-thread orchestrator class
4. Migrate input handling to postMessage
5. Migrate cursor updates to postMessage
6. Add fallback for unsupported browsers
7. Update DesktopStreamViewer to use new class

## Risks

1. **Cursor latency**: Custom cursor CSS must be applied on main thread
   - Mitigation: postMessage cursor updates, accept ~1 frame latency

2. **Input latency**: Mouse events go main→worker→WebSocket
   - Mitigation: Use Transferable objects, batch high-frequency events

3. **Debugging**: Worker errors harder to trace
   - Mitigation: Good error forwarding to main thread

## Success Metrics

- Receive jitter min > 10ms (no 0ms batching)
- Receive jitter max < 30ms (no 100ms+ pauses)
- Video unaffected by React renders or polling
