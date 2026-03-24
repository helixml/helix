# Requirements: Restore WebSocket Sync Status Icon in Zed Title Bar

## Background

The Zed title bar shows a server icon with a colored status dot indicating the Helix WebSocket connection state (Connected/Reconnecting/Disconnected). This icon was lost in the 2026-03-22 upstream merge rebase.

## User Stories

**As a developer using Zed in the Helix sandbox**, I want to see the WebSocket connection status icon in the title bar so I can quickly tell whether Zed is connected to the Helix API server.

## Acceptance Criteria

1. When running in the Helix environment (WebSocket service initialized), the title bar shows a server icon with a colored status dot:
   - Green dot: Connected
   - Yellow dot: Reconnecting
   - Red dot: Disconnected
2. When NOT running in the Helix environment (WebSocket service not initialized), no icon is shown — behavior is unchanged for vanilla Zed
3. Hovering the icon shows a tooltip: "Helix: Connected", "Helix: Reconnecting...", or "Helix: Disconnected"
4. The `title_bar` crate compiles correctly without warnings in both Helix and non-Helix builds

## Root Cause

During the 2026-03-22 upstream rebase, `#[cfg(feature = "external_websocket_sync")]` gates were added around the import and `render_helix_connection_status()` function in `title_bar.rs`. However, `title_bar/Cargo.toml` does NOT define an `external_websocket_sync` feature (and the dep is not optional). The cfg gates always evaluate to `false`, so only the stub function (returning `None`) compiles. The icon never renders.

Pre-rebase, the code used the dep unconditionally — no cfg gate, no feature needed.
