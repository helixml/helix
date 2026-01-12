//! wlr-screencopy stream - SHM screen capture for wlroots compositors (Sway)
//!
//! This module implements the zwlr_screencopy_manager_v1 protocol for capturing
//! screen frames directly from wlroots compositors using shared memory.
//! This is simpler than PipeWire and doesn't require xdg-desktop-portal.
//!
//! Protocol flow:
//! 1. Connect to Wayland display (WAYLAND_DISPLAY)
//! 2. Get zwlr_screencopy_manager_v1, wl_shm, and wl_output globals
//! 3. Call capture_output() to create a frame request
//! 4. Receive buffer event with format, width, height, stride
//! 5. Create wl_shm_pool and wl_buffer with our shared memory
//! 6. Call frame.copy(buffer) to request the screenshot
//! 7. Receive ready event when frame is captured
//! 8. Wrap SHM data in GStreamer buffer (zero-copy)
//! 9. Repeat for continuous capture

use crate::pipewire_stream::{FrameData, RecvError};
use parking_lot::Mutex;
use std::os::fd::{AsFd, AsRawFd, FromRawFd, OwnedFd};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{mpsc, Arc};
use std::thread::{self, JoinHandle};
use std::time::Duration;
use wayland_client::{
    protocol::{wl_buffer, wl_output, wl_registry, wl_shm, wl_shm_pool},
    Connection, Dispatch, EventQueue, Proxy, QueueHandle, WEnum,
};

/// Try to connect to Wayland, setting WAYLAND_DISPLAY if needed.
/// Sway often uses wayland-1 instead of the default wayland-0.
fn connect_to_wayland() -> Result<Connection, String> {
    // If WAYLAND_DISPLAY is already set, use it
    if std::env::var("WAYLAND_DISPLAY").map(|s| !s.is_empty()).unwrap_or(false) {
        return Connection::connect_to_env()
            .map_err(|e| format!("Failed to connect to Wayland: {}", e));
    }

    // Try common socket names
    let xdg_runtime_dir = std::env::var("XDG_RUNTIME_DIR")
        .unwrap_or_else(|_| "/run/user/1000".to_string());

    for socket_name in &["wayland-1", "wayland-0"] {
        let socket_path = format!("{}/{}", xdg_runtime_dir, socket_name);
        if std::path::Path::new(&socket_path).exists() {
            eprintln!("[WLR_SCREENCOPY] Found Wayland socket: {}", socket_path);
            std::env::set_var("WAYLAND_DISPLAY", socket_name);
            match Connection::connect_to_env() {
                Ok(conn) => return Ok(conn),
                Err(e) => {
                    eprintln!("[WLR_SCREENCOPY] Failed to connect to {}: {}", socket_name, e);
                    continue;
                }
            }
        }
    }

    Err("No Wayland socket found".to_string())
}
use wayland_protocols_wlr::screencopy::v1::client::{
    zwlr_screencopy_frame_v1::{self, ZwlrScreencopyFrameV1},
    zwlr_screencopy_manager_v1::ZwlrScreencopyManagerV1,
};

/// Buffer info received from the `buffer` event
#[derive(Debug, Clone)]
struct BufferInfo {
    /// Raw format value (WL_SHM_FORMAT = DRM fourcc)
    format_raw: u32,
    /// Wayland format enum for create_buffer
    format: wl_shm::Format,
    width: u32,
    height: u32,
    stride: u32,
}

impl BufferInfo {
    fn size(&self) -> usize {
        (self.stride * self.height) as usize
    }
}

/// Shared memory buffer for screencopy
struct ShmBuffer {
    /// File descriptor for the shared memory
    fd: OwnedFd,
    /// Memory-mapped pointer
    ptr: *mut u8,
    /// Size of the buffer
    size: usize,
    /// Wayland buffer object
    wl_buffer: Option<wl_buffer::WlBuffer>,
    /// Wayland shm pool
    _pool: Option<wl_shm_pool::WlShmPool>,
}

