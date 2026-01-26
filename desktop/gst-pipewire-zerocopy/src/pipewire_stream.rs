//! PipeWire stream handling - outputs smithay Dmabuf directly
//!
//! For CUDA path: EGL import + CUDA copy happens HERE in the PipeWire thread
//! BEFORE returning the buffer to the compositor. This prevents race conditions
//! where the compositor reuses the GPU buffer while we're still reading from it.

// GStreamer imports (crate names from Cargo.toml: gst, gst-video)
use gst::prelude::*;
use gst::Buffer as GstBuffer;
use gst_video::{VideoCapsBuilder, VideoFormat, VideoInfo, VideoInfoDmaDrm};

use parking_lot::Mutex;
use pipewire::spa::param::format::{FormatProperties, MediaSubtype, MediaType};
use pipewire::spa::param::ParamType;
use pipewire::spa::pod::serialize::PodSerializer;
use pipewire::spa::pod::Pod;
use pipewire::spa::pod::{ChoiceValue, Object, Property, PropertyFlags, Value};
use pipewire::spa::sys::{spa_buffer, spa_meta, spa_meta_header, SPA_META_Header};
use pipewire::spa::utils::{Choice, ChoiceEnum, ChoiceFlags, Fraction, Id, SpaTypes};
use pipewire::{
    context::Context,
    main_loop::MainLoop,
    properties::properties,
    spa,
    stream::{Stream, StreamFlags},
};
use smithay::backend::allocator::dmabuf::{Dmabuf, DmabufFlags};
use smithay::backend::allocator::{Fourcc, Modifier};
use std::io::Cursor;
use std::os::fd::{BorrowedFd, FromRawFd};
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread::{self, JoinHandle};
use std::time::Duration;

// Use crossbeam-channel instead of std::sync::mpsc to avoid race conditions
// where recv_timeout can miss messages sent during timeout processing.
// See: https://github.com/rust-lang/rust/issues/94518
use crossbeam_channel::{self as channel, Receiver, Sender};

// CUDA imports for zero-copy path
use smithay::backend::egl::ffi::egl::types::EGLDisplay as RawEGLDisplay;
use smithay::backend::egl::EGLDisplay;
use waylanddisplaycore::utils::allocator::cuda::{
    CUDABufferPool, CUDAContext, CUDAImage, EGLImage, CAPS_FEATURE_MEMORY_CUDA_MEMORY,
};

/// Cursor bitmap image data
#[derive(Debug, Clone)]
pub struct CursorBitmap {
    /// DRM fourcc format (e.g., ARGB8888)
    pub format: u32,
    /// Bitmap width in pixels
    pub width: u32,
    /// Bitmap height in pixels
    pub height: u32,
    /// Stride in bytes
    pub stride: i32,
    /// Raw pixel data
    pub data: Vec<u8>,
}

/// Cursor data extracted from PipeWire SPA_META_Cursor metadata
#[derive(Debug, Clone)]
pub struct CursorData {
    pub id: u32,
    pub position_x: i32,
    pub position_y: i32,
    pub hotspot_x: i32,
    pub hotspot_y: i32,
    pub bitmap: Option<CursorBitmap>,
}

/// Frame received from PipeWire
pub enum FrameData {
    /// DMA-BUF frame (zero-copy) - directly usable with waylanddisplaycore
    /// pts_ns is the compositor timestamp in nanoseconds (from spa_meta_header)
    DmaBuf {
        dmabuf: Dmabuf,
        pts_ns: i64,
    },
    /// CUDA buffer - EGL import + CUDA copy already completed in PipeWire thread
    /// This is the preferred path for NVIDIA: no race condition with buffer reuse
    CudaBuffer {
        buffer: GstBuffer,
        width: u32,
        height: u32,
        format: VideoFormat,
        pts_ns: i64,
    },
    /// SHM fallback
    /// pts_ns is the compositor timestamp in nanoseconds (from spa_meta_header)
    Shm {
        data: Vec<u8>,
        width: u32,
        height: u32,
        stride: u32,
        format: u32,
        pts_ns: i64,
    },
}

/// Error type for frame receive operations
#[derive(Debug, Clone)]
pub enum RecvError {
    /// Timeout waiting for frame (normal for damage-based GNOME ScreenCast)
    Timeout,
    /// Channel disconnected
    Disconnected,
    /// Other error with message
    Error(String),
}

impl std::fmt::Display for RecvError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            RecvError::Timeout => write!(f, "timeout waiting for frame"),
            RecvError::Disconnected => write!(f, "channel disconnected"),
            RecvError::Error(msg) => write!(f, "{}", msg),
        }
    }
}

/// Video parameters from PipeWire format negotiation
#[derive(Debug, Clone, Default)]
pub struct VideoParams {
    pub width: u32,
    pub height: u32,
    pub format: u32,
    pub modifier: u64,
}

/// GPU-supported DMA-BUF modifiers for a specific format.
/// These are queried from EGL before PipeWire negotiation.
#[derive(Debug, Clone, Default)]
pub struct DmaBufCapabilities {
    /// Modifiers supported for BGRA/BGRx formats (common for screen capture)
    pub modifiers: Vec<u64>,
    /// Whether DMA-BUF is available at all
    pub dmabuf_available: bool,
}

/// CUDA resources for zero-copy GPU path.
/// These are passed to the PipeWire thread to do EGL import + CUDA copy
/// BEFORE returning the buffer to the compositor (preventing race conditions).
///
/// The key insight: We must complete the CUDA copy BEFORE calling `queue_raw_buffer`
/// because returning the buffer allows Mutter to reuse the underlying GPU memory.
/// The DmaBuf file descriptor we dup'd still points to the same GPU buffer!
pub struct CudaResources {
    /// Smithay EGLDisplay - used to import DmaBuf as EGLImage
    pub egl_display: Arc<EGLDisplay>,
    /// CUDA context for GPU operations
    pub cuda_context: Arc<std::sync::Mutex<CUDAContext>>,
    /// Buffer pool state - wrapped in Mutex for interior mutability
    pub buffer_pool_state: Arc<Mutex<BufferPoolState>>,
}

/// Buffer pool state for CUDA buffer allocation
pub struct BufferPoolState {
    pub pool: Option<CUDABufferPool>,
    pub configured: bool,
}

// CudaResources needs to be Send to pass to the PipeWire thread
// Safety: EGL display and CUDA context are thread-safe when properly synchronized
unsafe impl Send for CudaResources {}

/// PipeWire stream wrapper

pub struct PipeWireStream {
    thread: Option<JoinHandle<()>>,
    frame_rx: Receiver<FrameData>,
    shutdown: Arc<AtomicBool>,
    video_info: Arc<Mutex<VideoParams>>,
    error: Arc<Mutex<Option<String>>>,
}

impl PipeWireStream {
    /// Connect to a PipeWire ScreenCast node.
    ///
    /// # Arguments
    /// * `node_id` - The PipeWire node ID from ScreenCast portal
    /// * `pipewire_fd` - Optional FD from OpenPipeWireRemote portal call.
    ///                   This FD grants access to ScreenCast nodes that aren't visible
    ///                   to the default PipeWire daemon connection.
    ///                   Without it, connecting to ScreenCast nodes returns "target not found".
    /// * `dmabuf_caps` - GPU-supported DMA-BUF capabilities queried from EGL.
    ///                   If provided, we'll offer formats with modifiers for zero-copy.
    ///                   If None, we only offer SHM formats (MemFd).
    /// * `target_fps` - Target frames per second. The max_framerate sent to Mutter will be
    ///                  target_fps * 2 to avoid the 16ms/17ms timing boundary issue.
    /// * `cuda_resources` - Optional CUDA resources for GPU zero-copy path.
    ///                      If provided, EGL import + CUDA copy happens in the PipeWire thread
    ///                      BEFORE returning the buffer to the compositor, preventing race conditions.
    pub fn connect(
        node_id: u32,
        pipewire_fd: Option<i32>,
        dmabuf_caps: Option<DmaBufCapabilities>,
        target_fps: u32,
        cuda_resources: Option<CudaResources>,
    ) -> Result<Self, String> {
        // Use larger buffer to reduce backpressure from GStreamer consumer
        // This helps prevent frame drops when GNOME delivers frames faster than we consume them
        // crossbeam::bounded is used instead of std::mpsc::sync_channel to avoid race conditions
        // where recv_timeout can miss messages sent during timeout processing
        let (frame_tx, frame_rx) = channel::bounded(8);
        let shutdown = Arc::new(AtomicBool::new(false));
        let shutdown_clone = shutdown.clone();
        let video_info = Arc::new(Mutex::new(VideoParams::default()));
        let video_info_clone = video_info.clone();
        let error = Arc::new(Mutex::new(None));
        let error_clone = error.clone();

        // Request max_framerate=0 to DISABLE Mutter's frame rate limiter entirely.
        // When max_framerate>0, Mutter skips frames causing judder and caps FPS at ~30-40.
        // With max_framerate=0/1, Mutter sends all damage events without frame limiting.
        //
        // Trade-off: Static screens produce zero frames from Mutter. The pipewirezerocopysrc
        // keepalive mechanism (keepalive-time=500) resends the last buffer every 500ms,
        // ensuring 2 FPS minimum on static screens to keep the stream alive.
        let negotiated_max_fps = 0;
        eprintln!(
            "[PIPEWIRE_DEBUG] target_fps={}, negotiated_max_fps={} (0=disabled frame limiter), cuda={}",
            target_fps, negotiated_max_fps, cuda_resources.is_some()
        );

        let thread = thread::Builder::new()
            .name("pipewire-stream".to_string())
            .spawn(move || {
                if let Err(e) = run_pipewire_loop(
                    node_id,
                    pipewire_fd,
                    dmabuf_caps,
                    negotiated_max_fps,
                    frame_tx,
                    shutdown_clone,
                    video_info_clone,
                    cuda_resources,
                ) {
                    tracing::error!("PipeWire loop error: {}", e);
                    *error_clone.lock() = Some(e);
                }
            })
            .map_err(|e| format!("Failed to spawn PipeWire thread: {}", e))?;

        thread::sleep(Duration::from_millis(100));

        if let Some(err) = error.lock().take() {
            shutdown.store(true, Ordering::SeqCst);
            return Err(err);
        }

        Ok(PipeWireStream {
            thread: Some(thread),
            frame_rx,
            shutdown,
            video_info,
            error,
        })
    }

    /// Receive a frame with the default timeout (30 seconds).
    /// For GNOME ScreenCast damage-based delivery, use `recv_frame_timeout` with keepalive.
    pub fn recv_frame(&self) -> Result<FrameData, RecvError> {
        self.recv_frame_timeout(Duration::from_secs(30))
    }

    /// Receive a frame with a configurable timeout.
    ///
    /// Returns `RecvError::Timeout` if no frame arrives within the timeout.
    /// This is normal for GNOME 49+ damage-based ScreenCast - static desktops
    /// produce no frames. Callers should implement keepalive by resending the
    /// last buffer when timeout occurs.
    ///
    /// See: design/2026-01-06-pipewire-keepalive-mechanism.md
    pub fn recv_frame_timeout(&self, timeout: Duration) -> Result<FrameData, RecvError> {
        if let Some(err) = self.error.lock().take() {
            return Err(RecvError::Error(err));
        }
        self.frame_rx.recv_timeout(timeout).map_err(|e| match e {
            channel::RecvTimeoutError::Timeout => RecvError::Timeout,
            channel::RecvTimeoutError::Disconnected => RecvError::Disconnected,
        })
    }

    #[allow(dead_code)]
    pub fn video_params(&self) -> VideoParams {
        self.video_info.lock().clone()
    }
}

impl Drop for PipeWireStream {
    fn drop(&mut self) {
        self.shutdown.store(true, Ordering::SeqCst);
        if let Some(thread) = self.thread.take() {
            let _ = thread.join();
        }
    }
}

