//! PipeWire ScreenCast source - SIMPLIFIED: explicit control, no auto-detection
//!
//! Go code tells us exactly what to do via properties:
//! - capture-source: "pipewire" (GNOME) or "wayland" (Sway ext-image-copy-capture)
//! - buffer-type: "dmabuf" (GNOME+NVIDIA CUDA) or "shm" (everything else)
//!
//! No EGL probing, no fallbacks, no guessing. The element does exactly what it's told.

use crate::ext_image_copy_capture::ExtImageCopyCaptureStream;
use crate::pipewire_stream::{DmaBufCapabilities, FrameData, PipeWireStream, RecvError};
use gst::glib;
use gst::prelude::*;
use gst::subclass::prelude::*;
use gst_base::prelude::BaseSrcExt;
use gst_base::subclass::base_src::CreateSuccess;
use gst_base::subclass::prelude::*;
use gst_video::{VideoCapsBuilder, VideoFormat, VideoInfo, VideoInfoDmaDrm};
use gstreamer_allocators::{DmaBufAllocator, FdMemoryFlags};
use once_cell::sync::Lazy;
use parking_lot::Mutex;
use smithay::backend::allocator::Buffer;
use smithay::backend::drm::{DrmNode, NodeType};
use smithay::backend::egl::ffi::egl::types::EGLDisplay as RawEGLDisplay;
use smithay::backend::egl::{EGLDevice, EGLDisplay};
use std::sync::atomic::AtomicPtr;
use std::sync::Arc;
use std::time::Duration;

// CUDA imports for GNOME+NVIDIA zero-copy path
use waylanddisplaycore::utils::allocator::cuda::{
    gst_cuda_handle_context_query_wrapped, init_cuda, CUDABufferPool, CUDAContext, CUDAImage,
    EGLImage, GstCudaContext, CAPS_FEATURE_MEMORY_CUDA_MEMORY,
};
use waylanddisplaycore::Fourcc as DrmFourcc;

/// Convert DRM fourcc to GStreamer VideoFormat.
/// Uses explicit byte-order formats for consistency across GStreamer versions.
fn drm_fourcc_to_video_format(fourcc: DrmFourcc) -> VideoFormat {
    match fourcc {
        DrmFourcc::Argb8888 => VideoFormat::Bgra,
        DrmFourcc::Abgr8888 => VideoFormat::Rgba,
        DrmFourcc::Rgba8888 => VideoFormat::Abgr,
        DrmFourcc::Bgra8888 => VideoFormat::Argb,
        DrmFourcc::Xrgb8888 => VideoFormat::Bgrx,
        DrmFourcc::Xbgr8888 => VideoFormat::Rgbx,
        DrmFourcc::Rgbx8888 => VideoFormat::Xbgr,
        DrmFourcc::Bgrx8888 => VideoFormat::Xrgb,
        DrmFourcc::Bgr888 => VideoFormat::Rgb,
        DrmFourcc::Rgb888 => VideoFormat::Bgr,
        _ => {
            eprintln!(
                "[PIPEWIRESRC] Unknown fourcc {:?}, defaulting to Bgra",
                fourcc
            );
            VideoFormat::Bgra
        }
    }
}

/// Convert DRM fourcc and modifier to GStreamer drm-format string.
fn drm_format_to_gst_string(fourcc: DrmFourcc, modifier: u64) -> String {
    let fourcc_str = fourcc.to_string();
    let fourcc_str = fourcc_str.trim();
    if modifier == 0 {
        format!("{:<4}", fourcc_str)
    } else {
        format!("{:<4}:0x{:016x}", fourcc_str, modifier)
    }
}

static CAT: Lazy<gst::DebugCategory> = Lazy::new(|| {
    gst::DebugCategory::new(
        "pipewirezerocopysrc",
        gst::DebugColorFlags::empty(),
        Some("PipeWire zero-copy source"),
    )
});

/// Create EGL display from render node path
fn create_egl_display(node_path: &str) -> Result<EGLDisplay, String> {
    let drm_node = DrmNode::from_path(node_path).map_err(|e| format!("DrmNode: {:?}", e))?;
    let drm_render = drm_node
        .node_with_type(NodeType::Render)
        .and_then(Result::ok)
        .unwrap_or(drm_node);
    let device = EGLDevice::enumerate()
        .map_err(|e| format!("enumerate: {:?}", e))?
        .find(|d| d.try_get_render_node().unwrap_or_default() == Some(drm_render.clone()))
        .ok_or_else(|| "No EGLDevice for node".to_string())?;
    unsafe { EGLDisplay::new(device).map_err(|e| format!("EGLDisplay: {:?}", e)) }
}

/// Get NVIDIA ScreenCast modifiers for DmaBuf negotiation
fn get_nvidia_dmabuf_caps() -> DmaBufCapabilities {
    // NVIDIA ScreenCast modifiers (0xe08xxx family) from pw-dump observations
    DmaBufCapabilities {
        modifiers: vec![
            0x300000000e08010,
            0x300000000e08011,
            0x300000000e08012,
            0x300000000e08013,
            0x300000000e08014,
            0x300000000e08015,
            0x300000000e08016,
            0x0, // LINEAR fallback for Sway compatibility
        ],
        dmabuf_available: true,
    }
}

