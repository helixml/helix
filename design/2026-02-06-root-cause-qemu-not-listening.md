# Root Cause Found: QEMU Not Listening on Socket

**Date:** 2026-02-06 08:20
**Status:** 🔴 BLOCKER IDENTIFIED

## 🎯 Root Cause

**QEMU's helix-frame-export module failed to initialize properly. The Unix socket file exists but QEMU is not listening on it.**

### Evidence

1. **Socket file exists:**
   ```bash
   $ ls -lah /Users/luke/Library/Group\ Containers/WDNLXAD4W8.com.utmapp.UTM/helix-frame-export.sock
   srwxr-xr-x  1 luke  staff  0B  6 Feb 07:40 helix-frame-export.sock
   ```

2. **But NO process is listening:**
   ```bash
   $ lsof helix-frame-export.sock
   # (no output - no process has it open)
   ```

3. **Manual connection test FAILS:**
   ```bash
   $ python3 test-qemu-socket.py
   ❌ Connection refused - QEMU not listening?
   ```

4. **QEMU process is running** (PID 51270, started 07:40:19)

5. **Socket was created when QEMU started** (timestamp 07:40)

### Conclusion

QEMU created the socket file but then either:
- `bind()` failed
- `listen()` failed
- `accept()` thread failed to start
- Socket was closed after creation

**The failure was SILENT - no error logs written to helix-debug.log.**

## Why No Logs?

The `helix_log()` function writes to `/Users/luke/Library/Group Containers/.../helix-debug.log`, but the log file is empty (0 bytes).

Looking at the initialization code:
```c
int helix_frame_export_init(void *virtio_gpu, int vsock_port) {
    error_report("[HELIX] Initializing frame export on vsock port %d", vsock_port);
    // ... socket creation ...
    error_report("[HELIX] Frame export ready: socket=%s, proxy=10.0.2.2:%d\n", ...);
    return 0;
}
```

If initialization fails before the first `error_report()`, or if `error_report()` isn't working, we get no logs.

### Why error_report() Might Not Work

1. **Redirected to stderr** - `helix_log()` writes to both file and stderr
2. **UTM captures stderr** - QEMU's stderr goes to UTM app (PID 1720) via pipes
3. **File I/O might fail silently** - If `fopen()` fails, only stderr gets the message
4. **Sandboxing** - macOS sandbox might block file writes

## Impact Analysis

This explains the entire symptom chain:

```
vsockenc: "Connected via TCP to 10.0.2.2:5900" ✅
  ↓
socat listening on 127.0.0.1:5900 ✅
  ↓
socat tries to connect to Unix socket ❌ CONNECTION REFUSED
  ↓
vsockenc send() succeeds (TCP buffer accepts data) ✅
  ↓
But socat can't forward to QEMU (socket refused) ❌
  ↓
vsockenc recv() blocks forever waiting for response ❌
  ↓
0 frames delivered to WebSocket client ❌
```

## Why Did It Work Earlier?

Earlier in the session (around 04:48), QEMU logs showed successful encoding:
```
[HELIX] Frame sent successfully: 22376 bytes, pts=103331525, keyframe=1
```

That was a DIFFERENT QEMU process that has since been restarted. The current QEMU (started 07:40) has the initialization bug.

### What Changed?

Possible causes:
1. **Different QEMU binary** - Earlier used different build
2. **Environment change** - UTM config or macOS state different
3. **Race condition** - Initialization sometimes fails
4. **Resource conflict** - Something blocking the socket

## Next Steps

### 1. Add Initialization Error Logging

Modify `helix-frame-export.m` to log ALL errors during init:

