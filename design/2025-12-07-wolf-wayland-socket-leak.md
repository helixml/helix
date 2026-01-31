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

## Wolf Fix (Applied)

The proper fix was implemented in Wolf's wayland-display-core (commit c4e3b2d):

### `comp/mod.rs` - Socket creation with cleanup on failure

Changed `ListeningSocketSource::new_auto().unwrap()` to a `match` expression that:
1. On success, returns the socket source directly
2. On failure, logs a warning, cleans up stale wayland sockets from `XDG_RUNTIME_DIR`, and retries

```rust
let source = match ListeningSocketSource::new_auto() {
    Ok(source) => source,
    Err(e) => {
        tracing::warn!(?e, "Failed to create wayland socket, attempting cleanup...");
        // Clean up stale wayland sockets in XDG_RUNTIME_DIR
        if let Ok(runtime_dir) = std::env::var("XDG_RUNTIME_DIR") {
            if let Ok(entries) = std::fs::read_dir(&runtime_dir) {
                let mut cleaned = 0;
                for entry in entries.flatten() {
                    let name = entry.file_name();
                    let name_str = name.to_string_lossy();
                    if name_str.starts_with("wayland-") {
                        let path = entry.path();
                        if std::fs::remove_file(&path).is_ok() || std::fs::remove_dir_all(&path).is_ok() {
                            cleaned += 1;
                        }
                    }
                }
                if cleaned > 0 {
                    tracing::info!(cleaned, "Cleaned up stale wayland sockets, retrying...");
                }
            }
        }
        // Retry after cleanup
        ListeningSocketSource::new_auto()
            .expect("Failed to create wayland socket even after cleanup")
    }
};
```

This provides a "belt and braces" approach - the Helix workaround prevents accumulation, and the Wolf fix handles recovery if accumulation does occur

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
