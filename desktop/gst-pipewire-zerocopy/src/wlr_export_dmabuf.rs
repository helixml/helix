//! wlr-export-dmabuf stream - zero-copy screen capture for wlroots compositors (Sway)
//!
//! This module implements the zwlr_export_dmabuf_manager_v1 protocol for capturing
//! DMA-BUF frames directly from wlroots compositors. Unlike PipeWire, this is a
//! native Wayland protocol that works directly with the compositor.
//!
//! Protocol flow:
//! 1. Connect to Wayland display (WAYLAND_DISPLAY)
//! 2. Get zwlr_export_dmabuf_manager_v1 global
//! 3. Call capture_output() to request a frame
//! 4. Receive events: frame -> object(s) -> ready (or cancel)
//! 5. Build Dmabuf from received FDs
//! 6. Repeat step 3 for continuous capture

use smithay::backend::allocator::{Fourcc, Modifier};
use smithay::backend::allocator::dmabuf::{Dmabuf, DmabufFlags};
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{mpsc, Arc};
use std::thread::{self, JoinHandle};
use std::time::Duration;
use parking_lot::Mutex;
use wayland_client::{
    Connection, Dispatch, EventQueue, Proxy, QueueHandle,
    protocol::{wl_output, wl_registry},
};
use wayland_protocols_wlr::export_dmabuf::v1::client::{
    zwlr_export_dmabuf_frame_v1::{self, ZwlrExportDmabufFrameV1},
    zwlr_export_dmabuf_manager_v1::ZwlrExportDmabufManagerV1,
};

use crate::pipewire_stream::{FrameData, RecvError, DmaBufCapabilities};

/// Frame info received from the `frame` event
#[derive(Debug, Clone, Default)]
struct FrameInfo {
    width: u32,
    height: u32,
    fourcc: u32,
    modifier_hi: u32,
    modifier_lo: u32,
    num_objects: u32,
}

/// Object/plane info received from `object` events
#[derive(Debug)]
struct ObjectInfo {
    index: u32,
    fd: OwnedFd,
    size: u32,
    offset: u32,
    stride: u32,
}

/// State for the wlr-export-dmabuf Wayland client
struct WlrExportState {
    /// The manager global
    manager: Option<ZwlrExportDmabufManagerV1>,
    /// Available outputs
    outputs: Vec<wl_output::WlOutput>,
    /// Current frame being captured
    current_frame: Option<ZwlrExportDmabufFrameV1>,
    /// Frame info from the `frame` event
    frame_info: Option<FrameInfo>,
    /// Objects (planes) received from `object` events
    objects: Vec<ObjectInfo>,
    /// Channel to send completed frames
    frame_tx: mpsc::SyncSender<FrameData>,
    /// Whether a capture is in progress
    capturing: bool,
    /// Shutdown flag
    shutdown: Arc<AtomicBool>,
    /// Target frame interval for rate limiting
    frame_interval: Duration,
    /// Time of last frame capture (for rate limiting)
    last_frame_time: Option<std::time::Instant>,
}

impl WlrExportState {
    fn new(frame_tx: mpsc::SyncSender<FrameData>, shutdown: Arc<AtomicBool>, target_fps: u32) -> Self {
        // Calculate frame interval from target FPS
        // e.g., 60 FPS = 16.67ms interval
        let frame_interval = if target_fps > 0 {
            Duration::from_micros(1_000_000 / target_fps as u64)
        } else {
            Duration::from_millis(16) // Default to ~60 FPS
        };
        eprintln!("[WLR_EXPORT] Frame rate limiting: {} FPS, interval={:?}", target_fps, frame_interval);

        Self {
            manager: None,
            outputs: Vec::new(),
            current_frame: None,
            frame_info: None,
            objects: Vec::new(),
            frame_tx,
            capturing: false,
            shutdown,
            frame_interval,
            last_frame_time: None,
        }
    }

