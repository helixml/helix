# Design: Persist Browser State and Auto-Restore on Session Resume

## Approach

Follow the **existing `.claude-state` pattern** (`desktop/shared/helix-workspace-setup.sh:535-567`): symlink the ephemeral profile path under `~/.config/` into a directory under `/home/retro/work/`, which is the one and only persistent bind-mount in the container.

Two pieces:

1. **State persistence** — symlink `~/.config/google-chrome` → `$WORK_DIR/.chrome-state` (and `~/.config/chromium` → same dir on arm64) at startup, before anything launches the browser.
2. **Auto-restore** — flip the Chrome enterprise policy `RestoreOnStartup` from `5` (NTP) to `1` (restore last session), and add an auto-launch step in `startup-app.sh` that runs Chrome in the background iff a `was-running` marker exists in the state directory.

## Why this matches the codebase

- **No new storage primitive.** `~/work/` is already the contract for "things that survive". Adding another symlinked state dir matches what `.claude-state`, `.zed-state` (sway `startup-app.sh:63`), and `.git-credentials` handling already do. The user even suggested "a symlink into the work directory" in the original ask — this is correct.
- **No new lifecycle hook.** `helix-workspace-setup.sh` already runs before Zed launches and is the right place for `~/.config/*` setup. `startup-app.sh` already has the right order of operations to add a background `google-chrome-stable &` after Sway is up.
- **Single command on both arches.** `Dockerfile.sway-helix:683-684` already symlinks `chromium-browser` → `google-chrome-stable` on arm64, so the same `google-chrome-stable &` line works everywhere. But Chromium reads from `~/.config/chromium`, not `~/.config/google-chrome`, so the symlink step must cover both paths (one of them just won't exist on the other arch — `ln -sfn` is idempotent and harmless).

## Key decisions

### 1. Where in the persistent volume

`$WORK_DIR/.chrome-state` (i.e. `/home/retro/work/.chrome-state`). Mirrors `.claude-state` / `.zed-state` naming. The leading dot keeps it out of the way of repos in Zed's project picker.

### 2. RestoreOnStartup = 1 (Restore last session)

Set this in the existing enterprise policy JSON at `Dockerfile.sway-helix:695-713` and the equivalent block in `Dockerfile.ubuntu-helix`. Also update the arm64 Chromium policy at `Dockerfile.sway-helix:687`.

Trade-off considered: value `4` (open list of URLs) is more explicit but loses tabs the user opened mid-session. Value `1` is what Chrome's own "Continue where you left off" setting does, and is what the user is asking for.

Risk: Chrome's "session restore" only triggers when Chrome wasn't closed cleanly *or* when this policy is set. With the policy set, it triggers on every launch — exactly what we want.

### 3. Marker file for auto-launch

`$WORK_DIR/.chrome-state/.was-running` (touched whenever Chrome is up).

Two reasonable implementations; pick the cheaper one in implementation:

- **(a) Touch on launch, delete on intentional close.** A wrapper around `google-chrome-stable` touches the marker on exec, and a SIGTERM trap deletes it. Brittle because crashes leave the marker.
- **(b) Periodic touch while running, delete-if-stale on startup.** A small `while pgrep chrome; do touch …; sleep 30; done &` loop kept alive by the desktop session, and a startup check that ignores markers older than ~5 min. Survives crashes.

Recommend (b). The startup-app.sh script already runs long-lived background loops (e.g. settings-sync-daemon, dbus), so adding one more is consistent.

### 4. Auto-launch placement

In `desktop/sway-config/startup-app.sh`, after Sway is up and the i3 assignment rules for Chrome are loaded (around line 469), add:

```bash
if [ -f "$WORK_DIR/.chrome-state/.was-running" ] && \
   [ $(($(date +%s) - $(stat -c %Y "$WORK_DIR/.chrome-state/.was-running"))) -lt 300 ]; then
    google-chrome-stable >/dev/null 2>&1 &
fi
```

Mirror in `desktop/ubuntu-config/startup-app.sh`.

### 5. Profile compatibility across Chrome upgrades

Chrome maintains backward compat for profile data within the same major version family, but a *downgrade* (newer profile → older Chrome) can refuse to start. Since `helix-ubuntu`/`helix-sway` images update Chrome via `apt-get install -y google-chrome-stable`, version skew is possible.

Mitigation: on startup, if `$WORK_DIR/.chrome-state/Default/Preferences` exists but Chrome fails to launch within ~10s, log it and continue (don't block session startup). Don't try to be clever — Chrome's own recovery handles most cases.

## Files to change (for the implementation phase)

| File | Change |
|---|---|
| `desktop/shared/helix-workspace-setup.sh` | Add `~/.config/google-chrome` + `~/.config/chromium` symlink block, mirroring the `.claude` block at line 535-567. Run before Chrome is ever launched. |
| `Dockerfile.sway-helix` | (a) Change `RestoreOnStartup: 5 → 1` in both the chromium policy (line 687) and the chrome policy (line 707). (b) Keep the first-run sentinel logic at line 724-729 intact — it should be skipped naturally once a persistent profile exists. |
| `Dockerfile.ubuntu-helix` | Same RestoreOnStartup change. |
| `desktop/sway-config/startup-app.sh` | Add auto-launch block + the periodic-touch background loop. |
| `desktop/ubuntu-config/startup-app.sh` | Same. |

No new Go code, no new packages, no docker-compose changes.

## Risks and gotchas

- **First-run sentinel collision.** `Dockerfile.sway-helix:724-729` pre-creates `~/.config/google-chrome/First Run` and a Preferences file in `/etc/skel`. On a fresh persistent profile dir these get copied in via the home init, but the *symlink* approach bypasses `/etc/skel`. Implementation must seed an empty `$WORK_DIR/.chrome-state/` with the same sentinels on first run.
- **Saved logins.** Chrome on Linux defaults to `--password-store=basic` (set explicitly at `Dockerfile.sway-helix:715,718`), which means passwords are stored unencrypted on disk. With persistence on, these passwords now survive container restart. That's the same trade-off as the rest of the workspace (e.g. `.git-credentials` is already persisted). Worth a note in the SWAY-USER-GUIDE.md but not a blocker.
- **`SyncDisabled: true`** in the policy means Chrome's own cloud sync is off — fine, that's an unrelated privacy decision, and our persistence works without it.
- **Concurrent sessions are not a concern.** A sandbox is bound to a single host (`controller_provision.go:336-364`), so two Chrome processes won't ever race for the same profile dir.
- **Profile lock file (`SingletonLock`).** If the container dies hard, a stale lock can block Chrome on next launch. Auto-launch should `rm -f $WORK_DIR/.chrome-state/Singleton*` before invoking Chrome.

## Verification

- Launch a Sway sandbox, open Chrome with 3 tabs (e.g. helix.ml, github.com, duckduckgo.com), restart the container, confirm tabs come back and Chrome was already running on workspace 3.
- Repeat with `helix-ubuntu` (GNOME) sandbox.
- Repeat with Chrome explicitly closed before container restart — confirm Chrome stays closed.
- Check `ls -la /home/retro/.config/google-chrome` inside the container — must be a symlink.

### Why manual verification can't run from the implementing agent

The implementation runs inside an existing container started from the *old*
image — `RestoreOnStartup: 5` is still in effect, `/etc/chromium/policies/managed/helix.json`
doesn't exist (this host is amd64/Chrome), and `/home/retro/work/.chrome-state`
hasn't been created (helix-workspace-setup.sh runs at session start). The
end-to-end flow only kicks in after `./stack build-ubuntu` + sway equivalent
and starting a brand new session. Verifying it requires terminating the
current session, which would kill the implementing agent. Reviewer to run
the manual checks after merging or in a separate fresh session.

## Notes for future agents working on similar tasks

- The `.claude-state` block in `helix-workspace-setup.sh:535-567` is the **canonical persistence template** in this repo. Copy its structure — including the "preserve any files written before the symlink was set up" cp step — for any new symlinked state dir.
- Chrome enterprise policies on Linux live in two places: `/etc/opt/chrome/policies/managed/*.json` (Google Chrome, amd64) and `/etc/chromium/policies/managed/*.json` (Chromium, arm64). Both must be kept in sync.
- `helix-sway` and `helix-ubuntu` desktop variants share `desktop/shared/*` but have **separate** `startup-app.sh` files in `desktop/sway-config/` and `desktop/ubuntu-config/`. Any startup-time change usually needs to be mirrored.
- Image rebuild + new session is required after any Dockerfile change: `./stack build-ubuntu` (or sway equivalent) then start a fresh session — existing containers keep the old policy.