/// Convert SPA VideoFormat to DRM fourcc code
/// SPA format enum values: https://docs.pipewire.org/spa_2param_2video_2format_8h.html
fn spa_video_format_to_drm_fourcc(format: spa::param::video::VideoFormat) -> u32 {
    // DRM fourcc codes (little-endian):
    // AR24 = 0x34325241 = ARGB8888
    // AB24 = 0x34324241 = ABGR8888
    // XR24 = 0x34325258 = XRGB8888
    // XB24 = 0x34324258 = XBGR8888
    // RA24 = 0x34324152 = RGBA8888
    // BA24 = 0x34324142 = BGRA8888
    // RX24 = 0x34325852 = RGBX8888
    // BX24 = 0x34325842 = BGRX8888
    // NV12 = 0x3231564e = NV12
    match format {
        spa::param::video::VideoFormat::BGRA => 0x34324142, // BA24 = BGRA8888
        spa::param::video::VideoFormat::RGBA => 0x34324152, // RA24 = RGBA8888
        // BGRx/RGBx: Use ARGB/ABGR for CUDA compatibility
        // CUDA rejects both XRGB8888 and BGRX8888 with NVIDIA tiled modifiers.
        // Use formats with alpha channels (ARGB/ABGR) which CUDA accepts.
        // BGRx bytes: B,G,R,x -> ARGB8888 bytes: B,G,R,A (same layout, alpha treated as opaque)
        // RGBx bytes: R,G,B,x -> ABGR8888 bytes: R,G,B,A (same layout, alpha treated as opaque)
        spa::param::video::VideoFormat::BGRx => 0x34325241, // AR24 = ARGB8888 (CUDA accepts with tiled modifiers)
        spa::param::video::VideoFormat::RGBx => 0x34324241, // AB24 = ABGR8888 (CUDA accepts with tiled modifiers)
        spa::param::video::VideoFormat::ARGB => 0x34325241, // AR24 = ARGB8888
        spa::param::video::VideoFormat::ABGR => 0x34324241, // AB24 = ABGR8888
        spa::param::video::VideoFormat::xRGB => 0x34325258, // XR24 = XRGB8888
        spa::param::video::VideoFormat::xBGR => 0x34324258, // XB24 = XBGR8888
        spa::param::video::VideoFormat::NV12 => 0x3231564e, // NV12
        spa::param::video::VideoFormat::I420 => 0x32315549, // I420
        // RGB/BGR 24-bit formats (for xdg-desktop-portal-wlr SHM fallback)
        // RG24 = 0x34324752 = RGB888
        // BG24 = 0x34324742 = BGR888
        spa::param::video::VideoFormat::RGB => 0x34324752, // RG24 = RGB888
        spa::param::video::VideoFormat::BGR => 0x34324742, // BG24 = BGR888
        _ => {
            tracing::warn!(
                "Unknown SPA video format {:?}, defaulting to ARGB8888",
                format
            );
            0x34325241 // AR24 = ARGB8888
        }
    }
}

/// Extract PTS (presentation timestamp) from PipeWire buffer's spa_meta_header.
/// The PTS is set by the compositor when the frame was captured.
/// Returns 0 if no header metadata is present.
///
/// # Safety
/// The buffer pointer must be valid and point to a spa_buffer.
unsafe fn extract_pts_from_buffer(buffer: *mut spa_buffer) -> i64 {
    if buffer.is_null() {
        return 0;
    }
    let n_metas = (*buffer).n_metas;
    if n_metas == 0 {
        return 0;
    }
    let mut meta_ptr = (*buffer).metas;
    let metas_end = (*buffer).metas.wrapping_add(n_metas as usize);
    while meta_ptr != metas_end {
        if (*meta_ptr).type_ == SPA_META_Header {
            let meta_header: &spa_meta_header = &*((*meta_ptr).data as *const spa_meta_header);
            return meta_header.pts;
        }
        meta_ptr = meta_ptr.wrapping_add(1);
    }
    0
}

/// SPA_META_Cursor type constant (from spa/buffer/meta.h)
const SPA_META_CURSOR: u32 = 5;

/// Extract cursor metadata from PipeWire buffer's spa_meta_cursor.
/// This is sent when cursor-mode=2 (Metadata) is set in ScreenCast.
/// NOTE: No longer used in Rust - Go PipeWire client handles cursor via its own session.
///
/// # Safety
/// The buffer pointer must be valid and point to a spa_buffer.
#[allow(dead_code)]
unsafe fn extract_cursor_from_buffer(buffer: *mut spa_buffer) -> Option<CursorData> {
    if buffer.is_null() {
        return None;
    }
    let n_metas = (*buffer).n_metas;
    if n_metas == 0 {
        return None;
    }

    let mut meta_ptr: *mut spa_meta = (*buffer).metas;
    let metas_end = (*buffer).metas.wrapping_add(n_metas as usize);

    while meta_ptr != metas_end {
        if (*meta_ptr).type_ == SPA_META_CURSOR {
            let data_ptr = (*meta_ptr).data as *const u8;
            let meta_size = (*meta_ptr).size;

            // spa_meta_cursor is 28 bytes minimum
            if meta_size < 28 {
                meta_ptr = meta_ptr.wrapping_add(1);
                continue;
            }

            // Parse spa_meta_cursor fields
            // Offsets: id(0), flags(4), position.x(8), position.y(12),
            //          hotspot.x(16), hotspot.y(20), bitmap_offset(24)
            let id = *(data_ptr as *const u32);
            let position_x = *(data_ptr.add(8) as *const i32);
            let position_y = *(data_ptr.add(12) as *const i32);
            let hotspot_x = *(data_ptr.add(16) as *const i32);
            let hotspot_y = *(data_ptr.add(20) as *const i32);
            let bitmap_offset = *(data_ptr.add(24) as *const u32);

            // Check if there's a valid bitmap
            let bitmap = if bitmap_offset >= 28 && (bitmap_offset as usize) + 20 <= meta_size as usize {
                let bitmap_ptr = data_ptr.add(bitmap_offset as usize);
                let format = *(bitmap_ptr as *const u32);
                let width = *(bitmap_ptr.add(4) as *const u32);
                let height = *(bitmap_ptr.add(8) as *const u32);
                let stride = *(bitmap_ptr.add(12) as *const i32);
                let pixel_offset = *(bitmap_ptr.add(16) as *const u32);

                if format == 0 || width == 0 || height == 0 {
                    None
                } else {
                    let pixel_data_start = bitmap_ptr.add(pixel_offset as usize);
                    let pixel_data_size = (stride.abs() as u32 * height) as usize;
                    let total_offset = bitmap_offset as usize + pixel_offset as usize + pixel_data_size;

                    if total_offset <= meta_size as usize {
                        let data = std::slice::from_raw_parts(pixel_data_start, pixel_data_size).to_vec();
                        Some(CursorBitmap {
                            format,
                            width,
                            height,
                            stride,
                            data,
                        })
                    } else {
                        None
                    }
                }
            } else {
                None
            };

            return Some(CursorData {
                id,
                position_x,
                position_y,
                hotspot_x,
                hotspot_y,
                bitmap,
            });
        }
        meta_ptr = meta_ptr.wrapping_add(1);
    }
    None
}

/// Write cursor data to a file for IPC with Go WebSocket handler.
/// NOTE: No longer used - Go PipeWire client reads cursor directly from its own session.
#[allow(dead_code)]
fn write_cursor_to_file(cursor: &CursorData) {
    use std::io::Write;

    // Track last cursor state to avoid redundant writes
    static LAST_CURSOR_HASH: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);

    // Hash cursor state (hotspot + bitmap, NOT position - position changes every frame)
    let cursor_hash = {
        let mut h: u64 = cursor.id as u64;
        h = h.wrapping_mul(31).wrapping_add(cursor.hotspot_x as u64);
        h = h.wrapping_mul(31).wrapping_add(cursor.hotspot_y as u64);
        if let Some(ref bmp) = cursor.bitmap {
            h = h.wrapping_mul(31).wrapping_add(bmp.width as u64);
            h = h.wrapping_mul(31).wrapping_add(bmp.height as u64);
            if bmp.data.len() >= 8 {
                h = h.wrapping_mul(31).wrapping_add(u64::from_le_bytes([
                    bmp.data[0], bmp.data[1], bmp.data[2], bmp.data[3],
                    bmp.data[4], bmp.data[5], bmp.data[6], bmp.data[7],
                ]));
            }
        }
        h
    };

    // Only write if cursor state changed
    let last_hash = LAST_CURSOR_HASH.swap(cursor_hash, std::sync::atomic::Ordering::Relaxed);
    if cursor_hash == last_hash {
        return;
    }

    // Binary format: header (28 bytes) + optional bitmap
    let bitmap_header_size = if cursor.bitmap.is_some() { 16 } else { 0 };
    let bitmap_data_size = cursor.bitmap.as_ref().map(|b| b.data.len()).unwrap_or(0);
    let total_size = 28 + bitmap_header_size + bitmap_data_size;

    let mut buf = Vec::with_capacity(total_size);
    buf.extend_from_slice(&0x43555253u32.to_le_bytes()); // "CURS" magic
    buf.extend_from_slice(&1u32.to_le_bytes()); // version
    buf.extend_from_slice(&cursor.position_x.to_le_bytes());
    buf.extend_from_slice(&cursor.position_y.to_le_bytes());
    buf.extend_from_slice(&cursor.hotspot_x.to_le_bytes());
    buf.extend_from_slice(&cursor.hotspot_y.to_le_bytes());
    buf.extend_from_slice(&((bitmap_header_size + bitmap_data_size) as u32).to_le_bytes());

    if let Some(ref bmp) = cursor.bitmap {
        buf.extend_from_slice(&bmp.format.to_le_bytes());
        buf.extend_from_slice(&bmp.width.to_le_bytes());
        buf.extend_from_slice(&bmp.height.to_le_bytes());
        buf.extend_from_slice(&bmp.stride.to_le_bytes());
        buf.extend_from_slice(&bmp.data);
    }

    // Atomic write via temp file
    let cursor_path = "/tmp/helix-cursor.bin";
    let temp_path = "/tmp/helix-cursor.bin.tmp";
    if let Ok(mut file) = std::fs::File::create(temp_path) {
        if file.write_all(&buf).is_ok() {
            let _ = std::fs::rename(temp_path, cursor_path);
        }
    }
}

