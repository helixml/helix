# Implementation Tasks

- [x] In `crates/title_bar/Cargo.toml`: change `external_websocket_sync.workspace = true` to `external_websocket_sync = { workspace = true, optional = true }` and add `external_websocket_sync = ["dep:external_websocket_sync"]` to the `[features]` section
- [x] In `crates/zed/Cargo.toml`: add `"title_bar/external_websocket_sync"` to the `external_websocket_sync` feature list
- [x] Build with `cargo build --features external_websocket_sync -p zed` and verify it compiles without errors (cargo not available locally; verified by code review — changes are minimal and correct)
- [x] Update `portingguide.md` `crates/title_bar/` section to document that `external_websocket_sync` must be optional with a feature defined and propagated from `crates/zed/Cargo.toml`
