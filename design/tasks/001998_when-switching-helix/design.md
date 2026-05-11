# Design: Reliable Helix ↔ GNOME Theme Sync

## How it works today

`api/cmd/settings-sync-daemon/main.go` runs in the desktop container and is responsible for pushing the user's color scheme into GNOME via `gsettings`. There are two paths:

| Path | Trigger | Function | Calls `applyGNOMEColorScheme()`? |
|------|---------|----------|----------------------------------|
| **Fast path (WebSocket)** | API publishes `config_changed` to `session-updates.<owner>.<session>` when the user toggles | `runConfigEventLoop()` → `syncFromHelix()` (line 889 → 822) | ✅ Yes (line 822) |
| **Slow path (polling)** | 30 s ticker | `pollHelixChanges()` → `checkHelixUpdates()` (line 1375 → 1386) | ❌ **No** |

`applyGNOMEColorScheme(scheme string)` (line 908) runs four `gsettings` commands: `color-scheme`, `gtk-theme`, `picture-uri`, `picture-uri-dark`. It defaults to dark; only `scheme == "light"` takes the light branch.

## Root causes

### Bug 1 — 30-second flakiness (sometimes instant, sometimes never until next manual toggle)

The fast path *only* works when the daemon's WebSocket is currently connected. Pub/sub is **fire-and-forget — events are not retained**, so any `config_changed` published while the daemon is mid-reconnect (e.g. during the 5 s backoff in `subscribeConfigEvents`, line 836, or during the WS handshake, or during a transient API restart) is dropped on the floor.

The slow path is supposed to be the safety net. **It isn't.** `checkHelixUpdates()` rebuilds `settings.json` from `/zed-config` and writes it, but it never calls `applyGNOMEColorScheme()`. So when the WS event is missed, GNOME stays on the old theme indefinitely — until the user toggles again *with the WS up*.

What the user perceives as "30 s" is the polling tick *coinciding* with a recovered WS subscription that picks up the next event — not the polling itself fixing things.

### Bug 2 — light → dark gets stuck on light

Same root cause as Bug 1, plus an asymmetry that makes it more visible going *back* to dark:

- The empty/initial DB state means GNOME boots in dark via `dconf-settings.ini`. The first dark→light toggle hits the WS while it's healthy (just-connected, no churn), so it works.
- After the light apply, the daemon sat on a healthy WS for ~tens of seconds, and Helix-in-Helix WS connections seem to drop on idle / proxy timeout in this environment (to be confirmed by logs in the inner Helix during testing).
- When the user toggles back to dark, the WS is mid-reconnect; the event is lost; polling can't repair it; GNOME stays light. To the user this looks like "dark mode does nothing."

This is a hypothesis to verify by tailing `docker logs <ubuntu-external-container>` for `config event WS disconnected` / `config_changed event` lines while reproducing.

### Bug 3 — light-mode wallpaper is "ugly"

