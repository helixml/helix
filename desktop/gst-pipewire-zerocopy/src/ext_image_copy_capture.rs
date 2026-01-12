//! ext-image-copy-capture-v1 stream - Modern screen capture for wlroots/Sway 1.10+
//!
//! This module implements the ext-image-copy-capture-v1 protocol, which is the
//! standardized replacement for wlr-screencopy-unstable-v1. It provides:
//! - Damage tracking (only send changed regions)
//! - Session-based capture (persistent sessions with better state management)
//! - Cross-compositor support (Sway 1.10+, KDE Plasma 6.2+, COSMIC, etc.)
//!
//! Protocol flow:
//! 1. Bind ext_image_capture_source_manager_v1 and ext_image_copy_capture_manager_v1
//! 2. Create output source via source_manager.create_output_source(output)
//! 3. Create session via capture_manager.create_session(source)
//! 4. Receive format events from session (buffer formats supported)
//! 5. Call session.create_frame() to request a frame capture
//! 6. Receive buffer events from frame (size, format requirements)
//! 7. Create SHM buffer and call frame.attach_buffer(buffer)
//! 8. Call frame.capture() to start capture
//! 9. Receive ready event when capture completes
//! 10. Repeat from step 5 for continuous capture

use crate::pipewire_stream::{FrameData, RecvError};
use parking_lot::Mutex;
use std::os::fd::{AsFd, AsRawFd, BorrowedFd, FromRawFd, OwnedFd};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{mpsc, Arc};
use std::thread::{self, JoinHandle};
use std::time::Duration;
use wayland_client::{
    protocol::{wl_buffer, wl_output, wl_registry, wl_shm, wl_shm_pool},
    Connection, Dispatch, EventQueue, Proxy, QueueHandle, WEnum,
};
use wayland_protocols::ext::image_capture_source::v1::client::{
    ext_image_capture_source_v1::ExtImageCaptureSourceV1,
    ext_output_image_capture_source_manager_v1::ExtOutputImageCaptureSourceManagerV1,
};
use wayland_protocols::ext::image_copy_capture::v1::client::{
    ext_image_copy_capture_frame_v1::{self, ExtImageCopyCaptureFrameV1},
    ext_image_copy_capture_manager_v1::{self, ExtImageCopyCaptureManagerV1},
    ext_image_copy_capture_session_v1::{self, ExtImageCopyCaptureSessionV1},
};

/// Supported buffer format from session's shm_format event
#[derive(Debug, Clone)]
struct ShmFormat {
    format: wl_shm::Format,
    format_raw: u32,
}

/// Buffer requirements from frame's buffer_size event
#[derive(Debug, Clone)]
struct BufferSize {
    width: u32,
    height: u32,
}

/// Shared memory buffer for screen capture
struct ShmBuffer {
    fd: OwnedFd,
    ptr: *mut u8,
    size: usize,
    wl_buffer: Option<wl_buffer::WlBuffer>,
    _pool: Option<wl_shm_pool::WlShmPool>,
}

