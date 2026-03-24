# title_bar: Restore WebSocket sync status icon lost in rebase

## Summary

The Helix WebSocket connection status icon disappeared from the Zed title bar after the 2026-03-22 upstream rebase.

During the rebase, `#[cfg(feature = "external_websocket_sync")]` gates were added around the import and `render_helix_connection_status()` in `title_bar.rs`. However, `title_bar/Cargo.toml` never defined an `external_websocket_sync` feature (or made the dep optional), so the cfg gate always evaluated to `false`. Only the stub function (returning `None`) compiled — the icon never rendered.

## Changes

- `crates/title_bar/Cargo.toml`: make `external_websocket_sync` dep optional; add `external_websocket_sync = ["dep:external_websocket_sync"]` to `[features]`
- `crates/zed/Cargo.toml`: add `"title_bar/external_websocket_sync"` to the existing `external_websocket_sync` feature list so the feature activates when building with Helix support
- `portingguide.md`: document this requirement so it isn't lost in future rebases

## Release Notes

- N/A
