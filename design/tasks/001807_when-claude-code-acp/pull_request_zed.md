# Auto-approve ACP permission requests for Helix autonomous mode

## Summary
When running as a Helix spectask (with `external_websocket_sync` feature enabled), ACP permission requests — including plan mode exit confirmations — are auto-approved so sessions run autonomously without user interaction.

## Changes
- Add `#[cfg(feature = "external_websocket_sync")]` block in `ClientDelegate::request_permission()` (`crates/agent_servers/src/acp.rs`) that selects the first AllowOnce option and returns immediately, bypassing the UI permission flow
- Add `external_websocket_sync` feature to `crates/agent_servers/Cargo.toml`
- Propagate feature from `crates/zed/Cargo.toml`

Release Notes:

- N/A
