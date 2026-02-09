# 2026-02-09: Dev Environment Breakage Incident

## Summary

Between 12:28 and ~13:10 UTC on Feb 9, the macOS ARM dev environment stopped working. Desktop containers start but GNOME Shell freezes (stuck in `dma_fence_default_wait`), ScreenCast D-Bus activation times out, desktop-bridge HTTP server on :9876 never starts. Rolling back both repos to pre-12:28 code does NOT fix the issue, suggesting the problem is either baked into VM state or caused by a change that persists across reboots.

## Last Known Working State

- **Time**: ~12:28 UTC, Feb 9
- **What was working**: All-keyframe H.264 encoding, video streaming, desktop containers starting correctly
- **QEMU binary**: Built at 12:04 UTC from this session (f884115a), which included the multi-scanout commit `8136753a09` from Feb 8
- **Helix repo**: `feature/macos-arm-desktop-port` at approximately `655ffbffe`

## Timeline of Events (All UTC, Feb 9)

### QEMU Builds (from Claude session history)

Each build kills UTM and reinstalls the QEMU binary. VM must be manually restarted.

| Time | Session | Notes |
|------|---------|-------|
| 08:46 | f884115a | |
| 09:43 | f884115a | |
| 10:35 | f884115a | |
| 10:47 | f884115a | |
| 11:50 | f884115a | |
| **12:04** | f884115a | **Last build before things were confirmed working at 12:28** |
| **12:29** | f884115a | Kills UTM, rebuilds. Now includes `9abf11ab94` (P-frame revert) |
| 12:42 | f884115a | Includes `c0b93d21cd` (MaxFrameDelayCount=0) |
| 13:36 | f884115a | Includes `1b16e93d7e` (revert encoder + safety timer) |

### VM-Modifying Commands (from Claude session history)

| Time | Command | Impact |
|------|---------|--------|
| 12:01:30 | `sudo tee /etc/systemd/system/helix-drm-manager.service` — changed `After=multi-user.target` to `After=systemd-udev-settle.service` | Takes effect on next daemon-reload or reboot |
| **13:10:55** | `sudo reboot` | **Critical event**: VM reboots with new systemd service file active |

### QEMU Commits (12:00–14:00)

| Time | Hash | What |
|------|------|------|
| 12:06 | `9abf11ab94` | Revert to P-frame encoding (helix-frame-export.m only) |
| 12:31 | `c0b93d21cd` | MaxFrameDelayCount=0 for VT encoder (helix-frame-export.m only) |
| 12:44 | `9b976d7dd2` | ConstrainedBaseline + ReferenceBufferCount=1 — **known to cause VT callback to never fire, permanently blocking renderer_blocked** |
| 13:36 | `1b16e93d7e` | Reverted encoder config, added gl_block safety timer |

### Helix Commits (12:00–14:00)

| Time | Hash | What |
|------|------|------|
| 12:06 | `fde9cfb3d` | systemd cycle fix (scripts only — provision-vm.sh, setup-vm-inside.sh) |
| 12:09 | `655ffbffe` | Don't kill ScanoutSource on client disconnect (ws_stream.go) |
| 12:31 | `7fe6d2348` | Docs only |
| 12:50 | `1d355b0eb` | Disable SPS rewriting (ws_stream.go) |
| 13:16 | `b8a06051a` | Re-disable SPS rewriting (ws_stream.go) |

## Key Suspect: Multi-Scanout Commit

Commit `8136753a09` (Feb 8, 16:39 UTC) — "Multi-scanout support for UTM SPICE GL display"

This commit changed 7 files and made fundamental changes to how QEMU handles multiple scanouts:

1. **Secondary consoles get non-GL ops** — scanouts 1-15 return `GRAPHIC_FLAGS_NONE`, preventing SPICE GL draw path
2. **Non-GL consoles skip GL scanout setup** — `virgl_cmd_set_scanout` and `virgl_cmd_set_scanout_blob` take early return for non-GL consoles, only setting `resource_id`
3. **SPICE channels skipped for consoles >0** — UTM only sees 1 SPICE display channel
4. **SPICE gl_block_timer gutted** — handler changed to no-op (was already just a warning logger, never actually unblocked)
5. **GL context fallback to scanout 0** — `virgl_create_context` and `virgl_make_context_current` fall back to scanout 0 for non-GL consoles

Every problem since the breakage is a symptom consistent with this commit:
- `renderer_blocked` deadlock
- GNOME Shell `dma_fence_wait` freeze
- ScreenCast D-Bus timeout
- 0 video frames
- Blank UTM console

**However**: This commit was included in the QEMU build at 12:04 and things were working at 12:28. So either:
- The commit works but is fragile and breaks on VM restart
- Something else that changed between 12:04 and 13:10 is the real cause
- The commit only breaks when combined with the systemd service change

## Changes Baked Into VM State

These persist across reboots and wouldn't be reverted by rolling back git repos:

