# Design: Diagnose Helix→Zed Theme Sync Stuck After Toggle Post-Resume

## Current state (post-001998)

The settings-sync-daemon already implements the obvious fix the user is half-remembering ("it special-cases certain themes"). In `api/cmd/settings-sync-daemon/main.go`:

- `HELIX_MANAGED_THEMES = {"One Light": true, "Ayu Dark": true}` (lines 1116–1119) — the two themes the daemon itself writes from a color-scheme preference.
- `effectiveTheme(apiTheme string) string` (lines 1126–1143) — returns `apiTheme` when on-disk `theme` is unset, empty, or in `HELIX_MANAGED_THEMES`; otherwise returns the on-disk value to preserve a user's manual Zed-UI choice.
- `syncFromHelix` (line 954) and `checkHelixUpdates` (line 1709) both call `effectiveTheme`.
- `extractUserOverrides` (line 1440) skips `"theme"` so the daemon's local decision can't be uploaded as a user override and replayed back.
- `checkHelixUpdates` (line 1692) calls `applyGNOMEColorScheme` on every poll — idempotent, repairs missed WS events.

Commits in `git log`:
- `8053d6948` — the 001998 main fix
- `462d5e661` — follow-up tightening WS reconnect for faster convergence

Both are merged. So the user's current symptom is **either a regression / new failure mode**, or the running container image doesn't contain those commits.

## Hypotheses (ranked)

### H1 — Structured `theme` on disk bypasses `effectiveTheme` correctly but the daemon's string write doesn't dislodge Zed's in-memory `Dynamic { mode: System }` state

If at some point Zed's `theme::ToggleMode` action ran (independent of the Helix toggle), the on-disk `theme` could be:

```json
"theme": { "mode": "system", "light": "One Light", "dark": "Ayu Dark" }
```

(See the separately-identified Zed `set_mode` defect, which hardcodes `mode: System` when converting `Static → Dynamic` — out of scope here, but relevant context.)

`effectiveTheme` does `existing["theme"].(string)` — for a structured value `ok = false`, so `!ok` triggers and it returns `apiTheme`. The daemon then writes `"theme": "Ayu Dark"` (string). That **should** dislodge the structured form on Zed's next reload.

Possible failure: if Zed's settings live-reload doesn't fully reset the in-memory theme when the structure changes from object → string (or if `mode: System` is sticky and tied to a `SystemAppearance` snapshot that doesn't refresh), the rendered theme stays on the system-driven value. The daemon log would still show "wrote Ayu Dark" — making the failure invisible to anyone reading daemon logs only.

**Test:** Reproduce, then inspect both `settings.json` (proves daemon wrote correctly) and Zed's rendered theme (proves Zed picked it up or didn't). Divergence between the two is the smoking gun for a Zed-side reload bug.

### H2 — The deployed image doesn't actually contain the 001998 fix

`./stack build-ubuntu` doesn't always transfer cleanly to the sandbox (documented in `helix/design/2026-03-12-settings-sync-daemon-fixes.md` "Bug 2"). It's possible the user is hitting a session running an older image where `theme` was still in `USER_PREFERENCE_FIELDS` and `mergeSettings` pinned the on-disk value.

**Test:** Check the running image tag, `docker exec` into the desktop container, and inspect the daemon binary's build date or run `--version` if present. Confirm against `sandbox-images/helix-ubuntu.version`.

### H3 — Session-resume timing race

On session resume, ordering is:
1. Container starts → start-zed-core.sh launches the daemon
2. Daemon's `main()` runs `syncFromHelix()` with retries
3. Zed starts (separately, also from start-zed-core.sh)
4. Zed reads `~/.config/zed/settings.json` at startup, applies theme
5. Daemon's WS subscriber connects, may re-sync

If Zed reads settings.json **before** the daemon has finished its initial sync write, Zed comes up with the stale theme from the previous session's file. Subsequent Helix toggles do update settings.json — but if Zed's reactive-reload watcher missed the early write window or if Zed has cached state from start-up, the toggle wouldn't apply.

**Test:** Inspect daemon log timestamps (`Updated settings.json`) vs. Zed log timestamps (Zed startup / theme load) on a fresh resume. Watch a single Helix toggle post-resume and confirm whether settings.json's `theme` field actually changes on disk.