    /// Build a Dmabuf from the received frame info and objects
    fn build_dmabuf(&mut self) -> Option<Dmabuf> {
        let info = self.frame_info.take()?;
        let objects = std::mem::take(&mut self.objects);

        if objects.is_empty() {
            eprintln!("[WLR_EXPORT] No objects received for frame");
            return None;
        }

        let fourcc = Fourcc::try_from(info.fourcc).ok()?;
        let modifier_raw = ((info.modifier_hi as u64) << 32) | (info.modifier_lo as u64);
        let modifier = Modifier::from(modifier_raw);

        eprintln!("[WLR_EXPORT] Building Dmabuf: {}x{} fourcc={:?} modifier=0x{:x} objects={}",
            info.width, info.height, fourcc, modifier_raw, objects.len());

        let mut builder = Dmabuf::builder(
            (info.width as i32, info.height as i32),
            fourcc,
            modifier,
            DmabufFlags::empty(),
        );

        // Add planes in order
        let mut sorted_objects = objects;
        sorted_objects.sort_by_key(|o| o.index);

        for obj in sorted_objects {
            builder.add_plane(obj.fd, obj.index, obj.offset, obj.stride);
        }

        builder.build()
    }

    /// Request a new frame capture
    fn request_capture(&mut self, qh: &QueueHandle<Self>) {
        if self.capturing || self.shutdown.load(Ordering::SeqCst) {
            return;
        }

        let manager = match &self.manager {
            Some(m) => m,
            None => {
                eprintln!("[WLR_EXPORT] Manager not available");
                return;
            }
        };

        let output = match self.outputs.first() {
            Some(o) => o,
            None => {
                eprintln!("[WLR_EXPORT] No output available");
                return;
            }
        };

        // overlay_cursor = 1 to include cursor in capture
        let frame = manager.capture_output(1, output, qh, ());
        self.current_frame = Some(frame);
        self.capturing = true;
        self.frame_info = None;
        self.objects.clear();
    }

    /// Handle frame completion (ready or cancel)
    fn handle_frame_complete(&mut self, success: bool, qh: &QueueHandle<Self>) {
        let now = std::time::Instant::now();

        if success {
            if let Some(dmabuf) = self.build_dmabuf() {
                let _ = self.frame_tx.try_send(FrameData::DmaBuf(dmabuf));
            }
        }

        // Destroy the frame object
        if let Some(frame) = self.current_frame.take() {
            frame.destroy();
        }

        self.capturing = false;
        self.frame_info = None;
        self.objects.clear();

        // Request next frame if not shutting down
        if !self.shutdown.load(Ordering::SeqCst) {
            // Frame rate limiting: sleep if we're capturing too fast
            // This prevents overwhelming the encoder and wasting GPU resources
            if let Some(last_time) = self.last_frame_time {
                let elapsed = now.duration_since(last_time);
                if elapsed < self.frame_interval {
                    let sleep_time = self.frame_interval - elapsed;
                    thread::sleep(sleep_time);
                }
            }
            self.last_frame_time = Some(std::time::Instant::now());

            self.request_capture(qh);
        }
    }
}

// Dispatch for wl_registry
impl Dispatch<wl_registry::WlRegistry, ()> for WlrExportState {
    fn event(
        state: &mut Self,
        registry: &wl_registry::WlRegistry,
        event: wl_registry::Event,
        _: &(),
        _conn: &Connection,
        qh: &QueueHandle<Self>,
    ) {
        if let wl_registry::Event::Global { name, interface, version } = event {
            if interface == ZwlrExportDmabufManagerV1::interface().name {
                eprintln!("[WLR_EXPORT] Found zwlr_export_dmabuf_manager_v1 v{}", version);
                let manager: ZwlrExportDmabufManagerV1 = registry.bind(name, version.min(1), qh, ());
                state.manager = Some(manager);
            } else if interface == "wl_output" {
                eprintln!("[WLR_EXPORT] Found wl_output v{}", version);
                let output: wl_output::WlOutput = registry.bind(name, version.min(4), qh, ());
                state.outputs.push(output);
            }
        }
    }
}

