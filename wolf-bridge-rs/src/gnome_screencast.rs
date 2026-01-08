//! GNOME Mutter ScreenCast D-Bus API (headless mode)
//!
//! Uses org.gnome.Mutter.ScreenCast directly instead of XDG Portal.
//! This API supports RecordVirtual which doesn't require user interaction,
//! making it suitable for headless containers.
//!
//! D-Bus interfaces:
//! - org.gnome.Mutter.ScreenCast - Main screen-cast service
//! - org.gnome.Mutter.ScreenCast.Session - Screen-cast session
//! - org.gnome.Mutter.ScreenCast.Stream - Individual stream with PipeWire node

use anyhow::{Context, Result};
use tracing::{debug, info, warn};
use zbus::{blocking::Connection, dbus_proxy};

const MUTTER_BUS_NAME: &str = "org.gnome.Mutter.ScreenCast";
const MUTTER_OBJECT_PATH: &str = "/org/gnome/Mutter/ScreenCast";

/// Properties for RecordVirtual
#[derive(Debug, Clone)]
pub struct VirtualDisplayProperties {
    pub width: u32,
    pub height: u32,
    pub refresh_rate: f64,
}

impl Default for VirtualDisplayProperties {
    fn default() -> Self {
        Self {
            width: 1920,
            height: 1080,
            refresh_rate: 60.0,
        }
    }
}

/// GNOME screen-cast session (headless mode)
pub struct GnomeScreencast {
    connection: Connection,
    session_path: Option<String>,
    stream_path: Option<String>,
}

impl GnomeScreencast {
    /// Create a new GNOME screen-cast connection
    pub fn new() -> Result<Self> {
        debug!("Connecting to D-Bus session bus for GNOME ScreenCast...");
        let connection = Connection::session()
            .context("Failed to connect to D-Bus session bus")?;

        // Verify the service is available
        let reply: Result<(bool,), _> = connection.call_method(
            Some("org.freedesktop.DBus"),
            "/org/freedesktop/DBus",
            Some("org.freedesktop.DBus"),
            "NameHasOwner",
            &(MUTTER_BUS_NAME,),
        ).map(|r| r.body().deserialize().unwrap_or((false,)));

        match reply {
            Ok((true,)) => {
                info!("GNOME Mutter ScreenCast service is available");
            }
            _ => {
                anyhow::bail!(
                    "GNOME Mutter ScreenCast service not available. \
                     Is gnome-shell running with --headless?"
                );
            }
        }

        Ok(Self {
            connection,
            session_path: None,
            stream_path: None,
        })
    }

    /// Create a screen-cast session
    pub fn create_session(&mut self) -> Result<()> {
        debug!("Creating GNOME ScreenCast session...");

        // Call CreateSession with properties
        use std::collections::HashMap;
        let properties: HashMap<&str, zbus::zvariant::Value> = HashMap::new();

        let reply: zbus::Message = self.connection.call_method(
            Some(MUTTER_BUS_NAME),
            MUTTER_OBJECT_PATH,
            Some("org.gnome.Mutter.ScreenCast"),
            "CreateSession",
            &(properties,),
        ).context("Failed to call CreateSession")?;

        let session_path: zbus::zvariant::OwnedObjectPath = reply.body().deserialize()
            .context("Failed to parse CreateSession response")?;

        self.session_path = Some(session_path.to_string());
        info!("Created session: {}", session_path);

        Ok(())
    }

    /// Start recording a virtual display (headless mode - no user interaction!)
    pub fn record_virtual(&mut self, props: &VirtualDisplayProperties) -> Result<u32> {
        let session_path = self.session_path.as_ref()
            .context("No session created - call create_session first")?;

        debug!(
            "Recording virtual display: {}x{} @ {}Hz",
            props.width, props.height, props.refresh_rate
        );

        // Build properties for RecordVirtual
        use std::collections::HashMap;
        use zbus::zvariant::Value;

        let mut cursor_mode_props: HashMap<&str, Value> = HashMap::new();
        cursor_mode_props.insert("cursor-mode", Value::U32(1)); // 1 = embedded cursor
        // is-platform: treat as real monitor (may improve framerate), available since API v3
        cursor_mode_props.insert("is-platform", Value::Bool(true));

        let reply: zbus::Message = self.connection.call_method(
            Some(MUTTER_BUS_NAME),
            session_path.as_str(),
            Some("org.gnome.Mutter.ScreenCast.Session"),
            "RecordVirtual",
            &(cursor_mode_props,),
        ).context("Failed to call RecordVirtual")?;

        let stream_path: zbus::zvariant::OwnedObjectPath = reply.body().deserialize()
            .context("Failed to parse RecordVirtual response")?;

        self.stream_path = Some(stream_path.to_string());
        info!("Created virtual stream: {}", stream_path);

        // Get the PipeWire node ID from the stream
        let node_id = self.get_pipewire_node_id()?;
        info!("PipeWire node ID: {}", node_id);

        Ok(node_id)
    }

