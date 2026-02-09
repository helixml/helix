# 2026-02-09: Dev Environment Breakage Incident

## Summary

Between 12:28 and ~13:10 UTC on Feb 9, the macOS ARM dev environment stopped working. Desktop containers start but GNOME Shell freezes (stuck in `dma_fence_default_wait`), ScreenCast D-Bus activation times out, desktop-bridge HTTP server on :9876 never starts.

**Root cause (confirmed):** Commit `b9b23692e` (Feb 8, 13:02 UTC) added a global `MESA_GL_VERSION_OVERRIDE=4.5` to `detect-render-node.sh` for virtio-gpu mode. This told gnome-shell that virgl supports GL 4.5 when it only supports GL 2.1, causing unsupported GL calls → GPU command failures → stuck DMA fences → permanent deadlock.

The commit was made 13 minutes after the last working desktop image build (`1ac9d5` at 12:49 UTC Feb 8). It was not deployed into a container until 31 hours later (image `9449ee` at 20:38 UTC Feb 9). The gap between cause and effect made debugging extremely difficult.

**Fix:** Commit `d0c3d674f` removes the global `MESA_GL_VERSION_OVERRIDE` from `detect-render-node.sh`. The override remains scoped to Ghostty's launch command only (in `start-zed-helix.sh`).

## Root Cause Deep Dive

### The Guilty Commit

```
b9b23692e  2026-02-08 13:02:06 +0000  fix: Use virgl hardware GL (not llvmpipe) + restore Venus in UTM plist
```

This commit changed `detect-render-node.sh` to add:
```bash
export MESA_GL_VERSION_OVERRIDE=4.5
export MESA_GLSL_VERSION_OVERRIDE=450
```

...in the `virtio)` case block. These environment variables are inherited by every process in the container, including gnome-shell. When gnome-shell sees GL 4.5, it uses GL features that virgl's actual GL 2.1 implementation cannot handle. The resulting GPU command failures cause virtio-gpu DMA fences to never signal, deadlocking gnome-shell's main thread in `dma_fence_default_wait`.

The original code had `LIBGL_ALWAYS_SOFTWARE=1` scoped only to Ghostty's launch line (because Ghostty needs GL 3.3+ core). The new commit moved the fix to a global env var but changed the approach from "use software rendering" to "lie about GL version" — and applied it container-wide instead of per-process.

### Why It Took So Long to Find

1. **13-minute gap**: Image `1ac9d5` was built at 12:49 UTC Feb 8 from commit `b7d52bd41`. The guilty commit `b9b23692e` landed at 13:02 — just 13 minutes later. No `./stack build-ubuntu` was run after this commit.

2. **31-hour deployment gap**: The next desktop image build was `9449ee` at 20:38 UTC Feb 9, from commit `655ffbffe` on the `-working2` branch. This was the first time `b9b23692e` was ever deployed into a container.

3. **Dozens of red herrings**: Between the guilty commit and its deployment, 20+ commits were made to both QEMU and Helix repos. QEMU encoder changes (`ConstrainedBaseline`, `ReferenceBufferCount`) were also capable of causing `gl_block` deadlocks, misdirecting investigation. The multi-scanout commit `8136753a09` was another suspect.

4. **VM state corruption hypothesis**: The original VM (`linux-broken`) had genuinely corrupted GPU state from repeated QEMU kills during development. This led to the (incorrect) hypothesis that the `dma_fence_default_wait` was caused by GPU state corruption, not a code change.

5. **Fresh VM still broken**: Provisioning a completely new VM and rebuilding from scratch still hit the same deadlock — because the new image build picked up the guilty commit. This correctly ruled out VM state corruption but didn't immediately point to the real cause.

### How It Was Found

1. **Session transcript archaeology**: Deep-diving into Claude session transcripts (`.claude/projects/.../*.jsonl`) revealed that image `1ac9d5` was built at 12:49 Feb 8 from commit `b7d52bd41`.

2. **Testing the old code**: Checking out `b7d52bd41` in the VM, rebuilding the desktop image, and starting a session — **video streaming worked**. This confirmed the bug was in the helix code changes between `b7d52bd41` and `655ffbffe`.

3. **Narrowing the diff**: Only 2 commits between those two touched code that goes into the desktop image:
   - `b9b23692e` — global `MESA_GL_VERSION_OVERRIDE=4.5` (the culprit)
   - `655ffbffe` — ScanoutSource disconnect lifecycle (harmless)

4. **Mechanism confirmed**: The global GL version override causes gnome-shell to issue GL 4.5 calls that virgl (GL 2.1) can't handle → virtio-gpu command failures → DMA fences never signal → `dma_fence_default_wait` deadlock.

## Last Known Working State

- **Time**: ~12:28 UTC, Feb 9
- **Desktop image**: `helix-ubuntu:1ac9d5` (built Feb 8, 12:49 from `b7d52bd41`)
- **QEMU binary**: Built at 12:04 UTC from `a9825e4b79` (all-keyframe mode)
- **helix-drm-manager**: Status uncertain at 12:28. Was NOT running at 12:00 (systemd cycle bug). Systemd fix applied at 12:01, VM rebooted ~12:02. Likely started after reboot but no `systemctl status` was run to confirm.
- **Note**: No desktop containers were confirmed running at exactly 12:28. The user was observing the QEMU host display. Container video streaming had been working earlier in the session.

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
| 12:44 | `9b976d7dd2` | ConstrainedBaseline + ReferenceBufferCount=1 — caused VT callback to never fire, also blocked `gl_block` |
| 13:36 | `1b16e93d7e` | Reverted encoder config, added gl_block safety timer |

