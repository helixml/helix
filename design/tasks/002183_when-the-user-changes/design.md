# Design: Fix Zed Theme Not Reverting on OS Dark/Light Mode Toggle

## Summary

The OS-appearance → theme update chain in the cross-platform code is **provably
correct and unconditional**. Tracing it end to end (below) shows that *if the
appearance-change event fires with the correct value, the theme always updates*.
Therefore the reverse-transition failure is almost certainly upstream of that
chain — in the **platform layer that delivers the appearance-change signal**
(on Linux: the XDG appearance / `org.freedesktop.appearance` `color-scheme`
portal watcher). The fix must be localized by instrumentation first, then the
break repaired, and finally locked in with a regression test — which has been
the missing piece every prior attempt lacked.

## Verified Call Chain (cross-platform — confirmed correct)

```
OS toggles appearance
  → platform window emits on_appearance_changed callback   [PLATFORM LAYER — not in this checkout]
  → Window::appearance_changed()                            crates/gpui/src/window.rs:2274
        self.appearance = self.platform_window.appearance();
        appearance_observers.retain(|cb| cb(self, cx))      // observers kept (cb returns true)
  → Workspace observe_window_appearance closure             crates/workspace/src/workspace.rs:1781
        *SystemAppearance::global_mut(cx) = window.appearance().into();
        theme_settings::reload_theme(cx);
        theme_settings::reload_icon_theme(cx);
  → reload_theme → configured_theme                         crates/theme_settings/src/theme_settings.rs:155
        reads SystemAppearance::global(cx) FRESH each call
        theme_name = theme.name(system_appearance)          crates/theme_settings/src/settings.rs:148
  → GlobalTheme::update_theme (UNCONDITIONAL set)           crates/theme/src/theme.rs:329
  → cx.refresh_windows()
```

Each step was inspected:
- `observe_window_appearance` registers an observer whose callback returns `true`,
  so it is **retained** across transitions (`gpui/src/window.rs:1833`). Not a
  one-shot.
- `WindowAppearance → Appearance` conversion handles all four variants correctly
  (`theme/src/theme.rs:94`).
- `configured_theme` re-reads the global appearance on every call — no stale
  capture (`theme_settings/src/theme_settings.rs:155`).
- `GlobalTheme::update_theme` sets the theme unconditionally — no
  name-dedup/cache that could swallow the reverse switch (`theme/src/theme.rs:329`).

Because every cross-platform step is correct and unconditional, the value
reaching `Workspace` (`window.appearance()`) must be wrong/stale on the reverse
transition, or the platform callback does not fire the second time.

## Root-Cause Hypotheses (ranked)

1. **(Most likely) Platform appearance signal is wrong/one-directional.** The
   Linux portal watcher updates the cached appearance on the first change but
   reports a stale value (or does not re-dispatch `appearance_changed`) on the
   reverse change. Look in the full repo's `crates/gpui/src/platform/linux/`
   (X11 + Wayland clients and the XDG settings/portal watcher) for where
   `WindowAppearance` is cached and where the `color-scheme` `SettingChanged`
   signal is handled. Typical bug shapes: signal handler connected once /
   filtered by an equality check that compares against an un-updated cache, or
   the change is applied to a client-global but `appearance_changed` is only
   dispatched to one window.

2. **Per-window dispatch gap.** `appearance_changed` is per-window; if the
   portal change is only routed to the focused window and the focused
   window's cached appearance is already updated, the reverse dispatch may be
   suppressed. Verify all live windows receive the event on every change.

3. **(Secondary / defensive) `theme_settings` observer dedup desync.** The
   `observe_global::<SettingsStore>` closure caches `prev_theme_name` /
   `prev_icon_theme_name` at init and only updates them inside its own handler
   (`theme_settings/src/theme_settings.rs:~70-152`). Direct `reload_theme()`
   calls from the appearance path never refresh these caches, so the two reload
   paths can disagree. This does not by itself cause the OS-driven bug (that path
   bypasses the dedup), but it is a latent trap that should be made consistent so
   future changes do not reintroduce a swallow.

## Why prior fixes did not stick

There is **no automated coverage**: the test platform stubs the callback as a
no-op — `fn on_appearance_changed(&self, _callback) {}`
(`crates/gpui/src/platform/test/window.rs:293`) — and `TestPlatform` has no way
to simulate an appearance change. Every prior fix was hand-verified once and
silently regressed on a later rebase from upstream Zed. **A regression test is a
required deliverable, not optional.**

## Approach

### Step 1 — Reproduce & instrument (localize the break)
Add temporary `log::info!` at the four chain points above
(`appearance_changed`, the workspace closure, `configured_theme` after computing
`theme_name`, and `update_theme`). Toggle OS appearance light→dark→light and read
the log to find the exact step where the reverse transition diverges:
- Callback does **not** fire on reverse → bug is in the platform signal/portal
  watcher (Hypothesis 1/2).
- Callback fires but `window.appearance()` returns the stale value → platform
  appearance cache not updated (Hypothesis 1).
- Correct value reaches `configured_theme` but theme does not visibly change →
  investigate `refresh_windows`/render (unlikely given unconditional update).

### Step 2 — Fix at the localized point
Repair the identified break in the platform layer (Linux portal/window
appearance cache + dispatch). Keep the change behind no feature gate (this is
upstream gpui behaviour, not Helix-specific) and follow the repo's Rust
guidelines (no `unwrap`, log/propagate errors, no summary comments).

### Step 3 — Defensive consistency
Make the appearance-driven reload path and the `SettingsStore` observer share a
single source of truth for the "currently applied" theme/icon-theme name so the
dedup cache cannot desync (Hypothesis 3). Smallest viable change preferred.

### Step 4 — Regression test (required)
Give `TestPlatform`/`TestWindow` the ability to simulate an appearance change
(store the `on_appearance_changed` callback instead of dropping it, plus a
`set_appearance(WindowAppearance)` test helper that invokes it). Add a gpui/theme
test that:
1. Sets theme mode = `System` with distinct light/dark theme names.
2. Simulates light → asserts active theme == light theme.
3. Simulates dark → asserts active theme == dark theme.
4. Simulates light again → asserts active theme == light theme (the bug).
5. Confirms a `Static` theme is unaffected by the same simulated toggles.

## Files In Scope (full repo)

| File | Role |
|------|------|
| `crates/gpui/src/platform/linux/**` (portal/window appearance watcher) | **Primary fix site (Hyp. 1/2)** |
| `crates/gpui/src/window.rs` (`appearance_changed`, ~2274) | Dispatch point — verify, likely unchanged |
| `crates/workspace/src/workspace.rs` (~1781) | Appearance→theme bridge — verify, likely unchanged |
| `crates/theme_settings/src/theme_settings.rs` (~70-174) | Defensive dedup-consistency fix (Hyp. 3) |
| `crates/gpui/src/platform/test/{platform,window}.rs` | Test hooks for simulating appearance changes |
| theme/gpui test module | New regression test |

## Risks & Notes

- The platform appearance code is **not present in this stripped checkout**; the
  implementer must work in the full Zed repo. Do not assume the fix is in
  cross-platform code — the evidence says it is not.
- This is a Helix fork of Zed. Keep the change as close to upstream as possible
  to minimize future rebase friction; record any modified upstream files in
  `portingguide.md` per fork convention.
- Build/verify with `cargo build --features external_websocket_sync -p zed`; run
  the new test with the relevant crate's `cargo test`.
