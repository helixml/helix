# Implementation Tasks

- [~] In `crates/title_bar/Cargo.toml`: change `external_websocket_sync.workspace = true` to `external_websocket_sync = { workspace = true, optional = true }` and add `external_websocket_sync = ["dep:external_websocket_sync"]` to the `[features]` section
- [~] In `crates/zed/Cargo.toml`: add `"title_bar/external_websocket_sync"` to the `external_websocket_sync` feature list
- [ ] Build with `cargo build --features external_websocket_sync -p zed` and verify it compiles without errors
- [ ] Update `portingguide.md` `crates/title_bar/` section to document that `external_websocket_sync` must be optional with a feature defined and propagated from `crates/zed/Cargo.toml`