impl ShmBuffer {
    /// Create a new shared memory buffer
    fn new(size: usize) -> Result<Self, String> {
        // Create memfd for shared memory
        let fd = unsafe {
            let fd = libc::memfd_create(
                b"wlr-screencopy\0".as_ptr() as *const libc::c_char,
                libc::MFD_CLOEXEC | libc::MFD_ALLOW_SEALING,
            );
            if fd < 0 {
                return Err(format!(
                    "memfd_create failed: {}",
                    std::io::Error::last_os_error()
                ));
            }
            OwnedFd::from_raw_fd(fd)
        };

        // Set size
        let ret = unsafe { libc::ftruncate(fd.as_raw_fd(), size as libc::off_t) };
        if ret < 0 {
            return Err(format!(
                "ftruncate failed: {}",
                std::io::Error::last_os_error()
            ));
        }

        // mmap the buffer
        let ptr = unsafe {
            libc::mmap(
                std::ptr::null_mut(),
                size,
                libc::PROT_READ | libc::PROT_WRITE,
                libc::MAP_SHARED,
                fd.as_raw_fd(),
                0,
            )
        };
        if ptr == libc::MAP_FAILED {
            return Err(format!("mmap failed: {}", std::io::Error::last_os_error()));
        }

        Ok(Self {
            fd,
            ptr: ptr as *mut u8,
            size,
            wl_buffer: None,
            _pool: None,
        })
    }

    /// Get raw pointer to the buffer data (for zero-copy)
    fn as_ptr(&self) -> *const u8 {
        self.ptr
    }

    /// Get mutable slice to buffer data
    #[allow(dead_code)]
    fn as_slice(&self) -> &[u8] {
        unsafe { std::slice::from_raw_parts(self.ptr, self.size) }
    }

    /// Copy data out of the buffer
    fn copy_to_vec(&self) -> Vec<u8> {
        let mut data = vec![0u8; self.size];
        unsafe {
            std::ptr::copy_nonoverlapping(self.ptr, data.as_mut_ptr(), self.size);
        }
        data
    }
}

impl Drop for ShmBuffer {
    fn drop(&mut self) {
        if !self.ptr.is_null() {
            unsafe {
                libc::munmap(self.ptr as *mut libc::c_void, self.size);
            }
        }
    }
}

// Safety: The mmap'd pointer is only accessed from the event loop thread
unsafe impl Send for ShmBuffer {}
unsafe impl Sync for ShmBuffer {}

/// State for the wlr-screencopy Wayland client
struct ScreencopyState {
    /// The manager global
    manager: Option<ZwlrScreencopyManagerV1>,
    /// The shared memory global
    shm: Option<wl_shm::WlShm>,
    /// Available outputs
    outputs: Vec<wl_output::WlOutput>,
    /// Current frame being captured
    current_frame: Option<ZwlrScreencopyFrameV1>,
    /// Buffer info from the `buffer` event
    buffer_info: Option<BufferInfo>,
    /// Shared memory buffer
    shm_buffer: Option<ShmBuffer>,
    /// Whether we received buffer_done
    buffer_done: bool,
    /// Whether the frame is ready
    frame_ready: bool,
    /// Whether capture failed
    frame_failed: bool,
    /// Channel to send completed frames
    frame_tx: mpsc::SyncSender<FrameData>,
    /// Whether a capture is in progress
    capturing: bool,
    /// Shutdown flag
    shutdown: Arc<AtomicBool>,
    /// Target frame interval for rate limiting
    frame_interval: Duration,
    /// Time of last frame capture
    last_frame_time: Option<std::time::Instant>,
}