/// Capture source - explicitly set by Go code
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum CaptureSource {
    #[default]
    PipeWire, // GNOME ScreenCast via PipeWire
    Wayland, // Sway via ext-image-copy-capture
}

impl CaptureSource {
    fn from_str(s: &str) -> Self {
        match s.to_lowercase().as_str() {
            "wayland" | "sway" | "ext-image-copy-capture" => Self::Wayland,
            _ => Self::PipeWire,
        }
    }
}

/// Buffer type - explicitly set by Go code
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum BufferType {
    #[default]
    Shm, // SHM/MemFd - works everywhere
    DmaBuf, // DMA-BUF with NVIDIA modifiers - GNOME+NVIDIA only
}

impl BufferType {
    fn from_str(s: &str) -> Self {
        match s.to_lowercase().as_str() {
            "dmabuf" | "dma-buf" | "gpu" | "cuda" => Self::DmaBuf,
            _ => Self::Shm,
        }
    }
}

/// Output mode for downstream elements
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum OutputMode {
    #[default]
    System, // System memory
    Cuda,   // CUDA memory (GNOME+NVIDIA)
    DmaBuf, // DMA-BUF pass-through
}

#[derive(Debug)]
pub struct Settings {
    pipewire_node_id: Option<u32>,
    pipewire_fd: Option<i32>,
    render_node: Option<String>,
    cuda_device_id: i32,
    keepalive_time_ms: u32,
    target_fps: u32,
    capture_source: CaptureSource,
    buffer_type: BufferType,
    cuda_context: Option<Arc<std::sync::Mutex<CUDAContext>>>,
    cuda_raw_ptr: AtomicPtr<GstCudaContext>,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            pipewire_node_id: None,
            pipewire_fd: None,
            render_node: Some("/dev/dri/renderD128".into()),
            cuda_device_id: -1,
            keepalive_time_ms: 500,
            target_fps: 60,
            capture_source: CaptureSource::PipeWire,
            buffer_type: BufferType::Shm,
            cuda_context: None,
            cuda_raw_ptr: AtomicPtr::new(std::ptr::null_mut()),
        }
    }
}

/// Unified stream abstraction: PipeWire (GNOME) or ext-image-copy-capture (Sway)
pub enum FrameStream {
    PipeWire(PipeWireStream),
    ExtImageCopyCapture(ExtImageCopyCaptureStream),
}

impl FrameStream {
    pub fn recv_frame_timeout(&self, timeout: Duration) -> Result<FrameData, RecvError> {
        match self {
            FrameStream::PipeWire(s) => s.recv_frame_timeout(timeout),
            FrameStream::ExtImageCopyCapture(s) => s.recv_frame_timeout(timeout),
        }
    }
}

/// Timing statistics for performance measurement (all paths)
#[derive(Default)]
struct TimingStats {
    // Common stats
    samples: u32,
    total_total_us: u64,
    total_max_us: u64,
    last_log: Option<std::time::Instant>,
    // Inter-frame timing to detect encoder backup
    last_frame: Option<std::time::Instant>,
    interval_total_us: u64,
    interval_max_us: u64,
    // CUDA path specific
    egl_total_us: u64,
    cuda_reg_total_us: u64,
    copy_total_us: u64,
    egl_max_us: u64,
    cuda_reg_max_us: u64,
    copy_max_us: u64,
    // Path tracking
    path_name: &'static str,
}

impl TimingStats {
    fn record_frame(&mut self, total_us: u64) {
        let now = std::time::Instant::now();
        self.samples += 1;
        self.total_total_us += total_us;
        self.total_max_us = self.total_max_us.max(total_us);

        // Track inter-frame interval
        if let Some(last) = self.last_frame {
            let interval_us = now.duration_since(last).as_micros() as u64;
            self.interval_total_us += interval_us;
            self.interval_max_us = self.interval_max_us.max(interval_us);
        }
        self.last_frame = Some(now);
    }

    fn record_cuda_frame(&mut self, egl_us: u64, cuda_reg_us: u64, copy_us: u64, total_us: u64) {
        self.record_frame(total_us);
        self.egl_total_us += egl_us;
        self.cuda_reg_total_us += cuda_reg_us;
        self.copy_total_us += copy_us;
        self.egl_max_us = self.egl_max_us.max(egl_us);
        self.cuda_reg_max_us = self.cuda_reg_max_us.max(cuda_reg_us);
        self.copy_max_us = self.copy_max_us.max(copy_us);
    }

    fn should_log(&self) -> bool {
        self.last_log.map(|l| l.elapsed().as_secs() >= 10).unwrap_or(true)
    }