### H4 — `onFileChanged` races with the WS-triggered `syncFromHelix`

After the daemon writes settings.json, fsnotify fires. The `lastModified < 1*time.Second` guard at `onFileChanged` (line 1587) is supposed to suppress own-write echoes — but it relies on wall-clock comparison from `d.lastModified = time.Now()` set inside `writeSettings`. If multiple writes pile up (initial sync + immediate WS push), this guard can race.

If `onFileChanged` does run, it calls `extractUserOverrides` (which skips `theme` — defensive, post-001998) and uploads to the API. Even if theme is skipped, the upload itself shouldn't corrupt the daemon's view. But it's worth confirming the guard holds in the failure scenario.

**Test:** Add temporary INFO logging to `onFileChanged` showing the elapsed-since-last-write, then reproduce the failure.

### H5 — User's settings.json contains a custom theme that effectiveTheme correctly preserves, masking the perceived bug

If the user previously set Zed to e.g. `"theme": "Solarized Dark"`, `effectiveTheme` returns `"Solarized Dark"` regardless of what Helix sends — by design. The user perceives this as "Zed doesn't switch", because `Solarized Dark` does not change when they toggle. This is **working as intended** per 001998's preserve-custom-themes design.

**Test:** Read `~/.config/zed/settings.json` and check whether `theme` is `One Light` / `Ayu Dark` or something else. If something else, this is a UX issue (user expectation vs. documented behaviour), not a code bug.

## Approach

This task is primarily an **investigation**, not a coding task. A code change should only land once the failure mode is reproduced and the right hypothesis confirmed.

### Phase 1 — reproduce and capture

1. Start an inner-Helix session, let it stabilise, close it (resume target).
2. Resume; capture daemon logs from startup.
3. Toggle Helix dark → light → dark via the UI.
4. After each toggle, capture: GNOME `color-scheme`, `settings.json.theme`, Zed's rendered theme (visible in window chrome / via Zed's command palette).
5. Compare daemon logs against on-disk state and rendered state.

### Phase 2 — root-cause

Use the captures from Phase 1 to identify which of H1–H5 fired (or a new H6). Update this design doc with the finding.

### Phase 3 — fix the actual bug

- **If H1:** the fix is in Zed (see Risks), not in the daemon. Open a follow-up against the `zed` repo.
- **If H2:** rebuild and redeploy the image (`./stack build-ubuntu` + verify `sandbox-images/helix-ubuntu.version` matches across desktop and sandbox per `2026-03-12-settings-sync-daemon-fixes.md`).
- **If H3:** add a startup ordering guarantee — e.g. block Zed launch on the daemon's initial sync completing, or have the daemon write settings.json synchronously before any other init step runs.
- **If H4:** strengthen the own-write guard (track writes by file `mtime` or a sequence counter; switch to a `wasWrittenByDaemon` boolean that the watcher resets after the next event).
- **If H5:** documentation / UX fix — surface the special-casing in the Helix UI or in the toggle's tooltip ("theme follows your custom Zed selection").

### Phase 4 — add observability so the next regression is debuggable from logs alone

Regardless of which hypothesis lands, add one structured INFO log line per sync that touches `theme`:

```
theme sync: branch=managed_overwrite on_disk="One Light" wrote="Ayu Dark" api="Ayu Dark"
theme sync: branch=preserve_custom on_disk="Solarized Dark" wrote=<none> api="Ayu Dark"
theme sync: branch=structured_replace on_disk=<object> wrote="Ayu Dark" api="Ayu Dark"
theme sync: branch=no_api_theme on_disk=… wrote=<none> api=""
```

This makes Phase 1 of any future investigation a `grep`, not a code-walk.

## Risks / Open Questions

