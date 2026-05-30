# Requirements: Diagnose Helix→Zed Theme Sync Stuck After Toggle Post-Resume

## Background

The settings-sync-daemon (`api/cmd/settings-sync-daemon/main.go`) mirrors the session owner's `color-scheme` preference into **two surfaces**:

1. **GNOME** — `gsettings` writes (`applyGNOMEColorScheme`) for `color-scheme`, `gtk-theme`, and wallpaper.
2. **Zed** — writes the `theme` key in `~/.config/zed/settings.json` (live-reloaded by Zed).

These two paths are independent. The user reports:

- The **GNOME sync continues to work reliably** — clicking the Helix toggle flips the GNOME shell/chrome.
- The **Zed sync is unreliable**, specifically after resuming a session: toggling from dark → light works once, but toggling back to dark does not return Zed to a dark theme. The two surfaces end up out of sync.

This recurs despite the prior fix from task 001998 (`fix(settings-sync-daemon): make Helix↔GNOME↔Zed theme sync reliable`, commit `8053d6948`) which introduced the special-casing the user remembers:

```go
var HELIX_MANAGED_THEMES = map[string]bool{
    "One Light": true,
    "Ayu Dark":  true,
}
```

`effectiveTheme(apiTheme)` writes the API-supplied theme only when the on-disk value is unset or in `HELIX_MANAGED_THEMES`; anything else is treated as a deliberate user choice and preserved. `extractUserOverrides` also filters `theme` so the daemon's local decision can't replay back through the API as an override.

The hypothesis for this task is that the 001998 mechanism is structurally sound but is being bypassed in at least one new scenario — most likely tied to session resume and/or to the on-disk theme being in a *structured* form (`{mode, light, dark}`) rather than a bare string, which `effectiveTheme` does not handle explicitly.

## User Stories

**US-1: Toggling Helix dark↔light reliably flips Zed**
As a Helix user, I want the Helix UI color-scheme toggle to flip Zed's editor theme in both directions, every time, including immediately after a session resume.

**US-2: Resumed sessions start with the right Zed theme**
As a Helix user who closed Zed mid-session with `color_scheme = dark` set, I want the resumed Zed to come up in `Ayu Dark` (or my custom dark theme) — not stranded on whichever theme settings.json happened to contain at resume.

**US-3: Special-casing is documented and observable**
As an engineer debugging future divergence, I want clear daemon logs showing which `effectiveTheme` branch was taken on each sync (managed-theme overwrite vs custom-theme preserve vs no-op) so I can tell from logs whether the daemon decided correctly or whether the failure is downstream in Zed.

**US-4: Structured `theme` values are handled**
As a user who has used Zed's own theme picker (which can persist `theme` as a `{mode, light, dark}` object rather than a string), I want Helix's color-scheme toggle to still drive my Zed theme — by replacing the structured form with the API-chosen string, or by updating the matching slot inside the structure if that's the safer behaviour.

## Acceptance Criteria

- **AC-1:** The reproduction must be confirmed live in a freshly resumed session, with daemon logs captured for at least one failing toggle, before any code change lands. If the symptom does not reproduce after a clean rebuild, that itself is a valid resolution (root cause: stale deployed image).
- **AC-2:** Verify that the running image actually contains commit `8053d6948` and `462d5e661`. Document the image tag and how it was checked.
- **AC-3:** Capture the literal contents of `~/.config/zed/settings.json` (specifically the `theme` field — string or object) immediately before and after a failing toggle, in the desktop container. If the daemon wrote the file but Zed didn't switch, the failure is in Zed's reactive reload; if the daemon didn't update the file, the failure is in the daemon.
- **AC-4:** Confirm whether the API publishes `config_changed` on the failing toggle. Use the daemon's `config event WS connected` / `config_changed event` log lines plus a direct check that GNOME flipped (proves the daemon ran).
- **AC-5:** If `effectiveTheme` returns the on-disk value when it should have returned `apiTheme` — fix the branch (most likely: handle the structured-theme case). Add unit-test coverage in `main_test.go` for: bare-string managed theme, bare-string custom theme, structured theme with managed slots, structured theme with custom slots, missing on-disk file.
- **AC-6:** If `mergeSettings` or `writeSettings` is being called on a path that overwrites the daemon's correct decision (e.g. an `onFileChanged` re-extract that races with the WS sync), add a guard or a sequencing fix.
- **AC-7:** Daemon logs at INFO level must include, on every sync that touches `theme`: which branch of `effectiveTheme` fired, what was on disk, and what was written. (One line per sync, structured.)
- **AC-8:** After the fix, manually verify in the inner Helix: resume an existing session, then cycle Helix dark→light→dark→light→dark. Each transition must flip Zed within ~2s; settings.json `theme` field must match the chosen color scheme each time.
- **AC-9:** If the failure turns out to be a Zed-side reactive-reload bug (file is updated but Zed doesn't pick it up), open a follow-up task targeting the `zed` repo and link it from this task's `pull_request_*.md`.

## Out of Scope

- Re-implementing the 001998 fix from scratch — extend it, do not replace it.
- Adding a third "System" option to the Helix toggle (already declined in 001997).
- Expanding `HELIX_MANAGED_THEMES` beyond `One Light` / `Ayu Dark` without a separate design decision.
- Pushing Zed's theme back out to GNOME (rejected: unidirectional sync, Helix→both).
- Changes to Zed's own `theme::ToggleMode` action — the user toggles in the Helix UI, not in Zed. Any Zed-side `set_mode()` bug is a separate task (see Risks).

## Related Prior Work

- `001998_when-switching-helix` — primary fix that introduced `HELIX_MANAGED_THEMES`, `effectiveTheme`, polling-applies-GNOME, and the `extractUserOverrides` theme filter.
- `001997_light-mode-and-dark-mode` — established that the most-recent-change-wins model drives both browser theme and inner desktop.
- `001825_still-often-seeing-dark` — earlier symptom in the same area.
- `001878_light-mode-for-the-helix` — original light-mode feature for the Helix UI.