fn run_pipewire_loop(
    node_id: u32,
    pipewire_fd: Option<i32>,
    dmabuf_caps: Option<DmaBufCapabilities>,
    negotiated_max_fps: u32,
    frame_tx: Sender<FrameData>,
    shutdown: Arc<AtomicBool>,
    video_info: Arc<Mutex<VideoParams>>,
    cuda_resources: Option<CudaResources>,
) -> Result<(), String> {
    pipewire::init();

    let mainloop = MainLoop::new(None).map_err(|e| format!("MainLoop: {}", e))?;
    let context = Context::new(&mainloop).map_err(|e| format!("Context: {}", e))?;

    // Connect to PipeWire - use portal FD if provided
    // The FD from OpenPipeWireRemote grants access to ScreenCast nodes that aren't
    // visible to the default PipeWire daemon. Without it, we get "target not found".
    let core = if let Some(fd) = pipewire_fd {
        eprintln!("[PIPEWIRE_DEBUG] Connecting via portal FD: {}", fd);
        // Create an OwnedFd from the raw fd
        // The portal passed us ownership, so we can wrap it in OwnedFd
        let owned_fd = unsafe { std::os::fd::OwnedFd::from_raw_fd(fd) };
        context
            .connect_fd(owned_fd, None)
            .map_err(|e| format!("Connect with FD {}: {}", fd, e))?
    } else {
        eprintln!("[PIPEWIRE_DEBUG] Connecting to default PipeWire daemon (no portal FD)");
        context
            .connect(None)
            .map_err(|e| format!("Connect: {}", e))?
    };

    // Request high framerate in stream properties
    // VIDEO_RATE hints to PipeWire what framerate we want
    let props = properties! {
        *pipewire::keys::MEDIA_TYPE => "Video",
        *pipewire::keys::MEDIA_CATEGORY => "Capture",
        *pipewire::keys::MEDIA_ROLE => "Screen",
        *pipewire::keys::VIDEO_RATE => "60/1",
    };

    let stream =
        Stream::new(&core, "helix-screencast", props).map_err(|e| format!("Stream: {}", e))?;

    // Note: SyncSender is thread-safe, no Mutex needed
    let video_info_param = video_info.clone();
    let frame_tx_process = frame_tx.clone();

    // Wrap cuda_resources in Arc for sharing with process callback
    // The process callback will do EGL import + CUDA copy BEFORE returning buffer to PipeWire
    let cuda_resources = cuda_resources.map(Arc::new);
    let cuda_resources_process = cuda_resources.clone();

    // Per-stream tracking for format negotiation.
    // CRITICAL: This must be per-stream (Arc), not static, because Wolf runs
    // multiple sessions in one process. A static would cause the first
    // session to set it, and subsequent sessions would skip update_params entirely.
    //
    // We track the last buffer type we requested (None, MemFd=4, DmaBuf=8).
    // If GNOME changes the modifier (e.g., from LINEAR to NVIDIA tiled), we need
    // to re-negotiate with the correct buffer type.
    use std::sync::atomic::AtomicU32;
    let last_buffer_type = Arc::new(AtomicU32::new(0)); // 0 = not yet negotiated
    let last_buffer_type_clone = last_buffer_type.clone();

    // Flag to track if we've called set_active (to avoid calling it multiple times)
    let stream_activated = Arc::new(AtomicBool::new(false));
    let stream_activated_clone = stream_activated.clone();

    // Flag to track if DmaBuf is available (from EGL caps)
    // When false, we request MemFd (SHM) instead of DmaBuf to avoid allocation failures
    let dmabuf_available = dmabuf_caps
        .as_ref()
        .map(|c| c.dmabuf_available)
        .unwrap_or(false);
    let dmabuf_available_clone = dmabuf_available;

    // Track if we've already called set_active in state_changed
    // (to avoid redundant calls from param_changed for GNOME)
    let set_active_in_paused = Arc::new(AtomicBool::new(false));
    let set_active_in_paused_clone = set_active_in_paused.clone();

    let _listener = stream
        .add_local_listener_with_user_data(spa::param::video::VideoInfoRaw::default())
        .state_changed(move |stream, _user_data, old, new| {
            eprintln!("[PIPEWIRE_DEBUG] PipeWire state: {:?} -> {:?}", old, new);

            // For xdg-desktop-portal-wlr (Sway), we need to call set_active(true) when
            // entering Paused state to trigger format negotiation. GNOME sends format
            // params automatically, but Sway's portal waits for set_active first.
            //
            // This is safe to call for GNOME too - it just means we call set_active
            // slightly earlier than strictly necessary.
            if new == pipewire::stream::StreamState::Paused {
                if !set_active_in_paused_clone.swap(true, Ordering::SeqCst) {
                    eprintln!("[PIPEWIRE_DEBUG] Paused state reached, calling set_active(true) to trigger format negotiation...");
                    if let Err(e) = stream.set_active(true) {
                        eprintln!("[PIPEWIRE_DEBUG] set_active(true) failed in state_changed: {}", e);
                    } else {
                        eprintln!("[PIPEWIRE_DEBUG] set_active(true) succeeded in state_changed!");
                    }
                }
            }

            if new == pipewire::stream::StreamState::Streaming {
                eprintln!("[PIPEWIRE_DEBUG] Stream transitioned to Streaming! Frames should start arriving now.");
            }

            // Log error state with details
            if let pipewire::stream::StreamState::Error(ref err) = new {
                eprintln!("[PIPEWIRE_DEBUG] Stream ERROR: {}", err);
            }
        })
        .add_buffer(|_stream, _user_data, buffer| {
            // This callback is called when PipeWire allocates a buffer
            // If we never see this, negotiation is failing
            static BUFFER_COUNT: std::sync::atomic::AtomicU32 = std::sync::atomic::AtomicU32::new(0);
            let count = BUFFER_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed) + 1;
            eprintln!("[PIPEWIRE_DEBUG] add_buffer callback #{} - buffer {:?}", count, buffer);
        })
        .param_changed(move |stream, user_data, id, pod| {
            // Print actual ParamType raw values for debugging
            // spa_param_type: Invalid=0, PropInfo=1, Props=2, EnumFormat=3, Format=4, Buffers=5, Meta=6
            let format_raw = spa::param::ParamType::Format.as_raw();
            let buffers_raw = spa::param::ParamType::Buffers.as_raw();
            let meta_raw = spa::param::ParamType::Meta.as_raw();

            let id_name = if id == format_raw {
                "Format"
            } else if id == buffers_raw {
                "Buffers"
            } else if id == meta_raw {
                "Meta"
            } else if id == spa::param::ParamType::EnumFormat.as_raw() {
                "EnumFormat"
            } else {
                "Unknown"
            };
            eprintln!("[PIPEWIRE_DEBUG] param_changed: id={} ({}), has_pod={} [Format={}, Buffers={}, Meta={}]",
                id, id_name, pod.is_some(), format_raw, buffers_raw, meta_raw);
            if id != format_raw {
                return;
            }
            let Some(param) = pod else {
                tracing::warn!("[PIPEWIRE_DEBUG] param_changed: pod is None");
                return;
            };

            // Parse media type and subtype
            let (media_type, media_subtype) = match spa::param::format_utils::parse_format(param) {
                Ok(v) => v,
                Err(e) => {
                    tracing::warn!("[PIPEWIRE_DEBUG] Failed to parse format: {:?}", e);
                    return;
                }
            };

            eprintln!("[PIPEWIRE_DEBUG] media_type={:?} media_subtype={:?}", media_type, media_subtype);

            // We only handle video/raw
            if media_type != spa::param::format::MediaType::Video
                || media_subtype != spa::param::format::MediaSubtype::Raw
            {
                tracing::warn!("[PIPEWIRE_DEBUG] Ignoring non-raw video format");
                return;
            }

            // Parse the VideoInfoRaw from the pod
            if let Err(e) = user_data.parse(param) {
                tracing::warn!("[PIPEWIRE_DEBUG] Failed to parse VideoInfoRaw: {:?}", e);
                return;
            }

            let width = user_data.size().width;
            let height = user_data.size().height;
            let format_raw = user_data.format().as_raw();

            // Get both framerate and max_framerate - mutter uses max_framerate for frame pacing
            let framerate = user_data.framerate();
            let max_framerate = user_data.max_framerate();

            eprintln!(
                "[PIPEWIRE_DEBUG] PipeWire video format: {}x{} format={} ({:?}) framerate={}/{} max_framerate={}/{}",
                width,
                height,
                format_raw,
                user_data.format(),
                framerate.num,
                framerate.denom,
                max_framerate.num,
                max_framerate.denom
            );

            // CRITICAL: Log the max_framerate since mutter uses this for frame pacing!
            // If max_framerate is 0/0 or 0/1, mutter's frame pacing is disabled.
            // If it's some low value like 30/1, we'll be limited to 30 FPS.
            if max_framerate.num == 0 {
                eprintln!("[PIPEWIRE_DEBUG] WARNING: max_framerate.num=0 means NO frame pacing limit from mutter");
            } else {
                let fps = max_framerate.num as f64 / max_framerate.denom.max(1) as f64;
                eprintln!("[PIPEWIRE_DEBUG] Frame pacing limit: {:.1} FPS (min interval: {:.2}ms)",
                    fps, 1000.0 / fps);
            }

            // Update VideoParams - convert SPA video format to DRM fourcc
            // SPA formats: BGRA=2, RGBA=4, BGRx=5, RGBx=6, ARGB=7, ABGR=8, xRGB=9, xBGR=10
            // See: https://pipewire.pages.freedesktop.org/pipewire/group__spa__param.html
            let drm_fourcc = spa_video_format_to_drm_fourcc(user_data.format());
            let modifier = user_data.modifier();
            eprintln!(
                "[PIPEWIRE_DEBUG] Converted to DRM fourcc: 0x{:x}, modifier: 0x{:x}",
                drm_fourcc, modifier
            );
            let mut params = video_info_param.lock();
            params.width = width;
            params.height = height;
            params.format = drm_fourcc;
            params.modifier = modifier;
            drop(params); // Release lock before update_params

            // Buffer type selection based on modifier:
            //
            // OBS/gnome-remote-desktop pattern: Check if modifier FIELD is present.
            // If present (ANY value including 0x0 LINEAR), request DmaBuf.
            // If modifier field is absent, request MemFd.
            //
            // We always request DmaBuf because:
            // 1. We only offer DmaBuf format (no SHM fallback)
            // 2. GNOME will fail allocation if it can't do DmaBuf - we want explicit failure
            // 3. This matches how OBS handles modifier negotiation
            let vendor_id = modifier >> 56;
            let is_gpu_tiled = modifier != u64::MAX && modifier != 0 && vendor_id != 0;

            eprintln!("[PIPEWIRE_DEBUG] Format modifier=0x{:x} vendor_id=0x{:x} is_gpu_tiled={} dmabuf_available={}",
                modifier, vendor_id, is_gpu_tiled, dmabuf_available_clone);

            // Buffer type selection:
            // - If DmaBuf is available (GPU accessible), request DmaBuf (8) for zero-copy
            // - If DmaBuf is NOT available (no GPU or EGL failed), request MemFd (4) for SHM fallback
            // This prevents "error alloc buffers: Invalid argument" when GPU isn't accessible
            let (required_buffer_type, use_dmabuf): (u32, bool) = if dmabuf_available_clone {
                eprintln!("[PIPEWIRE_DEBUG] Requesting DmaBuf (buffer_type=8) for modifier 0x{:x}", modifier);
                (8, true)  // DmaBuf
            } else {
                eprintln!("[PIPEWIRE_DEBUG] Requesting MemFd (buffer_type=4) - DmaBuf not available");
                (4, false)  // MemFd (SHM)
            };

            // Check if we need to (re-)negotiate.
            // Only skip if we already requested DmaBuf with the same buffer type.
            // This prevents infinite loops while allowing re-negotiation when needed.
            let previous_buffer_type = last_buffer_type_clone.swap(required_buffer_type, Ordering::SeqCst);

            if previous_buffer_type == required_buffer_type {
                eprintln!("[PIPEWIRE_DEBUG] Format callback with same buffer type {} - skipping re-negotiation",
                    required_buffer_type);
                return;
            }

            if previous_buffer_type != 0 {
                eprintln!("[PIPEWIRE_DEBUG] Re-negotiating: buffer_type {} -> {}",
                    previous_buffer_type, required_buffer_type);
            }

            // Build negotiation params like gnome-remote-desktop does.
            // gnome-remote-desktop includes a Format confirmation param in update_params.
            // This confirms to GNOME which format was selected and stops the renegotiation loop.
            // We also include Buffers + Meta params.
            //
            // use_dmabuf is determined above based on whether GPU/EGL is available.
            // When GPU isn't accessible, we fall back to MemFd (SHM) for software encoding.
            let negotiation_params = build_negotiation_params(width, height, format_raw, modifier, use_dmabuf);

            if !negotiation_params.is_empty() {
                // Convert byte buffers to Pod references
                // Pod::from_bytes returns Option<&Pod>, so we collect references
                let mut pod_refs: Vec<&Pod> = negotiation_params.iter()
                    .filter_map(|bytes| Pod::from_bytes(bytes))
                    .collect();

                if !pod_refs.is_empty() {
                    eprintln!("[PIPEWIRE_DEBUG] Calling update_params with {} params (Buffers + Meta only - pipewiresrc pattern)", pod_refs.len());
                    if let Err(e) = stream.update_params(&mut pod_refs) {
                        tracing::error!("[PIPEWIRE_DEBUG] update_params failed: {}", e);
                    } else {
                        eprintln!("[PIPEWIRE_DEBUG] update_params succeeded - negotiation complete");

                        // gnome-remote-desktop pattern: call set_active AFTER update_params
                        // This triggers the transition from Paused to Streaming
                        if !stream_activated_clone.swap(true, Ordering::SeqCst) {
                            eprintln!("[PIPEWIRE_DEBUG] Calling set_active(true) after update_params...");
                            if let Err(e) = stream.set_active(true) {
                                tracing::error!("[PIPEWIRE_DEBUG] set_active(true) failed: {}", e);
                            } else {
                                eprintln!("[PIPEWIRE_DEBUG] set_active(true) succeeded!");
                            }
                        }
                    }
                } else {
                    tracing::warn!("[PIPEWIRE_DEBUG] No valid negotiation params pods - stream may not start");
                }
            } else {
                tracing::warn!("[PIPEWIRE_DEBUG] No negotiation params built - stream may not start");
            }
        })
        .process(move |stream, _| {
            // Use a static counter for frame stats logging
            use std::sync::atomic::{AtomicU64, AtomicBool, Ordering};
            static FRAME_COUNT: AtomicU64 = AtomicU64::new(0);
            static LAST_LOG: AtomicU64 = AtomicU64::new(0);
            static LOGGED_START: AtomicBool = AtomicBool::new(false);
            static PROCESS_COUNT: AtomicU64 = AtomicU64::new(0);

            let pcount = PROCESS_COUNT.fetch_add(1, Ordering::Relaxed) + 1;
            if pcount == 1 {
                eprintln!("[PIPEWIRE_DEBUG] PROCESS callback called for first time!");
            }
            if pcount <= 5 || pcount % 100 == 0 {
                eprintln!("[PIPEWIRE_DEBUG] process callback #{}", pcount);
            }

            // Use raw buffer access to extract PTS from spa_meta_header
            // The PTS is set by the compositor when the frame was captured
            let pw_buffer = unsafe { stream.dequeue_raw_buffer() };
            if pw_buffer.is_null() {
                // dequeue_buffer returned None - stream might not be in Streaming state yet
                static DEQUEUE_FAIL_COUNT: std::sync::atomic::AtomicU32 = std::sync::atomic::AtomicU32::new(0);
                let fail_count = DEQUEUE_FAIL_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed) + 1;
                if fail_count <= 5 || fail_count % 100 == 0 {
                    eprintln!("[PIPEWIRE_DEBUG] process #{}: dequeue_buffer returned None (no buffer available)", pcount);
                }
                return;
            }

            let spa_buffer = unsafe { (*pw_buffer).buffer };
            if spa_buffer.is_null() {
                eprintln!("[PIPEWIRE_DEBUG] process #{}: pw_buffer.buffer is null!", pcount);
                unsafe { stream.queue_raw_buffer(pw_buffer) };
                return;
            }

            // Extract PTS from buffer metadata (compositor timestamp in nanoseconds)
            let pts_ns = unsafe { extract_pts_from_buffer(spa_buffer) };

            // Detect out-of-order frames from compositor (PTS should be monotonically increasing)
            static LAST_PTS: std::sync::atomic::AtomicI64 = std::sync::atomic::AtomicI64::new(0);
            static OOO_COUNT: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);
            let prev_pts = LAST_PTS.swap(pts_ns, Ordering::SeqCst);
            if pts_ns > 0 && prev_pts > 0 && pts_ns < prev_pts {
                let ooo = OOO_COUNT.fetch_add(1, Ordering::Relaxed) + 1;
                eprintln!("[PIPEWIRE_OOO] OUT OF ORDER FRAME! #{} prev_pts={} curr_pts={} delta={}ns",
                    ooo, prev_pts, pts_ns, prev_pts - pts_ns);
            }

            // Note: Cursor metadata is handled by Go PipeWire client via separate session
            // The Rust plugin only handles video frames

            // Get datas from spa_buffer
            let n_datas = unsafe { (*spa_buffer).n_datas };
            if n_datas == 0 {
                // No video data in this buffer (empty buffer)
                unsafe { stream.queue_raw_buffer(pw_buffer) };
                return;
            }

            let datas = unsafe {
                std::slice::from_raw_parts_mut(
                    (*spa_buffer).datas as *mut pipewire::spa::buffer::Data,
                    n_datas as usize,
                )
            };

            let params = video_info.lock().clone();
            if let Some(frame) = extract_frame(datas, &params, pts_ns) {
                let count = FRAME_COUNT.fetch_add(1, Ordering::Relaxed) + 1;

                // Log first frame with PTS
                if !LOGGED_START.swap(true, Ordering::Relaxed) {
                    tracing::warn!("[PIPEWIRE_FRAME] First frame received from PipeWire ({}x{}) pts_ns={}",
                        params.width, params.height, pts_ns);
                }

                // Log every 100th frame or every 5 seconds
                let now = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .map(|d| d.as_secs())
                    .unwrap_or(0);
                let last = LAST_LOG.load(Ordering::Relaxed);
                if count % 100 == 0 || (now > last + 5) {
                    LAST_LOG.store(now, Ordering::Relaxed);
                    tracing::warn!("[PIPEWIRE_FRAME] Frame #{} received from PipeWire pts_ns={}", count, pts_ns);
                }

                // CRITICAL FIX: Process DmaBuf through CUDA BEFORE returning buffer to PipeWire.
                // This prevents the race condition where Mutter reuses the GPU buffer
                // while we're still reading from it in a different thread.
                let frame_to_send = match (&frame, cuda_resources_process.as_ref()) {
                    (FrameData::DmaBuf { dmabuf, pts_ns }, Some(cuda_res)) => {
                        // Do CUDA processing in PipeWire thread, synchronously
                        match process_dmabuf_to_cuda(dmabuf, cuda_res, *pts_ns, &params) {
                            Ok(cuda_frame) => Some(cuda_frame),
                            Err(e) => {
                                // Log error and drop frame - no fallback, corruption is worse
                                static CUDA_ERROR_COUNT: std::sync::atomic::AtomicU32 =
                                    std::sync::atomic::AtomicU32::new(0);
                                let err_count = CUDA_ERROR_COUNT.fetch_add(1, Ordering::Relaxed) + 1;
                                if err_count <= 10 || err_count % 100 == 0 {
                                    eprintln!("[PIPEWIRE_CUDA] ERROR: CUDA processing failed ({}): {}", err_count, e);
                                }
                                None  // Drop frame
                            }
                        }
                    }
                    _ => Some(frame),
                };

                if let Some(f) = frame_to_send {
                    let _ = frame_tx_process.try_send(f);
                }
            } else {
                // extract_frame returned None - log why (first 5 times only to avoid spam)
                static EXTRACT_FAIL_COUNT: std::sync::atomic::AtomicU32 = std::sync::atomic::AtomicU32::new(0);
                let fail_count = EXTRACT_FAIL_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed) + 1;
                if fail_count <= 5 {
                    let first = datas.first();
                    let data_type = first.map(|d| d.type_());
                    let chunk_size = first.map(|d| d.chunk().size());
                    eprintln!("[PIPEWIRE_DEBUG] process #{}: extract_frame returned None! params={}x{} fmt=0x{:x} data_type={:?} chunk_size={:?}",
                        pcount, params.width, params.height, params.format, data_type, chunk_size);
                }
            }

            // Re-queue the buffer AFTER CUDA copy is complete.
            // This is safe now because the GPU data has been copied to a CUDA buffer.
            unsafe { stream.queue_raw_buffer(pw_buffer) };
        })
        .register()
        .map_err(|e| format!("Listener: {}", e))?;

    // Build format pods based on DMA-BUF capabilities
    //
    // ZERO-COPY GPU PATH (2026-01-11):
    // We now offer NVIDIA ScreenCast modifiers (0xe08xxx family) directly, which
    // forces GNOME to pick a GPU-tiled modifier instead of LINEAR.
    // This enables true zero-copy DmaBuf path at 60 FPS.
    //
    // SHM PATH for Sway (xdg-desktop-portal-wlr):
    // Portal offers TWO format types that require DIFFERENT format pods:
    // 1. BGRx WITH modifiers (including LINEAR 0x0)
    // 2. RGB WITHOUT any modifier field
    // We must offer BOTH to ensure negotiation succeeds.
    let format_pods: Vec<Vec<u8>> = if let Some(ref caps) = dmabuf_caps {
        if caps.dmabuf_available && !caps.modifiers.is_empty() {
            eprintln!("[PIPEWIRE_DEBUG] DMA-BUF available with {} modifiers, offering DmaBuf ONLY (no SHM fallback)",
                caps.modifiers.len());
            // Offer format WITH modifiers ONLY (DMA-BUF, no SHM fallback)
            let format_with_mod =
                build_video_format_params_with_modifiers(&caps.modifiers, negotiated_max_fps);
            vec![format_with_mod]
        } else {
            eprintln!("[PIPEWIRE_DEBUG] DMA-BUF not available, offering SHM (two format pods for xdg-desktop-portal-wlr)");
            // Offer BOTH format pods for xdg-desktop-portal-wlr compatibility:
            // 1. BGRx/BGRA WITH LINEAR modifier (matches portal's first offer)
            // 2. RGB WITHOUT modifier (matches portal's second offer)
            vec![
                build_video_format_params_with_linear_modifier(negotiated_max_fps),
                build_video_format_params_no_modifier(negotiated_max_fps),
            ]
        }
    } else {
        eprintln!("[PIPEWIRE_DEBUG] No DMA-BUF caps provided, offering SHM (two format pods for xdg-desktop-portal-wlr)");
        // Offer BOTH format pods for xdg-desktop-portal-wlr compatibility
        vec![
            build_video_format_params_with_linear_modifier(negotiated_max_fps),
            build_video_format_params_no_modifier(negotiated_max_fps),
        ]
    };

    // Convert to Pod references
    let pod_refs: Vec<&Pod> = format_pods
        .iter()
        .filter_map(|bytes| Pod::from_bytes(bytes))
        .collect();

    if pod_refs.is_empty() {
        return Err("Failed to create any format pods".to_string());
    }

    // Need to convert to mutable slice for stream.connect()
    let mut params: Vec<&Pod> = pod_refs;
    eprintln!("[PIPEWIRE_DEBUG] Submitting {} format pod(s)", params.len());

    tracing::info!(
        "Connecting to PipeWire node {} with framerate range 0-360fps",
        node_id
    );

    // Use AUTOCONNECT | MAP_BUFFERS like pipewiresrc does.
    //
    // AUTOCONNECT: Automatically connect to the source node
    // MAP_BUFFERS: Map buffer data for CPU access (needed for SHM fallback)
    //
    // NOTE: Do NOT use RT_PROCESS! pipewiresrc doesn't use it for ScreenCast,
    // and it may cause GNOME ScreenCast to fail transitioning to Streaming state.
    // DONT_RECONNECT is not used by pipewiresrc for ScreenCast.
    stream
        .connect(
            pipewire::spa::utils::Direction::Input,
            Some(node_id),
            StreamFlags::AUTOCONNECT | StreamFlags::MAP_BUFFERS,
            &mut params,
        )
        .map_err(|e| format!("Connect to node {}: {}", node_id, e))?;

    tracing::info!("Connected to PipeWire node {}", node_id);

    // Main loop: iterate PipeWire events
    // set_active(true) is called in the state_changed callback when entering Paused state
    // (gnome-remote-desktop pattern: call set_active BEFORE format negotiation)
    //
    // CRITICAL: Use 1ms poll interval for low-latency frame delivery.
    // With 50ms, typing latency on static screens was 1-2 seconds because:
    // - Mutter's pending_process flag blocks subsequent damage until on_stream_process fires
    // - on_stream_process only fires when WE dequeue and return buffers
    // - Slow polling delays buffer return, blocking Mutter's frame clock
    while !shutdown.load(Ordering::SeqCst) {
        mainloop.loop_().iterate(Duration::from_millis(1));
    }

    Ok(())
}