### Helix Commits (12:00–14:00)

| Time | Hash | What |
|------|------|------|
| 12:06 | `fde9cfb3d` | systemd cycle fix (scripts only — provision-vm.sh, setup-vm-inside.sh) |
| 12:09 | `655ffbffe` | Don't kill ScanoutSource on client disconnect (ws_stream.go) |
| 12:31 | `7fe6d2348` | Docs only |
| 12:50 | `1d355b0eb` | Disable SPS rewriting (ws_stream.go) |
| 13:16 | `b8a06051a` | Re-disable SPS rewriting (ws_stream.go) |

## Recovery Timeline (evening of Feb 9)

### ~15:00-19:00 — VM provisioning and UTM struggles

- Original VM (`linux-broken`) abandoned due to (separately real) GPU state corruption
- New VM provisioned on `/Volumes/Big/` with Ubuntu 25.10 ARM64
- Multiple UTM registration failures (security-scoped bookmarks, hand-written config.plist)
- UTM reinstalled fresh
- Custom QEMU binary (`14e0d3ca62`) found to break UTM display rendering
- Created `-working2` branches pinned to last known-working QEMU (`9abf11ab94`)
- New VM config.plist fixed by copying UTM-generated format from linux-broken

### ~20:00-21:30 — Stack build and first failure

- Full stack built on new VM from working2 branch
- `.env` configured for macOS (COMPOSE_PROFILES=code-macos, GPU_VENDOR=virtio)
- Desktop container launched — same `dma_fence_default_wait` deadlock
- This ruled out VM state corruption as the cause

### ~21:30-22:30 — Root cause investigation

- Traced the hanging code path: `desktop.go` → `startSession()` → `RemoteDesktop.Start` D-Bus call → gnome-shell tries to render → `dma_fence_default_wait`
- Discovered scanout mode is triggered by `HELIX_VIDEO_MODE=scanout` env var, set by Hydra when `/run/helix-drm.sock` exists
- Deep-dived session transcripts to find that image `1ac9d5` (built Feb 8, 12:49) was the last working version
- Tested helix at `b7d52bd41` (the commit `1ac9d5` was built from) — **video streaming worked**
- Diffed `b7d52bd41..655ffbffe` — found `b9b23692e` added global `MESA_GL_VERSION_OVERRIDE=4.5`
- Confirmed this was the root cause

### Fix applied

- Removed global `MESA_GL_VERSION_OVERRIDE` from `detect-render-node.sh`
- Committed as `d0c3d674f` on `feature/macos-arm-desktop-port-working2`
- Awaiting verification with rebuilt desktop image

## Hypotheses That Were Wrong

| Hypothesis | Why it seemed right | Why it was wrong |
|-----------|-------------------|-----------------|
| Multi-scanout commit `8136753a09` | Symptoms consistent with GL fence issues | Was present and working for 17+ subsequent QEMU builds |
| VM GPU state corruption | linux-broken VM was genuinely corrupted | Fresh VM had same problem |
| QEMU encoder changes (`ConstrainedBaseline`) | Also capable of causing `gl_block` deadlock | Working2 branch doesn't include those changes, still broken |
| `dpy_gl_update` skip needed | Partial fix when added to QEMU | Addressed a different (SPICE display) issue |

## Lessons Learned

### Test every change immediately
A 13-minute gap between an untested commit and the next image build cost an entire day of debugging. Added rule to CLAUDE.md: never commit code changes without deploying and testing them in the same session.

### `MESA_GL_VERSION_OVERRIDE` is dangerous
Lying to gnome-shell about GL capabilities causes hard-to-debug GPU deadlocks. The override should only be applied per-process to specific apps that need it (like Ghostty), never globally.

### Session transcript archaeology works
Reading Claude session transcripts (JSONL files in `~/.claude/projects/`) was essential for reconstructing what image versions were running at specific times. Git logs alone weren't enough because the image build is a separate step from committing code.

### `git stash show` doesn't show untracked files
The incident report was "lost" during a branch switch but was actually safely in `stash@{0}^3` (the untracked files tree). Use `git show stash@{0}^3:path/to/file` to recover stashed untracked files.

### Multiple real issues can coexist
The original VM (`linux-broken`) DID have genuine GPU state corruption from repeated QEMU kills. This was a real problem that masked the actual code bug. The fresh VM ruled out corruption but initially seemed to confirm the multi-scanout theory.

## File Locations

- **new-dev-vm**: `/Volumes/Big/new-dev-vm.utm/`
  - Root disk: `B2CB8303-60F3-42AD-8056-772FB21E36A6.qcow2` (256GB)
  - ZFS disk: `74813216-2B7F-48E4-8060-061015E5B7A6.qcow2` (128GB)
- **linux-broken**: `/Volumes/Big/Linux-broken.utm/`
  - Disk: `780188AB-AB94-4FFE-BA6E-219BCBAAB83E.qcow2` (694GB)
- **Provisioning artifacts**: `/Volumes/Big/old-vm-artifacts/`

## Open Questions

1. **UTM renderer backend**: After UTM reinstall, `QEMURendererBackend` was reset to `0` (Default) instead of `2` (ANGLE Metal). Could this explain earlier failures to extract Metal textures for zero-copy encoding?
2. **QEMU display breakage**: Commits after `9abf11ab94` (specifically `14e0d3ca62` "dpy_gl_update skip") break UTM's native display rendering. These still need to be fixed for the full pipeline to work.
3. **P-frame corruption**: The VideoToolbox P-frame encoding corruption that started this whole investigation is still unresolved. All-keyframe mode works but wastes bandwidth.
