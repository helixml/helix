# Screenshot/Video Pipeline Interference in GNOME

**Date**: 2026-01-05
**Status**: ✅ RESOLVED
**Tasks**: #167, #168

## CRITICAL CONTEXT - READ FIRST

### Current State (2026-01-05)

1. **Screenshots**: ✅ Working - D-Bus Screenshot API with `--unsafe-mode` (~400ms)
2. **Video Stream**: ✅ Working - 288 frames @ 19fps verified via CLI test

### Test Results (2026-01-05)

**Video streaming** (from CLI test):
```
📊 Final Statistics (elapsed: 15s)
───────────────────────────────────────────────
Resolution:         1920x1080
Codec:              H.264
Video frames:       288 (5 keyframes)
Frame rate:         19.20 fps
Video bitrate:      860.8 Kbps/s
Avg frame size:     5.5 KB
───────────────────────────────────────────────
```

**Screenshot capture** (from container logs):
```
time=2026-01-05T12:18:52.287Z level=DEBUG msg="capturing via D-Bus org.gnome.Shell.Screenshot"
time=2026-01-05T12:18:52.672Z level=DEBUG msg="D-Bus Screenshot succeeded" filename=/tmp/screenshot-1767615532287893769.png
time=2026-01-05T12:18:52.674Z level=INFO msg="screenshot captured" format=png quality=70 size=546351
```

**Key insight**: The `--unsafe-mode` flag on gnome-shell unlocks the `org.gnome.Shell.Screenshot` D-Bus API, allowing direct screenshot capture without touching PipeWire. This eliminates video pipeline interference entirely.

### System Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         GNOME 49 Sandbox Container                       │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    screenshot-server (Go binary)                  │   │
│  │                         api/pkg/desktop/                          │   │
│  ├──────────────────────────────────────────────────────────────────┤   │
│  │                                                                   │   │
│  │  D-Bus Connection (s.conn) ─────────────────────────────────────┐│   │
│  │       │                                                         ││   │
│  │       ├── org.gnome.Mutter.RemoteDesktop  (input injection)     ││   │
│  │       │       └── NotifyPointerMotion, NotifyKeyboard, etc.     ││   │
│  │       │                                                         ││   │
│  │       ├── org.gnome.Mutter.ScreenCast (video → PipeWire node)   ││   │
│  │       │       └── RecordMonitor("Meta-0") → PipeWire node_id    ││   │
│  │       │                                                         ││   │
│  │       └── org.gnome.Shell.Screenshot (screenshots - NEW)        ││   │
│  │               └── Screenshot() method - NO PipeWire!            ││   │
│  │                                                                  ││   │
│  │  HTTP API (:9876) ───────────────────────────────────────────────┤│   │
│  │       ├── /screenshot  → captureGNOMEScreenshot() → D-Bus       ││   │
│  │       ├── /clipboard   → wl-copy/wl-paste                       ││   │
│  │       ├── /input       → D-Bus RemoteDesktop NotifyPointer/Key  ││   │
│  │       └── /upload      → file upload                            ││   │
│  │                                                                  ││   │
│  │  Input Socket (/run/user/1000/wolf-input.sock) ──────────────────┤│   │
│  │       └── Binary protocol: Wolf → InputBridge → D-Bus input     ││   │
│  │                                                                  ││   │
│  │  Wolf Lobby Socket (/var/run/wolf/lobby.sock) ───────────────────┘│   │
│  │       └── Reports PipeWire node_id and input socket path to Wolf │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    gnome-shell --headless                         │   │
│  │                    (GNOME 49 compositor)                          │   │
│  │                                                                   │   │
│  │  PipeWire ScreenCast Node ──────────────────────────────────────┐│   │
│  │       │                                                          ││   │
│  │       └──> Wolf reads from this node (pipewirezerocopysrc)       ││   │
│  │            See: wolf/gst-plugins/pipewirezerocopysrc.rs          ││   │
│  │                                                                  ││   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ PipeWire DMA-BUF (GPU memory)
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           Wolf Container                                 │
│                                                                          │
│  GStreamer Pipeline:                                                     │
│    pipewirezerocopysrc → nvh264enc → rtph264pay → WebRTC/Moonlight      │
│         │                                                                │
│         └── Our custom Rust element (gst-plugins/pipewirezerocopysrc.rs)│
│             Uses PipeWire node_id reported by screenshot-server          │
│                                                                          │
│  Input Flow:                                                             │
│    Moonlight/WebRTC → Wolf → Input Socket → screenshot-server input      │
│                              bridge → D-Bus RemoteDesktop                │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Key Files