/// Convert DRM fourcc (smithay Fourcc enum) to GStreamer VideoFormat.
/// This is needed for CUDA buffer creation in the PipeWire thread.
fn drm_fourcc_to_video_format(fourcc: Fourcc) -> VideoFormat {
    // Use the smithay Fourcc enum variants directly
    match fourcc {
        Fourcc::Argb8888 => VideoFormat::Bgra,
        Fourcc::Abgr8888 => VideoFormat::Rgba,
        Fourcc::Rgba8888 => VideoFormat::Abgr,
        Fourcc::Bgra8888 => VideoFormat::Argb,
        Fourcc::Xrgb8888 => VideoFormat::Bgrx,
        Fourcc::Xbgr8888 => VideoFormat::Rgbx,
        Fourcc::Rgbx8888 => VideoFormat::Xbgr,
        Fourcc::Bgrx8888 => VideoFormat::Xrgb,
        Fourcc::Bgr888 => VideoFormat::Rgb,
        Fourcc::Rgb888 => VideoFormat::Bgr,
        _ => {
            eprintln!("[PIPEWIRE_CUDA] Unknown DRM fourcc {:?}, defaulting to Bgra", fourcc);
            VideoFormat::Bgra
        }
    }
}

/// Process DmaBuf through CUDA in the PipeWire thread.
/// This does EGL import + CUDA copy synchronously BEFORE returning the buffer to PipeWire,
/// preventing the race condition where Mutter reuses the GPU buffer while we're reading.
fn process_dmabuf_to_cuda(
    dmabuf: &smithay::backend::allocator::dmabuf::Dmabuf,
    cuda_res: &CudaResources,
    pts_ns: i64,
    params: &VideoParams,
) -> Result<FrameData, String> {
    use smithay::backend::allocator::Buffer;

    let w = dmabuf.width() as u32;
    let h = dmabuf.height() as u32;

    // Get raw EGL display handle
    let raw_display: RawEGLDisplay = cuda_res.egl_display.get_display_handle().handle;

    // Step 1: Import DmaBuf as EGLImage
    let egl_image = EGLImage::from(dmabuf, &raw_display)
        .map_err(|e| format!("EGLImage::from failed: {}", e))?;

    // Step 2: Lock CUDA context and create CUDAImage
    let cuda_ctx = cuda_res.cuda_context.lock()
        .map_err(|e| format!("Failed to lock CUDA context: {}", e))?;

    let cuda_image = CUDAImage::from(egl_image, &cuda_ctx)
        .map_err(|e| format!("CUDAImage::from failed: {}", e))?;

    // Step 3: Build video info for buffer allocation
    let drm_format = dmabuf.format();
    let video_format = drm_fourcc_to_video_format(drm_format.code);
    let base_info = VideoInfo::builder(video_format, w, h)
        .build()
        .map_err(|e| format!("VideoInfo build failed: {:?}", e))?;
    // VideoInfoDmaDrm expects u32 fourcc code
    let dma_video_info = VideoInfoDmaDrm::new(
        base_info,
        drm_format.code as u32,
        drm_format.modifier.into(),
    );

    // Step 4: Configure buffer pool on first use
    {
        let mut pool_state = cuda_res.buffer_pool_state.lock();
        if !pool_state.configured {
            if let Some(ref pool) = pool_state.pool {
                let pool_caps = VideoCapsBuilder::new()
                    .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                    .format(video_format)
                    .width(w as i32)
                    .height(h as i32)
                    .framerate(gst::Fraction::new(60, 1))
                    .build();
                let buffer_size = w * h * 4;
                if pool.configure_basic(&pool_caps, buffer_size, 8, 16).is_ok() {
                    if pool.activate().is_ok() {
                        pool_state.configured = true;
                        eprintln!("[PIPEWIRE_CUDA] Buffer pool configured: {}x{} {:?}", w, h, video_format);
                    }
                }
            }
        }
    }

    // Step 5: Do the actual CUDA copy (this is synchronous - waits for GPU)
    let pool_state = cuda_res.buffer_pool_state.lock();
    let buffer = cuda_image
        .to_gst_buffer(dma_video_info, &cuda_ctx, pool_state.pool.as_ref())
        .map_err(|e| format!("to_gst_buffer failed: {}", e))?;
    drop(pool_state);

    // Step 6: CUDAImage drops here, which calls cuGraphicsUnregisterResource
    // This is safe because the CUDA copy is complete (synchronous)
    drop(cuda_image);
    drop(cuda_ctx);

    // Return the CUDA buffer
    Ok(FrameData::CudaBuffer {
        buffer,
        width: w,
        height: h,
        format: video_format,
        pts_ns,
    })
}

