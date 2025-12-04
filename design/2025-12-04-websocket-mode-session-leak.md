# WebSocket Mode Session Leak Investigation

**Date:** 2025-12-04
**Status:** In Progress

## Problem Statement

When switching streaming modes in the browser:
- **WebRTC → close**: Cleans up properly (threads return to baseline)
- **WebSocket → close**: LEAKS threads (4 threads per close)

Additionally, the Moonlight state dashboard has regressed:
- Shows "Blank" and "Select Agent" apps with "0 clients"
- Shows "No Moonlight clients" even when connected

## Investigation History

### Previous Fixes Applied (This Session)

1. **WebRTC mode: call cancel() before drop()** - Fixed WebRTC cleanup order
2. **Added PauseStreamEvent handlers to test pattern producers** - Test pattern producers now stop on both StopStreamEvent AND PauseStreamEvent

These fixes resolved WebRTC cleanup but NOT WebSocket cleanup.

## Architecture Overview

### WebRTC Mode Flow
```
Browser disconnects
  → WebRTC peer state change
  → StreamConnection::stop()
  → host.cancel() [fires StopStreamEvent in Wolf]
  → drop(stream) [ENET disconnect → PauseStreamEvent in Wolf]
  → Test pattern producers quit on Pause/Stop events
```

### WebSocket-Only Mode Flow
```
Browser closes WebSocket
  → web-server detects close in ws_stream_handler
  → sends ServerIpcMessage::Stop to streamer
  → streamer breaks main loop
  → host.cancel() [fires StopStreamEvent in Wolf]
  → drop(stream) [ENET disconnect → PauseStreamEvent in Wolf]
  → Test pattern producers SHOULD quit...
```

## Key Questions

1. Why does WebSocket cleanup leak when WebRTC cleanup doesn't?
2. Is cancel() finding the session in WebSocket mode?
3. Is PauseStreamEvent being fired with the correct session_id?
4. Why is the dashboard not showing connected clients?

## Files Involved

- `/prod/home/luke/pm/moonlight-web-stream/moonlight-web/web-server/src/api/stream.rs`
  - `start_host()` - WebRTC endpoint
  - `ws_stream_handler()` - WebSocket-only endpoint

- `/prod/home/luke/pm/moonlight-web-stream/moonlight-web/streamer/src/main.rs`
  - `StreamConnection::stop()` - WebRTC cleanup
  - `run_websocket_only_mode()` - WebSocket-only mode

- `/prod/home/luke/pm/wolf/src/moonlight-server/streaming/streaming.cpp`
  - `start_test_pattern_producer()` - Video test pattern
  - `start_test_audio_producer()` - Audio test pattern

- `/prod/home/luke/pm/wolf/src/moonlight-server/control/control.cpp`
  - ENET event handling, fires PauseStreamEvent

## Debugging Notes

(To be filled in during investigation)