- **Zed-side `set_mode` defect** (separately identified in the `zed` repo, `crates/theme_settings/src/settings.rs:292-315`): hardcodes `mode: System` and resets theme slots when converting `Static → Dynamic`. If the user's Zed has been in a state where this fired, `~/.config/zed/settings.json` may contain a structured `theme` that, when replaced by the daemon's string write, leaves Zed in a stale in-memory state. Out of scope for this Helix task; flag as a follow-up Zed task if H1 confirms.
- **`mergeSettings` does NOT call `effectiveTheme`.** It pulls `theme` from `helixSettings` (which was set by the caller's `effectiveTheme` call). If a future code path calls `mergeSettings` without first calling `effectiveTheme` on the API response, the merged settings will contain whatever theme is in `helixSettings` from the last sync — potentially stale. Audit during fix.
- **The Helix→GNOME path uses gsettings (direct, synchronous), the Helix→Zed path uses a JSON file write (indirect, reactive).** Failures will always be asymmetric between the two even when the daemon is correct — the GNOME path has fewer moving parts.
- **`http.Client` has no timeout** (called out in 001998 implementation notes line 157). If the API is briefly unreachable during resume, the daemon's initial `syncFromHelix` can block for ~30s on a TCP timeout, widening any startup race window in H3.

## Implementation Notes (after coding)

- **Built and shipped** `helix-ubuntu:12c14d` (image tag in `sandbox-images/helix-ubuntu.version`) containing the new structured logging on the `feature/002056-diagnose-helixzed-theme` branch.
- Code change is **purely observational**: split `effectiveTheme` into `computeEffectiveTheme(apiTheme) (result, branch, onDiskRepr)` (pure decision) + `effectiveTheme(apiTheme)` (logging wrapper). One INFO log line per call:
  ```
  theme sync: branch=<X> on_disk=<Y> wrote=<Z> api=<W>
  ```
- Branches: `no_api_theme | no_existing_file | unparseable | no_theme_key | structured_replace | empty_string | managed_overwrite | preserve_custom`.
- The `structured_replace` branch is broken out explicitly because Zed's own `ToggleMode` action can persist `theme` as a `{mode, light, dark}` object. The daemon's existing behavior (replace with bare string) is preserved — just made visible in logs.
- `mergeSettings` audit clean: `theme` is set in exactly two callsites (`syncFromHelix:959` and `checkHelixUpdates:1754`), both routed through `effectiveTheme`. `USER_PREFERENCE_FIELDS` is empty post-001998 so `mergeSettings`'s preserve-from-disk loop is a no-op for theme.
- Test coverage: `TestComputeEffectiveTheme` in `main_test.go` covers all 8 branches with 9 cases. Required converting `SettingsPath` / `KeymapPath` from `const` to `var` so tests can point them at a tempdir.
- **Live reproduction in inner Helix attempted but blocked**: registered test user, completed onboarding, queued a chat — but the spec-task pipeline returned an error and no `ubuntu-external` container started in this environment. The image is built, transferred, and ready in the sandbox; reproduction is best done by the user with a normal session start.

## What the user can do next

1. Start a fresh spec-task session (any project) — it will pull `helix-ubuntu:12c14d` automatically.
2. Tail the daemon logs filtered to the new line:
   ```
   docker compose exec sandbox-nvidia docker logs -f <ubuntu-external-container> 2>&1 | grep "theme sync"
   ```
3. Toggle Helix dark↔light a few times. Each toggle should emit one `theme sync:` line.
4. Reproduce the symptom (resume + toggle); the branch label in the matching `theme sync:` log line will pin which hypothesis from this design doc actually fires. Send that line back and I'll wire the targeted fix.

## Reference Files

- `/home/retro/work/helix/api/cmd/settings-sync-daemon/main.go` — daemon source
  - `HELIX_MANAGED_THEMES` (1116–1119)
  - `effectiveTheme()` (1126–1143)
  - `syncFromHelix` theme assign (954)
  - `checkHelixUpdates` theme assign (1709), GNOME apply (1692)
  - `extractUserOverrides` theme skip (1440)
  - `mergeSettings` USER_PREFERENCE_FIELDS loop (1337–1342)
  - `onFileChanged` own-write guard (1587)
- `/home/retro/work/helix/api/cmd/settings-sync-daemon/main_test.go` — extend with structured-theme + missing-file tests
- `/home/retro/work/helix-specs/design/tasks/001998_when-switching-helix/design.md` — prior fix rationale
- `/home/retro/work/helix/design/2026-03-12-settings-sync-daemon-fixes.md` — image-transfer gotchas relevant to H2
- `/home/retro/work/zed/crates/theme_settings/src/settings.rs:292-315` — Zed-side `set_mode` defect (related, out of scope for this task)