/// Extract DRM fourcc code from SPA video format - exposed for testing
pub fn spa_format_to_drm_fourcc(format: spa::param::video::VideoFormat) -> u32 {
    spa_video_format_to_drm_fourcc(format)
}

/// Build negotiation params like pipewiresrc does in on_stream_param_changed().
/// Returns a list of param byte buffers: [VideoCrop, Buffers, Header]
/// Note: Cursor metadata is handled by Go-side clients, not by this plugin.
///
/// NOTE: We do NOT include a Format param here. pipewiresrc doesn't send Format
/// in update_params - it only sends Buffers + Meta params. Including Format
/// in update_params causes GNOME to restart negotiation (endless loop).
///
/// The stream accepts the format by simply calling update_params with buffer
/// requirements, NOT by echoing back a Format param.
///
/// # Arguments
/// * `use_dmabuf` - If true, request DmaBuf buffer type (GPU-tiled modifiers).
///                  If false, request MemFd buffer type (LINEAR or SHM).
fn build_negotiation_params(
    width: u32,
    height: u32,
    _spa_format: u32,
    _modifier: u64,
    use_dmabuf: bool,
) -> Vec<Vec<u8>> {
    let mut params = Vec::new();

    // NOTE: Do NOT include Format param here!
    // pipewiresrc pattern: only send Buffers + Meta params in update_params.
    // Including Format causes GNOME to restart format negotiation.
    eprintln!("[PIPEWIRE_DEBUG] Building negotiation params WITHOUT Format (pipewiresrc pattern)");

    // Constants from spa/param/buffers.h (enum spa_param_meta)
    // These are simple enum values starting from 0:
    //   SPA_PARAM_META_START = 0
    //   SPA_PARAM_META_type = 1  (type of metadata)
    //   SPA_PARAM_META_size = 2  (expected max size)
    const SPA_PARAM_META_TYPE: u32 = 1;
    const SPA_PARAM_META_SIZE: u32 = 2;

    // Meta types from spa/buffer/meta.h
    const SPA_META_HEADER: u32 = 1; // struct spa_meta_header (24 bytes)
    const SPA_META_VIDEOCROP: u32 = 2; // struct spa_meta_region (16 bytes)
    const SPA_META_CURSOR: u32 = 5; // struct spa_meta_cursor + optional bitmap

    // Sizes
    const SPA_META_HEADER_SIZE: i32 = 24; // sizeof(struct spa_meta_header)
    const SPA_META_REGION_SIZE: i32 = 16; // sizeof(struct spa_meta_region) = 4 ints
    // Cursor meta size: spa_meta_cursor (28) + spa_meta_bitmap header (20) + 256x256 ARGB (262144)
    // Use generous size to accommodate large cursors
    const SPA_META_CURSOR_SIZE: i32 = 28 + 20 + 256 * 256 * 4;

    eprintln!(
        "[PIPEWIRE_DEBUG] build_negotiation_params: {}x{} use_dmabuf={}",
        width, height, use_dmabuf
    );

    // 1. VideoCrop meta (like OBS)
    let videocrop_obj = Object {
        type_: SpaTypes::ObjectParamMeta.as_raw(),
        id: ParamType::Meta.as_raw(),
        properties: vec![
            Property {
                key: SPA_PARAM_META_TYPE,
                flags: PropertyFlags::empty(),
                value: Value::Id(Id(SPA_META_VIDEOCROP)),
            },
            Property {
                key: SPA_PARAM_META_SIZE,
                flags: PropertyFlags::empty(),
                value: Value::Int(SPA_META_REGION_SIZE),
            },
        ],
    };
    if let Ok((cursor, _)) =
        PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(videocrop_obj))
    {
        params.push(cursor.into_inner());
    }

    // 2. Buffers param - ONLY dataType (like gnome-remote-desktop)
    //
    // CRITICAL: gnome-remote-desktop's add_param_buffers_param() ONLY includes dataType:
    //   params[(*n_params)++] = spa_pod_builder_add_object(pod_builder,
    //       SPA_TYPE_OBJECT_ParamBuffers, SPA_PARAM_Buffers,
    //       SPA_PARAM_BUFFERS_dataType, SPA_POD_CHOICE_FLAGS_Int(allowed_buffer_types));
    //
    // Our previous code included buffers, blocks, size, stride - this confused GNOME.
    // Simplify to match gnome-remote-desktop exactly.
    const SPA_PARAM_BUFFERS_DATATYPE: u32 = 6; // accepted data types

    // Buffer type bitmask (from spa/buffer/buffer.h):
    // SPA_DATA_MemFd = 2 (memory-mapped file descriptor - GNOME SHM)
    // SPA_DATA_DmaBuf = 3 (DMA-BUF fd - zero-copy GPU)
    //
    // CRITICAL: Always request BOTH buffer types and let PipeWire negotiate.
    // On AMD with GNOME headless, Mutter ONLY supports DmaBuf (not MemFd).
    // Our EGL check may return empty, but Mutter still requires DmaBuf.
    // By requesting both, PipeWire will pick the one Mutter supports.
    //
    // The use_dmabuf parameter now indicates our PREFERENCE:
    // - use_dmabuf=true: Prefer DmaBuf, fall back to MemFd
    // - use_dmabuf=false: Prefer MemFd, but still accept DmaBuf if Mutter requires it
    let buffer_types: i32 = (1 << 2) | (1 << 3); // MemFd | DmaBuf = 4 | 8 = 12
    let buffer_type_name = "MemFd+DmaBuf (let PipeWire negotiate)";
    eprintln!(
        "[PIPEWIRE_DEBUG] Buffer types: 0x{:x} ({}) - use_dmabuf={}",
        buffer_types, buffer_type_name, use_dmabuf
    );

    // Use FLAGS choice for dataType (like gnome-remote-desktop's SPA_POD_CHOICE_FLAGS_Int)
    // FLAGS choice tells PipeWire which buffer types we accept as a bitmask
    let buffer_types_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Flags {
            default: buffer_types,
            flags: vec![buffer_types],
        },
    );
    let buffer_obj = Object {
        type_: SpaTypes::ObjectParamBuffers.as_raw(),
        id: ParamType::Buffers.as_raw(),
        properties: vec![Property {
            key: SPA_PARAM_BUFFERS_DATATYPE,
            flags: PropertyFlags::empty(),
            value: Value::Choice(ChoiceValue::Int(buffer_types_choice)),
        }],
    };
    if let Ok((cursor, _)) =
        PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(buffer_obj))
    {
        eprintln!("[PIPEWIRE_DEBUG] Buffers param serialized with FLAGS choice");
        params.push(cursor.into_inner());
    }

    // 4. Header meta (like OBS - REQUIRED for GNOME to complete negotiation)
    let header_meta_obj = Object {
        type_: SpaTypes::ObjectParamMeta.as_raw(),
        id: ParamType::Meta.as_raw(),
        properties: vec![
            Property {
                key: SPA_PARAM_META_TYPE,
                flags: PropertyFlags::empty(),
                value: Value::Id(Id(SPA_META_HEADER)),
            },
            Property {
                key: SPA_PARAM_META_SIZE,
                flags: PropertyFlags::empty(),
                value: Value::Int(SPA_META_HEADER_SIZE),
            },
        ],
    };
    if let Ok((cursor, _)) =
        PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(header_meta_obj))
    {
        params.push(cursor.into_inner());
    }

    // 5. Cursor meta - request cursor metadata from GNOME ScreenCast
    // When cursor-mode=2 (Metadata) is set, GNOME will include cursor position
    // and bitmap in each frame buffer's metadata.
    //
    // OBS uses SPA_POD_CHOICE_RANGE_Int for cursor size:
    // CURSOR_META_SIZE(w,h) = sizeof(spa_meta_cursor) + sizeof(spa_meta_bitmap) + w*h*4
    // = 28 + 20 + w*h*4
    const CURSOR_META_SIZE_64: i32 = 28 + 20 + 64 * 64 * 4; // 16432
    const CURSOR_META_SIZE_1: i32 = 28 + 20 + 1 * 1 * 4; // 52
    const CURSOR_META_SIZE_256: i32 = 28 + 20 + 256 * 256 * 4; // 262192

    // Use CHOICE_RANGE like OBS: default=64x64, min=1x1, max=256x256
    let cursor_size_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: CURSOR_META_SIZE_64,
            min: CURSOR_META_SIZE_1,
            max: CURSOR_META_SIZE_256,
        },
    );
    let cursor_meta_obj = Object {
        type_: SpaTypes::ObjectParamMeta.as_raw(),
        id: ParamType::Meta.as_raw(),
        properties: vec![
            Property {
                key: SPA_PARAM_META_TYPE,
                flags: PropertyFlags::empty(),
                value: Value::Id(Id(SPA_META_CURSOR)),
            },
            Property {
                key: SPA_PARAM_META_SIZE,
                flags: PropertyFlags::empty(),
                value: Value::Choice(ChoiceValue::Int(cursor_size_choice)),
            },
        ],
    };
    if let Ok((cursor, _)) =
        PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(cursor_meta_obj))
    {
        params.push(cursor.into_inner());
        eprintln!("[PIPEWIRE_DEBUG] Added cursor meta param (RANGE size: default={}, min={}, max={})",
            CURSOR_META_SIZE_64, CURSOR_META_SIZE_1, CURSOR_META_SIZE_256);
    }

    eprintln!("[PIPEWIRE_DEBUG] Built {} negotiation params (VideoCrop + Buffers[dataType=0x{:x}] + Header + Cursor) - NO Format",
        params.len(), buffer_types);
    params
}