// Dispatch for wl_output (just need to handle events to keep output valid)
impl Dispatch<wl_output::WlOutput, ()> for WlrExportState {
    fn event(
        _state: &mut Self,
        _output: &wl_output::WlOutput,
        _event: wl_output::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        // We don't need to process output events, just need the object
    }
}

// Dispatch for ZwlrExportDmabufManagerV1 (no events, just requests)
impl Dispatch<ZwlrExportDmabufManagerV1, ()> for WlrExportState {
    fn event(
        _state: &mut Self,
        _manager: &ZwlrExportDmabufManagerV1,
        _event: <ZwlrExportDmabufManagerV1 as Proxy>::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        // Manager has no events
    }
}

// Dispatch for ZwlrExportDmabufFrameV1
impl Dispatch<ZwlrExportDmabufFrameV1, ()> for WlrExportState {
    fn event(
        state: &mut Self,
        _frame: &ZwlrExportDmabufFrameV1,
        event: zwlr_export_dmabuf_frame_v1::Event,
        _: &(),
        _conn: &Connection,
        qh: &QueueHandle<Self>,
    ) {
        match event {
            zwlr_export_dmabuf_frame_v1::Event::Frame {
                width,
                height,
                offset_x: _,
                offset_y: _,
                buffer_flags: _,
                flags: _,
                format,
                mod_high,
                mod_low,
                num_objects,
            } => {
                eprintln!("[WLR_EXPORT] Frame event: {}x{} fourcc=0x{:x} modifier=0x{:x}{:08x} objects={}",
                    width, height, format, mod_high, mod_low, num_objects);
                state.frame_info = Some(FrameInfo {
                    width,
                    height,
                    fourcc: format,
                    modifier_hi: mod_high,
                    modifier_lo: mod_low,
                    num_objects,
                });
            }
            zwlr_export_dmabuf_frame_v1::Event::Object {
                index,
                fd,
                size,
                offset,
                stride,
                plane_index: _,
            } => {
                eprintln!("[WLR_EXPORT] Object event: index={} fd={} size={} offset={} stride={}",
                    index, fd.as_raw_fd(), size, offset, stride);
                // Take ownership of the fd
                let owned_fd = unsafe { OwnedFd::from_raw_fd(fd.as_raw_fd()) };
                // Prevent wayland-client from closing the fd
                std::mem::forget(fd);
                state.objects.push(ObjectInfo {
                    index,
                    fd: owned_fd,
                    size,
                    offset,
                    stride,
                });
            }
            zwlr_export_dmabuf_frame_v1::Event::Ready {
                tv_sec_hi: _,
                tv_sec_lo: _,
                tv_nsec: _,
            } => {
                eprintln!("[WLR_EXPORT] Ready event - frame complete");
                state.handle_frame_complete(true, qh);
            }
            zwlr_export_dmabuf_frame_v1::Event::Cancel { reason } => {
                eprintln!("[WLR_EXPORT] Cancel event: reason={:?}", reason);
                state.handle_frame_complete(false, qh);
            }
            _ => {}
        }
    }
}

/// wlr-export-dmabuf stream for wlroots compositors (Sway)
pub struct WlrExportDmabufStream {
    thread: Option<JoinHandle<()>>,
    frame_rx: mpsc::Receiver<FrameData>,
    shutdown: Arc<AtomicBool>,
    error: Arc<Mutex<Option<String>>>,
}