impl ShmBuffer {
    fn new(size: usize) -> Result<Self, String> {
        let fd = unsafe {
            let fd = libc::memfd_create(
                b"ext-image-copy\0".as_ptr() as *const libc::c_char,
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

        let ret = unsafe { libc::ftruncate(fd.as_raw_fd(), size as libc::off_t) };
        if ret < 0 {
            return Err(format!(
                "ftruncate failed: {}",
                std::io::Error::last_os_error()
            ));
        }

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

// Safety: mmap'd pointer is only accessed from event loop thread
unsafe impl Send for ShmBuffer {}
unsafe impl Sync for ShmBuffer {}

/// State for ext-image-copy-capture Wayland client
struct ExtCaptureState {
    // Wayland globals
    capture_manager: Option<ExtImageCopyCaptureManagerV1>,
    source_manager: Option<ExtOutputImageCaptureSourceManagerV1>,
    shm: Option<wl_shm::WlShm>,
    outputs: Vec<wl_output::WlOutput>,

    // Capture state
    source: Option<ExtImageCaptureSourceV1>,
    session: Option<ExtImageCopyCaptureSessionV1>,
    current_frame: Option<ExtImageCopyCaptureFrameV1>,

    // Format negotiation
    shm_formats: Vec<ShmFormat>,
    selected_format: Option<ShmFormat>,
    buffer_size: Option<BufferSize>,

    // Buffer management
    shm_buffer: Option<ShmBuffer>,

    // Frame events
    session_stopped: bool,
    frame_ready: bool,
    frame_failed: bool,
    buffer_size_received: bool,

    // Communication
    frame_tx: mpsc::SyncSender<FrameData>,
    capturing: bool,
    shutdown: Arc<AtomicBool>,
    frame_interval: Duration,
    last_frame_time: Option<std::time::Instant>,
}

impl ExtCaptureState {
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
            "[EXT_IMAGE_COPY] Frame rate limiting: {} FPS, interval={:?}",
            target_fps, frame_interval
        );

        Self {
            capture_manager: None,
            source_manager: None,
            shm: None,
            outputs: Vec::new(),
            source: None,
            session: None,
            current_frame: None,
            shm_formats: Vec::new(),
            selected_format: None,
            buffer_size: None,
            shm_buffer: None,
            session_stopped: false,
            frame_ready: false,
            frame_failed: false,
            buffer_size_received: false,
            frame_tx,
            capturing: false,
            shutdown,
            frame_interval,
            last_frame_time: None,
        }
    }

    /// Initialize capture source and session
    fn init_session(&mut self, qh: &QueueHandle<Self>) -> Result<(), String> {
        let source_manager = self
            .source_manager
            .as_ref()
            .ok_or("No source manager")?;
        let capture_manager = self
            .capture_manager
            .as_ref()
            .ok_or("No capture manager")?;
        let output = self.outputs.first().ok_or("No output")?;

        // Create source from first output
        let source = source_manager.create_source(output, qh, ());
        eprintln!("[EXT_IMAGE_COPY] Created capture source");

        // Create capture session with paint_cursors to include mouse cursor
        let options = ext_image_copy_capture_manager_v1::Options::PaintCursors;
        let session = capture_manager.create_session(&source, options, qh, ());
        eprintln!("[EXT_IMAGE_COPY] Created capture session");

        self.source = Some(source);
        self.session = Some(session);

        Ok(())
    }

    /// Select best format from available SHM formats
    fn select_format(&mut self) {
        // Prefer XRGB8888/ARGB8888 for best compatibility
        let preferred = [
            wl_shm::Format::Xrgb8888,
            wl_shm::Format::Argb8888,
            wl_shm::Format::Xbgr8888,
            wl_shm::Format::Abgr8888,
        ];

        for pref in &preferred {
            if let Some(fmt) = self.shm_formats.iter().find(|f| f.format == *pref) {
                eprintln!("[EXT_IMAGE_COPY] Selected format: {:?}", fmt.format);
                self.selected_format = Some(fmt.clone());
                return;
            }
        }

        // Fall back to first available format
        if let Some(fmt) = self.shm_formats.first() {
            eprintln!(
                "[EXT_IMAGE_COPY] Using first available format: {:?}",
                fmt.format
            );
            self.selected_format = Some(fmt.clone());
        }
    }

    /// Create wl_buffer from SHM
    fn create_wl_buffer(&mut self, qh: &QueueHandle<Self>) -> Result<(), String> {
        let shm = self.shm.as_ref().ok_or("No wl_shm")?;
        let format = self.selected_format.as_ref().ok_or("No format selected")?;
        let size = self.buffer_size.as_ref().ok_or("No buffer size")?;
        let shm_buf = self.shm_buffer.as_mut().ok_or("No SHM buffer")?;

        // Stride = width * 4 bytes per pixel (RGBA/XRGB)
        let stride = size.width * 4;

        let pool = shm.create_pool(shm_buf.fd.as_fd(), shm_buf.size as i32, qh, ());

        let buffer = pool.create_buffer(
            0, // offset
            size.width as i32,
            size.height as i32,
            stride as i32,
            format.format,
            qh,
            (),
        );

        shm_buf.wl_buffer = Some(buffer);
        shm_buf._pool = Some(pool);

        Ok(())
    }

    /// Request a new frame capture
    fn request_capture(&mut self, qh: &QueueHandle<Self>) {
        if self.capturing || self.shutdown.load(Ordering::SeqCst) || self.session_stopped {
            return;
        }

        let session = match &self.session {
            Some(s) => s,
            None => {
                eprintln!("[EXT_IMAGE_COPY] No session available");
                return;
            }
        };

        // Reset frame state
        self.frame_ready = false;
        self.frame_failed = false;
        self.buffer_size_received = false;

        // Create frame
        let frame = session.create_frame(qh, ());
        self.current_frame = Some(frame);
        self.capturing = true;
        eprintln!("[EXT_IMAGE_COPY] Requested frame capture");
    }

    /// Handle buffer_size event - create buffer and attach
    fn handle_buffer_size(&mut self, width: u32, height: u32, qh: &QueueHandle<Self>) {
        self.buffer_size_received = true;
        eprintln!("[EXT_IMAGE_COPY] Buffer size: {}x{}", width, height);

        self.buffer_size = Some(BufferSize { width, height });

        // Select format if not already done
        if self.selected_format.is_none() {
            self.select_format();
        }

        let _format = match &self.selected_format {
            Some(f) => f.clone(),
            None => {
                eprintln!("[EXT_IMAGE_COPY] No format available!");
                self.handle_frame_failed(qh);
                return;
            }
        };

        // Calculate buffer size (stride * height)
        let stride = width * 4;
        let buf_size = (stride * height) as usize;

        // Create or reuse SHM buffer
        let need_new_buffer = match &self.shm_buffer {
            Some(buf) => buf.size != buf_size,
            None => true,
        };

        if need_new_buffer {
            match ShmBuffer::new(buf_size) {
                Ok(buf) => {
                    eprintln!("[EXT_IMAGE_COPY] Created SHM buffer: {} bytes", buf_size);
                    self.shm_buffer = Some(buf);
                }
                Err(e) => {
                    eprintln!("[EXT_IMAGE_COPY] Failed to create SHM buffer: {}", e);
                    self.handle_frame_failed(qh);
                    return;
                }
            }
        }

        // Create wl_buffer
        if let Err(e) = self.create_wl_buffer(qh) {
            eprintln!("[EXT_IMAGE_COPY] Failed to create wl_buffer: {}", e);
            self.handle_frame_failed(qh);
            return;
        }

        // Attach buffer and capture
        if let (Some(frame), Some(shm_buf)) = (&self.current_frame, &self.shm_buffer) {
            if let Some(ref wl_buf) = shm_buf.wl_buffer {
                eprintln!("[EXT_IMAGE_COPY] Attaching buffer and capturing...");
                frame.attach_buffer(wl_buf);
                // Request full damage (entire buffer)
                frame.damage_buffer(0, 0, width as i32, height as i32);
                frame.capture();
            }
        }
    }

    /// Handle frame ready - send frame data
    fn handle_frame_ready(&mut self, qh: &QueueHandle<Self>) {
        self.frame_ready = true;
        let now = std::time::Instant::now();

        if let (Some(size), Some(format), Some(shm_buf)) = (
            &self.buffer_size,
            &self.selected_format,
            &self.shm_buffer,
        ) {
            eprintln!("[EXT_IMAGE_COPY] Frame ready!");

            let data = shm_buf.copy_to_vec();
            let stride = size.width * 4;

            let frame = FrameData::Shm {
                data,
                width: size.width,
                height: size.height,
                stride,
                format: format.format_raw,
            };

            let _ = self.frame_tx.try_send(frame);
        }

        // Clean up frame
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
        if !self.shutdown.load(Ordering::SeqCst) && !self.session_stopped {
            self.request_capture(qh);
        }
    }

    /// Handle frame failed
    fn handle_frame_failed(&mut self, qh: &QueueHandle<Self>) {
        self.frame_failed = true;
        eprintln!("[EXT_IMAGE_COPY] Frame capture failed");

        if let Some(frame) = self.current_frame.take() {
            frame.destroy();
        }

        self.capturing = false;

        // Retry after delay
        if !self.shutdown.load(Ordering::SeqCst) && !self.session_stopped {
            thread::sleep(Duration::from_millis(100));
            self.request_capture(qh);
        }
    }
}

// Dispatch for wl_registry
impl Dispatch<wl_registry::WlRegistry, ()> for ExtCaptureState {
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
            match interface.as_str() {
                "ext_image_copy_capture_manager_v1" => {
                    eprintln!("[EXT_IMAGE_COPY] Found capture manager v{}", version);
                    let manager: ExtImageCopyCaptureManagerV1 =
                        registry.bind(name, version.min(1), qh, ());
                    state.capture_manager = Some(manager);
                }
                "ext_output_image_capture_source_manager_v1" => {
                    eprintln!("[EXT_IMAGE_COPY] Found source manager v{}", version);
                    let manager: ExtOutputImageCaptureSourceManagerV1 =
                        registry.bind(name, version.min(1), qh, ());
                    state.source_manager = Some(manager);
                }
                "wl_output" => {
                    eprintln!("[EXT_IMAGE_COPY] Found wl_output v{}", version);
                    let output: wl_output::WlOutput = registry.bind(name, version.min(4), qh, ());
                    state.outputs.push(output);
                }
                "wl_shm" => {
                    eprintln!("[EXT_IMAGE_COPY] Found wl_shm v{}", version);
                    let shm: wl_shm::WlShm = registry.bind(name, version.min(1), qh, ());
                    state.shm = Some(shm);
                }
                _ => {}
            }
        }
    }
}