/// Build a fixated format pod to confirm the negotiated format back to PipeWire.
///
/// CRITICAL: gnome-remote-desktop includes this in update_params() after receiving
/// the Format callback. This confirms "I accept this specific format" to GNOME.
///
/// From grd-rdp-pipewire-stream.c add_param_format_param():
/// - Uses SPA_PARAM_EnumFormat (not Format!)
/// - modifier has MANDATORY | DONT_FIXATE flags
/// - framerate is a CHOICE_RANGE
/// - maxFramerate is also included
fn build_fixated_format_param(format: u32, width: u32, height: u32, modifier: u64) -> Vec<u8> {
    use spa::utils::Rectangle;

    let mut properties = Vec::new();

    // Media type: Video
    properties.push(Property {
        key: FormatProperties::MediaType.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaType::Video.as_raw())),
    });

    // Media subtype: Raw
    properties.push(Property {
        key: FormatProperties::MediaSubtype.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaSubtype::Raw.as_raw())),
    });

    // Video format: The exact negotiated format (fixed Id, not a choice)
    properties.push(Property {
        key: FormatProperties::VideoFormat.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(format)),
    });

    // Video modifier: use DONT_FIXATE flag only (not MANDATORY)
    // MANDATORY caused "no more input formats" error with GNOME
    // DONT_FIXATE (0x10): Don't lock to a single value during negotiation
    let modifier_flags = PropertyFlags::from_bits_retain(0x10);
    properties.push(Property {
        key: FormatProperties::VideoModifier.as_raw(),
        flags: modifier_flags,
        value: Value::Long(modifier as i64),
    });
    eprintln!(
        "[PIPEWIRE_DEBUG] Fixated format modifier: 0x{:x} (flags: MANDATORY|DONT_FIXATE)",
        modifier
    );

    // Video size: The exact negotiated dimensions (fixed Rectangle)
    properties.push(Property {
        key: FormatProperties::VideoSize.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Rectangle(Rectangle { width, height }),
    });

    // Framerate: Fixed value matching what GNOME offered (0/1 = variable/damage-based)
    // For Format confirmation, use fixed values, not Choice ranges!
    properties.push(Property {
        key: FormatProperties::VideoFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Fraction(Fraction { num: 0, denom: 1 }),
    });

    // MaxFramerate: Used by GNOME for frame pacing in damage-based streaming.
    // When framerate=0/1 (damage-based), GNOME uses max_framerate for frame pacing.
    //
    // HYPOTHESIS (2026-01-11): We observed ~50% of frames being skipped as "too early"
    // in Mutter logs. The logs showed frames at alternating 16ms and 17ms intervals,
    // with 16ms frames being rejected. With max_framerate=60/1, min_interval = 16.666ms,
    // so 16ms frames would fail the check: time_since_last < min_interval.
    //
    // Testing: Using 120/1 instead of 60/1 gives min_interval = 8.33ms.
    // If this hypothesis is correct, both 16ms and 17ms intervals should pass,
    // potentially doubling effective frame rate from ~30fps to ~60fps.
    //
    // See: mutter's meta-screen-cast-stream-src.c frame pacing logic:
    //   if (priv->video_format.max_framerate.num > 0)
    //     min_interval_us = G_USEC_PER_SEC * denom / num;
    properties.push(Property {
        key: FormatProperties::VideoMaxFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Fraction(Fraction { num: 120, denom: 1 }),
    });

    // Create the format object - use Format (not EnumFormat) for confirmation
    // EnumFormat is for offering multiple options, Format is for confirming a single choice
    let obj = Object {
        type_: SpaTypes::ObjectParamFormat.as_raw(),
        id: ParamType::Format.as_raw(), // Format for confirmation, not EnumFormat!
        properties,
    };

    // Serialize to bytes
    match PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(obj)) {
        Ok((cursor, _len)) => {
            let bytes = cursor.into_inner();
            eprintln!(
                "[PIPEWIRE_DEBUG] Built fixated format: {}x{} format={} modifier=0x{:x} ({} bytes)",
                width,
                height,
                format,
                modifier,
                bytes.len()
            );
            bytes
        }
        Err(e) => {
            eprintln!(
                "[PIPEWIRE_DEBUG] ERROR: Failed to serialize fixated format: {:?}",
                e
            );
            Vec::new()
        }
    }
}

/// Build a single format pod (like OBS's build_format function).
/// Creates one EnumFormat pod for a specific video format with optional modifier.
fn build_single_format_pod(format: u32, with_modifier: bool) -> Vec<u8> {
    let mut properties = Vec::new();

    // Media type: Video
    properties.push(Property {
        key: FormatProperties::MediaType.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaType::Video.as_raw())),
    });

    // Media subtype: Raw
    properties.push(Property {
        key: FormatProperties::MediaSubtype.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaSubtype::Raw.as_raw())),
    });

    // Video format (single specific format, not an enum)
    properties.push(Property {
        key: FormatProperties::VideoFormat.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(format)),
    });

    // Modifier for DMA-BUF (like OBS does)
    // DRM_FORMAT_MOD_INVALID = ((1ULL << 56) - 1) = 0x00ffffffffffffff
    // This is the "implicit modifier" that tells the driver to choose
    if with_modifier {
        const DRM_FORMAT_MOD_INVALID: i64 = ((1i64 << 56) - 1);

        // OBS uses MANDATORY | DONT_FIXATE flags
        // MANDATORY (0x08): This property must be present in negotiated format
        // DONT_FIXATE (0x10): Don't lock to a single value during negotiation
        let modifier_flags = PropertyFlags::from_bits_retain(0x18);

        // Build an enum choice with the modifier
        // For enum, first value is preferred/default, rest are alternatives
        let modifier_choice = Choice(
            ChoiceFlags::empty(),
            ChoiceEnum::Enum {
                default: DRM_FORMAT_MOD_INVALID,
                alternatives: vec![DRM_FORMAT_MOD_INVALID],
            },
        );
        properties.push(Property {
            key: FormatProperties::VideoModifier.as_raw(),
            flags: modifier_flags,
            value: Value::Choice(ChoiceValue::Long(modifier_choice)),
        });
    }

    // Framerate: Range from 0/1 to 360/1, default 60/1
    let framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction { num: 60, denom: 1 },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(framerate_choice)),
    });

    // MaxFramerate: Range for damage-based streaming
    // When GNOME picks framerate=0/1, it uses maxFramerate for frame pacing
    // CRITICAL: min must be 0/1 to DISABLE Mutter's frame pacing entirely.
    // With min=1/1, Mutter clamps to 1 FPS! With min=0/1, the frame pacing check is bypassed.
    // See: meta-screen-cast-stream-src.c:1322 - if (max_framerate.num > 0) then limit applies
    let max_framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction { num: 0, denom: 1 },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoMaxFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(max_framerate_choice)),
    });

    // Create the format object
    let obj = Object {
        type_: SpaTypes::ObjectParamFormat.as_raw(),
        id: ParamType::EnumFormat.as_raw(),
        properties,
    };

    // Serialize to bytes
    match PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(obj)) {
        Ok((cursor, _len)) => cursor.into_inner(),
        Err(e) => {
            tracing::error!("Failed to serialize format pod: {:?}", e);
            Vec::new()
        }
    }
}

/// Build video format params with GPU-queried modifiers for DMA-BUF zero-copy.
/// This is the primary format pod - offers DMA-BUF with actual GPU-supported modifiers.
///
/// # Arguments
/// * `modifiers` - GPU-supported DMA-BUF modifiers from EGL
/// * `negotiated_max_fps` - Max framerate to negotiate with Mutter (should be target_fps * 2)
fn build_video_format_params_with_modifiers(modifiers: &[u64], negotiated_max_fps: u32) -> Vec<u8> {
    use spa::param::video::VideoFormat;
    let spa_bgra = VideoFormat::BGRA.as_raw();
    let spa_rgba = VideoFormat::RGBA.as_raw();
    let spa_bgrx = VideoFormat::BGRx.as_raw();
    let spa_rgbx = VideoFormat::RGBx.as_raw();

    eprintln!(
        "[PIPEWIRE_DEBUG] Building format pod with {} GPU modifiers",
        modifiers.len()
    );
    for (i, m) in modifiers.iter().enumerate().take(5) {
        eprintln!("[PIPEWIRE_DEBUG]   modifier[{}] = 0x{:x}", i, m);
    }
    if modifiers.len() > 5 {
        eprintln!("[PIPEWIRE_DEBUG]   ... and {} more", modifiers.len() - 5);
    }

    let mut properties = Vec::new();

    // Media type: Video
    properties.push(Property {
        key: FormatProperties::MediaType.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaType::Video.as_raw())),
    });

    // Media subtype: Raw
    properties.push(Property {
        key: FormatProperties::MediaSubtype.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaSubtype::Raw.as_raw())),
    });

    // Video format: Enum choice of all supported formats
    let format_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Enum {
            default: Id(spa_bgra),
            alternatives: vec![Id(spa_bgra), Id(spa_rgba), Id(spa_bgrx), Id(spa_rgbx)],
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFormat.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Id(format_choice)),
    });

    // Add modifiers from GPU query
    // MANDATORY | DONT_FIXATE flags tell PipeWire this must be present and can vary
    let modifier_flags = PropertyFlags::from_bits_retain(0x18); // MANDATORY | DONT_FIXATE

    // Convert u64 modifiers to i64 for spa pod
    let mod_i64: Vec<i64> = modifiers.iter().map(|&m| m as i64).collect();
    let default_mod = mod_i64.first().copied().unwrap_or(0);

    let modifier_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Enum {
            default: default_mod,
            alternatives: mod_i64,
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoModifier.as_raw(),
        flags: modifier_flags,
        value: Value::Choice(ChoiceValue::Long(modifier_choice)),
    });

    // Size: Range from 1x1 to 8192x4320
    use spa::utils::Rectangle;
    let size_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Rectangle {
                width: 1920,
                height: 1080,
            },
            min: Rectangle {
                width: 1,
                height: 1,
            },
            max: Rectangle {
                width: 8192,
                height: 4320,
            },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoSize.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Rectangle(size_choice)),
    });

    // Framerate: Range from 0/1 to 360/1
    let framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction { num: 60, denom: 1 },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(framerate_choice)),
    });

    // MaxFramerate: Request 0/1 to DISABLE Mutter's frame rate limiter entirely.
    // When max_framerate.num == 0, the check in meta-screen-cast-stream-src.c:1322 is skipped.
    // CRITICAL: min must be 0/1 to allow 0 to be negotiated - if min is 1/1, Mutter clamps to 1fps!
    let max_framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction {
                num: negotiated_max_fps,
                denom: 1,
            },
            min: Fraction { num: 0, denom: 1 }, // Allow 0 to disable limiter!
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoMaxFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(max_framerate_choice)),
    });

    // Create the format object
    let obj = Object {
        type_: SpaTypes::ObjectParamFormat.as_raw(),
        id: ParamType::EnumFormat.as_raw(),
        properties,
    };

    // Serialize to bytes
    match PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(obj)) {
        Ok((cursor, _len)) => {
            let bytes = cursor.into_inner();
            eprintln!(
                "[PIPEWIRE_DEBUG] Built format pod with {} GPU modifiers, max_fps={} ({} bytes)",
                modifiers.len(),
                negotiated_max_fps,
                bytes.len()
            );
            bytes
        }
        Err(e) => {
            eprintln!(
                "[PIPEWIRE_DEBUG] ERROR: Failed to serialize format pod with modifiers: {:?}",
                e
            );
            Vec::new()
        }
    }
}