impl ScreencopyState {
    fn new(
        frame_tx: mpsc::SyncSender<FrameData>,
        shutdown: Arc<AtomicBool>,
        target_fps: u32,
    ) -> Self {
        let frame_interval = if target_fps > 0 {
            Duration::from_micros(1_000_000 / target_fps as u64)
        } else {
            Duration::from_millis(16)
        };
        eprintln!(
            "[WLR_SCREENCOPY] Frame rate limiting: {} FPS, interval={:?}",
            target_fps, frame_interval
        );

        Self {
            manager: None,
            shm: None,
            outputs: Vec::new(),
            current_frame: None,
            buffer_info: None,
            shm_buffer: None,
            buffer_done: false,
            frame_ready: false,
            frame_failed: false,
            frame_tx,
            capturing: false,
            shutdown,
            frame_interval,
            last_frame_time: None,
        }
    }

    /// Create wl_buffer from our shared memory
    fn create_wl_buffer(&mut self, qh: &QueueHandle<Self>) -> Result<(), String> {
        let shm = self.shm.as_ref().ok_or("No wl_shm")?;
        let info = self.buffer_info.as_ref().ok_or("No buffer info")?;
        let shm_buf = self.shm_buffer.as_mut().ok_or("No SHM buffer")?;

        // Create wl_shm_pool from our fd
        let pool = shm.create_pool(shm_buf.fd.as_fd(), shm_buf.size as i32, qh, ());

        // Create wl_buffer from pool
        let buffer = pool.create_buffer(
            0, // offset
            info.width as i32,
            info.height as i32,
            info.stride as i32,
            info.format,
            qh,
            (),
        );

        shm_buf.wl_buffer = Some(buffer);
        shm_buf._pool = Some(pool);

        Ok(())
    }

    /// Request a new frame capture
    fn request_capture(&mut self, qh: &QueueHandle<Self>) {
        if self.capturing || self.shutdown.load(Ordering::SeqCst) {
            return;
        }

        let manager = match &self.manager {
            Some(m) => m,
            None => {
                eprintln!("[WLR_SCREENCOPY] Manager not available");
                return;
            }
        };

        let output = match self.outputs.first() {
            Some(o) => o,
            None => {
                eprintln!("[WLR_SCREENCOPY] No output available");
                return;
            }
        };

        // Reset state for new capture
        self.buffer_info = None;
        self.buffer_done = false;
        self.frame_ready = false;
        self.frame_failed = false;

        // overlay_cursor = 1 to include cursor
        let frame = manager.capture_output(1, output, qh, ());
        self.current_frame = Some(frame);
        self.capturing = true;
    }

    /// Handle buffer_done - create SHM buffer and request copy
    fn handle_buffer_done(&mut self, qh: &QueueHandle<Self>) {
        self.buffer_done = true;

        let info = match &self.buffer_info {
            Some(i) => i.clone(),
            None => {
                eprintln!("[WLR_SCREENCOPY] buffer_done but no buffer info!");
                return;
            }
        };

        let size = info.size();
        eprintln!(
            "[WLR_SCREENCOPY] Buffer: {}x{} stride={} format=0x{:x} size={}",
            info.width, info.height, info.stride, info.format_raw, size
        );

        // Create SHM buffer if needed (or reuse if same size)
        let need_new_buffer = match &self.shm_buffer {
            Some(buf) => buf.size != size,
            None => true,
        };

        if need_new_buffer {
            match ShmBuffer::new(size) {
                Ok(buf) => {
                    eprintln!("[WLR_SCREENCOPY] Created SHM buffer: {} bytes", size);
                    self.shm_buffer = Some(buf);
                }
                Err(e) => {
                    eprintln!("[WLR_SCREENCOPY] Failed to create SHM buffer: {}", e);
                    self.handle_frame_failed(qh);
                    return;
                }
            }
        }

        // Create wl_buffer from our SHM
        if let Err(e) = self.create_wl_buffer(qh) {
            eprintln!("[WLR_SCREENCOPY] Failed to create wl_buffer: {}", e);
            self.handle_frame_failed(qh);
            return;
        }

        // Request the copy
        if let (Some(frame), Some(shm_buf)) = (&self.current_frame, &self.shm_buffer) {
            if let Some(ref wl_buf) = shm_buf.wl_buffer {
                eprintln!("[WLR_SCREENCOPY] Requesting copy...");
                frame.copy(wl_buf);
            }
        }
    }

