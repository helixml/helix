# Wolf Wayland Socket Leak - Root Cause Analysis

## Issue
External agent sessions fail with "Lobby setup timed out" after the sandbox has been running for a while.

## Root Cause
Wolf creates wayland sockets (`wayland-N` where N is 0, 1, 2, ...) for each lobby in `$XDG_RUNTIME_DIR` (typically `/tmp/sockets`). When a lobby creation fails (timeout, video producer panic, etc.), the wayland socket and its `.lock` file are NOT cleaned up.

Over time, these stale sockets accumulate. Eventually (after 30+ failed lobbies), Wolf's `ListeningSocketSource::new_auto()` function panics when trying to create a new socket, causing all subsequent lobby creations to fail.

## Error Signature
```
thread '<unnamed>' panicked at wayland-display-core/src/comp/mod.rs:683:52:
...waylanddisplaycore::comp::init::hcc5e4ff338379eb8

thread '<unnamed>' panicked at wayland-display-core/src/lib.rs:284:26:
called `Result::unwrap()` on an `Err` value: RecvError
...waylanddisplaycore::WaylandDisplay::get_render_device::h88fbc73f560f489d
```

```
WARN | basesrc gstbasesrc.c:3616:gst_base_src_start:<wolf_wayland_source> error: Failed to start
WARN | basesrc gstbasesrc.c:3984:gst_base_src_activate_push:<wolf_wayland_source> Failed to start in push mode
ERROR | [GSTREAMER] Pipeline error: Panicked: called `Result::unwrap()` on an `Err` value: RecvError
```

## Affected Locations
- `/prod/home/luke/pm/wolf/gst-wayland-display/wayland-display-core/src/comp/mod.rs:683`
- `/prod/home/luke/pm/wolf/gst-wayland-display/wayland-display-core/src/lib.rs:284`

## Fix Applied (Helix Workaround)

Since this is a Wolf bug (cleanup should happen in Wolf when lobbies fail/end), we added workarounds in the Helix sandbox:

### 1. Startup Cleanup (Dockerfile.sandbox)
Clean all stale wayland sockets when Wolf starts:
```bash
if [ -n "$XDG_RUNTIME_DIR" ] && [ -d "$XDG_RUNTIME_DIR" ]; then
    WAYLAND_COUNT=$(ls -d $XDG_RUNTIME_DIR/wayland-* 2>/dev/null | wc -l)
    if [ "$WAYLAND_COUNT" -gt 0 ]; then
        echo "Cleaning up $WAYLAND_COUNT stale wayland sockets..."
        rm -rf $XDG_RUNTIME_DIR/wayland-* 2>/dev/null || true
    fi
fi
```

### 2. Periodic Cleanup Daemon
Background process that runs every 5 minutes and cleans up orphaned sockets:
- Compares active lobby count (from Wolf API) to socket count
- If there are 10+ orphaned sockets, cleans all sockets
- Wolf recreates sockets for active lobbies automatically

## Proper Fix (Wolf Upstream)
The proper fix should be in Wolf's wayland-display-core:
1. In `comp/mod.rs`, catch the panic from `ListeningSocketSource::new_auto()` and clean up stale sockets
2. In lobby deletion/timeout handlers, ensure wayland sockets are properly cleaned up

## Testing
```bash
# Clean stale sockets manually
docker compose -f docker-compose.dev.yaml exec sandbox-nvidia bash -c 'rm -rf $XDG_RUNTIME_DIR/wayland-*'

# Test lobby creation
curl -s -X POST "http://localhost:8080/api/v1/external-agents" \
  -H "Authorization: Bearer oh-hallo-insecure-token" \
  -H "Content-Type: application/json" \
  -d '{"input": "Test task"}'

# Check socket count
docker compose -f docker-compose.dev.yaml exec sandbox-nvidia bash -c 'ls $XDG_RUNTIME_DIR/ | grep wayland | wc -l'
```

## Related
- PulseAudio virtual sink accumulation (similar pattern, different resource)
- Wolf issue tracker: TODO - file upstream issue
