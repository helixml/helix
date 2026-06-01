# feat(settings-sync-daemon): log effectiveTheme branch per sync

## Summary

Adds structured per-sync logging to the settings-sync-daemon's theme decision so future regressions of the Helix→Zed theme sync are diagnosable by `grep`ping daemon logs, not by re-deriving the decision from source.

Follows up on task 001998 (commits `8053d6948` + `462d5e661`), which fixed the original "Zed stuck on One Light after dark→light→dark" bug by introducing `HELIX_MANAGED_THEMES` and `effectiveTheme()`. The user has reported a similar symptom recurring specifically after session resume — but with the existing logging it is impossible to tell from logs whether the daemon decided correctly and the failure is downstream in Zed, or whether the daemon itself is bypassing the managed-theme logic.

This change is purely observational + a refactor for testability. **No behaviour change** beyond the new log line.

## Changes

- Split `effectiveTheme` into:
  - `computeEffectiveTheme(apiTheme) (result, branch, onDiskRepr)` — pure decision function, no I/O outside the existing settings.json read.
  - `effectiveTheme(apiTheme) string` — thin wrapper that calls `computeEffectiveTheme` and emits one INFO line:
    ```
    theme sync: branch=managed_overwrite on_disk="One Light" wrote="Ayu Dark" api="Ayu Dark"
    ```
- Branches reported: `no_api_theme` | `no_existing_file` | `unparseable` | `no_theme_key` | `structured_replace` | `empty_string` | `managed_overwrite` | `preserve_custom`. The `structured_replace` branch is broken out explicitly because Zed's own theme picker / `ToggleMode` action can leave settings.json with a `{mode, light, dark}` object that the daemon replaces with a bare string — relevant to the 002056 investigation hypothesis H1 (sticky `Dynamic{mode:System}` state in Zed).
- Convert `SettingsPath` and `KeymapPath` from `const` to `var` so unit tests can point them at a tempdir without touching the real Zed config. `PollInterval` and `DebounceTime` stay `const`.
- Add `TestComputeEffectiveTheme` with 9 cases covering every branch.

## Test plan

- [x] `go test ./api/cmd/settings-sync-daemon/` — all tests pass locally, including the new `TestComputeEffectiveTheme` (9 subtests).
- [ ] Build a new desktop image (`./stack build-ubuntu`), start a fresh inner-Helix session, toggle Helix dark↔light a few times, confirm one `theme sync: …` log line per sync via:
  ```
  docker compose exec sandbox-nvidia docker logs -f <ubuntu-external> 2>&1 | grep "theme sync"
  ```
- [ ] Verify the structured-replace branch fires when a session's `~/.config/zed/settings.json` contains a `{mode,light,dark}` object on a Helix toggle.

## Why this lands first (before any "fix" for the user's symptom)

Per the 002056 design, this observability change is *prerequisite* to identifying which of the 5 hypotheses (stale image, structured-theme stickiness, resume race, own-write guard race, working-as-designed-with-custom-theme) actually fired in the user's session. Without it, we'd be guessing. Once a session running this build reproduces the symptom, one line of log output will pin the hypothesis.

Refs spec task 002056. Prior: 001998 (`fix(settings-sync-daemon): make Helix↔GNOME↔Zed theme sync reliable`).