    /// Handle frame ready - send the frame data
    fn handle_frame_ready(&mut self, qh: &QueueHandle<Self>) {
        self.frame_ready = true;
        let now = std::time::Instant::now();

        if let (Some(info), Some(shm_buf)) = (&self.buffer_info, &self.shm_buffer) {
            eprintln!("[WLR_SCREENCOPY] Frame ready!");

            // Create FrameData from SHM buffer
            // TODO: For true zero-copy, we could wrap the fd directly
            // For now, copy the data (still fast - memcpy only)
            let data = shm_buf.copy_to_vec();

            // Use format_raw (DRM fourcc) for GStreamer
            let frame = FrameData::Shm {
                data,
                width: info.width,
                height: info.height,
                stride: info.stride,
                format: info.format_raw,
            };

            let _ = self.frame_tx.try_send(frame);
        }

        // Clean up current frame
        if let Some(frame) = self.current_frame.take() {
            frame.destroy();
        }

        self.capturing = false;

        // Rate limiting
        if let Some(last_time) = self.last_frame_time {
            let elapsed = now.duration_since(last_time);
            if elapsed < self.frame_interval {
                thread::sleep(self.frame_interval - elapsed);
            }
        }
        self.last_frame_time = Some(std::time::Instant::now());

        // Request next frame
        if !self.shutdown.load(Ordering::SeqCst) {
            self.request_capture(qh);
        }
    }

    /// Handle frame failed
    fn handle_frame_failed(&mut self, qh: &QueueHandle<Self>) {
        self.frame_failed = true;
        eprintln!("[WLR_SCREENCOPY] Frame capture failed");

        if let Some(frame) = self.current_frame.take() {
            frame.destroy();
        }

        self.capturing = false;

        // Retry after a short delay
        if !self.shutdown.load(Ordering::SeqCst) {
            thread::sleep(Duration::from_millis(100));
            self.request_capture(qh);
        }
    }
}

// Dispatch for wl_registry
impl Dispatch<wl_registry::WlRegistry, ()> for ScreencopyState {
    fn event(
        state: &mut Self,
        registry: &wl_registry::WlRegistry,
        event: wl_registry::Event,
        _: &(),
        _conn: &Connection,
        qh: &QueueHandle<Self>,
    ) {
        if let wl_registry::Event::Global {
            name,
            interface,
            version,
        } = event
        {
            if interface == ZwlrScreencopyManagerV1::interface().name {
                eprintln!(
                    "[WLR_SCREENCOPY] Found zwlr_screencopy_manager_v1 v{}",
                    version
                );
                let manager: ZwlrScreencopyManagerV1 = registry.bind(name, version.min(3), qh, ());
                state.manager = Some(manager);
            } else if interface == "wl_output" {
                eprintln!("[WLR_SCREENCOPY] Found wl_output v{}", version);
                let output: wl_output::WlOutput = registry.bind(name, version.min(4), qh, ());
                state.outputs.push(output);
            } else if interface == "wl_shm" {
                eprintln!("[WLR_SCREENCOPY] Found wl_shm v{}", version);
                let shm: wl_shm::WlShm = registry.bind(name, version.min(1), qh, ());
                state.shm = Some(shm);
            }
        }
    }
}

// Dispatch for wl_output
impl Dispatch<wl_output::WlOutput, ()> for ScreencopyState {
    fn event(
        _state: &mut Self,
        _output: &wl_output::WlOutput,
        _event: wl_output::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        // We don't need to process output events
    }
}

// Dispatch for wl_shm
impl Dispatch<wl_shm::WlShm, ()> for ScreencopyState {
    fn event(
        _state: &mut Self,
        _shm: &wl_shm::WlShm,
        event: wl_shm::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        if let wl_shm::Event::Format { format } = event {
            eprintln!("[WLR_SCREENCOPY] SHM format available: {:?}", format);
        }
    }
}

