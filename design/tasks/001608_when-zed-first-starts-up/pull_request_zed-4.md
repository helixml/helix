# agent_ui: Suppress Zed Pro upsell in Helix builds

## Summary

On a fresh Helix install the `dismissed-trial-upsell` KVP key is absent, so `should_render_onboarding()` returns `true` and the "Welcome to Zed AI" / Zed Pro advertisement renders in the agent panel sidebar. In Helix builds, AI is configured centrally via the settings-sync-daemon, so this advertisement is irrelevant and confusing.

## Changes

- Added a `cfg!(feature = "external_websocket_sync")` early-return guard at the top of `AgentPanel::should_render_onboarding()` in `crates/agent_ui/src/agent_panel.rs`, matching the pattern used for the migration banner suppression (`commit 2f74e89657`)

Release Notes:

- Fixed Zed Pro advertisement appearing in the agent panel on first startup in Helix builds