// Dispatch for wl_output
impl Dispatch<wl_output::WlOutput, ()> for ExtCaptureState {
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
impl Dispatch<wl_shm::WlShm, ()> for ExtCaptureState {
    fn event(
        _state: &mut Self,
        _shm: &wl_shm::WlShm,
        event: wl_shm::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        if let wl_shm::Event::Format { format } = event {
            eprintln!("[EXT_IMAGE_COPY] SHM format available: {:?}", format);
        }
    }
}

// Dispatch for wl_shm_pool
impl Dispatch<wl_shm_pool::WlShmPool, ()> for ExtCaptureState {
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
impl Dispatch<wl_buffer::WlBuffer, ()> for ExtCaptureState {
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

// Dispatch for ExtOutputImageCaptureSourceManagerV1
impl Dispatch<ExtOutputImageCaptureSourceManagerV1, ()> for ExtCaptureState {
    fn event(
        _state: &mut Self,
        _manager: &ExtOutputImageCaptureSourceManagerV1,
        _event: <ExtOutputImageCaptureSourceManagerV1 as Proxy>::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        // No events from source manager
    }
}

// Dispatch for ExtImageCaptureSourceV1
impl Dispatch<ExtImageCaptureSourceV1, ()> for ExtCaptureState {
    fn event(
        _state: &mut Self,
        _source: &ExtImageCaptureSourceV1,
        _event: <ExtImageCaptureSourceV1 as Proxy>::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        // Source has no events in this version
    }
}

// Dispatch for ExtImageCopyCaptureManagerV1
impl Dispatch<ExtImageCopyCaptureManagerV1, ()> for ExtCaptureState {
    fn event(
        _state: &mut Self,
        _manager: &ExtImageCopyCaptureManagerV1,
        _event: <ExtImageCopyCaptureManagerV1 as Proxy>::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        // Manager has no events
    }
}

// Dispatch for ExtImageCopyCaptureSessionV1
impl Dispatch<ExtImageCopyCaptureSessionV1, ()> for ExtCaptureState {
    fn event(
        state: &mut Self,
        _session: &ExtImageCopyCaptureSessionV1,
        event: ext_image_copy_capture_session_v1::Event,
        _: &(),
        _conn: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        match event {
            ext_image_copy_capture_session_v1::Event::BufferSize { width, height } => {
                eprintln!(
                    "[EXT_IMAGE_COPY] Session buffer size: {}x{}",
                    width, height
                );
                // Store initial buffer size hint from session
                if state.buffer_size.is_none() {
                    state.buffer_size = Some(BufferSize { width, height });
                }
            }
            ext_image_copy_capture_session_v1::Event::ShmFormat { format } => {
                let (format_enum, format_raw) = match format {
                    WEnum::Value(f) => (f, f as u32),
                    WEnum::Unknown(v) => (wl_shm::Format::Argb8888, v),
                };
                eprintln!(
                    "[EXT_IMAGE_COPY] Session SHM format: {:?} (0x{:x})",
                    format_enum, format_raw
                );
                state.shm_formats.push(ShmFormat {
                    format: format_enum,
                    format_raw,
                });
            }
            ext_image_copy_capture_session_v1::Event::DmabufDevice { .. } => {
                eprintln!("[EXT_IMAGE_COPY] Session DMA-BUF device available");
                // We use SHM for now, but could use DMA-BUF in the future
            }
            ext_image_copy_capture_session_v1::Event::DmabufFormat { .. } => {
                // DMA-BUF format available
            }
            ext_image_copy_capture_session_v1::Event::Done => {
                eprintln!("[EXT_IMAGE_COPY] Session format negotiation done");
                // Select format and start capturing
                state.select_format();
            }
            ext_image_copy_capture_session_v1::Event::Stopped => {
                eprintln!("[EXT_IMAGE_COPY] Session stopped");
                state.session_stopped = true;
            }
            _ => {}
        }
    }
}

// Dispatch for ExtImageCopyCaptureFrameV1
impl Dispatch<ExtImageCopyCaptureFrameV1, ()> for ExtCaptureState {
    fn event(
        state: &mut Self,
        _frame: &ExtImageCopyCaptureFrameV1,
        event: ext_image_copy_capture_frame_v1::Event,
        _: &(),
        _conn: &Connection,
        qh: &QueueHandle<Self>,
    ) {
        match event {
            ext_image_copy_capture_frame_v1::Event::Transform { .. } => {
                // Transform info
            }
            ext_image_copy_capture_frame_v1::Event::Damage { .. } => {
                // Damage region (useful for damage-based streaming)
            }
            ext_image_copy_capture_frame_v1::Event::PresentationTime { .. } => {
                // Presentation timestamp
            }
            ext_image_copy_capture_frame_v1::Event::Ready => {
                state.handle_frame_ready(qh);
            }
            ext_image_copy_capture_frame_v1::Event::Failed { reason } => {
                eprintln!("[EXT_IMAGE_COPY] Frame failed: {:?}", reason);
                state.handle_frame_failed(qh);
            }
            _ => {}
        }
    }
}

/// ext-image-copy-capture stream for Sway 1.10+
pub struct ExtImageCopyCaptureStream {
    thread: Option<JoinHandle<()>>,
    frame_rx: mpsc::Receiver<FrameData>,
    shutdown: Arc<AtomicBool>,
    error: Arc<Mutex<Option<String>>>,
}

impl ExtImageCopyCaptureStream {
    /// Connect and start capturing
    pub fn connect(target_fps: u32) -> Result<Self, String> {
        let (frame_tx, frame_rx) = mpsc::sync_channel(8);
        let shutdown = Arc::new(AtomicBool::new(false));
        let shutdown_clone = shutdown.clone();
        let error = Arc::new(Mutex::new(None));
        let error_clone = error.clone();

        let thread = thread::Builder::new()
            .name("ext-image-copy".to_string())
            .spawn(move || {
                if let Err(e) = run_capture_loop(frame_tx, shutdown_clone, target_fps) {
                    eprintln!("[EXT_IMAGE_COPY] Loop error: {}", e);
                    *error_clone.lock() = Some(e);
                }
            })
            .map_err(|e| format!("Failed to spawn thread: {}", e))?;

        // Wait for connection/error
        thread::sleep(Duration::from_millis(300));

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

    /// Check if ext-image-copy-capture is available
    ///
    /// This is a more robust check than just XDG_CURRENT_DESKTOP:
    /// we actually try to connect and check for the protocol.
    pub fn is_available() -> bool {
        // First check if we're on a wlroots compositor
        if let Ok(desktop) = std::env::var("XDG_CURRENT_DESKTOP") {
            let desktop_lower = desktop.to_lowercase();
            if !desktop_lower.contains("sway") && !desktop_lower.contains("wlroots") {
                return false;
            }
        } else {
            return false;
        }

        // Try to connect and check for the protocol
        match Connection::connect_to_env() {
            Ok(conn) => {
                let display = conn.display();
                let mut event_queue = conn.new_event_queue();
                let qh = event_queue.handle();

                struct ProtocolCheck {
                    has_capture_manager: bool,
                    has_source_manager: bool,
                }

                impl Dispatch<wl_registry::WlRegistry, ()> for ProtocolCheck {
                    fn event(
                        state: &mut Self,
                        _registry: &wl_registry::WlRegistry,
                        event: wl_registry::Event,
                        _: &(),
                        _conn: &Connection,
                        _qh: &QueueHandle<Self>,
                    ) {
                        if let wl_registry::Event::Global { interface, .. } = event {
                            if interface == "ext_image_copy_capture_manager_v1" {
                                state.has_capture_manager = true;
                            } else if interface == "ext_output_image_capture_source_manager_v1" {
                                state.has_source_manager = true;
                            }
                        }
                    }
                }

                let mut check = ProtocolCheck {
                    has_capture_manager: false,
                    has_source_manager: false,
                };
                let _registry = display.get_registry(&qh, ());
                let _ = event_queue.roundtrip(&mut check);

                check.has_capture_manager && check.has_source_manager
            }
            Err(_) => false,
        }
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

impl Drop for ExtImageCopyCaptureStream {
    fn drop(&mut self) {
        self.shutdown.store(true, Ordering::SeqCst);
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
        }
    }
}

fn run_capture_loop(
    frame_tx: mpsc::SyncSender<FrameData>,
    shutdown: Arc<AtomicBool>,
    target_fps: u32,
) -> Result<(), String> {
    eprintln!(
        "[EXT_IMAGE_COPY] Connecting to Wayland display (target_fps={})...",
        target_fps
    );

    let conn =
        Connection::connect_to_env().map_err(|e| format!("Failed to connect to Wayland: {}", e))?;

    let display = conn.display();
    let mut event_queue: EventQueue<ExtCaptureState> = conn.new_event_queue();
    let qh = event_queue.handle();

    let mut state = ExtCaptureState::new(frame_tx, shutdown.clone(), target_fps);

    // Get registry and bind globals
    let _registry = display.get_registry(&qh, ());

    // Roundtrip to get globals
    event_queue
        .roundtrip(&mut state)
        .map_err(|e| format!("Initial roundtrip failed: {}", e))?;

    // Check we have required globals
    if state.capture_manager.is_none() {
        return Err(
            "ext_image_copy_capture_manager_v1 not available (Sway 1.10+ required)".to_string(),
        );
    }
    if state.source_manager.is_none() {
        return Err(
            "ext_output_image_capture_source_manager_v1 not available".to_string(),
        );
    }
    if state.shm.is_none() {
        return Err("wl_shm not available".to_string());
    }
    if state.outputs.is_empty() {
        return Err("No wl_output found".to_string());
    }

    eprintln!(
        "[EXT_IMAGE_COPY] Connected! Found {} outputs",
        state.outputs.len()
    );

    // Initialize session
    state
        .init_session(&qh)
        .map_err(|e| format!("Failed to init session: {}", e))?;

    // Roundtrip to get session format events
    event_queue
        .roundtrip(&mut state)
        .map_err(|e| format!("Session roundtrip failed: {}", e))?;

    eprintln!(
        "[EXT_IMAGE_COPY] Session ready, {} formats available",
        state.shm_formats.len()
    );

    // Start first capture
    state.request_capture(&qh);

    // Get the Wayland connection's file descriptor for polling
    let wayland_fd = conn.as_fd();

    // Main loop with polling for timeout-based dispatch
    // This allows us to:
    // 1. Check shutdown flag periodically
    // 2. Allow pipewiresrc's keepalive to work (recv_frame_timeout returns Timeout)
    // 3. Handle static screens where no damage events occur
    let poll_timeout = Duration::from_millis(100); // 10Hz polling

    while !shutdown.load(Ordering::SeqCst) && !state.session_stopped {
        // Flush any pending requests
        conn.flush().map_err(|e| format!("Flush failed: {}", e))?;

        // Use prepare_read/read with polling for timeout support
        if let Some(guard) = event_queue.prepare_read() {
            // Poll the Wayland fd with timeout
            let mut pollfd = libc::pollfd {
                fd: wayland_fd.as_raw_fd(),
                events: libc::POLLIN,
                revents: 0,
            };

            let poll_result = unsafe {
                libc::poll(
                    &mut pollfd,
                    1,
                    poll_timeout.as_millis() as libc::c_int,
                )
            };

            if poll_result > 0 {
                // Data available - read events
                guard
                    .read()
                    .map_err(|e| format!("Read failed: {}", e))?;
            } else if poll_result == 0 {
                // Timeout - drop guard to cancel read, loop will check shutdown flag
                drop(guard);
                continue;
            } else {
                // Error
                let errno = unsafe { *libc::__errno_location() };
                if errno == libc::EINTR {
                    // Interrupted - drop guard and retry
                    drop(guard);
                    continue;
                }
                drop(guard);
                return Err(format!("poll() failed: errno {}", errno));
            }
        }

        // Dispatch any pending events
        event_queue
            .dispatch_pending(&mut state)
            .map_err(|e| format!("Dispatch failed: {}", e))?;
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_available_respects_env() {
        // Test runs without panic
        let _ = ExtImageCopyCaptureStream::is_available();
    }
}
