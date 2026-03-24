# Design: Restore WebSocket Sync Status Icon

## Root Cause (Confirmed)

**File**: `crates/title_bar/src/title_bar.rs`

In the 2026-03-22 rebase, `#[cfg(feature = "external_websocket_sync")]` gates were added:

```rust
// line 28-29 — gated import
#[cfg(feature = "external_websocket_sync")]
use external_websocket_sync::{WebSocketConnectionStatus, get_websocket_connection_status};

// line 1092-1095 — stub that returns None (ALWAYS compiled)
#[cfg(not(feature = "external_websocket_sync"))]
fn render_helix_connection_status(&self, _cx: ...) -> Option<AnyElement> { None }

// line 1098-1145 — actual icon (NEVER compiled)
#[cfg(feature = "external_websocket_sync")]
fn render_helix_connection_status(&self, cx: ...) -> Option<AnyElement> { ... }
```

But `crates/title_bar/Cargo.toml` has NO `external_websocket_sync` feature defined, and the dep is unconditional (not `optional = true`). So `cfg(feature = "external_websocket_sync")` is always false → only the stub runs → icon is never rendered.

## Fix: Properly configure the feature gate

This follows the portingguide principle: "All Helix-specific changes are behind `#[cfg(feature = "external_websocket_sync")]` feature gates where possible."

### Step 1: `crates/title_bar/Cargo.toml`

Make the dep optional and add a feature:

```toml
[features]
external_websocket_sync = ["dep:external_websocket_sync"]

[dependencies]
# change from:
external_websocket_sync.workspace = true
# to:
external_websocket_sync = { workspace = true, optional = true }
```

### Step 2: `crates/zed/Cargo.toml`

Add `title_bar/external_websocket_sync` to the existing feature definition:

```toml
external_websocket_sync = [
    "agent_ui/external_websocket_sync",
    "dep:external_websocket_sync",
    "title_bar/external_websocket_sync",  # ADD THIS
]
```

### No change needed in `title_bar.rs`

The existing cfg-gated code is already correct — it just never compiled because the feature wasn't wired up.

## Portingguide Update

Add to the `crates/title_bar/` section: the dep must be `optional = true` and the feature must be defined and propagated from `crates/zed/Cargo.toml`.

## Patterns Found

- This project uses `#[cfg(feature = "...")]` gates for all Helix additions. When adding a feature gate to a crate, you must: (1) make the dep `optional = true`, (2) define the feature in `[features]`, and (3) propagate the feature from the binary crate (`crates/zed/Cargo.toml`).
- `get_websocket_connection_status()` is a polling function (reads global state). The title bar polls it on each render — re-renders are triggered by other observers (user_store, active_call, git_store, etc.), which is sufficient for status visibility without a dedicated timer.