/// Build video format params with multiple formats as an enum choice.
/// This allows PipeWire to negotiate ANY format we support with what GNOME offers.
#[allow(dead_code)]
fn build_video_format_params() -> Vec<u8> {
    // Use VideoFormat enum's as_raw() to get correct SPA format IDs
    // PipeWire's actual values: BGRx=8, BGRA=12, RGBx=10, RGBA=14 (NOT 2,4,5,6!)
    use spa::param::video::VideoFormat;
    let spa_bgra = VideoFormat::BGRA.as_raw();
    let spa_rgba = VideoFormat::RGBA.as_raw();
    let spa_bgrx = VideoFormat::BGRx.as_raw();
    let spa_rgbx = VideoFormat::RGBx.as_raw();

    eprintln!(
        "[PIPEWIRE_DEBUG] SPA format IDs: BGRA={}, RGBA={}, BGRx={}, RGBx={}",
        spa_bgra, spa_rgba, spa_bgrx, spa_rgbx
    );

    let mut properties = Vec::new();

    // Media type: Video
    properties.push(Property {
        key: FormatProperties::MediaType.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaType::Video.as_raw())),
    });

    // Media subtype: Raw
    properties.push(Property {
        key: FormatProperties::MediaSubtype.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaSubtype::Raw.as_raw())),
    });

    // Video format: Enum choice of all supported formats
    // This tells PipeWire "I accept any of these formats, prefer BGRA"
    // GNOME offers BGRx(8) and BGRA(12) with modifiers, so match those
    let format_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Enum {
            default: Id(spa_bgra),
            alternatives: vec![Id(spa_bgra), Id(spa_rgba), Id(spa_bgrx), Id(spa_rgbx)],
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFormat.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Id(format_choice)),
    });

    // Add modifier for DMA-BUF support
    // GNOME offers formats with specific NVIDIA modifiers. We need to match at least one.
    // Options:
    // - DRM_FORMAT_MOD_LINEAR (0x0): Linear layout, widely supported
    // - DRM_FORMAT_MOD_INVALID (0x00ffffffffffffff): Implicit modifier
    //
    // Use DONT_FIXATE flag (no MANDATORY) so negotiation can fall back if needed.
    // This tells PipeWire "I prefer DmaBuf with linear, but can accept other modifiers"
    const DRM_FORMAT_MOD_LINEAR: i64 = 0;
    const DRM_FORMAT_MOD_INVALID: i64 = ((1i64 << 56) - 1);

    // Use DONT_FIXATE without MANDATORY - allows fallback
    let modifier_flags = PropertyFlags::from_bits_retain(0x10); // DONT_FIXATE only

    // Build an enum choice with both linear and implicit modifiers
    let modifier_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Enum {
            default: DRM_FORMAT_MOD_LINEAR,
            alternatives: vec![DRM_FORMAT_MOD_LINEAR, DRM_FORMAT_MOD_INVALID],
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoModifier.as_raw(),
        flags: modifier_flags,
        value: Value::Choice(ChoiceValue::Long(modifier_choice)),
    });
    eprintln!(
        "[PIPEWIRE_DEBUG] Added modifiers: LINEAR(0x0) + INVALID(0x{:x}) with DONT_FIXATE",
        DRM_FORMAT_MOD_INVALID
    );

    // Size: Range from 1x1 to 8192x4320 (like OBS does)
    // This is REQUIRED for format negotiation with GNOME ScreenCast
    use spa::utils::Rectangle;
    let size_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Rectangle {
                width: 1920,
                height: 1080,
            },
            min: Rectangle {
                width: 1,
                height: 1,
            },
            max: Rectangle {
                width: 8192,
                height: 4320,
            },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoSize.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Rectangle(size_choice)),
    });

    // Framerate: Range from 0/1 to 360/1, default 60/1
    let framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction { num: 60, denom: 1 },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(framerate_choice)),
    });

    // MaxFramerate: Range for damage-based streaming
    // CRITICAL: min must be 0/1 to DISABLE Mutter's frame pacing entirely.
    // With min=1/1, Mutter clamps to 1 FPS! With min=0/1, the frame pacing check is bypassed.
    let max_framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction { num: 0, denom: 1 },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoMaxFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(max_framerate_choice)),
    });

    // Create the format object
    let obj = Object {
        type_: SpaTypes::ObjectParamFormat.as_raw(),
        id: ParamType::EnumFormat.as_raw(),
        properties,
    };

    // Serialize to bytes
    match PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(obj)) {
        Ok((cursor, _len)) => {
            let bytes = cursor.into_inner();
            eprintln!(
                "[PIPEWIRE_DEBUG] Built format pod with LINEAR+INVALID modifiers ({} bytes)",
                bytes.len()
            );
            bytes
        }
        Err(e) => {
            eprintln!(
                "[PIPEWIRE_DEBUG] ERROR: Failed to serialize format pod: {:?}",
                e
            );
            Vec::new()
        }
    }
}

/// Build video format params WITH LINEAR modifier (for SHM fallback with modifier support).
/// xdg-desktop-portal-wlr offers BGRx WITH modifier 0 (LINEAR), so we need a matching offer.
///
/// # Arguments
/// * `negotiated_max_fps` - Max framerate to negotiate
fn build_video_format_params_with_linear_modifier(negotiated_max_fps: u32) -> Vec<u8> {
    use spa::param::video::VideoFormat;
    let spa_bgra = VideoFormat::BGRA.as_raw();
    let spa_rgba = VideoFormat::RGBA.as_raw();
    let spa_bgrx = VideoFormat::BGRx.as_raw();
    let spa_rgbx = VideoFormat::RGBx.as_raw();

    eprintln!("[PIPEWIRE_DEBUG] LINEAR modifier format - SPA format IDs: BGRA={}, RGBA={}, BGRx={}, RGBx={}",
        spa_bgra, spa_rgba, spa_bgrx, spa_rgbx);

    let mut properties = Vec::new();

    // Media type: Video
    properties.push(Property {
        key: FormatProperties::MediaType.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaType::Video.as_raw())),
    });

    // Media subtype: Raw
    properties.push(Property {
        key: FormatProperties::MediaSubtype.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaSubtype::Raw.as_raw())),
    });

    // Video format: 32-bit formats that typically have modifier support
    let format_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Enum {
            default: Id(spa_bgrx),
            alternatives: vec![Id(spa_bgrx), Id(spa_bgra), Id(spa_rgbx), Id(spa_rgba)],
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFormat.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Id(format_choice)),
    });

    // LINEAR modifier (0x0) - matches xdg-desktop-portal-wlr's BGRx offer
    // Also include DRM_FORMAT_MOD_INVALID as fallback (driver picks best option)
    //
    // CRITICAL: Must use Choice, not single value! PipeWire format negotiation
    // expects modifiers as an enum choice, even with one option. Using a single
    // Long value breaks negotiation with xdg-desktop-portal-wlr.
    const DRM_FORMAT_MOD_LINEAR: i64 = 0;
    const DRM_FORMAT_MOD_INVALID: i64 = ((1i64 << 56) - 1); // 0x00ffffffffffffff

    // Use MANDATORY | DONT_FIXATE flags like GPU modifier path
    let modifier_flags = PropertyFlags::from_bits_retain(0x18);

    let modifier_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Enum {
            default: DRM_FORMAT_MOD_LINEAR,
            alternatives: vec![DRM_FORMAT_MOD_LINEAR, DRM_FORMAT_MOD_INVALID],
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoModifier.as_raw(),
        flags: modifier_flags,
        value: Value::Choice(ChoiceValue::Long(modifier_choice)),
    });
    eprintln!(
        "[PIPEWIRE_DEBUG] Modifier choice: LINEAR(0x0) + INVALID(0x{:x})",
        DRM_FORMAT_MOD_INVALID
    );

    // Size: Range from 1x1 to 8192x4320
    use spa::utils::Rectangle;
    let size_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Rectangle {
                width: 1920,
                height: 1080,
            },
            min: Rectangle {
                width: 1,
                height: 1,
            },
            max: Rectangle {
                width: 8192,
                height: 4320,
            },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoSize.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Rectangle(size_choice)),
    });

    // Framerate: Range from 0/1 to 360/1
    let framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction { num: 60, denom: 1 },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(framerate_choice)),
    });

    // MaxFramerate
    let max_framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction {
                num: negotiated_max_fps,
                denom: 1,
            },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoMaxFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(max_framerate_choice)),
    });

    // Create the format object
    let obj = Object {
        type_: SpaTypes::ObjectParamFormat.as_raw(),
        id: ParamType::EnumFormat.as_raw(),
        properties,
    };

    // Serialize to bytes
    match PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(obj)) {
        Ok((cursor, _len)) => {
            let bytes = cursor.into_inner();
            eprintln!(
                "[PIPEWIRE_DEBUG] Built format pod WITH LINEAR modifier (0x0) ({} bytes)",
                bytes.len()
            );
            bytes
        }
        Err(e) => {
            eprintln!(
                "[PIPEWIRE_DEBUG] ERROR: Failed to serialize LINEAR modifier format pod: {:?}",
                e
            );
            Vec::new()
        }
    }
}

