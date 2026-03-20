# Design: Disable Zed Pro Advertisement on Startup

## Background

When the Zed agent panel is first opened, `AgentPanel::should_render_onboarding()` (`crates/agent_ui/src/agent_panel.rs:3051`) checks a persistent KVP flag (`dismissed-trial-upsell`, struct `OnboardingUpsell`). On a fresh install the flag is absent, so the method returns `true` and the `AgentPanelOnboarding` entity is rendered, showing the `ZedAiOnboarding` component — "Welcome to Zed AI" with a "Try Zed Pro for Free" / "Start Free Trial" call to action.

An earlier fix (`commit 2f74e89657`) used the same pattern to suppress a different startup banner (the settings migration banner). That fix is the template for this one.

## Root Cause

`on_boarding_upsell_dismissed` is initialised from `OnboardingUpsell::dismissed()` (line 1041). On a clean install the KVP key is absent, so this returns `false`. The code at line 870 sets the flag to `true` only when an external WebSocket thread is opened — so the ad is briefly (or permanently, if no thread is opened) visible before that happens.

## Fix

Add an early-return guard in `should_render_onboarding()` using `cfg!(feature = "external_websocket_sync")`, matching the pattern used for the migration banner:

```rust
fn should_render_onboarding(&self, cx: &mut Context<Self>) -> bool {
    // Helix manages AI via the WebSocket sync layer; don't show Zed Pro ads.
    if cfg!(feature = "external_websocket_sync") {
        return false;
    }
    // ... existing logic ...
}
```

**File to change:** `crates/agent_ui/src/agent_panel.rs`, function `should_render_onboarding` (~line 3051).

## Decision: cfg! guard vs. initialising flag to true

A `cfg!` guard is preferred because:
- It is unconditional and cannot be undone by a stale KVP entry from a previous non-Helix build
- It follows the established project pattern (`migrate.rs`)
- No persistent state is written unnecessarily

## Codebase patterns discovered

- All Helix-specific suppressions use `cfg!(feature = "external_websocket_sync")` (not `#[cfg(...)]` attributes on whole functions, to keep diff minimal)
- Modified upstream files are tracked in `portingguide.md`
- PR titles must be imperative, no conventional-commit prefixes, with a `Release Notes:` section
