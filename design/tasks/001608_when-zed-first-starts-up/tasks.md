# Implementation Tasks

- [ ] In `crates/agent_ui/src/agent_panel.rs`, add `if cfg!(feature = "external_websocket_sync") { return false; }` at the top of `should_render_onboarding()` (around line 3051), with a brief comment explaining why
- [ ] Open a PR against the `helix-4` repo with an imperative title (e.g. `agent_ui: Suppress Zed Pro upsell in Helix builds`) and a `Release Notes:` section (`- Fixed Zed Pro advertisement appearing on first startup in Helix builds`)