    fn log_and_reset(&mut self, w: u32, h: u32) {
        if self.samples == 0 {
            return;
        }
        let n = self.samples as u64;
        let interval_avg = if self.samples > 1 { self.interval_total_us / (n - 1) } else { 0 };
        let fps = if interval_avg > 0 { 1_000_000 / interval_avg } else { 0 };

        // Check if this is CUDA path (has cuda_reg stats)
        if self.cuda_reg_total_us > 0 {
            eprintln!(
                "[TIMING] {} {}x{} {} frames: interval avg={}us max={}us ({}fps) | EGL avg={}us max={}us | CUDA_REG avg={}us max={}us | COPY avg={}us max={}us | TOTAL avg={}us max={}us",
                self.path_name, w, h, self.samples,
                interval_avg, self.interval_max_us, fps,
                self.egl_total_us / n, self.egl_max_us,
                self.cuda_reg_total_us / n, self.cuda_reg_max_us,
                self.copy_total_us / n, self.copy_max_us,
                self.total_total_us / n, self.total_max_us,
            );
        } else {
            eprintln!(
                "[TIMING] {} {}x{} {} frames: interval avg={}us max={}us ({}fps) | TOTAL avg={}us max={}us",
                self.path_name, w, h, self.samples,
                interval_avg, self.interval_max_us, fps,
                self.total_total_us / n, self.total_max_us,
            );
        }

        // Reset stats
        self.samples = 0;
        self.total_total_us = 0;
        self.total_max_us = 0;
        self.interval_total_us = 0;
        self.interval_max_us = 0;
        self.egl_total_us = 0;
        self.cuda_reg_total_us = 0;
        self.copy_total_us = 0;
        self.egl_max_us = 0;
        self.cuda_reg_max_us = 0;
        self.copy_max_us = 0;
        self.last_log = Some(std::time::Instant::now());
    }
}

pub struct State {
    stream: Option<FrameStream>,
    video_info: Option<VideoInfo>,
    egl_display: Option<Arc<EGLDisplay>>,
    buffer_pool: Option<CUDABufferPool>,
    output_mode: OutputMode,
    frame_count: u64,
    last_buffer: Option<gst::Buffer>,
    buffer_pool_configured: bool,
    dmabuf_allocator: Option<DmaBufAllocator>,
    drm_format_string: Option<String>,
    timing: TimingStats,
}

impl Default for State {
    fn default() -> Self {
        Self {
            stream: None,
            video_info: None,
            egl_display: None,
            buffer_pool: None,
            output_mode: OutputMode::System,
            frame_count: 0,
            last_buffer: None,
            buffer_pool_configured: false,
            dmabuf_allocator: None,
            drm_format_string: None,
            timing: TimingStats::default(),
        }
    }
}

pub struct PipeWireZeroCopySrc {
    settings: Mutex<Settings>,
    state: Mutex<Option<State>>,
}

impl Default for PipeWireZeroCopySrc {
    fn default() -> Self {
        Self {
            settings: Mutex::new(Settings::default()),
            state: Mutex::new(None),
        }
    }
}

#[glib::object_subclass]
impl ObjectSubclass for PipeWireZeroCopySrc {
    const NAME: &'static str = "GstPipeWireZeroCopySrc";
    type Type = super::PipeWireZeroCopySrc;
    type ParentType = gst_base::PushSrc;
}

