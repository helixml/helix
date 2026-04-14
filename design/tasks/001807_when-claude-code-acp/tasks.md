# Implementation Tasks

- [~] Add `#[cfg(feature = "external_websocket_sync")]` auto-approve block at top of `ClientDelegate::request_permission()` in `crates/agent_servers/src/acp.rs` (~line 1448) — find first AllowOnce/AllowAlways option and return immediately
- [ ] Verify build compiles with `cargo build --features external_websocket_sync -p zed`
- [ ] Verify build compiles without the feature: `cargo build -p zed` (ensure normal Zed is unaffected)
- [ ] Run existing tests: `cargo test -p agent_servers` and `cargo test -p acp_thread`