1. **helix-drm-manager systemd service**: Changed from `After=multi-user.target` to `After=systemd-udev-settle.service` (written at 12:01, effective after 13:10 reboot)
2. **Docker ordering drop-in** (`/etc/systemd/system/docker.service.d/helix-drm.conf`): Makes Docker depend on helix-drm-manager (written by overnight session 993bc357)
3. **GRUB kernel parameter**: `video=Virtual-1:1920x1080` added to kernel cmdline (written by overnight session, took effect on reboot)
4. **helix-drm-manager binary**: Last updated Feb 8 07:39 UTC (unchanged since)

## Rollback Attempts

### Attempt 1: Roll back to pre-12:28 code
- Created `utm-edition-venus-helix-working` branch at `9abf11ab94` (QEMU)
- Created `feature/macos-arm-desktop-port-working` branch at `655ffbffe` (Helix)
- Rebuilt QEMU, restarted VM, checked out working branch in VM
- **Result: STILL BROKEN** — same symptoms (desktop-bridge :9876 connection refused)
- **Note**: This branch still includes `8136753a09` (multi-scanout commit)

### Attempt 2: Rebuild desktop image
- Ran `./stack build-ubuntu` from working branch
- Image hash `699966` was identical — no code changes in desktop image
- **Result: Image unchanged, problem not in desktop image**

## Container Diagnostics

### Symptoms inside desktop container
- GNOME Shell starts (PID 398/401), uses 227MB RSS, 22 threads
- gnome-shell process state: `S (sleeping)` in `dma_fence_default_wait`
- PipeWire first instance starts OK (PID 179), second instance fails: `failed to create context: Resource temporarily unavailable`
- D-Bus activation timeouts:
  - `org.gnome.Shell.Screencast`: timed out (120000ms)
  - `org.freedesktop.portal.Desktop`: timed out
  - `org.freedesktop.impl.portal.desktop.gnome`: timed out
  - `org.freedesktop.impl.portal.desktop.gtk`: timed out
- desktop-bridge HTTP server never starts on :9876

### VM-level diagnostics
- Disk: 672GB/1007GB used (70%) — plenty of space
- Memory: 7.4GB/60GB used — plenty of RAM
- No OOM kills
- Docker: healthy, sandbox running
- helix-drm-manager: running, granting leases successfully

## Session 2 Investigation (Continuation)

### Attempt 3: Revert systemd ordering
- Reverted helix-drm-manager from `After=systemd-udev-settle.service` back to `After=multi-user.target`
- Rebooted VM, started new container
- **Result: STILL BROKEN** — gnome-shell still stuck in `dma_fence_default_wait`
- **Also**: Reverting to `After=multi-user.target` created a systemd ordering cycle (Docker → helix-drm-manager → multi-user.target → Docker), which prevented helix-drm-manager from starting
- **Conclusion**: Systemd ordering is NOT the root cause. Restored `After=systemd-udev-settle.service`.