    /// Start recording a specific monitor (for non-headless with known monitor)
    pub fn record_monitor(&mut self, connector: &str) -> Result<u32> {
        let session_path = self.session_path.as_ref()
            .context("No session created - call create_session first")?;

        debug!("Recording monitor: {}", connector);

        use std::collections::HashMap;
        use zbus::zvariant::Value;

        let mut props: HashMap<&str, Value> = HashMap::new();
        props.insert("cursor-mode", Value::U32(1));
        // is-platform: treat as real monitor (may improve framerate), available since API v3
        props.insert("is-platform", Value::Bool(true));

        let reply: zbus::Message = self.connection.call_method(
            Some(MUTTER_BUS_NAME),
            session_path.as_str(),
            Some("org.gnome.Mutter.ScreenCast.Session"),
            "RecordMonitor",
            &(connector, props),
        ).context("Failed to call RecordMonitor")?;

        let stream_path: zbus::zvariant::OwnedObjectPath = reply.body().deserialize()
            .context("Failed to parse RecordMonitor response")?;

        self.stream_path = Some(stream_path.to_string());
        info!("Created monitor stream: {}", stream_path);

        let node_id = self.get_pipewire_node_id()?;
        Ok(node_id)
    }

    /// Get the PipeWire node ID from the stream
    fn get_pipewire_node_id(&self) -> Result<u32> {
        let stream_path = self.stream_path.as_ref()
            .context("No stream created")?;

        // Get the PipeWireNodeId property
        let reply: zbus::Message = self.connection.call_method(
            Some(MUTTER_BUS_NAME),
            stream_path.as_str(),
            Some("org.freedesktop.DBus.Properties"),
            "Get",
            &("org.gnome.Mutter.ScreenCast.Stream", "PipeWireNodeId"),
        ).context("Failed to get PipeWireNodeId property")?;

        let variant: zbus::zvariant::OwnedValue = reply.body().deserialize()
            .context("Failed to parse property response")?;

        let node_id: u32 = variant.downcast_ref::<u32>()
            .copied()
            .context("PipeWireNodeId is not u32")?;

        Ok(node_id)
    }

    /// Start the session (enables the stream)
    pub fn start(&self) -> Result<()> {
        let session_path = self.session_path.as_ref()
            .context("No session created")?;

        debug!("Starting session...");

        self.connection.call_method(
            Some(MUTTER_BUS_NAME),
            session_path.as_str(),
            Some("org.gnome.Mutter.ScreenCast.Session"),
            "Start",
            &(),
        ).context("Failed to call Start")?;

        info!("Session started");
        Ok(())
    }

    /// Stop the session
    pub fn stop(&self) -> Result<()> {
        if let Some(session_path) = &self.session_path {
            debug!("Stopping session...");

            let _ = self.connection.call_method(
                Some(MUTTER_BUS_NAME),
                session_path.as_str(),
                Some("org.gnome.Mutter.ScreenCast.Session"),
                "Stop",
                &(),
            );
        }
        Ok(())
    }
}

impl Drop for GnomeScreencast {
    fn drop(&mut self) {
        if let Err(e) = self.stop() {
            warn!("Failed to stop session on drop: {}", e);
        }
    }
}

/// High-level function to create a headless screen-cast and get PipeWire node ID
pub fn create_headless_screencast(width: u32, height: u32) -> Result<(GnomeScreencast, u32)> {
    let mut screencast = GnomeScreencast::new()?;
    screencast.create_session()?;

    let props = VirtualDisplayProperties {
        width,
        height,
        ..Default::default()
    };

    let node_id = screencast.record_virtual(&props)?;
    screencast.start()?;

    Ok((screencast, node_id))
}
