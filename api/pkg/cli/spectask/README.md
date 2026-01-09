# Spectask CLI Testing

The `helix spectask` CLI provides tools for testing and debugging Helix sandbox sessions.

## Prerequisites

### 1. User Token (hl- API Key)

You need a valid `hl-` API key linked to your user account. This key is created through the Helix UI when you generate an API key. Keycloak is NOT required for user tokens - the API key is looked up directly from the database.

### 2. Environment Setup

Create `.env.usercreds` in the project root:
```bash
HELIX_API_KEY=hl-your-api-key-here
HELIX_URL=http://localhost:8080
HELIX_UBUNTU_AGENT=app_01kchs65wezc7ewxemj9px0gcv
HELIX_PROJECT=prj_01ke4cyxvk98mjedfmqet02hy4
```

Or use the dev token (no keycloak required, but limited functionality):
```bash
HELIX_API_KEY=oh-hallo-insecure-token
```

### 3. Wolf Container Running

Wolf must be running to create sandboxes:
```bash
docker ps | grep helix-sandbox
```

### 4. Ubuntu Agent Configured

Create an agent app with `agent_type: "zed_external"` and `desktop_type: "ubuntu"` in the Helix UI.

## Commands

### Start a Sandbox Session

```bash
source .env.usercreds
helix spectask start --project $HELIX_PROJECT --agent $HELIX_UBUNTU_AGENT -n "Test Task" --prompt "Description"
```

### List Active Sessions

```bash
helix spectask list
```

### Take Screenshot

```bash
helix spectask screenshot <session-id>
```

### Stream Video Statistics

Connect to a running session and display real-time video statistics:

```bash
# Basic usage - run until Ctrl+C
helix spectask stream <session-id>

# Run for 30 seconds
helix spectask stream <session-id> --duration 30

# Save raw video frames to file
helix spectask stream <session-id> --output video.h264

# Verbose mode - show each frame
helix spectask stream <session-id> -v
```

**Statistics displayed:**
- Resolution and codec (H.264/HEVC/AV1)
- Frame rate (FPS)
- Video bitrate (Mbps)
- Keyframe count
- Average/min/max frame sizes

### Stop a Session

```bash
helix spectask stop <session-id>
helix spectask stop --all  # Stop all sessions
```

## Video Stream Protocol

**Status:** Working! The `stream` command connects directly to the desktop container via RevDial, bypassing Wolf/Moonlight.

The `stream` command uses the WebSocket-only protocol (not WebRTC) for raw video frame access:

**Endpoint:** `/api/v1/external-agents/{session_id}/ws/stream`

**Connection Flow:**
1. CLI connects to WebSocket endpoint (proxied via RevDial to desktop container)
2. CLI sends `{ type: "init", session_id, client_unique_id, width, height, ... }`
3. Server responds with `StreamInit` (codec, resolution, FPS)
4. Server responds with `ConnectionComplete`
5. Server streams H.264 video frames directly from PipeWire/GStreamer

**Binary Message Types:**
- `0x01` - VideoFrame: codec(1) + flags(1) + pts(8) + width(2) + height(2) + NAL data
- `0x02` - AudioFrame: codec(1) + flags(1) + pts(8) + audio data
- `0x03` - VideoBatch: count(2) + [length(4) + frame_data]...
- `0x30` - StreamInit: codec(1) + width(2) + height(2) + fps(1) + ...

**Video Codecs:**
- `0x01` = H.264
- `0x02` = H.264 High 4:4:4
- `0x10` = HEVC
- `0x11` = HEVC Main10
- `0x20` = AV1 Main8
- `0x21` = AV1 Main10

## Troubleshooting

### "failed to get user for git config: not found"

Keycloak is not running. Start it with `./stack start` or use the full docker-compose.

### "timeout waiting for sandbox to start"

Check Wolf logs:
```bash
docker compose logs api | grep -E "wolf|sandbox|lobby"
```

### "Access denied - you don't have permission"

The session doesn't exist or you're using the wrong auth token.

### Stream shows 0 frames

1. Verify the session is active: `helix spectask list`
2. Check Wolf has video producer running
3. Ensure PipeWire mode is working (Ubuntu desktop)
