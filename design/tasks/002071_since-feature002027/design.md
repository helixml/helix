# Design: Fix Chrome Auto-Relaunch on Session Resume

## TL;DR

Delete the 5-minute freshness check on the marker file. Add a one-line diagnostic log. That's it.

```diff
- if [ -f "$CHROME_MARKER" ] && [ $(($(date +%s) - $(stat -c %Y "$CHROME_MARKER"))) -lt 300 ]; then
+ if [ -f "$CHROME_MARKER" ]; then
```

Two files, two near-identical hunks, no Dockerfile changes, no Go changes, no rebuild of anything except the desktop images.

## Root cause recap

The marker is touched while the container is alive; on container stop, the heartbeat freezes and the marker's `mtime` becomes "the moment before stop". On resume, `now - mtime` equals the elapsed off-time of the sandbox, which is almost always much greater than 300 seconds. So the relaunch branch is dead.

The original design.md justified the freshness window as "Survives crashes [where] crashes leave the marker [stale]". But this confuses two scenarios:

| Scenario | What heartbeat does | What user wants |
|---|---|---|
| Chrome closed cleanly | `rm -f marker` on next tick | Don't relaunch |
| Chrome crashed | Detects no Chrome → `rm -f marker` on next tick | Don't relaunch (it was crashing anyway) |
| Container crashed/killed mid-session with Chrome up | Heartbeat frozen, marker stays | **Relaunch** (this is exactly the case we want) |
| Container stopped cleanly with Chrome up | Heartbeat frozen, marker stays | **Relaunch** |
| Container stopped cleanly with Chrome closed | Heartbeat already removed marker before stop | Don't relaunch |

In every row, "marker exists at next startup" is exactly what we should key the relaunch on. The freshness check adds no signal — it only filters out the cases we want to trigger on.

## Fix

### File 1: `desktop/sway-config/startup-app.sh`

Around line 597-603, replace the freshness check with a plain existence check and add a diagnostic log so we know what happened:

```bash
CHROME_MARKER="/home/retro/work/.chrome-state/.was-running"
CHROME_LOG="/tmp/chrome-autolaunch.log"
if [ -f "$CHROME_MARKER" ]; then
    # Hard container kill can leave singleton locks behind.
    rm -f /home/retro/work/.chrome-state/Singleton* 2>/dev/null || true
    echo "[$(date -Is)] [sway-session] Auto-launching Chrome (marker present from previous session)" \
        | tee -a "$CHROME_LOG"
    WAYLAND_DISPLAY=wayland-1 google-chrome-stable >>"$CHROME_LOG" 2>&1 &
else
    echo "[$(date -Is)] [sway-session] Skipping Chrome auto-launch (no marker; was closed or never opened)" \
        >> "$CHROME_LOG"
fi
```

Heartbeat block immediately below stays as-is — it's correct.

### File 2: `desktop/ubuntu-config/startup-app.sh`

Around line 313-335 (inside the `<<GNOME_EOF` heredoc — keep the backslash escapes on `\$`, `\${...}`, `\$(...)`), make the same change. The `gow_log` helper is already sourced at the top of `start_gnome`:

```bash
CHROME_MARKER="/home/retro/work/.chrome-state/.was-running"
CHROME_LOG="/tmp/chrome-autolaunch.log"
# Wait for wayland-0 so Chrome can connect to the compositor.
for i in \$(seq 1 60); do
    [ -S "\${XDG_RUNTIME_DIR}/wayland-0" ] && break
    sleep 1
done
if [ -f "\$CHROME_MARKER" ]; then
    rm -f /home/retro/work/.chrome-state/Singleton* 2>/dev/null || true
    gow_log "[start] Auto-launching Chrome (marker present from previous session)"
    echo "[\$(date -Is)] Auto-launching Chrome" >> "\$CHROME_LOG"
    WAYLAND_DISPLAY=wayland-0 google-chrome-stable >>"\$CHROME_LOG" 2>&1 &
else
    gow_log "[start] Skipping Chrome auto-launch (no marker; was closed or never opened)"
    echo "[\$(date -Is)] Skipping Chrome auto-launch (no marker)" >> "\$CHROME_LOG"
fi
```

Heartbeat block (lines 326-334) stays as-is.

## Key decisions

### Why not shorten the heartbeat interval instead?

You could reduce the 30 s heartbeat to 5 s to narrow the "user closed Chrome right before stopping the container" window. But that wouldn't fix the bug — the freshness check would still reject every multi-minute pause. And running pgrep + touch every 5 s is wasted I/O on every container forever, to mitigate a corner case (occasional stray Chrome launch). Skip.

### Why not wrap `google-chrome-stable` with a trap?

This was rejected in the 002027 design ("brittle because crashes leave the marker") and the reasoning still holds: if Chrome SIGKILLs, the trap never fires. The heartbeat is the right primitive for "is Chrome still up right now?" because it's stateless polling.

### Why log to `/tmp/chrome-autolaunch.log` specifically?