```c
int helix_frame_export_init(void *virtio_gpu, int vsock_port) {
    fprintf(stderr, "[HELIX-INIT] Starting initialization\n");
    fflush(stderr);

    FILE *logfile = fopen("/Users/luke/Library/Group Containers/.../helix-debug.log", "a");
    if (!logfile) {
        fprintf(stderr, "[HELIX-INIT] WARNING: Could not open log file: %s\n", strerror(errno));
    } else {
        fprintf(logfile, "[HELIX-INIT] Starting initialization\n");
        fclose(logfile);
    }

    // ... existing code ...

    if (bind(...) < 0) {
        fprintf(stderr, "[HELIX-INIT] FATAL: bind() failed: %s\n", strerror(errno));
        fflush(stderr);
        // ... existing error handling ...
    }

    fprintf(stderr, "[HELIX-INIT] Initialization complete\n");
    fflush(stderr);

    return 0;
}
```

### 2. Check QEMU stderr via UTM

The UTM app (PID 1720) reads QEMU's stderr. Check if UTM has a console/log viewer:
- UTM menu → View → Console
- Check Console.app for UTM logs
- Check if UTM writes logs to disk

### 3. Test with Simpler Socket Path

Current code uses relative path `"helix-frame-export.sock"` which resolves to QEMU's CWD.

Try absolute path to avoid any CWD issues:
```c
const char *socket_path = "/tmp/helix-frame-export.sock";
```

Then update socat:
```bash
socat TCP-LISTEN:5900,bind=127.0.0.1,fork,reuseaddr UNIX-CONNECT:/tmp/helix-frame-export.sock
```

### 4. Check for Socket Conflicts

Maybe something else created the socket file:
```bash
# Before starting QEMU
rm -f "/Users/luke/Library/Group Containers/.../helix-frame-export.sock"

# Start QEMU
# Check if socket is listening
nc -U /path/to/socket </dev/null && echo "Listening" || echo "Not listening"
```

### 5. Verify helix_frame_export_init is Called

Add a marker file to prove the function runs:
```c
int helix_frame_export_init(...) {
    system("touch /tmp/helix-init-called");
    // ... rest of function ...
}
```

Then check:
```bash
ls -l /tmp/helix-init-called
```

If the file doesn't exist, the function was never called.

### 6. Check for virgl_renderer_init Failure

The init function is only called if `virgl_renderer_init()` succeeds:
```c
// in virtio-gpu-virgl.c
if (virgl_renderer_init(...) == 0) {
    error_report("[HELIX-DEBUG] virgl_renderer_init succeeded, calling helix_frame_export_init");
    helix_frame_export_init(g, 5900);
} else {
    error_report("[HELIX-DEBUG] virgl_renderer_init FAILED");
}
```

Check if virgl init is failing.

## Files to Modify

1. **qemu-utm/hw/display/helix/helix-frame-export.m**
   - Add `fprintf(stderr, ...)` to all init steps
   - Add explicit error logging for bind/listen/pthread_create
   - Use `fflush(stderr)` to ensure immediate output

2. **qemu-utm/hw/display/virtio-gpu-virgl.c**
   - Add logging before/after helix_frame_export_init call
   - Check virgl_renderer_init return value

## Test Plan

After adding logging:
1. Rebuild QEMU
2. Install to UTM
3. Restart VM
4. Check stderr output in Console.app or UTM console
5. Check if `/tmp/helix-init-called` exists (if using marker file approach)
6. Try manual socket test again

## Summary

**Problem:** QEMU creates the socket file but doesn't listen on it.

**Symptom:** Connection refused when trying to connect.

**Impact:** vsockenc sends frame requests but never gets responses.

**Root Cause:** Silent initialization failure in helix-frame-export module.

**Fix:** Add comprehensive error logging to identify exact failure point.

## Related Files

- Test script: `/tmp/test-qemu-socket.py`
- Socket path: `/Users/luke/Library/Group Containers/WDNLXAD4W8.com.utmapp.UTM/helix-frame-export.sock`
- QEMU PID: 51270 (started 07:40:19)
- socat PID: 59230 (TCP 5900 → Unix socket)
