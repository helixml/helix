# Fix Chrome auto-relaunch on session resume

## Summary

Feature [002027](https://github.com/helixml/helix/pull/2452) shipped two things: Chrome
profile persistence (works) and auto-relaunch of Chrome on the next session if
it was running when the previous session ended (broken). After 002027, profiles
persist correctly but Chrome never auto-reopens, so users have to launch it by
hand every time.

This PR makes auto-relaunch actually fire.

## Why it was broken

The marker file (`/home/retro/work/.chrome-state/.was-running`) is touched
every 30 s by a heartbeat loop while Chrome is running and removed as soon as
Chrome stops. The auto-relaunch check was:

```bash
if [ -f "$CHROME_MARKER" ] && [ $(($(date +%s) - $(stat -c %Y "$CHROME_MARKER"))) -lt 300 ]; then
```

The heartbeat only runs while the container is alive, so on container stop the
marker's `mtime` freezes at "moment before stop". On resume, `now - mtime` is
the sandbox's off-time, which is almost always > 5 minutes — so the freshness
branch is effectively dead and Chrome is never relaunched. The 5-minute window
only works if the user resumes within five minutes of stopping, which is not
how Helix sandboxes are used.

The correct semantics — already implemented by the heartbeat — is "marker
exists ⇒ Chrome was up when the heartbeat last ran ⇒ relaunch; marker absent
⇒ Chrome was closed ⇒ skip". The freshness check adds no signal, only filters
out the cases we want to trigger on.

## Changes

- **`desktop/sway-config/startup-app.sh`** — replace `[ ... -lt 300 ]` with a
  plain `[ -f "$CHROME_MARKER" ]`. Route the auto-launch decision and Chrome's
  own stdout/stderr to `/tmp/chrome-autolaunch.log` so a silent failure can be
  diagnosed by tailing one file. Heartbeat block underneath is untouched.

- **`desktop/ubuntu-config/startup-app.sh`** — same change inside the
  `<<GNOME_EOF` heredoc that builds the gnome start script, preserving the
  `\$` / `\${...}` / `\$(...)` heredoc escapes. Emits both a `gow_log` line
  (lands in the standard start log) and a line to `/tmp/chrome-autolaunch.log`
  in each branch (launching / skipping).

## Arch coverage

Single code path for both architectures, no branching:

- `google-chrome-stable` is already symlinked to `chromium-browser` on arm64
  by the Dockerfiles (002027).
- The persistence symlink set up in `helix-workspace-setup.sh` covers both
  `~/.config/google-chrome` and `~/.config/chromium`.
- The unchanged heartbeat block already checks `pgrep -x chrome || pgrep -x
  chromium`, so the marker is maintained correctly for either binary.
- Both the Chrome and Chromium managed policies were set to
  `RestoreOnStartup: 1` in 002027.

Net effect: this one-line change makes auto-relaunch start working on amd64
(Chrome) and arm64 (Chromium) sandboxes alike.

## Tested

- `bash -n` on both modified scripts passes.
- For the Ubuntu file: extracted the `<<GNOME_EOF` heredoc body, applied the
  `\$` → `$` unescaping that bash does at heredoc-write time, and `bash -n`
  on the resulting `start_gnome` script passes.

End-to-end verification (open Chrome → stop session → wait > 5 min → resume
→ see tabs) requires `./stack build-ubuntu` (and the sway equivalent) plus a
fresh session, which would terminate the spec-task agent that authored this
PR — reviewer to run those checks. See
`design/tasks/002071_since-feature002027/design.md` for the rationale.

## Notes for reviewers

- The risk surface is tiny: this change strictly broadens the conditions
  under which Chrome auto-launches. Anywhere the old code launched, the new
  code still launches; anywhere the old code skipped because of staleness,
  the new code now launches — and that's exactly the fix.
- A small race remains by design: the heartbeat samples every 30 s, so if
  the user closes Chrome < 30 s before the container stops, the marker is
  left stale-positive and Chrome will auto-launch once on next resume. This
  is acceptable (one stray launch is much smaller harm than "never launches")
  and the alternative (wrapping Chrome with an EXIT trap) was already
  rejected in 002027 because crashes would lose the marker that the user
  actually wants restored.
- New diagnostic log: `/tmp/chrome-autolaunch.log` inside the desktop
  container. Captures one line per session start indicating whether the
  relaunch fired, plus Chrome's own stdout/stderr if it did.