### Attempt 4: Skip `dpy_gl_update` on macOS
- Added `#ifdef __APPLE__ return; #endif` to `virtio_gpu_rect_update` to skip SPICE's `dpy_gl_update` path entirely
- This prevents SPICE from calling `gl_draw_async` → `gl_block(true)`, which UTM never completes with `gl_draw_done`
- Rebuilt QEMU, restarted VM
- **Result: PARTIAL SUCCESS**
  - Port 9876 now listening — desktop-bridge HTTP server starts
  - But gnome-shell still eventually freezes in `dma_fence_default_wait`
  - Screenshots fail: "D-Bus Screenshot call failed: context deadline exceeded"
  - UTM console is blank (expected — SPICE can't render without `dpy_gl_update`)

### Key Findings

1. **Two SPICE `gl_block(true)` paths**: The `dpy_gl_update` skip only blocks ONE path. SPICE also calls `gl_block(true)` through:
   - `dpy_gfx_update` → `spice_gl_update` → increments `gl_updates` → `spice_gl_refresh` calls `gl_block(true)`
   - `spice_iosurface_blit_metal` calls `gl_block(true)` directly

2. **FRAME_READY entries continue in broken boots**: In the helix debug log, FRAME_READY entries continue (#3000-#10000) AFTER scanout 1 is set up. This proves `renderer_blocked` is NOT permanently stuck during those broken boots — the real issue is deeper.

3. **Likely root cause**: virgl GL fences for scanout 1 never complete. The multi-scanout commit makes `virgl_create_context` and `virgl_make_context_current` fall back to scanout 0's GL context for non-GL consoles. This means all virgl rendering for secondary scanouts uses scanout 0's SPICE display context, which may cause fence completion issues.

4. **Critical question**: The 12:04 QEMU build included the multi-scanout commit AND worked at 12:28. But the code at `9abf11ab94` (the "working" branch tip, committed at 12:06) also includes the multi-scanout commit and does NOT work after a reboot. This suggests either:
   - The working state at 12:28 was from a session started BEFORE the multi-scanout commit's effects took hold (i.e., the container was running from an earlier boot)
   - Some VM state changed between 12:04 and the 13:10 reboot that interacts with the multi-scanout commit

## Commit Matrix for Testing

### QEMU Commits (chronological, on `utm-edition-venus-helix`)

The 12:04 build that was working at 12:28 was built from HEAD of `utm-edition-venus-helix` at that time. Looking at commit timestamps:

| # | Hash | Time (Feb 8-9) | Description | Notes |
|---|------|-----------------|-------------|-------|
| 1 | `cdb613359a` | Feb 8, 14:57 | Revert max_outputs to 1 | **PARENT of multi-scanout** |
| 2 | `8136753a09` | Feb 8, 16:39 | Multi-scanout support | **Primary suspect** |
| 3 | `3a9f7737f6` | Feb 8, ~17:00 | Snapshot IOSurface on flush | |
| 4 | `9ee56ac7ee` | Feb 8, ~17:30 | Capture Metal texture for non-GL scanout | |
| 5 | `cb9b329a8e` | Feb 8, ~18:00 | GPU blit path for zero-copy encoding | |
| 6 | `985cea2890` | Feb 8, ~18:30 | ANGLE IOSurface hint constants | |
| 7 | `d55b1165f1` | Feb 8, ~19:00 | Race condition crash fix | |
| 8 | `8711bf2416` | Feb 8, ~19:30 | glFinish sync before GPU blit | |
| 9 | `98f397b7fc` | Feb 8, ~20:00 | Metal IOSurface snapshot | |
| 10 | `9e19517a12` | Feb 8, ~20:30 | Zero-copy GL blit with triple-buffered ring | |
| 11 | `535af85418` | Feb 8, ~21:00 | Backpressure + IOSurfaceLock fence | |
| 12 | `da57c68e1e` | Feb 8, ~21:30 | Remove gl_block backpressure | |
| 13 | `f1b024e2c2` | Feb 8, ~22:00 | Save/restore EGL context + glFinish fence | |
| 14 | `67d79c6580` | Feb 8, ~22:30 | Replace glFinish with glFlush | |
| 15 | `098b5532cc` | Feb 8, ~23:00 | Deferred backpressure via BH | |
| 16 | `a9825e4b79` | Feb 9, ~08:00 | glFinish on virgl context + all-keyframe | **Likely last commit in 12:04 build** |
| 17 | `9abf11ab94` | Feb 9, 12:06 | Revert to P-frame encoding | Working branch tip |
| 18 | `c0b93d21cd` | Feb 9, 12:31 | MaxFrameDelayCount=0 | |
| 19 | `9b976d7dd2` | Feb 9, 12:44 | ConstrainedBaseline (KNOWN BAD) | |
| 20 | `1b16e93d7e` | Feb 9, 13:36 | Revert encoder + safety timer | |
| 21 | `14e0d3ca62` | Feb 9, ~15:00 | Keepalive timer + dpy_gl_update skip | Main branch tip |

### Helix Commits (on `feature/macos-arm-desktop-port`)

| # | Hash | Time (Feb 9) | Description |
|---|------|--------------|-------------|
| 1 | `fde9cfb3d` | 12:06 | systemd cycle fix (scripts only) |
| 2 | `655ffbffe` | 12:09 | Don't kill ScanoutSource on disconnect |
| 3 | `7fe6d2348` | 12:31 | Docs only |
| 4 | `1d355b0eb` | 12:50 | Disable SPS rewriting |
| 5 | `b8a06051a` | 13:16 | Re-disable SPS rewriting |

### What Was Running at 12:28?

- **QEMU**: Built at 12:04. The last commit before 12:06 was `a9825e4b79` (glFinish on virgl context + all-keyframe). So the 12:04 build was at `a9825e4b79`.
- **Helix in VM**: The VM had NOT been rebooted since the stack was last started. The Helix code running in containers depends on the desktop image (unchanged) and the Go code checked out in the VM. At 12:09, `655ffbffe` was committed, but the running containers would have been using whatever was deployed before that.
- **Key insight**: The container running at 12:28 may have been started BEFORE any Feb 9 code changes — it could have been running from a Feb 8 session that was never restarted.

### Test Plan

Test combinations starting with the most likely working state:

1. **QEMU `a9825e4b79` + Helix `655ffbffe`** — the most likely 12:28 combo
2. **QEMU `cdb613359a` + Helix `655ffbffe`** — before multi-scanout commit (isolates whether multi-scanout is the cause)
3. **QEMU `14e0d3ca62` + Helix `655ffbffe`** — latest main branch tip (has dpy_gl_update skip built-in)

## Next Steps

1. Discard uncommitted QEMU changes (diagnostic logging) to test clean commits
2. Systematically test combinations from the test plan above
3. For each test: checkout commit → rebuild QEMU → restart VM → start container → check port 9876 + gnome-shell responsiveness