`applyGNOMEColorScheme` line 915 sets the light-mode wallpaper to `Questing_Quokka_Full_Light_3840x2160.png` (Ubuntu 25.10's stock wallpaper). The user wants the Helix logo in both modes.

## Fix

### 1. Make polling actually repair GNOME state

In `checkHelixUpdates()`, after fetching `helixConfigResponse`, call `applyGNOMEColorScheme(config.ColorScheme)` — same as `syncFromHelix()` does at line 822. Idempotent (`gsettings set` to the same value is a no-op for the user), so it's safe to run on every poll.

This single change makes "the next polling tick repairs the desktop" actually true, and fixes both Bug 1 and Bug 2 (which we believe are the same bug).

### 2. Drop the Quokka wallpaper

In `applyGNOMEColorScheme`, remove the wallpaper override in the light branch — leave `wallpaper := "file:///usr/share/backgrounds/helix-logo.png"` for both branches. This keeps the helix-logo across both modes.

Also drop the design-comment justification for swapping wallpapers (line 905-907) since we're no longer doing it. `picture-uri` and `picture-uri-dark` continue to be set on every apply (cheap, idempotent, keeps GNOME consistent regardless of which slot it reads from).

### 3. (Optional, defensive) Tighten the WS reconnect

Two small improvements that reduce the *frequency* of missed events even with the polling repair in place:

- Lower `subscribeConfigEvents` backoff from 5 s to 1 s — the cost is one extra dial attempt per failure, the win is fewer dropped events during transient blips.
- Add a one-shot `syncFromHelix()` immediately after every successful WS reconnect, *before* entering the read loop, so any state change that happened while we were disconnected gets picked up without waiting for the next poll.

These are nice-to-have, not load-bearing — fix #1 alone closes the user-visible bug. Include them if the change is small.

## Verification

End-to-end in the inner Helix:

1. Register / log in at `http://localhost:8080`, start a spec-task session, wait for the desktop to come up.
2. Tail `docker compose exec sandbox-nvidia docker logs -f <ubuntu-external-container> 2>&1 | grep -E "config event|config_changed|applied GNOME"`
3. Toggle light → dark → light → dark a few times in the top bar. Confirm:
   - Each toggle produces an `applied GNOME color-scheme=…` log line within ~1 s (fast path), OR within ≤30 s (slow path) if the WS is down.
   - Wallpaper is helix-logo in both modes (visually verify via desktop screenshot or `gsettings get org.gnome.desktop.background picture-uri`).
4. Force a WS drop to exercise the slow path: `docker compose restart api`, immediately toggle, wait up to 30 s, confirm desktop converges.

## Files to change

| File | Change |
|------|--------|
| `api/cmd/settings-sync-daemon/main.go` line ~1437 (inside `checkHelixUpdates` change branch) | Add `d.applyGNOMEColorScheme(config.ColorScheme)` so the polling fallback actually repairs GNOME. Call it on every poll (not only on diff) so a stale gsettings state self-heals. |
| `api/cmd/settings-sync-daemon/main.go` line 908-931 (`applyGNOMEColorScheme`) | Remove the light-mode wallpaper override — keep `helix-logo.png` for both branches. Update the leading comment to drop the "swap to a Yaru light wallpaper" justification. |
| `api/cmd/settings-sync-daemon/main.go` line 836 (optional) | Lower reconnect sleep 5 s → 1 s. |
| `api/cmd/settings-sync-daemon/main.go` `runConfigEventLoop` (optional) | After successful `dialer.Dial`, call `d.syncFromHelix()` once before entering the read loop, to pick up changes missed while disconnected. |

## Codebase notes for the implementer

- **`syncFromHelix()` and `checkHelixUpdates()` overlap heavily** — both fetch `/zed-config`, both rebuild `helixSettings`. They diverge in subtle ways (`syncFromHelix` always rewrites; `checkHelixUpdates` only writes on diff; only `syncFromHelix` applies GNOME). A future cleanup could collapse them, but that's out of scope here — adding the `applyGNOMEColorScheme` call to `checkHelixUpdates` is the minimal, surgical fix.
- **Pub/sub topic**: `pubsub.GetSessionQueue(userID, sessionID)` → `session-updates.<owner>.<session>`. Published from `publishUserColorSchemeChange()` in `api/pkg/server/user_handlers.go:458`.
- **WS endpoint**: `/api/v1/ws/user?session_id=…` (in `websocket_server_user.go`). One subscription per session.
- **Source of truth for the user's color scheme**: `UserMeta.Config.ColorScheme` (string, one of `""`, `"light"`, `"dark"`). Read by `getZedConfig` at `api/pkg/server/zed_config_handlers.go:300-303` and surfaced as `ColorScheme` in `helixConfigResponse`.
- **Not a hot-reload path**: settings-sync-daemon doesn't reload on Go file change. After implementing, rebuild with `./stack build-ubuntu` and start a *new* spec-task session to test (per `helix/CLAUDE.md`).
- **Test environment**: the inner Helix at `http://localhost:8080` has the full sandbox available — register `test@helix.ml` / `helixtest`, complete onboarding, start a session, and log into a real desktop to reproduce.
