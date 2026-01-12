//! GStreamer PipeWire source with zero-copy GPU buffer handling
//!
//! Captures PipeWire ScreenCast DMA-BUFs and converts them to CUDA buffers
//! using the proven code from gst-wayland-display.
//!
//! Supports multiple capture backends:
//! - PipeWire ScreenCast (GNOME via xdg-desktop-portal)
//! - wlr-screencopy (Sway/wlroots via native Wayland protocol)
//! - ext-image-copy-capture (Sway 1.10+ via modern Wayland protocol)

use gst::glib;

mod ext_image_copy_capture;
mod pipewire_stream;
mod pipewiresrc;
mod wlr_export_dmabuf;
mod wlr_screencopy;

fn plugin_init(plugin: &gst::Plugin) -> Result<(), glib::BoolError> {
    pipewiresrc::register(plugin)?;
    Ok(())
}

// Plugin name MUST match the library name (without 'lib' and 'gst' prefix)
// Library is 'libgstpipewirezerocopy.so' -> plugin name is 'pipewirezerocopy'
// The element name (pipewirezerocopysrc) is registered separately in pipewiresrc/mod.rs
gst::plugin_define!(
    pipewirezerocopy,
    env!("CARGO_PKG_DESCRIPTION"),
    plugin_init,
    concat!(env!("CARGO_PKG_VERSION"), "-", env!("COMMIT_ID")),
    "MIT",
    env!("CARGO_PKG_NAME"),
    env!("CARGO_PKG_NAME"),
    env!("CARGO_PKG_REPOSITORY"),
    env!("BUILD_REL_DATE")
);