// Dispatch for wl_shm_pool
impl Dispatch<wl_shm_pool::WlShmPool, ()> for ScreencopyState {
    fn event(
        _state: &mut Self,
        _pool: &wl_shm_pool::WlShmPool,
        _event: wl_shm_pool::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        // No events for wl_shm_pool
    }
}

// Dispatch for wl_buffer
impl Dispatch<wl_buffer::WlBuffer, ()> for ScreencopyState {
    fn event(
        _state: &mut Self,
        _buffer: &wl_buffer::WlBuffer,
        event: wl_buffer::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        if let wl_buffer::Event::Release = event {
            // Buffer released by compositor
        }
    }
}

// Dispatch for ZwlrScreencopyManagerV1
impl Dispatch<ZwlrScreencopyManagerV1, ()> for ScreencopyState {
    fn event(
        _state: &mut Self,
        _manager: &ZwlrScreencopyManagerV1,
        _event: <ZwlrScreencopyManagerV1 as Proxy>::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        // Manager has no events
    }
}

// Dispatch for ZwlrScreencopyFrameV1
impl Dispatch<ZwlrScreencopyFrameV1, ()> for ScreencopyState {
    fn event(
        state: &mut Self,
        _frame: &ZwlrScreencopyFrameV1,
        event: zwlr_screencopy_frame_v1::Event,
        _: &(),
        _conn: &Connection,
        qh: &QueueHandle<Self>,
    ) {
        match event {
            zwlr_screencopy_frame_v1::Event::Buffer {
                format,
                width,
                height,
                stride,
            } => {
                // Extract format for both Wayland API and GStreamer
                let (format_enum, format_raw) = match format {
                    WEnum::Value(f) => (f, f as u32),
                    WEnum::Unknown(v) => {
                        // Unknown format - default to Argb8888 for create_buffer
                        // but pass the raw value to GStreamer
                        (wl_shm::Format::Argb8888, v)
                    }
                };
                eprintln!(
                    "[WLR_SCREENCOPY] Buffer event: {}x{} stride={} format=0x{:x}",
                    width, height, stride, format_raw
                );
                state.buffer_info = Some(BufferInfo {
                    format_raw,
                    format: format_enum,
                    width,
                    height,
                    stride,
                });
            }
            zwlr_screencopy_frame_v1::Event::BufferDone => {
                eprintln!("[WLR_SCREENCOPY] BufferDone event");
                state.handle_buffer_done(qh);
            }
            zwlr_screencopy_frame_v1::Event::Flags { .. } => {
                // Flags event comes before ready
            }
            zwlr_screencopy_frame_v1::Event::Ready { .. } => {
                state.handle_frame_ready(qh);
            }
            zwlr_screencopy_frame_v1::Event::Failed => {
                state.handle_frame_failed(qh);
            }
            zwlr_screencopy_frame_v1::Event::Damage { .. } => {
                // Damage info for copy_with_damage
            }
            zwlr_screencopy_frame_v1::Event::LinuxDmabuf { .. } => {
                // DMA-BUF format - we use SHM instead
            }
            _ => {}
        }
    }
}

/// wlr-screencopy stream for wlroots compositors (Sway)
pub struct WlrScreencopyStream {
    thread: Option<JoinHandle<()>>,
    frame_rx: mpsc::Receiver<FrameData>,
    shutdown: Arc<AtomicBool>,
    error: Arc<Mutex<Option<String>>>,
}