/// Build video format params WITHOUT any modifier property.
/// xdg-desktop-portal-wlr offers RGB WITHOUT modifier field - we must match exactly.
/// This is critical for Sway - if we include a modifier property when portal doesn't,
/// PipeWire cannot find a matching format.
///
/// # Arguments
/// * `negotiated_max_fps` - Max framerate to negotiate
fn build_video_format_params_no_modifier(negotiated_max_fps: u32) -> Vec<u8> {
    use spa::param::video::VideoFormat;
    // RGB/BGR 24-bit formats that xdg-desktop-portal-wlr offers without modifier
    let spa_rgb = VideoFormat::RGB.as_raw();
    let spa_bgr = VideoFormat::BGR.as_raw();
    // Also include 32-bit formats in case portal negotiates without modifier
    let spa_bgra = VideoFormat::BGRA.as_raw();
    let spa_rgba = VideoFormat::RGBA.as_raw();
    let spa_bgrx = VideoFormat::BGRx.as_raw();
    let spa_rgbx = VideoFormat::RGBx.as_raw();

    eprintln!(
        "[PIPEWIRE_DEBUG] SPA format IDs: RGB={}, BGR={}, BGRA={}, RGBA={}, BGRx={}, RGBx={}",
        spa_rgb, spa_bgr, spa_bgra, spa_rgba, spa_bgrx, spa_rgbx
    );

    let mut properties = Vec::new();

    // Media type: Video
    properties.push(Property {
        key: FormatProperties::MediaType.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaType::Video.as_raw())),
    });

    // Media subtype: Raw
    properties.push(Property {
        key: FormatProperties::MediaSubtype.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Id(Id(MediaSubtype::Raw.as_raw())),
    });

    // Video format: Include all formats, prefer RGB (matches portal's no-modifier offer)
    let format_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Enum {
            default: Id(spa_rgb), // Prefer RGB - matches portal's no-modifier offer
            alternatives: vec![
                Id(spa_rgb),
                Id(spa_bgr),
                Id(spa_bgra),
                Id(spa_rgba),
                Id(spa_bgrx),
                Id(spa_rgbx),
            ],
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFormat.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Id(format_choice)),
    });

    // NO MODIFIER PROPERTY - this is intentional!
    // xdg-desktop-portal-wlr's RGB offer has no modifier field.
    // Including any modifier property prevents format matching.

    // Size: Range from 1x1 to 8192x4320
    use spa::utils::Rectangle;
    let size_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Rectangle {
                width: 1920,
                height: 1080,
            },
            min: Rectangle {
                width: 1,
                height: 1,
            },
            max: Rectangle {
                width: 8192,
                height: 4320,
            },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoSize.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Rectangle(size_choice)),
    });

    // Framerate: Range from 0/1 to 360/1
    let framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction { num: 60, denom: 1 },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(framerate_choice)),
    });

    // MaxFramerate
    let max_framerate_choice = Choice(
        ChoiceFlags::empty(),
        ChoiceEnum::Range {
            default: Fraction {
                num: negotiated_max_fps,
                denom: 1,
            },
            min: Fraction { num: 0, denom: 1 },
            max: Fraction { num: 360, denom: 1 },
        },
    );
    properties.push(Property {
        key: FormatProperties::VideoMaxFramerate.as_raw(),
        flags: PropertyFlags::empty(),
        value: Value::Choice(ChoiceValue::Fraction(max_framerate_choice)),
    });

    // Create the format object
    let obj = Object {
        type_: SpaTypes::ObjectParamFormat.as_raw(),
        id: ParamType::EnumFormat.as_raw(),
        properties,
    };

    // Serialize to bytes
    match PodSerializer::serialize(Cursor::new(Vec::new()), &Value::Object(obj)) {
        Ok((cursor, _len)) => {
            let bytes = cursor.into_inner();
            eprintln!("[PIPEWIRE_DEBUG] Built format pod WITHOUT modifier (for RGB/xdg-desktop-portal-wlr) ({} bytes)", bytes.len());
            bytes
        }
        Err(e) => {
            eprintln!(
                "[PIPEWIRE_DEBUG] ERROR: Failed to serialize no-modifier format pod: {:?}",
                e
            );
            Vec::new()
        }
    }
}

fn extract_frame(
    datas: &mut [pipewire::spa::buffer::Data],
    params: &VideoParams,
    pts_ns: i64,
) -> Option<FrameData> {
    // Get chunk info from first element before we need mutable access
    let (size, stride, data_type, fd, offset) = {
        let first = datas.first()?;
        let chunk = first.chunk();
        (
            chunk.size() as usize,
            chunk.stride(),
            first.type_(),
            first.as_raw().fd as i32,
            chunk.offset(),
        )
    };
    if size == 0 {
        return None;
    }

    // DMA-BUF path - build smithay Dmabuf directly
    if data_type == pipewire::spa::buffer::DataType::DmaBuf {
        if fd < 0 {
            return None;
        }

        // Use params from format negotiation, fallback to calculated values only if not set
        let width = if params.width > 0 {
            params.width
        } else {
            (stride / 4) as u32
        };
        let height = if params.height > 0 {
            params.height
        } else if stride > 0 {
            (size as u32) / (stride as u32)
        } else {
            0
        };

        // Use WARN level to ensure visibility with RUST_LOG=WARN
        tracing::warn!(
            "[PIPEWIRE_DEBUG] extract_frame: params={}x{} format=0x{:x} modifier=0x{:x}, chunk: size={} stride={} offset={}, fd={}",
            params.width, params.height, params.format, params.modifier, size, stride, offset, fd
        );

        if width == 0 || height == 0 {
            return None;
        }

        // Build smithay Dmabuf from PipeWire buffer info
        let fourcc = Fourcc::try_from(params.format).unwrap_or(Fourcc::Argb8888);
        // Use the modifier from PipeWire format negotiation:
        // - 0x0 (Linear) = explicit linear layout, most common
        // - 0xffffffffffffff (Invalid) = implicit modifier, let driver decide
        // - Other values = explicit tiled/compressed formats (GPU-specific)
        // We pass through whatever PipeWire negotiated; EGL/CUDA will reject incompatible formats
        let modifier = Modifier::from(params.modifier);
        tracing::warn!(
            "[PIPEWIRE_DEBUG] Using fourcc={:?} modifier={:?} (raw: 0x{:x})",
            fourcc,
            modifier,
            params.modifier
        );

        // Clone the fd to create OwnedFd
        let owned_fd = unsafe { BorrowedFd::borrow_raw(fd).try_clone_to_owned().ok()? };

        let mut builder = Dmabuf::builder(
            (width as i32, height as i32),
            fourcc,
            modifier,
            DmabufFlags::empty(),
        );
        builder.add_plane(owned_fd, 0, offset, stride as u32);

        // Add additional planes if present
        for (idx, data) in datas.iter().enumerate().skip(1) {
            if data.type_() == pipewire::spa::buffer::DataType::DmaBuf {
                let raw = data.as_raw();
                let plane_fd_raw = raw.fd as i32;
                if plane_fd_raw >= 0 {
                    let plane_fd = unsafe {
                        BorrowedFd::borrow_raw(plane_fd_raw)
                            .try_clone_to_owned()
                            .ok()?
                    };
                    builder.add_plane(
                        plane_fd,
                        idx as u32,
                        data.chunk().offset(),
                        data.chunk().stride() as u32,
                    );
                }
            }
        }

        if let Some(dmabuf) = builder.build() {
            tracing::debug!("DMA-BUF frame: {}x{} pts_ns={}", width, height, pts_ns);
            return Some(FrameData::DmaBuf { dmabuf, pts_ns });
        }
    }

    // SHM fallback (MemFd or MemPtr) - need mutable access for data()
    // gnome-remote-desktop uses MemFd for SHM buffers, which PipeWire maps for us
    if let Some(first_mut) = datas.first_mut() {
        // Log SHM buffer details for debugging
        eprintln!(
            "[PIPEWIRE_DEBUG] extract_frame SHM: type={:?} fd={} size={} stride={} params={}x{}",
            data_type, fd, size, stride, params.width, params.height
        );

        if let Some(data_ptr) = first_mut.data() {
            let width = if params.width > 0 {
                params.width
            } else if stride > 0 {
                (stride / 4) as u32
            } else {
                0
            };
            let height = if params.height > 0 {
                params.height
            } else if stride > 0 {
                (size / stride as usize) as u32
            } else {
                0
            };
            if width == 0 || height == 0 {
                eprintln!("[PIPEWIRE_DEBUG] extract_frame SHM: invalid dimensions {}x{} (stride={}, size={})", width, height, stride, size);
                return None;
            }

            eprintln!(
                "[PIPEWIRE_DEBUG] SHM frame SUCCESS: {}x{} stride={} format=0x{:x} pts_ns={}",
                width, height, stride, params.format, pts_ns
            );

            let data = unsafe { std::slice::from_raw_parts(data_ptr.as_ptr(), size) }.to_vec();
            return Some(FrameData::Shm {
                data,
                width,
                height,
                stride: stride as u32,
                format: params.format,
                pts_ns,
            });
        } else {
            // data() returned None - buffer not mapped!
            // This can happen if MAP_BUFFERS flag wasn't set or if it's a different buffer type
            eprintln!("[PIPEWIRE_DEBUG] extract_frame SHM: data() returned None! Buffer not mapped? type={:?} fd={}",
                data_type, fd);
        }
    }

    tracing::warn!(
        "[PIPEWIRE_DEBUG] extract_frame: no valid buffer data, type={:?} fd={} size={}",
        data_type,
        fd,
        size
    );
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use pipewire::spa;

    /// Test SPA video format to DRM fourcc conversion for common formats
    #[test]
    fn test_spa_to_drm_fourcc_bgra() {
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::BGRA);
        // BA24 = BGRA8888 = 0x34324142
        assert_eq!(fourcc, 0x34324142, "BGRA should map to BA24");
    }

    #[test]
    fn test_spa_to_drm_fourcc_rgba() {
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::RGBA);
        // RA24 = RGBA8888 = 0x34324152
        assert_eq!(fourcc, 0x34324152, "RGBA should map to RA24");
    }

    #[test]
    fn test_spa_to_drm_fourcc_bgrx() {
        // BGRx maps to ARGB8888 for CUDA compatibility
        // CUDA rejects XRGB8888 and BGRX8888 with tiled modifiers, but accepts ARGB8888
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::BGRx);
        assert_eq!(
            fourcc, 0x34325241,
            "BGRx should map to ARGB8888 (AR24) for CUDA compatibility"
        );
    }

    #[test]
    fn test_spa_to_drm_fourcc_rgbx() {
        // RGBx maps to ABGR8888 for CUDA compatibility
        // CUDA rejects XBGR8888 and RGBX8888 with tiled modifiers, but accepts ABGR8888
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::RGBx);
        assert_eq!(
            fourcc, 0x34324241,
            "RGBx should map to ABGR8888 (AB24) for CUDA compatibility"
        );
    }

    #[test]
    fn test_spa_to_drm_fourcc_argb() {
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::ARGB);
        // AR24 = ARGB8888 = 0x34325241
        assert_eq!(fourcc, 0x34325241, "ARGB should map to AR24");
    }

    #[test]
    fn test_spa_to_drm_fourcc_abgr() {
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::ABGR);
        // AB24 = ABGR8888 = 0x34324241
        assert_eq!(fourcc, 0x34324241, "ABGR should map to AB24");
    }

    #[test]
    fn test_spa_to_drm_fourcc_xrgb() {
        // xRGB should map to XRGB8888 (XR24)
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::xRGB);
        assert_eq!(fourcc, 0x34325258, "xRGB should map to XRGB8888 (XR24)");
    }

    #[test]
    fn test_spa_to_drm_fourcc_xbgr() {
        // xBGR should map to XBGR8888 (XB24)
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::xBGR);
        assert_eq!(fourcc, 0x34324258, "xBGR should map to XBGR8888 (XB24)");
    }

    #[test]
    fn test_spa_to_drm_fourcc_nv12() {
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::NV12);
        // NV12 = 0x3231564e
        assert_eq!(fourcc, 0x3231564e, "NV12 should map correctly");
    }

    #[test]
    fn test_spa_to_drm_fourcc_i420() {
        let fourcc = spa_video_format_to_drm_fourcc(spa::param::video::VideoFormat::I420);
        // I420 = 0x32315549
        assert_eq!(fourcc, 0x32315549, "I420 should map correctly");
    }

    /// Test VideoParams default values
    #[test]
    fn test_video_params_default() {
        let params = VideoParams::default();
        assert_eq!(params.width, 0);
        assert_eq!(params.height, 0);
        assert_eq!(params.format, 0);
        assert_eq!(params.modifier, 0);
    }
}