impl ObjectImpl for PipeWireZeroCopySrc {
    fn properties() -> &'static [glib::ParamSpec] {
        static PROPERTIES: Lazy<Vec<glib::ParamSpec>> = Lazy::new(|| {
            vec![
                glib::ParamSpecUInt::builder("pipewire-node-id")
                    .nick("PipeWire Node ID")
                    .blurb("PipeWire node ID from ScreenCast portal")
                    .construct()
                    .build(),
                glib::ParamSpecInt::builder("pipewire-fd")
                    .nick("PipeWire FD")
                    .blurb("PipeWire file descriptor from portal")
                    .minimum(-1)
                    .maximum(65535)
                    .default_value(-1)
                    .construct()
                    .build(),
                glib::ParamSpecString::builder("render-node")
                    .nick("DRM Render Node")
                    .blurb("DRM render node path")
                    .default_value(Some("/dev/dri/renderD128"))
                    .construct()
                    .build(),
                glib::ParamSpecInt::builder("cuda-device-id")
                    .nick("CUDA Device ID")
                    .blurb("CUDA device ID (-1 for auto)")
                    .minimum(-1)
                    .maximum(16)
                    .default_value(-1)
                    .construct()
                    .build(),
                glib::ParamSpecUInt::builder("keepalive-time")
                    .nick("Keepalive Time")
                    .blurb("Resend last buffer after this many ms (0=disabled)")
                    .minimum(0)
                    .maximum(60000)
                    .default_value(500)
                    .construct()
                    .build(),
                glib::ParamSpecUInt::builder("target-fps")
                    .nick("Target FPS")
                    .blurb("Target frames per second")
                    .minimum(1)
                    .maximum(240)
                    .default_value(60)
                    .construct()
                    .build(),
                // === EXPLICIT CONTROL PROPERTIES ===
                glib::ParamSpecString::builder("capture-source")
                    .nick("Capture Source")
                    .blurb("'pipewire' (GNOME) or 'wayland' (Sway ext-image-copy-capture)")
                    .default_value(Some("pipewire"))
                    .construct()
                    .build(),
                glib::ParamSpecString::builder("buffer-type")
                    .nick("Buffer Type")
                    .blurb("'shm' (SHM everywhere) or 'dmabuf' (GNOME+NVIDIA CUDA)")
                    .default_value(Some("shm"))
                    .construct()
                    .build(),
            ]
        });
        PROPERTIES.as_ref()
    }

    fn set_property(&self, _id: usize, value: &glib::Value, pspec: &glib::ParamSpec) {
        let mut s = self.settings.lock();
        match pspec.name() {
            "pipewire-node-id" => s.pipewire_node_id = Some(value.get().unwrap()),
            "pipewire-fd" => {
                let fd: i32 = value.get().unwrap();
                s.pipewire_fd = if fd >= 0 { Some(fd) } else { None };
            }
            "render-node" => s.render_node = value.get().unwrap(),
            "cuda-device-id" => s.cuda_device_id = value.get().unwrap(),
            "keepalive-time" => s.keepalive_time_ms = value.get().unwrap(),
            "target-fps" => s.target_fps = value.get().unwrap(),
            "capture-source" => {
                s.capture_source = value
                    .get::<Option<String>>()
                    .unwrap()
                    .as_deref()
                    .map(CaptureSource::from_str)
                    .unwrap_or_default()
            }
            "buffer-type" => {
                s.buffer_type = value
                    .get::<Option<String>>()
                    .unwrap()
                    .as_deref()
                    .map(BufferType::from_str)
                    .unwrap_or_default()
            }
            _ => {}
        }
    }

    fn property(&self, _id: usize, pspec: &glib::ParamSpec) -> glib::Value {
        let s = self.settings.lock();
        match pspec.name() {
            "pipewire-node-id" => s.pipewire_node_id.unwrap_or(0).to_value(),
            "pipewire-fd" => s.pipewire_fd.unwrap_or(-1).to_value(),
            "render-node" => s
                .render_node
                .clone()
                .unwrap_or_else(|| "/dev/dri/renderD128".into())
                .to_value(),
            "cuda-device-id" => s.cuda_device_id.to_value(),
            "keepalive-time" => s.keepalive_time_ms.to_value(),
            "target-fps" => s.target_fps.to_value(),
            "capture-source" => match s.capture_source {
                CaptureSource::PipeWire => "pipewire",
                CaptureSource::Wayland => "wayland",
            }
            .to_value(),
            "buffer-type" => match s.buffer_type {
                BufferType::Shm => "shm",
                BufferType::DmaBuf => "dmabuf",
            }
            .to_value(),
            _ => unreachable!(),
        }
    }

    fn constructed(&self) {
        self.parent_constructed();
        let obj = self.obj();
        obj.set_element_flags(gst::ElementFlags::SOURCE);
        obj.set_live(true);
        obj.set_format(gst::Format::Time);
        obj.set_automatic_eos(false);
        obj.set_do_timestamp(true);
    }
}

impl GstObjectImpl for PipeWireZeroCopySrc {}

impl ElementImpl for PipeWireZeroCopySrc {
    fn metadata() -> Option<&'static gst::subclass::ElementMetadata> {
        static META: Lazy<gst::subclass::ElementMetadata> = Lazy::new(|| {
            gst::subclass::ElementMetadata::new(
                "PipeWire Zero-Copy Source",
                "Source/Video",
                "Captures PipeWire ScreenCast or Wayland ext-image-copy-capture",
                "Helix Project",
            )
        });
        Some(&*META)
    }

    fn pad_templates() -> &'static [gst::PadTemplate] {
        static TEMPLATES: Lazy<Vec<gst::PadTemplate>> = Lazy::new(|| {
            let formats = [
                VideoFormat::Bgra,
                VideoFormat::Rgba,
                VideoFormat::Argb,
                VideoFormat::Abgr,
                VideoFormat::Bgrx,
                VideoFormat::Rgbx,
                VideoFormat::Xrgb,
                VideoFormat::Xbgr,
                VideoFormat::Bgr,
                VideoFormat::Rgb,
                VideoFormat::Nv12,
            ];
            let mut caps = gst::Caps::new_empty();
            caps.merge(
                VideoCapsBuilder::new()
                    .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                    .format_list(formats)
                    .build(),
            );
            caps.merge(
                VideoCapsBuilder::new()
                    .features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF])
                    .format(VideoFormat::DmaDrm)
                    .build(),
            );
            caps.merge(VideoCapsBuilder::new().format_list(formats).build());
            vec![gst::PadTemplate::new(
                "src",
                gst::PadDirection::Src,
                gst::PadPresence::Always,
                &caps,
            )
            .unwrap()]
        });
        TEMPLATES.as_ref()
    }

    fn change_state(
        &self,
        transition: gst::StateChange,
    ) -> Result<gst::StateChangeSuccess, gst::StateChangeError> {
        match self.parent_change_state(transition) {
            Ok(gst::StateChangeSuccess::Success) if transition.next() == gst::State::Paused => {
                Ok(gst::StateChangeSuccess::NoPreroll)
            }
            x => x,
        }
    }

    fn set_context(&self, context: &gst::Context) {
        let elem = self.obj().upcast_ref::<gst::Element>().to_owned();
        let cuda_raw_ptr = self.settings.lock().cuda_raw_ptr.as_ptr();

        match CUDAContext::new_from_set_context(&elem, context, -1, cuda_raw_ptr) {
            Ok(ctx) => {
                let mut settings = self.settings.lock();
                if settings.cuda_context.is_none() {
                    eprintln!("[PIPEWIRESRC] Received CUDA context via set_context");
                    settings.cuda_context = Some(Arc::new(std::sync::Mutex::new(ctx)));
                }
            }
            Err(_) => {}
        }
        self.parent_set_context(context)
    }
}

