# Requirements: Fix Zed Theme Not Reverting on OS Dark/Light Mode Toggle

## Background

Zed (this Helix fork) follows the OS light/dark appearance when the user's theme
is configured in `System` mode. When the OS appearance changes, Zed should swap
between the configured light theme and dark theme automatically.

Today the swap works on the **first** transition (e.g. light → dark) but does
**not** swap back on the **reverse** transition (dark → light). The theme stays
stuck on the first-switched appearance until Zed is restarted. This has been
attempted multiple times without a durable fix, partly because there is no
regression test covering OS appearance changes (the test platform stubs the
appearance-change callback as a no-op).

## User Stories

### US-1: Theme follows OS appearance in both directions
As a user with my theme set to `System` mode,
I want Zed to switch themes every time my OS toggles between light and dark,
so that Zed always matches my system appearance no matter how many times I toggle.

### US-2: Reliable repeated toggling
As a user,
I want light→dark→light→dark (and back) to keep working indefinitely within a
single Zed session,
so that I never have to restart Zed to recover correct theming.

### US-3: Icon theme follows too
As a user,
I want the icon theme (when set to `System`/dynamic mode) to follow the same
appearance changes as the color theme, in both directions.

### US-4: Protected by a regression test
As a maintainer,
I want an automated test that simulates repeated OS appearance changes,
so that this bug cannot silently regress again on future rebases from upstream.

## Acceptance Criteria

- [ ] With theme mode = `System`: toggling OS appearance light → dark switches Zed
      to the configured dark theme.
- [ ] Toggling OS appearance dark → light **then** switches Zed back to the
      configured light theme (the bug under repair).
- [ ] Repeating the toggle ≥ 3 times in a row continues to switch correctly each
      time, with no restart required.
- [ ] The icon theme follows the same transitions when configured as dynamic
      `System` mode.
- [ ] Themes explicitly set to `Static` (not `System`) are unaffected — they do
      not change when the OS appearance changes.
- [ ] A regression test simulates ≥ 2 full appearance transitions (light→dark→light)
      and asserts the active theme is correct after each one.
- [ ] No regression to startup theme selection (the initial appearance is still
      respected on launch).

## Out of Scope

- Adding new theme-selection UI or settings.
- Changing the default light/dark theme names.
- Reworking the theme registry or theme override system.
