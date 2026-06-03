# Requirements: Fix Chrome Auto-Relaunch on Session Resume

## Background

Feature [002027](../002027_can-we-persist-browser/) shipped two things:

1. Chrome profile persistence via `~/.config/google-chrome` → `$WORK_DIR/.chrome-state` symlink. **This works.**
2. Auto-relaunch of Chrome on the next session if it was running at the end of the previous one. **This is broken.** Chrome never auto-reopens, regardless of whether it was open last session.

### Why the auto-relaunch never fires (root cause)

The marker file mechanism lives in two startup scripts:

- `desktop/sway-config/startup-app.sh:597-618`
- `desktop/ubuntu-config/startup-app.sh:307-335` (inside the `start_gnome` heredoc)

Both scripts have the same logic shape:

```bash
CHROME_MARKER="/home/retro/work/.chrome-state/.was-running"
# A: should we relaunch?
if [ -f "$CHROME_MARKER" ] && [ $(($(date +%s) - $(stat -c %Y "$CHROME_MARKER"))) -lt 300 ]; then
    google-chrome-stable >/dev/null 2>&1 &
fi
# B: heartbeat that keeps the marker fresh while Chrome runs
( while true; do
      if pgrep -x chrome >/dev/null 2>&1 || pgrep -x chromium >/dev/null 2>&1; then
          touch "$CHROME_MARKER"
      else
          rm -f "$CHROME_MARKER"
      fi
      sleep 30
  done ) &
```

The intent in `design.md` of 002027 was: stale marker (> 5 min old) means Chrome was deliberately closed, so don't relaunch. But the heartbeat runs **only while the container is alive**. The marker's `mtime` is "when the previous container last touched it", not "when the user closed Chrome".

So the check `(now - marker.mtime) < 300` becomes `(time_since_previous_container_stopped + heartbeat_slop) < 300`, i.e. "was the session resumed within the last 5 minutes?" Helix sandboxes are routinely paused for hours or days between resumes — the condition is almost never true. The `if [...] -lt 300` branch is functionally dead, so the auto-launch line is never reached.

This matches the symptom the user reports: persistence works, auto-launch never fires.

### Secondary issues found while investigating

- All auto-launch output is redirected to `>/dev/null 2>&1`, so when something does go wrong (Wayland not ready, Singleton lock not cleared, wrapper missing, etc.) there is no diagnostic trail.
- A small race remains by design: the heartbeat samples every 30 s, so if the user closes Chrome < 30 s before the container stops, the marker is left in a stale-positive state and Chrome will be auto-launched on next resume. This is acceptable (one stray launch is much smaller harm than "never launches"), and the alternative — wrapping Chrome with a trap on EXIT — was already rejected in 002027 because crashes would lose the marker that the user actually wants restored.

## User stories

### Story 1: Tabs come back after a real session pause

**As a** user resuming a sandbox session that has been paused for an hour or more
**I want** Chrome to be running automatically with my tabs restored
**So that** I don't have to manually re-launch Chrome every single time, defeating the point of feature 002027

### Story 2: Closed Chrome stays closed

**As a** user who deliberately closed Chrome before stopping the previous session
**I want** Chrome to stay closed on the next resume
**So that** the desktop isn't cluttered with a browser I didn't want

### Story 3: Diagnose silently-broken auto-launch

**As a** developer investigating why Chrome didn't come back
**I want** a log line in a known location showing whether the auto-launch ran, was skipped, or failed
**So that** I don't have to read git blame to figure out what should have happened

## Acceptance criteria

- [ ] On a Sway sandbox with Chrome open at session stop, resuming the session **any** amount of time later (1 minute, 1 hour, 1 week) results in Chrome being launched automatically with the previous tabs visible.
- [ ] Same on a `helix-ubuntu` (GNOME) sandbox.
- [ ] Same on an **arm64** sandbox running Chromium (invoked via the existing `google-chrome-stable` symlink from 002027) — Chromium auto-relaunches with tabs restored, identical mechanics.
- [ ] On any runtime / arch, deliberately closing the browser before stopping the session results in it **not** being launched on the next resume.
- [ ] On any runtime / arch, a log line is emitted on every session start saying one of: `auto-launching Chrome`, `marker absent, skipping`, or `auto-launch failed: <reason>`. Log location must be readable from inside a running session (e.g. `/tmp/chrome-autolaunch.log` or appended to the existing `start_gnome` / `sway-session` stream).
- [ ] No regression for users who have never opened the browser — the very first session still gets the default desktop with no browser window and no error log noise.
- [ ] No regression for users who close the browser cleanly mid-session and re-open it before stopping — the marker should reflect "browser currently running" by the next heartbeat tick.

## Out of scope

- Changing the persistence mechanism (`.chrome-state` symlink) — that is correct and shipped.
- Changing Chrome's `RestoreOnStartup` policy — set to `1` in both Dockerfiles already.
- Saving open Zed windows, terminal state, etc. — separate concern, same as it was in 002027.
- Replacing the heartbeat with a wrapper-around-Chrome trap mechanism — explicitly rejected in 002027 design.md and not worth revisiting just for this fix.
