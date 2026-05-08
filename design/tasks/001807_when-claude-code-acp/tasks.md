# Implementation Tasks

- [x] Add `#[cfg(feature = "external_websocket_sync")]` auto-approve block at top of `ClientDelegate::request_permission()` in `crates/agent_servers/src/acp.rs` (~line 1448) — find first AllowOnce/AllowAlways option and return immediately
- [x] Add `external_websocket_sync` feature to `crates/agent_servers/Cargo.toml` and propagate from `crates/zed/Cargo.toml`
- [x] Verify build compiles (no Rust toolchain available in this env — deferred to CI)
- [x] Run existing tests (deferred to CI)
- [x] Create PR description and push code
