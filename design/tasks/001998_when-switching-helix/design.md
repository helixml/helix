# Design: Reliable Helix ↔ GNOME ↔ Zed Theme Sync

## How it works today

`api/cmd/settings-sync-daemon/main.go` runs in the desktop container. It owns two surfaces:

1. **GNOME** — applied via `gsettings` in `applyGNOMEColorScheme()` (line 908).
2. **Zed** — applied by writing the `theme` key into `~/.config/zed/settings.json`. Zed live-reloads and applies.

There are two trigger paths. They behave differently:

| Path | Trigger | Function | Updates GNOME? | Updates Zed `theme`? |
|------|---------|----------|----------------|----------------------|
| **Fast (WebSocket)** | API publishes `config_changed` on toggle | `runConfigEventLoop` → `syncFromHelix` (line 889 → 822) | ✅ Yes (line 822) | ✅ Sets `d.helixSettings["theme"]` (line 784-786) and writes directly via `writeSettings(d.helixSettings)` — bypasses `mergeSettings` |
| **Slow (polling)** | 30 s ticker | `pollHelixChanges` → `checkHelixUpdates` (line 1375 → 1386) | ❌ **No** | ❌ **No** — line 1427 explicitly skips `theme`; goes through `mergeSettings`, which preserves the on-disk value via `USER_PREFERENCE_FIELDS` (line 1076-1080) |

`USER_PREFERENCE_FIELDS = {"theme": true}` (line 945) was added to protect a user's manual Zed-UI theme choice from being overwritten on every poll. Once the user said "use my color scheme to drive the theme," that protection became a footgun: it now also protects the *stale* on-disk value from being overwritten by an updated Helix-driven theme.

## Root causes (revised after reviewer feedback)

The reviewer correctly observed that GNOME does flip back to dark, but **Zed gets stuck on `One Light`**. That separates two bugs that I had originally conflated:

### Bug A — Zed stuck on `One Light` after dark → light → dark

This is structural in the daemon, independent of any WebSocket flake:

1. User toggles dark → light. WS fires. `syncFromHelix` sets `d.helixSettings["theme"] = "One Light"` and writes it directly. On-disk: `theme: "One Light"`. Zed reloads and goes light. ✅
2. **Anything** that subsequently rewrites `settings.json` — Zed's own settings persistence, the polling tick at 30 s, an `onFileChanged` re-extract — will, the next time the polling path runs, hit `mergeSettings`. `mergeSettings` reads the *current* on-disk theme (`"One Light"`) and pins it into the merged result regardless of `d.helixSettings["theme"]`.
3. User toggles light → dark. WS fires. `syncFromHelix` writes `theme: "Ayu Dark"`. But on the next polling tick, `checkHelixUpdates` rebuilds settings from defaults (no `theme` key from API per line 1427), then `mergeSettings` reads on-disk — and **whatever's on disk wins**. If Zed has rewritten the file between the WS write and the poll (Zed does occasionally rewrite), the on-disk value is `"One Light"` again, and polling pins it back.

The asymmetry the user sees ("light works, dark doesn't") is just because the *first* light apply has no prior on-disk theme to defend against. Every subsequent toggle is at risk.

GNOME doesn't have this problem because `gsettings` writes are direct and there's no merge layer in between.

### Bug B — 30-second flakiness on the GNOME side

Independent of Bug A. `checkHelixUpdates` (the 30 s polling fallback) does not call `applyGNOMEColorScheme`. Pubsub events aren't retained, so any toggle published while the daemon's WS is mid-reconnect (5 s backoff at line 836, plus handshake) is dropped on the floor with no way for polling to repair it. The user experiences this as occasional ~30 s lag — really it's "wait until I happen to reconnect *and* something else publishes."

### Bug C — Light-mode wallpaper is "ugly"

`applyGNOMEColorScheme` line 915 swaps to `Questing_Quokka_Full_Light_3840x2160.png` in light mode. User wants `helix-logo.png` in both modes.

## Fix

### 1. Make Helix authoritative for the Zed theme — but only when on-disk is one of *our* themes

The right model is narrower than "remove `theme` from `USER_PREFERENCE_FIELDS` and let Helix always win." We want:

- If the on-disk Zed theme is one of the two themes the daemon itself sets (`One Light` or `Ayu Dark`) — or unset — Helix's color-scheme-driven theme overrides it. This is what fixes the stuck-on-light bug.
- If the on-disk Zed theme is **anything else** (the user manually picked `Solarized Dark`, `Monokai`, `Tokyo Night`, … in Zed's UI), leave it alone. The user has expressed a real preference and toggling Helix's color scheme should not silently overwrite it.

**Implementation**: introduce a small set and a helper.

```go
// Themes the daemon itself writes from a Helix color-scheme preference.
// On-disk values in this set (or empty) are considered "Helix-owned" and
// may be overwritten on the next sync. Anything else is treated as a
// deliberate user choice and preserved.
var HELIX_MANAGED_THEMES = map[string]bool{
    "One Light": true,
    "Ayu Dark":  true,
}

// effectiveTheme returns the theme value the daemon should write to
// settings.json. apiTheme is what /zed-config returned. Falls back to the
// user's custom on-disk theme when present.
func (d *SettingsDaemon) effectiveTheme(apiTheme string) string {
    if apiTheme == "" {
        return ""
    }
    onDisk := readOnDiskTheme()        // "" if unset / missing / unparseable
    if onDisk == "" || HELIX_MANAGED_THEMES[onDisk] {
        return apiTheme
    }
    return onDisk
}
```

Then:

- In **`syncFromHelix`** (line 784-786): replace the assignment with `if t := d.effectiveTheme(config.Theme); t != "" { d.helixSettings["theme"] = t }`.
- In **`checkHelixUpdates`** (~ line 1424): add the mirror block — same line, same helper. This is what fixes the polling-never-touches-theme half of the bug.
- In **`mergeSettings`** (line 1076-1080): remove `theme` from the `USER_PREFERENCE_FIELDS` loop so the per-call `effectiveTheme` decision is the single source of truth and isn't doubled-back-on by `mergeSettings`. Drop `theme` from `USER_PREFERENCE_FIELDS` entirely (the constant becomes empty — keep the symbol or delete it). Leave `telemetry` preservation untouched (it's keyed off `SECURITY_PROTECTED_FIELDS`, separate concern).
- Truth table:

| On-disk `theme` | Helix color scheme | API returns | What we write |
|---|---|---|---|
| `Ayu Dark` | dark→light | `One Light` | `One Light` ✅ |
| `One Light` | light→dark | `Ayu Dark` | `Ayu Dark` ✅ (the bug fix) |
| `Monokai` | dark→light | `One Light` | `Monokai` (user's choice preserved) |
| unset | dark | `Ayu Dark` | `Ayu Dark` |
| unset | (none set) | `""` | unchanged on disk |

**Trade-off — when does a user re-engage color-scheme theming after picking a custom theme?** They have to manually set their Zed theme back to `One Light` or `Ayu Dark`. Acceptable: this only matters for users who picked a non-Helix theme in the first place, and there's no Helix UI for "reset Zed theme" today (out of scope).

**Caveat**: if the user manually picks `Ayu Dark` or `One Light` in Zed's UI while their Helix color scheme is the *opposite*, the next sync will flip them to the matching one. That's a minor edge case — picking one of the two protected themes manually counts as opting into Helix-managed behavior.

### 2. Make the polling fallback also apply GNOME

In `checkHelixUpdates`, after the response decode, call `d.applyGNOMEColorScheme(config.ColorScheme)` — same as `syncFromHelix` does at line 822. Idempotent, cheap, and makes "polling repairs missed events" actually true. Fixes Bug B.

### 3. Drop the Quokka wallpaper

In `applyGNOMEColorScheme`, remove the wallpaper override in the light branch — leave `wallpaper := "file:///usr/share/backgrounds/helix-logo.png"` for both branches. Update the leading comment so the next reader doesn't add it back.

### 4. (Optional, defensive) Tighten WS reconnect

- Lower the `subscribeConfigEvents` reconnect sleep from 5 s → 1 s.
- After every successful WS reconnect, call `syncFromHelix()` once before entering the read loop — this picks up any state change that happened during the disconnect without waiting for the next 30 s poll.

These are nice-to-have. Fix #1 + #2 + #3 alone close the user-visible bugs.

## What about a user manually changing the theme in Zed's UI?

Covered above by the `HELIX_MANAGED_THEMES` set. A user who picks anything outside `{One Light, Ayu Dark}` in Zed's UI keeps their choice — Helix toggles change GNOME but leave Zed alone. A user who re-picks one of the two managed themes opts back into color-scheme-driven syncing on the next toggle.

## Verification (live)

I was not able to run an end-to-end live test during the design phase — the inner Helix at `http://localhost:8080` is up but no spec-task session is currently running, and registering / onboarding / starting a fresh session is substantial setup. The implementation phase should:

1. Register at `http://localhost:8080`, complete onboarding, start a spec-task session, wait for the desktop.
2. Find the desktop container: `docker compose -f /home/retro/work/helix/docker-compose.dev.yaml exec -T sandbox-nvidia docker ps --format "{{.Names}}" | grep ubuntu-external`.
3. Tail daemon logs: `docker compose exec sandbox-nvidia docker logs -f <name> 2>&1 | grep -E "config event|config_changed|applied GNOME|Updated settings.json"`.
4. Inspect Zed's on-disk theme between toggles: `docker compose exec sandbox-nvidia docker exec <name> cat /home/retro/.config/zed/settings.json | jq '.theme'`.
5. Toggle dark → light → dark → light a few times in the Helix top bar. After each, confirm:
   - GNOME: `gsettings get org.gnome.desktop.interface color-scheme` matches.
   - Zed file: `.theme` matches (`"One Light"` or `"Ayu Dark"`).
   - Wallpaper: `gsettings get org.gnome.desktop.background picture-uri` is `helix-logo.png` in both modes.
6. Force a missed event: `docker compose restart api`, immediately toggle, wait ≤30 s, confirm both surfaces converge.

The user offered to help click through this if needed — flag for help if blocking on UI access.

## Files to change

| File | Change |
|------|--------|
| `api/cmd/settings-sync-daemon/main.go` line 945-947 (`USER_PREFERENCE_FIELDS`) | Remove `theme` from the map. Update the leading comment. |
| `api/cmd/settings-sync-daemon/main.go` (new code, near other constants) | Add the `HELIX_MANAGED_THEMES` set and the `effectiveTheme(apiTheme string) string` helper described above. |
| `api/cmd/settings-sync-daemon/main.go` `syncFromHelix` (line 784-786) | Replace with `if t := d.effectiveTheme(config.Theme); t != "" { d.helixSettings["theme"] = t }`. |
| `api/cmd/settings-sync-daemon/main.go` `checkHelixUpdates` (~ line 1424-1428) | Add the same `effectiveTheme`-guarded assignment, mirroring the new syncFromHelix block. Update the "Note: theme is a USER_PREFERENCE_FIELD" comment to reflect the new ownership. |
| `api/cmd/settings-sync-daemon/main.go` `checkHelixUpdates` (after response decode) | Call `d.applyGNOMEColorScheme(config.ColorScheme)` so the polling fallback also repairs GNOME. Call it on every poll (not only on diff) so a stale gsettings state self-heals. |
| `api/cmd/settings-sync-daemon/main.go` `applyGNOMEColorScheme` (line 908-931) | Remove the light-mode wallpaper override — keep `helix-logo.png` for both branches. Update the leading comment. |
| `api/cmd/settings-sync-daemon/main.go` line 836 (optional) | Lower reconnect sleep 5 s → 1 s. |
| `api/cmd/settings-sync-daemon/main.go` `runConfigEventLoop` (optional) | After successful `dialer.Dial`, call `d.syncFromHelix()` once before entering the read loop. |

## Codebase notes for the implementer

- **Where the API computes the theme**: `api/pkg/server/zed_config_handlers.go:300-309`. Reads `UserMeta.Config.ColorScheme` from the session owner; maps `"light"` → `"One Light"`, `"dark"` → `"Ayu Dark"`. If the owner has no preference set, the API returns whatever theme the agent's config specified (could be empty). This means with the changes above, `theme` writes only happen when ColorScheme is set or the agent has a non-empty default — which is the right behavior.
- **`syncFromHelix` vs `checkHelixUpdates` overlap**: both fetch `/zed-config` and rebuild `helixSettings`, but they diverge in subtle ways (sync always rewrites; check only writes on diff; only sync applies GNOME and theme today). A future cleanup could collapse them. Out of scope here — keep the surgical patches.
- **Pub/sub topic**: `pubsub.GetSessionQueue(userID, sessionID)` → `session-updates.<owner>.<session>`. Published from `publishUserColorSchemeChange` in `api/pkg/server/user_handlers.go:458`.
- **WS endpoint**: `/api/v1/ws/user?session_id=…` (in `websocket_server_user.go`). One subscription per session.
- **Source of truth for the user's color scheme**: `UserMeta.Config.ColorScheme`, one of `""`, `"light"`, `"dark"`. Read by `getZedConfig`.
- **Not a hot-reload path**: settings-sync-daemon doesn't reload on Go file change. Per `helix/CLAUDE.md`: rebuild with `./stack build-ubuntu` and start a *new* spec-task session to test.
- **`mergeSettings`'s preserve-from-disk logic** (line 1067-1082) preserves `telemetry` (security) and any field in `USER_PREFERENCE_FIELDS`. After this change, only `telemetry` will be preserved that way — which is the safer default.