impl BaseSrcImpl for PipeWireZeroCopySrc {
    fn start(&self) -> Result<(), gst::ErrorMessage> {
        let mut state_guard = self.state.lock();
        if state_guard.is_some() {
            return Ok(());
        }

        // Extract all settings
        let (node_id, pipewire_fd, render_node, device_id, target_fps, capture_source, buffer_type) = {
            let s = self.settings.lock();
            let node_id = s.pipewire_node_id.ok_or_else(|| {
                gst::error_msg!(
                    gst::LibraryError::Settings,
                    ("pipewire-node-id must be set")
                )
            })?;
            (
                node_id,
                s.pipewire_fd,
                s.render_node.clone(),
                s.cuda_device_id,
                s.target_fps,
                s.capture_source,
                s.buffer_type,
            )
        };

        eprintln!(
            "[PIPEWIRESRC] start: capture_source={:?} buffer_type={:?} node_id={}",
            capture_source, buffer_type, node_id
        );

        let mut state = State::default();
        let elem = self.obj().upcast_ref::<gst::Element>().to_owned();

        // === CASE 1: Wayland (Sway) - always SHM via ext-image-copy-capture ===
        if capture_source == CaptureSource::Wayland {
            eprintln!("[PIPEWIRESRC] Using ext-image-copy-capture (Sway) - SHM mode");
            state.output_mode = OutputMode::System;
            let stream = ExtImageCopyCaptureStream::connect(target_fps).map_err(|e| {
                gst::error_msg!(gst::LibraryError::Init, ("ext-image-copy-capture: {}", e))
            })?;
            state.stream = Some(FrameStream::ExtImageCopyCapture(stream));
            *state_guard = Some(state);
            return Ok(());
        }

        // === CASE 2: PipeWire (GNOME) ===
        match buffer_type {
            BufferType::DmaBuf => {
                // GNOME + NVIDIA: DmaBuf with CUDA
                eprintln!("[PIPEWIRESRC] Using PipeWire DmaBuf (GNOME+NVIDIA CUDA mode)");

                // Initialize CUDA
                if let Err(e) = init_cuda() {
                    return Err(gst::error_msg!(
                        gst::LibraryError::Init,
                        ("CUDA init failed: {:?}", e)
                    ));
                }

                // Get or acquire CUDA context
                let have_context = self.settings.lock().cuda_context.is_some();
                if !have_context {
                    let cuda_raw_ptr = self.settings.lock().cuda_raw_ptr.as_ptr();
                    match CUDAContext::new_from_gstreamer(&elem, device_id, cuda_raw_ptr) {
                        Ok(ctx) => {
                            let mut settings = self.settings.lock();
                            if settings.cuda_context.is_none() {
                                settings.cuda_context = Some(Arc::new(std::sync::Mutex::new(ctx)));
                            } else {
                                std::mem::forget(ctx); // Prevent double-unref
                            }
                        }
                        Err(e) => {
                            return Err(gst::error_msg!(
                                gst::LibraryError::Init,
                                ("CUDA context failed: {}", e)
                            ));
                        }
                    }
                }

                // Create EGL display for DmaBuf import
                let node_path = render_node.as_deref().unwrap_or("/dev/dri/renderD128");
                let display = create_egl_display(node_path).map_err(|e| {
                    gst::error_msg!(gst::LibraryError::Init, ("EGL display failed: {}", e))
                })?;

                // Create CUDA buffer pool
                let cuda_context = self.settings.lock().cuda_context.clone().unwrap();
                let cuda_ctx = cuda_context.lock().unwrap();
                let pool = CUDABufferPool::new(&cuda_ctx).map_err(|e| {
                    gst::error_msg!(gst::LibraryError::Init, ("CUDA buffer pool failed: {}", e))
                })?;
                drop(cuda_ctx);

                state.egl_display = Some(Arc::new(display));
                state.buffer_pool = Some(pool);
                state.output_mode = OutputMode::Cuda;

                // Connect to PipeWire with NVIDIA DmaBuf modifiers
                let dmabuf_caps = get_nvidia_dmabuf_caps();
                let stream =
                    PipeWireStream::connect(node_id, pipewire_fd, Some(dmabuf_caps), target_fps)
                        .map_err(|e| {
                            gst::error_msg!(
                                gst::LibraryError::Init,
                                ("PipeWire DmaBuf failed: {}", e)
                            )
                        })?;
                state.stream = Some(FrameStream::PipeWire(stream));
            }
            BufferType::Shm => {
                // GNOME + AMD/Intel or any SHM case: No DmaBuf, no CUDA
                eprintln!("[PIPEWIRESRC] Using PipeWire SHM (GNOME+AMD or fallback)");
                state.output_mode = OutputMode::System;

                // Connect to PipeWire WITHOUT DmaBuf caps - forces SHM negotiation
                let stream = PipeWireStream::connect(node_id, pipewire_fd, None, target_fps)
                    .map_err(|e| {
                        gst::error_msg!(gst::LibraryError::Init, ("PipeWire SHM failed: {}", e))
                    })?;
                state.stream = Some(FrameStream::PipeWire(stream));
            }
        }

        *state_guard = Some(state);
        Ok(())
    }