| File | Purpose |
|------|---------|
| `api/pkg/desktop/desktop.go` | Server main, D-Bus connection, HTTP routes |
| `api/pkg/desktop/session.go` | D-Bus session creation (RemoteDesktop + ScreenCast) |
| `api/pkg/desktop/screenshot.go` | Screenshot capture (D-Bus Screenshot, gnome-screenshot CLI, PipeWire fallback) |
| `api/pkg/desktop/input.go` | Input bridge (Wolf socket → D-Bus RemoteDesktop NotifyPointer/NotifyKeyboard) |
| `/prod/home/luke/pm/wolf/gst-pipewire-zerocopy/src/pipewiresrc/imp.rs` | Custom GStreamer element for zero-copy PipeWire capture |

### Video Pipeline Details

```
gnome-shell --headless
       │
       ▼ ScreenCast D-Bus API
┌──────────────────────────────────────────────────────────────────┐
│ screenshot-server createSession() / startSession()               │
│   - Calls org.gnome.Mutter.ScreenCast.CreateSession              │
│   - Calls RecordMonitor("Meta-0") → gets stream path             │
│   - Waits for PipeWireStreamAdded signal → gets node_id (e.g. 41)│
│   - Reports node_id to Wolf via /set-pipewire-node-id            │
└──────────────────────────────────────────────────────────────────┘
       │ node_id=41
       ▼
┌──────────────────────────────────────────────────────────────────┐
│ Wolf creates GStreamer pipeline:                                  │
│   pipewirezerocopysrc pipewire-node-id=41                        │
│     ! cudaconvertscale                                            │
│     ! nvh264enc                                                   │
│     ! rtph264pay                                                  │
│     ! [WebRTC/Moonlight]                                          │
│                                                                   │
│ pipewirezerocopysrc (our Rust element):                          │
│   - Connects to PipeWire using node_id                           │
│   - Receives DMA-BUF frames from GNOME ScreenCast                │
│   - Converts to CUDA memory via EGL for zero-copy                │
└──────────────────────────────────────────────────────────────────┘
```

### Input Bridge Details

```
Moonlight/WebRTC (browser)
       │
       ▼ WebSocket/proprietary protocol
Wolf (input multiplexer)
       │
       ▼ JSON over Unix socket
screenshot-server input bridge (/run/user/1000/wolf-input.sock)
       │ handleInputClient() reads JSON: {"type":"mouse_move_abs","x":100,"y":200}
       ▼ injectInput() calls D-Bus
org.gnome.Mutter.RemoteDesktop.Session.NotifyPointerMotionAbsolute(stream, x, y)
       │
       ▼
gnome-shell processes input → UI responds
```

### Component Status (Verified 2026-01-05)

| Component | Status | Notes |
|-----------|--------|-------|
| D-Bus session creation | ✅ Working | Creates RemoteDesktop + ScreenCast sessions |
| PipeWire node ID reporting | ✅ Working | Wolf receives node ID=45 via lobby socket |
| Screenshots | ✅ Working | PipeWire fallback works (~15s); D-Bus Screenshot blocked in headless |
| Input bridge (Go) | ✅ Working | Receives input events from Wolf, injects via D-Bus |
| Video stream | ✅ Working | 288 frames @ 19fps verified; zero-copy CUDA path working |
| pipewirezerocopysrc | ✅ Working | Logs show successful EGLImage → CUDAImage conversion |

---

## Original Problem

Screenshots were intermittently failing on GNOME/Ubuntu desktop, and users reported video stream interruptions when screenshots were requested.

## Root Cause

Both the screenshot server and Wolf's video pipeline were connecting to the **same PipeWire ScreenCast node**, causing buffer renegotiation conflicts:

```
┌──────────────────────────────────────────────────────────────────┐
│                    GNOME Mutter ScreenCast                       │
│              (PipeWire node_id from D-Bus session)               │
└──────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
    ┌─────────────────┐  ┌─────────────────┐
    │ screenshot.go   │  │ Wolf pipewiresrc│
    │ (temporary)     │  │ (persistent)    │
    │                 │  │                 │
    │ gst-launch-1.0  │  │ pipewirezero-   │
    │ pipewiresrc     │  │ copysrc for     │
    │ num-buffers=1   │  │ video streaming │
    └─────────────────┘  └─────────────────┘
           │                      │
           └──────────┬───────────┘
                      ▼
          BUFFER RENEGOTIATION!
          (interrupts video stream)
```

