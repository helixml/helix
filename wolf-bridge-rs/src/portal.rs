//! XDG Desktop Portal screen-cast session
//!
//! Uses ashpd crate to interact with the portal, which works on:
//! - GNOME (via xdg-desktop-portal-gnome)
//! - KDE (via xdg-desktop-portal-kde)
//! - Sway/wlroots (via xdg-desktop-portal-wlr)

use anyhow::{Context, Result};
use ashpd::{
    desktop::screencast::{CursorMode, PersistMode, Screencast, SourceType},
    WindowIdentifier,
};
use tracing::{debug, info};

/// Active screen-cast session handle
pub struct ScreencastSession {
    // Session is kept alive by this handle
    // Dropping it will close the session
    _session: ashpd::desktop::Session<'static, Screencast<'static>>,
}

impl Drop for ScreencastSession {
    fn drop(&mut self) {
        info!("Screen-cast session closed");
    }
}

/// Create a screen-cast session and return the PipeWire node ID
pub async fn create_screencast_session() -> Result<(ScreencastSession, u32)> {
    debug!("Connecting to XDG Desktop Portal...");

    let proxy = Screencast::new()
        .await
        .context("Failed to connect to ScreenCast portal")?;

    // Create session
    debug!("Creating screen-cast session...");
    let session = proxy
        .create_session()
        .await
        .context("Failed to create screen-cast session")?;

    // Select sources (monitor or virtual display)
    debug!("Selecting sources...");
    proxy
        .select_sources(
            &session,
            CursorMode::Embedded,      // Include cursor in the stream
            SourceType::Monitor | SourceType::Virtual,  // Monitor or virtual display
            false,                      // Don't allow multiple sources
            None,                       // No restore token
            PersistMode::DoNot,        // Don't persist selection
        )
        .await
        .context("Failed to select screen-cast sources")?;

    // Start the session (this may show a user dialog)
    debug!("Starting screen-cast session...");
    let response = proxy
        .start(&session, &WindowIdentifier::default())
        .await
        .context("Failed to start screen-cast session")?;

    // Get the PipeWire node ID from the streams
    let streams = response.streams();
    if streams.is_empty() {
        anyhow::bail!("No streams returned from screen-cast");
    }

    let stream = &streams[0];
    let node_id = stream.pipe_wire_node_id();

    info!(
        "Screen-cast started: node_id={}, size={:?}",
        node_id,
        stream.size()
    );

    Ok((
        ScreencastSession { _session: session },
        node_id,
    ))
}