    fn stop(&self) -> Result<(), gst::ErrorMessage> {
        let mut g = self.state.lock();
        if let Some(s) = g.take() {
            drop(s);
        }
        Ok(())
    }

    fn is_seekable(&self) -> bool {
        false
    }

    fn query(&self, query: &mut gst::QueryRef) -> bool {
        if query.type_() == gst::QueryType::Context {
            let settings = self.settings.lock();
            if let Some(ref cuda_context) = settings.cuda_context {
                let cuda_ctx = cuda_context.lock().unwrap();
                return gst_cuda_handle_context_query_wrapped(
                    self.obj().as_ref().as_ref(),
                    query,
                    &cuda_ctx,
                );
            }
        }
        BaseSrcImplExt::parent_query(self, query)
    }

    fn caps(&self, filter: Option<&gst::Caps>) -> Option<gst::Caps> {
        let g = self.state.lock();
        let output_mode = g
            .as_ref()
            .map(|s| s.output_mode)
            .unwrap_or(OutputMode::System);
        drop(g);

        let formats = [
            VideoFormat::Bgra,
            VideoFormat::Rgba,
            VideoFormat::Argb,
            VideoFormat::Abgr,
            VideoFormat::Bgrx,
            VideoFormat::Rgbx,
            VideoFormat::Xrgb,
            VideoFormat::Xbgr,
            VideoFormat::Bgr,
            VideoFormat::Rgb,
            VideoFormat::Nv12,
        ];

        let mut caps = match output_mode {
            OutputMode::Cuda => VideoCapsBuilder::new()
                .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                .format_list(formats)
                .build(),
            OutputMode::DmaBuf => VideoCapsBuilder::new()
                .features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF])
                .format(VideoFormat::DmaDrm)
                .build(),
            OutputMode::System => VideoCapsBuilder::new().format_list(formats).build(),
        };

        if let Some(f) = filter {
            caps = caps.intersect(f);
        }
        Some(caps)
    }

    fn set_caps(&self, caps: &gst::Caps) -> Result<(), gst::LoggableError> {
        if let Ok(info) = VideoInfo::from_caps(caps) {
            if let Some(s) = self.state.lock().as_mut() {
                s.video_info = Some(info);
            }
        }
        self.parent_set_caps(caps)
    }
}