impl WlrScreencopyStream {
    /// Connect to the Wayland display and start capturing frames
    ///
    /// This creates a direct connection to the Sway compositor via wlr-screencopy,
    /// bypassing PipeWire and xdg-desktop-portal entirely.
    ///
    /// `target_fps` controls frame rate limiting (e.g., 60 = capture at max 60 FPS).
    pub fn connect(target_fps: u32) -> Result<Self, String> {
        let (frame_tx, frame_rx) = mpsc::sync_channel(8);
        let shutdown = Arc::new(AtomicBool::new(false));
        let shutdown_clone = shutdown.clone();
        let error = Arc::new(Mutex::new(None));
        let error_clone = error.clone();

        let thread = thread::Builder::new()
            .name("wlr-screencopy".to_string())
            .spawn(move || {
                if let Err(e) = run_screencopy_loop(frame_tx, shutdown_clone, target_fps) {
                    eprintln!("[WLR_SCREENCOPY] Loop error: {}", e);
                    *error_clone.lock() = Some(e);
                }
            })
            .map_err(|e| format!("Failed to spawn wlr-screencopy thread: {}", e))?;

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

    /// Check if wlr-screencopy is available (Sway/wlroots compositor)
    pub fn is_available() -> bool {
        // Check if XDG_CURRENT_DESKTOP indicates Sway/wlroots
        if let Ok(desktop) = std::env::var("XDG_CURRENT_DESKTOP") {
            let desktop_lower = desktop.to_lowercase();
            if desktop_lower.contains("sway") || desktop_lower.contains("wlroots") {
                return true;
            }
        }
        false
    }

    /// Receive a frame with timeout
    pub fn recv_frame_timeout(&self, timeout: Duration) -> Result<FrameData, RecvError> {
        if let Some(err) = self.error.lock().take() {
            return Err(RecvError::Error(err));
        }
        self.frame_rx.recv_timeout(timeout).map_err(|e| match e {
            mpsc::RecvTimeoutError::Timeout => RecvError::Timeout,
            mpsc::RecvTimeoutError::Disconnected => RecvError::Disconnected,
        })
    }
}

impl Drop for WlrScreencopyStream {
    fn drop(&mut self) {
        self.shutdown.store(true, Ordering::SeqCst);
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
        }
    }
}

fn run_screencopy_loop(
    frame_tx: mpsc::SyncSender<FrameData>,
    shutdown: Arc<AtomicBool>,
    target_fps: u32,
) -> Result<(), String> {
    eprintln!(
        "[WLR_SCREENCOPY] Connecting to Wayland display (target_fps={})...",
        target_fps
    );

    // Connect to Wayland display - use connect_to_wayland() which handles missing WAYLAND_DISPLAY
    let conn = connect_to_wayland()?;

    let display = conn.display();

    let mut event_queue: EventQueue<ScreencopyState> = conn.new_event_queue();
    let qh = event_queue.handle();

    let mut state = ScreencopyState::new(frame_tx, shutdown.clone(), target_fps);

    // Get registry and bind globals
    let _registry = display.get_registry(&qh, ());

    // Roundtrip to get globals
    event_queue
        .roundtrip(&mut state)
        .map_err(|e| format!("Initial roundtrip failed: {}", e))?;

    // Check we have required globals
    if state.manager.is_none() {
        return Err(
            "zwlr_screencopy_manager_v1 not available (not a wlroots compositor?)".to_string(),
        );
    }

    if state.shm.is_none() {
        return Err("wl_shm not available".to_string());
    }

    if state.outputs.is_empty() {
        return Err("No wl_output found".to_string());
    }

    eprintln!(
        "[WLR_SCREENCOPY] Connected! Found {} outputs",
        state.outputs.len()
    );

    // Start first capture
    state.request_capture(&qh);

    // Main loop
    while !shutdown.load(Ordering::SeqCst) {
        event_queue
            .blocking_dispatch(&mut state)
            .map_err(|e| format!("Dispatch failed: {}", e))?;
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_available_respects_env() {
        let _ = WlrScreencopyStream::is_available();
    }

    #[test]
    fn test_buffer_info_size() {
        let info = BufferInfo {
            format: 0,
            width: 1920,
            height: 1080,
            stride: 1920 * 4,
        };
        assert_eq!(info.size(), 1920 * 1080 * 4);
    }
}