- `/tmp` exists in both runtimes.
- The user reporting the bug can `tail /tmp/chrome-autolaunch.log` after resume and immediately see whether the script even tried.
- We already redirect Chrome's stdout/stderr there, so a crash in Chrome itself is captured.
- Doesn't pollute the workspace volume.

The `gow_log` calls in the Ubuntu version stay because that's how the rest of `start_gnome` reports status — consistency wins over de-duplication.

### Update the 002027 docs?

Yes, but lightly. Add a one-paragraph "Known issue fixed in 002071" note to `002027_can-we-persist-browser/design.md` (or just leave a breadcrumb at the top — "see 002071 for follow-up fix"). Optional, but otherwise the next person reading 002027 thinks the freshness check is intentional.

## Risks

- **None significant.** This change strictly broadens the conditions under which Chrome auto-launches: anywhere the old code launched it, the new code still launches it; anywhere the old code skipped it because of staleness, the new code now launches it — and that's exactly the bug fix.
- The marker file is in `/home/retro/work/.chrome-state/`. That dir is created and seeded by `helix-workspace-setup.sh` before either startup script runs, so the `[ -f ... ]` check can never see a half-created tree.
- The Singleton lock cleanup runs only when we are about to launch, so we don't accidentally yank a lock from a Chrome instance the user started by hand.

## Arch coverage: this fix works for Chromium on ARM as well

The same code path covers Chrome on amd64 and Chromium on arm64, with no arch-specific branching needed:

- **Launch invocation.** Both startup scripts call `google-chrome-stable`. On arm64 the `Dockerfile.sway-helix` / `Dockerfile.ubuntu-helix` already symlink `chromium-browser` → `google-chrome-stable`, so the same binary name resolves correctly on both architectures (this was the design choice from 002027 — see `Dockerfile.sway-helix:683-684`).
- **Marker path.** `/home/retro/work/.chrome-state/.was-running` is arch-agnostic — same path on both. The persistence symlink set up by `helix-workspace-setup.sh:691-693` already covers both `~/.config/google-chrome` and `~/.config/chromium`, so on arm64 Chromium reads its profile from the same persistent dir that touches the marker.
- **Process detection.** The heartbeat check is `pgrep -x chrome ... || pgrep -x chromium`, so the marker is correctly maintained whether the running browser registers as `chrome` (amd64) or `chromium` (arm64). The fix doesn't touch this block, so it stays correct.
- **RestoreOnStartup policy.** Both the Chrome managed policy and the Chromium managed policy were set to `1` in 002027 (`/etc/opt/chrome/policies/managed/helix.json` and `/etc/chromium/policies/managed/helix.json`), so tabs restore on the next launch regardless of arch.

Net effect: the `[ -f "$CHROME_MARKER" ]` change makes auto-relaunch start working on **both** amd64 (Chrome) and arm64 (Chromium) sandboxes from the same line of code.

## Verification

The implementation agent cannot self-test for the same reason 002027's couldn't (see 002027 design.md "Why manual verification can't run from the implementing agent" — restarting kills the session). Reviewer to:

1. Build both images: `./stack build-ubuntu` and the sway equivalent.
2. Start a fresh `helix-sway` session, open Chrome with 2-3 tabs, stop the session, wait > 5 minutes, resume → expect Chrome to come back with tabs.
3. Repeat on `helix-ubuntu`.
4. On either runtime: open Chrome, close it, wait 1 minute (let the heartbeat tick), stop the session, resume → expect Chrome to stay closed.
5. Check `/tmp/chrome-autolaunch.log` inside the resumed session — should contain a single line per session start saying whether the relaunch fired.

## Files to change (implementation phase)

| File | Change |
|---|---|
| `desktop/sway-config/startup-app.sh` | Replace the `-lt 300` freshness check with a bare `[ -f "$CHROME_MARKER" ]`; route diagnostics to `/tmp/chrome-autolaunch.log`. |
| `desktop/ubuntu-config/startup-app.sh` | Same change inside the `start_gnome` heredoc, preserving backslash escapes; emit `gow_log` lines so it shows up in the standard start log too. |
| `design/tasks/002027_can-we-persist-browser/design.md` (optional, in helix-specs) | One-line breadcrumb pointing at 002071. |

No Dockerfile changes. No Go changes. No new files in the runtime image.

## Notes for future agents working on similar tasks

- **Persistence vs. lifecycle are different problems.** "The file survives" (handled by 002027's symlink) is independent of "the process gets restarted" (this fix). Don't conflate them.
- **`mtime`-based "freshness" semantics across container restarts are almost always wrong.** Inside a paused container, wall-clock time stops; on resume, the gap between the last in-container event and `date +%s` is by definition unbounded. If you need "did this happen recently from the user's point of view", you need to record the relevant boolean state explicitly (file exists / file absent), not encode it in the file's mtime.
- The heartbeat-touch loop is fine as a "Chrome currently running" detector at heartbeat granularity. Just don't try to use the mtime for anything else.
- Both desktop variants (`helix-sway`, `helix-ubuntu`) need every startup change mirrored. The Ubuntu version's auto-launch is **inside** the `<<GNOME_EOF` heredoc, so all `$` must be backslash-escaped — easy to miss.