impl PushSrcImpl for PipeWireZeroCopySrc {
    fn create(
        &self,
        _buffer: Option<&mut gst::BufferRef>,
    ) -> Result<CreateSuccess, gst::FlowError> {
        let (cuda_context, keepalive_time_ms) = {
            let s = self.settings.lock();
            (s.cuda_context.clone(), s.keepalive_time_ms)
        };

        let mut g = self.state.lock();
        let state = g.as_mut().ok_or(gst::FlowError::Eos)?;
        let stream = state.stream.as_ref().ok_or(gst::FlowError::Error)?;

        // Timeout: 30s for first frame, keepalive_time_ms for subsequent
        let has_last = state.last_buffer.is_some();
        let timeout = if !has_last {
            Duration::from_secs(30)
        } else if keepalive_time_ms > 0 {
            Duration::from_millis(keepalive_time_ms as u64)
        } else {
            Duration::from_secs(30)
        };

        let frame = match stream.recv_frame_timeout(timeout) {
            Ok(f) => f,
            Err(RecvError::Timeout) if keepalive_time_ms > 0 && has_last => {
                // Keepalive: resend last buffer
                let buf = state.last_buffer.as_ref().unwrap().copy();
                state.frame_count += 1;
                drop(g);
                return Ok(CreateSuccess::NewBuffer(buf));
            }
            Err(RecvError::Disconnected) => return Err(gst::FlowError::Eos),
            Err(_) => return Err(gst::FlowError::Error),
        };

        let (buffer, actual_format, width, height) = match frame {
            FrameData::DmaBuf(dmabuf) if state.output_mode == OutputMode::Cuda => {
                // CUDA path: DmaBuf → EGL → CUDA
                // === TIMING INSTRUMENTATION ===
                let t_start = std::time::Instant::now();

                let cuda_ctx_arc = cuda_context.as_ref().ok_or(gst::FlowError::Error)?;
                let cuda_ctx = cuda_ctx_arc.lock().unwrap();
                let egl_display = state.egl_display.as_ref().ok_or(gst::FlowError::Error)?;
                let raw_display: RawEGLDisplay = egl_display.get_display_handle().handle;

                let w = dmabuf.width() as u32;
                let h = dmabuf.height() as u32;

                let t_egl_start = std::time::Instant::now();
                let egl_image =
                    EGLImage::from(&dmabuf, &raw_display).map_err(|_| gst::FlowError::Error)?;
                let t_egl_done = std::time::Instant::now();

                let t_cuda_reg_start = std::time::Instant::now();
                let cuda_image =
                    CUDAImage::from(egl_image, &cuda_ctx).map_err(|_| gst::FlowError::Error)?;
                let t_cuda_reg_done = std::time::Instant::now();

                let drm_format = dmabuf.format();
                let video_format = drm_fourcc_to_video_format(drm_format.code);
                let base_info = VideoInfo::builder(video_format, w, h)
                    .build()
                    .map_err(|_| gst::FlowError::Error)?;
                let dma_video_info = VideoInfoDmaDrm::new(
                    base_info,
                    drm_format.code as u32,
                    drm_format.modifier.into(),
                );

                // Configure buffer pool on first frame
                if !state.buffer_pool_configured {
                    if let Some(ref pool) = state.buffer_pool {
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
                                state.buffer_pool_configured = true;
                            }
                        }
                    }
                }

                let t_copy_start = std::time::Instant::now();
                let buf = cuda_image
                    .to_gst_buffer(dma_video_info, &cuda_ctx, state.buffer_pool.as_ref())
                    .map_err(|_| gst::FlowError::Error)?;
                let t_copy_done = std::time::Instant::now();

                // cuda_image drops here, which calls cuGraphicsUnregisterResource
                // === AGGREGATE TIMING ===
                let egl_us = t_egl_done.duration_since(t_egl_start).as_micros() as u64;
                let cuda_reg_us = t_cuda_reg_done.duration_since(t_cuda_reg_start).as_micros() as u64;
                let copy_us = t_copy_done.duration_since(t_copy_start).as_micros() as u64;
                let total_us = t_start.elapsed().as_micros() as u64;

                state.timing.path_name = "CUDA";
                state.timing.record_cuda_frame(egl_us, cuda_reg_us, copy_us, total_us);
                if state.timing.should_log() {
                    state.timing.log_and_reset(w, h);
                }

                let actual_fmt = buf
                    .meta::<gst_video::VideoMeta>()
                    .map(|m| m.format())
                    .unwrap_or(video_format);
                (buf, actual_fmt, w, h)
            }
            FrameData::DmaBuf(dmabuf) => {
                // Should not happen with explicit buffer_type, but handle gracefully
                let t_start = std::time::Instant::now();
                let w = dmabuf.width() as u32;
                let h = dmabuf.height() as u32;
                let video_format = drm_fourcc_to_video_format(dmabuf.format().code);
                let buf = self.dmabuf_to_system(&dmabuf)?;

                // Timing for DmaBuf→System fallback path
                state.timing.path_name = "DmaBuf-SHM";
                state.timing.record_frame(t_start.elapsed().as_micros() as u64);
                if state.timing.should_log() {
                    state.timing.log_and_reset(w, h);
                }

                (buf, video_format, w, h)
            }
            FrameData::Shm {
                data,
                width,
                height,
                stride,
                format,
            } => {
                let t_start = std::time::Instant::now();
                let video_format = if format != 0 {
                    DrmFourcc::try_from(format)
                        .map(drm_fourcc_to_video_format)
                        .unwrap_or(VideoFormat::Bgra)
                } else {
                    VideoFormat::Bgra
                };
                let buf = self.create_system_buffer(&data, width, height, stride, video_format)?;

                // Timing for SHM path (GNOME+AMD, Sway+any)
                state.timing.path_name = "SHM";
                state.timing.record_frame(t_start.elapsed().as_micros() as u64);
                if state.timing.should_log() {
                    state.timing.log_and_reset(width, height);
                }

                (buf, video_format, width, height)
            }
        };

        // Update caps if format/size changed
        let needs_update = match &state.video_info {
            Some(info) => {
                info.format() != actual_format || info.width() != width || info.height() != height
            }
            None => true,
        };

        if needs_update {
            let fps = state
                .video_info
                .as_ref()
                .map(|i| i.fps())
                .unwrap_or(gst::Fraction::new(60, 1));
            let new_caps = match state.output_mode {
                OutputMode::Cuda => VideoCapsBuilder::new()
                    .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                    .format(actual_format)
                    .width(width as i32)
                    .height(height as i32)
                    .framerate(fps)
                    .build(),
                OutputMode::DmaBuf => {
                    let drm_fmt = state
                        .drm_format_string
                        .clone()
                        .unwrap_or_else(|| "XR24".to_string());
                    VideoCapsBuilder::new()
                        .features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF])
                        .format(VideoFormat::DmaDrm)
                        .field("drm-format", &drm_fmt)
                        .width(width as i32)
                        .height(height as i32)
                        .framerate(fps)
                        .build()
                }
                OutputMode::System => VideoCapsBuilder::new()
                    .format(actual_format)
                    .width(width as i32)
                    .height(height as i32)
                    .framerate(fps)
                    .build(),
            };

            if let Ok(info) = VideoInfo::from_caps(&new_caps) {
                state.video_info = Some(info);
            }

            if keepalive_time_ms > 0 {
                state.last_buffer = Some(buffer.clone());
            }

            drop(g);

            let pad = self.obj().static_pad("src").expect("src pad");
            pad.push_event(gst::event::Caps::new(&new_caps));
        } else {
            if keepalive_time_ms > 0 {
                state.last_buffer = Some(buffer.clone());
            }
            state.frame_count += 1;
            drop(g);
        }

        Ok(CreateSuccess::NewBuffer(buffer))
    }
}