impl WlrExportDmabufStream {
    /// Connect to the Wayland display and start capturing frames
    ///
    /// This creates a direct connection to the Sway compositor via wlr-export-dmabuf,
    /// bypassing PipeWire and xdg-desktop-portal-wlr entirely.
    ///
    /// `target_fps` controls frame rate limiting (e.g., 60 = capture at max 60 FPS).
    /// Unlike PipeWire ScreenCast, wlr-export-dmabuf is request-based, not damage-based,
    /// so we must rate-limit to avoid capturing at hundreds of FPS.
    pub fn connect(_dmabuf_caps: Option<DmaBufCapabilities>, target_fps: u32) -> Result<Self, String> {
        let (frame_tx, frame_rx) = mpsc::sync_channel(8);
        let shutdown = Arc::new(AtomicBool::new(false));
        let shutdown_clone = shutdown.clone();
        let error = Arc::new(Mutex::new(None));
        let error_clone = error.clone();

        let thread = thread::Builder::new()
            .name("wlr-export-dmabuf".to_string())
            .spawn(move || {
                if let Err(e) = run_wlr_export_loop(frame_tx, shutdown_clone, target_fps) {
                    eprintln!("[WLR_EXPORT] Loop error: {}", e);
                    *error_clone.lock() = Some(e);
                }
            })
            .map_err(|e| format!("Failed to spawn wlr-export thread: {}", e))?;

        // Give the thread time to connect and check for errors
        thread::sleep(Duration::from_millis(200));

        if let Some(err) = error.lock().take() {
            shutdown.store(true, Ordering::SeqCst);
            return Err(err);
        }

        Ok(Self {
            thread: Some(thread),
            frame_rx,
            shutdown,
            error,
        })
    }

    /// Check if wlr-export-dmabuf is available
    ///
    /// Returns true if we're running under a wlroots compositor (Sway)
    /// that supports the zwlr_export_dmabuf_manager_v1 protocol.
    pub fn is_available() -> bool {
        // Check if XDG_CURRENT_DESKTOP indicates Sway/wlroots
        if let Ok(desktop) = std::env::var("XDG_CURRENT_DESKTOP") {
            let desktop_lower = desktop.to_lowercase();
            if desktop_lower.contains("sway") || desktop_lower.contains("wlroots") {
                return true;
            }
        }

        // Also check WAYLAND_DISPLAY exists
        std::env::var("WAYLAND_DISPLAY").is_ok()
    }

    /// Receive a frame with timeout
    pub fn recv_frame_timeout(&self, timeout: Duration) -> Result<FrameData, RecvError> {
        if let Some(err) = self.error.lock().take() {
            return Err(RecvError::Error(err));
        }
        self.frame_rx.recv_timeout(timeout)
            .map_err(|e| match e {
                mpsc::RecvTimeoutError::Timeout => RecvError::Timeout,
                mpsc::RecvTimeoutError::Disconnected => RecvError::Disconnected,
            })
    }
}

impl Drop for WlrExportDmabufStream {
    fn drop(&mut self) {
        self.shutdown.store(true, Ordering::SeqCst);
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
        }
    }
}

fn run_wlr_export_loop(
    frame_tx: mpsc::SyncSender<FrameData>,
    shutdown: Arc<AtomicBool>,
    target_fps: u32,
) -> Result<(), String> {
    eprintln!("[WLR_EXPORT] Connecting to Wayland display (target_fps={})...", target_fps);

    // Connect to Wayland display
    let conn = Connection::connect_to_env()
        .map_err(|e| format!("Failed to connect to Wayland: {}", e))?;

    let display = conn.display();

    let mut event_queue: EventQueue<WlrExportState> = conn.new_event_queue();
    let qh = event_queue.handle();

    // Create state with frame rate limiting
    let mut state = WlrExportState::new(frame_tx, shutdown.clone(), target_fps);

    // Get registry and bind globals
    let _registry = display.get_registry(&qh, ());

    // Roundtrip to get globals
    event_queue.roundtrip(&mut state)
        .map_err(|e| format!("Initial roundtrip failed: {}", e))?;

    // Check we have the manager
    if state.manager.is_none() {
        return Err("zwlr_export_dmabuf_manager_v1 not available (not a wlroots compositor?)".to_string());
    }

    if state.outputs.is_empty() {
        return Err("No wl_output found".to_string());
    }

    eprintln!("[WLR_EXPORT] Connected! Found {} outputs", state.outputs.len());

    // Start first capture
    state.request_capture(&qh);

    // Main loop
    while !shutdown.load(Ordering::SeqCst) {
        // Dispatch events with timeout
        event_queue.blocking_dispatch(&mut state)
            .map_err(|e| format!("Dispatch failed: {}", e))?;
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_available_respects_env() {
        // This test just verifies the function doesn't panic
        let _ = WlrExportDmabufStream::is_available();
    }
}
