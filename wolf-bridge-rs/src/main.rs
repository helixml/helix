//! Wolf Bridge - Desktop screen-cast to Wolf's Wayland compositor
//!
//! Bridges PipeWire screen-cast streams from GNOME, KDE, or Sway
//! to Wolf's Wayland compositor using zero-copy DMA-BUF when available.
//!
//! Operating modes:
//! - Headless (default): Uses direct GNOME D-Bus API (RecordVirtual)
//! - Interactive: Uses XDG Desktop Portal (shows user dialog)

mod gnome_screencast;
mod pipewire_stream;
mod portal;
mod wayland;

use anyhow::Result;
use clap::Parser;
use tracing::{info, warn, Level};
use tracing_subscriber::FmtSubscriber;

/// Screen-cast session handle (either GNOME direct or Portal)
enum ScreencastSession {
    Gnome(gnome_screencast::GnomeScreencast),
    Portal(portal::ScreencastSession),
}

#[derive(Parser, Debug)]
#[command(name = "wolf-bridge")]
#[command(about = "Bridge desktop screen-cast to Wolf's Wayland compositor")]
struct Args {
    /// Wayland display to connect to
    #[arg(short, long, default_value = "wayland-1")]
    display: String,

    /// Display width
    #[arg(short = 'W', long, default_value = "1920")]
    width: u32,

    /// Display height
    #[arg(short = 'H', long, default_value = "1080")]
    height: u32,

    /// Use interactive mode (XDG Portal with user dialog)
    /// Default is headless mode using direct GNOME D-Bus API
    #[arg(long)]
    interactive: bool,

    /// Desktop environment to use for headless mode
    /// Options: gnome, kde (auto-detected if not specified)
    #[arg(long)]
    desktop: Option<String>,

    /// Enable verbose logging
    #[arg(short, long)]
    verbose: bool,
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    // Initialize logging
    let level = if args.verbose { Level::DEBUG } else { Level::INFO };
    FmtSubscriber::builder()
        .with_max_level(level)
        .with_target(false)
        .init();

    info!("Wolf Bridge starting...");
    info!(
        "Display: {}, Resolution: {}x{}, Mode: {}",
        args.display,
        args.width,
        args.height,
        if args.interactive { "interactive (Portal)" } else { "headless (direct D-Bus)" }
    );

    // Step 1: Connect to Wolf's Wayland compositor
    info!("Connecting to Wolf Wayland display...");
    let mut wayland = wayland::WaylandClient::connect(&args.display)?;
    info!(
        "Connected to Wayland, DMA-BUF supported: {}",
        wayland.has_dmabuf()
    );

    // Step 2: Create screen-cast session
    let (session, node_id) = if args.interactive {
        // Interactive mode: Use XDG Portal (shows user dialog)
        info!("Creating screen-cast session via XDG Desktop Portal...");
        info!("NOTE: This will show a dialog asking to select a screen");
        let (session, node_id) = portal::create_screencast_session().await?;
        (ScreencastSession::Portal(session), node_id)
    } else {
        // Headless mode: Use direct GNOME D-Bus API (no user interaction)
        info!("Creating screen-cast session via GNOME Mutter ScreenCast...");
        let (session, node_id) =
            gnome_screencast::create_headless_screencast(args.width, args.height)?;
        (ScreencastSession::Gnome(session), node_id)
    };
    info!("Screen-cast session created, PipeWire node: {}", node_id);

    // Step 3: Connect to PipeWire stream
    info!("Connecting to PipeWire stream...");
    let stream = pipewire_stream::PipeWireStream::connect(node_id, args.width, args.height)?;

    // Step 4: Main event loop - forward frames from PipeWire to Wolf
    info!("Entering main loop...");
    run_event_loop(&mut wayland, stream).await?;

    // Cleanup
    info!("Shutting down...");
    drop(session);

    Ok(())
}

async fn run_event_loop(
    wayland: &mut wayland::WaylandClient,
    mut stream: pipewire_stream::PipeWireStream,
) -> Result<()> {
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::sync::Arc;

    let running = Arc::new(AtomicBool::new(true));
    let r = running.clone();

    ctrlc::set_handler(move || {
        r.store(false, Ordering::SeqCst);
    })?;

    let mut frame_count: u64 = 0;
    let mut dmabuf_count: u64 = 0;
    let mut shm_count: u64 = 0;

    while running.load(Ordering::SeqCst) {
        // Process Wayland events
        wayland.dispatch()?;

        // Get next frame from PipeWire
        if let Some(frame) = stream.try_dequeue_frame()? {
            frame_count += 1;

            match frame {
                pipewire_stream::Frame::DmaBuf {
                    fd,
                    width,
                    height,
                    stride,
                    format,
                    modifier,
                } => {
                    dmabuf_count += 1;
                    wayland.submit_dmabuf(fd, width, height, stride, format, modifier)?;
                }
                pipewire_stream::Frame::Shm {
                    data,
                    width,
                    height,
                    stride,
                    format,
                } => {
                    shm_count += 1;
                    wayland.submit_shm(&data, width, height, stride, format)?;
                }
            }

            // Log stats every 300 frames (~5 seconds at 60fps)
            if frame_count % 300 == 0 {
                info!(
                    "Frames: {} total, {} DMA-BUF (zero-copy), {} SHM (fallback)",
                    frame_count, dmabuf_count, shm_count
                );
            }
        }

        // Small sleep to prevent busy-waiting
        tokio::time::sleep(std::time::Duration::from_micros(100)).await;
    }

    Ok(())
}