impl PipeWireZeroCopySrc {
    fn dmabuf_to_system(
        &self,
        dmabuf: &smithay::backend::allocator::dmabuf::Dmabuf,
    ) -> Result<gst::Buffer, gst::FlowError> {
        use std::os::fd::AsRawFd;
        let size = (dmabuf.height() as usize) * (dmabuf.strides().next().unwrap_or(0) as usize);
        let mut data = vec![0u8; size];

        unsafe {
            let fd = dmabuf
                .handles()
                .next()
                .ok_or(gst::FlowError::Error)?
                .as_raw_fd();
            let ptr = libc::mmap(
                std::ptr::null_mut(),
                size,
                libc::PROT_READ,
                libc::MAP_SHARED,
                fd,
                0,
            );
            if ptr == libc::MAP_FAILED {
                return Err(gst::FlowError::Error);
            }
            std::ptr::copy_nonoverlapping(ptr as *const u8, data.as_mut_ptr(), size);
            libc::munmap(ptr, size);
        }

        let video_format = drm_fourcc_to_video_format(dmabuf.format().code);
        self.create_system_buffer(
            &data,
            dmabuf.width() as u32,
            dmabuf.height() as u32,
            dmabuf.strides().next().unwrap_or(0),
            video_format,
        )
    }

    fn create_system_buffer(
        &self,
        data: &[u8],
        width: u32,
        height: u32,
        stride: u32,
        format: VideoFormat,
    ) -> Result<gst::Buffer, gst::FlowError> {
        let mut buffer = gst::Buffer::with_size(data.len()).map_err(|_| gst::FlowError::Error)?;
        {
            let buf = buffer.get_mut().unwrap();
            buf.map_writable()
                .map_err(|_| gst::FlowError::Error)?
                .copy_from_slice(data);
        }
        {
            let buf = buffer.get_mut().unwrap();
            gst_video::VideoMeta::add_full(
                buf,
                gst_video::VideoFrameFlags::empty(),
                format,
                width,
                height,
                &[0],
                &[stride as i32],
            )
            .map_err(|_| gst::FlowError::Error)?;
        }
        Ok(buffer)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_capture_source_from_str() {
        assert_eq!(CaptureSource::from_str("pipewire"), CaptureSource::PipeWire);
        assert_eq!(CaptureSource::from_str("wayland"), CaptureSource::Wayland);
        assert_eq!(CaptureSource::from_str("sway"), CaptureSource::Wayland);
        assert_eq!(CaptureSource::from_str("unknown"), CaptureSource::PipeWire);
    }

    #[test]
    fn test_buffer_type_from_str() {
        assert_eq!(BufferType::from_str("shm"), BufferType::Shm);
        assert_eq!(BufferType::from_str("dmabuf"), BufferType::DmaBuf);
        assert_eq!(BufferType::from_str("cuda"), BufferType::DmaBuf);
        assert_eq!(BufferType::from_str("unknown"), BufferType::Shm);
    }

    #[test]
    fn test_drm_fourcc_to_video_format() {
        gst::init().unwrap();
        assert_eq!(
            drm_fourcc_to_video_format(DrmFourcc::Argb8888),
            VideoFormat::Bgra
        );
        assert_eq!(
            drm_fourcc_to_video_format(DrmFourcc::Xrgb8888),
            VideoFormat::Bgrx
        );
        assert_eq!(
            drm_fourcc_to_video_format(DrmFourcc::Bgr888),
            VideoFormat::Rgb
        );
    }
}