When screenshot's temporary `pipewiresrc` connected:
1. PipeWire renegotiated buffers between all consumers
2. Wolf's persistent video pipeline was interrupted
3. After screenshot disconnected, Wolf pipeline might timeout or produce corrupted frames

### Evidence from Web Research

From [Arun Raghavan's blog](https://arunraghavan.net/2024/12/gstreamer-pipewire-a-todo-list/):
> "don't try to share a stream from pipewiresink to pipewiresrc unless you are looking for trouble"

From [Collabora's PipeWire blog](https://www.collabora.com/news-and-blog/blog/2024/06/05/hacking-on-the-pipewire-gstreamer-elements/):
> "When the link is created, a set of buffers is negotiated between them"

## Solution

Use the `--unsafe-mode` flag when starting gnome-shell to unlock the `org.gnome.Shell.Screenshot` D-Bus API.

### The `--unsafe-mode` Flag

GNOME 41+ restricts the `org.gnome.Shell.Screenshot` D-Bus API to whitelisted callers only (gnome-screenshot, GNOME Shell UI, etc.). However, mutter/gnome-shell have a hidden `--unsafe-mode` flag that disables these restrictions.

From the GNOME source code:
```c
// mutter/src/meta/meta-context.h
META_EXPORT
void meta_context_set_unsafe_mode (MetaContext *context, gboolean enable);
```

When `--unsafe-mode` is enabled:
- D-Bus Screenshot API is accessible to any caller
- No user confirmation dialogs required
- No PipeWire involvement (pure D-Bus method)

### Implementation

**Dockerfile.ubuntu-helix** (gnome-shell startup):
```bash
# --unsafe-mode: Allow screenshot-server to use org.gnome.Shell.Screenshot D-Bus API
gnome-shell --headless --unsafe-mode --virtual-monitor ${GAMESCOPE_WIDTH}x${GAMESCOPE_HEIGHT}@${GAMESCOPE_REFRESH}
```

**api/pkg/desktop/screenshot.go** (simplified to D-Bus only):
```go
func (s *Server) captureScreenshot(format string, quality int) ([]byte, string, error) {
    // GNOME: Use D-Bus Screenshot API exclusively (no fallbacks)
    // gnome-shell must be started with --unsafe-mode to allow D-Bus access
    if isGNOMEEnvironment() {
        return s.captureGNOMEScreenshot(format, quality)
    }
    // KDE, Sway, X11 fallbacks...
}
```

### Why This Works

1. **No PipeWire involvement**: D-Bus Screenshot API captures directly from the compositor, bypassing PipeWire entirely
2. **Fast**: ~400ms vs. ~15s for PipeWire fallback
3. **No video interference**: Wolf's pipewiresrc video pipeline continues uninterrupted
4. **Simple**: Single D-Bus call, no GStreamer pipelines or temp files

## Desktop-Specific Screenshot Methods

| Desktop | Method | Protocol | Video Conflict? |
|---------|--------|----------|-----------------|
| GNOME | gnome-screenshot | D-Bus Screenshot API | No |
| KDE | D-Bus KWin.ScreenShot2 | D-Bus | No |
| Sway | grim | wlr-screencopy | No |
| X11 | scrot | X11 | N/A |
| Fallback | pipewiresrc | PipeWire | **YES** |

## Testing

### Build and Deploy

```bash
# Build updated image
./stack build-ubuntu

# Check image version (should show new hash)
cat sandbox-images/helix-ubuntu.version
```

### Test with Helix CLI

```bash
# Build the CLI
cd api && CGO_ENABLED=0 go build -o /tmp/helix . && cd ..

# Set up authentication
source .env.userkey
export HELIX_URL="http://localhost:8080"

# List sessions (old sessions use old image - create a NEW session)
/tmp/helix spectask list

# Take screenshot - saves to current directory
/tmp/helix spectask screenshot <session-id>

# Test video stream (should NOT be interrupted by screenshots)
/tmp/helix spectask stream <session-id> --duration 30

# In another terminal, take screenshots during streaming
/tmp/helix spectask screenshot <session-id>
```

### Verify Results

```bash
# Check screenshot file
file screenshot-*.png  # Should show: PNG image data, 1920 x 1080

# Check container logs for capture method used
docker compose exec -T sandbox-nvidia docker logs <container-name> 2>&1 | grep -E "gnome-screenshot|capture"
# Should show: "capturing via gnome-screenshot" NOT "capturing via PipeWire"
```

## Files Changed

- `api/pkg/desktop/screenshot.go` - Reordered capture methods
- `api/pkg/desktop/session.go` - Added D-Bus session monitoring (`monitorSession()`, `handleSessionClosed()`)
- `api/pkg/desktop/desktop.go` - Added session monitor goroutine
- `Dockerfile.ubuntu-helix` - Added WirePlumber configuration to disable 5-second stream suspension

## WirePlumber 5-Second Suspension Fix

**Problem**: PipeWire/WirePlumber has a default behavior to suspend streams after 5 seconds of "inactivity". This was causing video streams to stop producing frames exactly 5 seconds after the last frame, even though the ScreenCast session was still alive.

**Root Cause**: From [Arch Linux Forums](https://bbs.archlinux.org/viewtopic.php?id=309630) and [Ubuntu fix blog](https://www.lexo.ch/blog/2024/09/fix-audio-delays-and-missing-audio-notifications-in-ubuntu-and-linux-mint-disabling-pipewire-and-wireplumber-suspend/):
> "The root of the problem lies in PipeWire's default behavior: it's configured to enter suspend mode after just 5 seconds of inactivity."

**Failed Approaches**:
1. Config file approach (51-disable-suspension.conf) - WirePlumber ignored it
2. Commenting out `hooks.node.suspend` component - created invalid config
3. Removing entire component block - `policy.node` depends on `hooks.node.suspend`

**Working Solution**: Modify the default timeout in `suspend-node.lua` from 5 seconds to 86400 seconds (1 day):

```dockerfile
# In Dockerfile.ubuntu-helix
# The script has: tonumber(node.properties["session.suspend-timeout-seconds"]) or 5
# Change the default from 5 to 86400 (1 day)
sed -i 's/) or 5$/) or 86400/' /usr/share/wireplumber/scripts/node/suspend-node.lua
```

This approach works because:
- We can't disable or remove `hooks.node.suspend` (other components depend on it)
- But we can change when it activates (86400s = 1 day effectively disables it for streaming sessions)
- The sed pattern matches the end of the line in `suspend-node.lua` line 41

**Status**: ✅ Implemented in helix-ubuntu:d369c0.

**Verification**:
```bash
$ docker run --rm --entrypoint grep helix-ubuntu:d369c0 -n "or 5\|or 86400" /usr/share/wireplumber/scripts/node/suspend-node.lua
41:          tonumber(node.properties["session.suspend-timeout-seconds"]) or 86400
```

## pipewirezerocopysrc Frame Timeout Fix

**Problem**: After ~20 seconds of successful streaming, pipewirezerocopysrc would timeout with a 5-second error, causing the video producer to exit.

**Root Cause**: Under investigation. The 5-second timeout was too aggressive for some GNOME ScreenCast scenarios.

**Solution**: Increase pipewirezerocopysrc timeout from 5s to 30s (`wolf/gst-pipewire-zerocopy/src/pipewire_stream.rs`):
```rust
// 30s timeout: Generous timeout for frame gaps
self.frame_rx.recv_timeout(Duration::from_secs(30))
```

**Status**: ✅ Implemented

## References

- [Arun Raghavan: GStreamer PipeWire TODO List](https://arunraghavan.net/2024/12/gstreamer-pipewire-a-todo-list/)
- [Collabora: Hacking on PipeWire GStreamer Elements](https://www.collabora.com/news-and-blog/blog/2024/06/05/hacking-on-the-pipewire-gstreamer-elements/)
- [GNOME GitLab: Screenshot API Restrictions](https://gitlab.gnome.org/GNOME/gnome-shell/-/issues/3943)
- [GNOME Discourse: Screenshot via D-Bus](https://discourse.gnome.org/t/take-screenshot-in-gnome-environment-via-its-dbus-api/21144)
- [GNOME Kiosk Updates 2025](https://blogs.gnome.org/shell-dev/2025/09/10/gnome-kiosk-updates/)
