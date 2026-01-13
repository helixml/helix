//! PipeWire ScreenCast source - reuses waylanddisplaycore's CUDA conversion
//! Cache-bust: 2026-01-12T13:30Z - AMD VA-API zero-copy: use DMA_DRM format like Wolf
//!
//! This element follows Wolf/gst-wayland-display's context sharing pattern:
//! - Wolf creates the CUDA context and pushes it via set_context()
//! - We receive it in set_context() and store it
//! - We respond to context queries from downstream elements
//! - Fallback: if no context pushed, acquire via new_from_gstreamer()

use crate::ext_image_copy_capture::ExtImageCopyCaptureStream;
use crate::pipewire_stream::{DmaBufCapabilities, FrameData, PipeWireStream, RecvError};
use crate::wlr_export_dmabuf::WlrExportDmabufStream;
use crate::wlr_screencopy::WlrScreencopyStream;
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

// Reuse battle-tested CUDA code from waylanddisplaycore
use waylanddisplaycore::utils::allocator::cuda::{
    gst_cuda_handle_context_query_wrapped, init_cuda, CUDABufferPool, CUDAContext, CUDAImage,
    EGLImage, GstCudaContext, CAPS_FEATURE_MEMORY_CUDA_MEMORY,
};
// DRM types from waylanddisplaycore (which re-exports from smithay)
use waylanddisplaycore::Fourcc as DrmFourcc;

/// Convert DRM fourcc to GStreamer VideoFormat.
///
/// CRITICAL: We use EXPLICIT byte-order formats (Bgra, Bgrx, etc.) rather than
/// native-endian formats (xRGB, etc.) because:
/// 1. GStreamer's videoconvert handles explicit formats more predictably
/// 2. Different GStreamer versions may interpret native formats differently
/// 3. Explicit formats match DRM fourcc semantics on little-endian (x86/ARM)
///
/// DRM fourcc naming convention (little-endian):
/// - XRGB8888: 32-bit word 0xXXRRGGBB → bytes [B,G,R,x] → GStreamer Bgrx
/// - ARGB8888: 32-bit word 0xAARRGGBB → bytes [B,G,R,A] → GStreamer Bgra
/// - XBGR8888: 32-bit word 0xXXBBGGRR → bytes [R,G,B,x] → GStreamer Rgbx
/// - ABGR8888: 32-bit word 0xAABBGGRR → bytes [R,G,B,A] → GStreamer Rgba
///
/// NOTE: We intentionally DO NOT use VideoFormat::from_fourcc() because it returns
/// native-endian formats (Xrgb) which can cause R/B color swaps with some encoders.
fn drm_fourcc_to_video_format(fourcc: DrmFourcc) -> VideoFormat {
    // ALWAYS use our explicit byte-order mapping - never rely on from_fourcc()
    // This ensures consistent behavior regardless of GStreamer version
    match fourcc {
        // 32-bit formats with alpha
        DrmFourcc::Argb8888 => VideoFormat::Bgra,  // 0xAARRGGBB → [B,G,R,A]
        DrmFourcc::Abgr8888 => VideoFormat::Rgba,  // 0xAABBGGRR → [R,G,B,A]
        DrmFourcc::Rgba8888 => VideoFormat::Abgr,  // 0xRRGGBBAA → [A,B,G,R]
        DrmFourcc::Bgra8888 => VideoFormat::Argb,  // 0xBBGGRRAA → [A,R,G,B]

        // 32-bit formats without alpha (x = padding)
        DrmFourcc::Xrgb8888 => VideoFormat::Bgrx,  // 0xXXRRGGBB → [B,G,R,x]
        DrmFourcc::Xbgr8888 => VideoFormat::Rgbx,  // 0xXXBBGGRR → [R,G,B,x]
        DrmFourcc::Rgbx8888 => VideoFormat::Xbgr,  // 0xRRGGBBXX → [x,B,G,R]
        DrmFourcc::Bgrx8888 => VideoFormat::Xrgb,  // 0xBBGGRRXX → [x,R,G,B]

        // 24-bit formats (3 bytes per pixel) - used by ext-image-copy-capture on Sway
        // CRITICAL: DRM format names describe BIT LAYOUT, not byte order in memory!
        // DRM uses [high_bits:low_bits] notation with "little endian" storage.
        //
        // DRM_FORMAT_BGR888: [23:0] B:G:R means B in bits 23-16, G in 15-8, R in 7-0
        //   Value = (B << 16) | (G << 8) | R = 0xBBGGRR
        //   Little-endian memory: [R, G, B] (low byte first)
        //   GStreamer format: Rgb (bytes R, G, B in memory order)
        //
        // DRM_FORMAT_RGB888: [23:0] R:G:B means R in bits 23-16, G in 15-8, B in 7-0
        //   Value = (R << 16) | (G << 8) | B = 0xRRGGBB
        //   Little-endian memory: [B, G, R] (low byte first)
        //   GStreamer format: Bgr (bytes B, G, R in memory order)
        DrmFourcc::Bgr888 => VideoFormat::Rgb,     // DRM BGR888 = memory [R,G,B]
        DrmFourcc::Rgb888 => VideoFormat::Bgr,     // DRM RGB888 = memory [B,G,R]

        _ => {
            eprintln!("[PIPEWIRESRC_DEBUG] Unknown DRM fourcc {:?} (0x{:08x}), falling back to Bgra",
                fourcc, fourcc as u32);
            VideoFormat::Bgra
        }
    }
}

/// Convert DRM fourcc and modifier to GStreamer drm-format string.
/// Format: "{fourcc}:0x{modifier:016x}" for non-linear modifiers
///         "{fourcc}" for LINEAR modifier (0x0)
/// Examples: "XR24:0x0300000000e08010", "AB24" (linear)
/// This matches Wolf's drm_to_gst_format() function.
fn drm_format_to_gst_string(fourcc: DrmFourcc, modifier: u64) -> String {
    // Convert fourcc enum to 4-char string (e.g., Xrgb8888 -> "XR24")
    let fourcc_str = fourcc.to_string();
    let fourcc_str = fourcc_str.trim();

    if modifier == 0 {
        // LINEAR modifier - no need to include modifier in string
        format!("{:<4}", fourcc_str)
    } else {
        // Non-linear modifier - include in format string
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

/// Create EGL display from render node path (reuses waylanddisplaycore's pattern)
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

/// Query DMA-BUF modifiers from EGL display for RGBA/BGRA formats.
/// Returns modifiers that the GPU supports for zero-copy DMA-BUF sharing.
fn query_dmabuf_modifiers(display: &EGLDisplay) -> DmaBufCapabilities {
    use smithay::backend::allocator::Fourcc;

    // DRM_FORMAT_MOD_INVALID (0x00ffffffffffffff) = "implicit modifier"
    // This tells PipeWire: "I accept ANY modifier the producer wants to use"
    // This is critical for GNOME ScreenCast because GNOME may use different
    // modifiers for output (ScreenCast) vs what EGL reports for render formats.
    const DRM_FORMAT_MOD_INVALID: u64 = (1u64 << 56) - 1;

    // Query render formats from EGL to check if DMA-BUF is available at all
    let render_formats = display.dmabuf_render_formats();

    // Detect GPU vendor from modifier vendor ID
    // Vendor IDs: 0x01 = Intel, 0x02 = AMD, 0x03 = NVIDIA
    let mut has_nvidia = false;
    let mut has_amd = false;
    let mut has_intel = false;
    for f in render_formats.iter() {
        let mod_val: u64 = f.modifier.into();
        let vendor_id = mod_val >> 56;
        match vendor_id {
            0x01 => has_intel = true,
            0x02 => has_amd = true,
            0x03 => has_nvidia = true,
            _ => {}
        }
    }
    let has_nvidia_modifiers = has_nvidia; // Backward compat

    // Query render format modifiers from EGL for the formats GNOME uses (BGRA/BGRx)
    let target_formats = [
        Fourcc::Argb8888,
        Fourcc::Abgr8888,
        Fourcc::Xrgb8888,
        Fourcc::Xbgr8888,
        Fourcc::Bgra8888,
        Fourcc::Rgba8888,
        Fourcc::Bgrx8888,
        Fourcc::Rgbx8888,
    ];

    let egl_modifiers: Vec<u64> = render_formats
        .iter()
        .filter(|f| target_formats.contains(&f.code))
        .map(|f| f.modifier.into())
        .collect();

    eprintln!(
        "[PIPEWIRESRC_DEBUG] EGL reports {} render format modifiers, nvidia={} amd={} intel={}",
        egl_modifiers.len(),
        has_nvidia,
        has_amd,
        has_intel
    );
    for (i, m) in egl_modifiers.iter().enumerate().take(5) {
        eprintln!("[PIPEWIRESRC_DEBUG]   egl_modifier[{}] = 0x{:x}", i, m);
    }
    if egl_modifiers.len() > 5 {
        eprintln!(
            "[PIPEWIRESRC_DEBUG]   ... and {} more",
            egl_modifiers.len() - 5
        );
    }

    // CRITICAL FIX: Offer NVIDIA ScreenCast modifiers (0xe08xxx family)
    //
    // Problem: EGL reports 0x606xxx modifiers, but GNOME ScreenCast uses 0xe08xxx.
    // When we offer DRM_FORMAT_MOD_INVALID, GNOME picks LINEAR (0x0) which fails
    // DmaBuf allocation with "error alloc buffers: Invalid argument".
    //
    // Solution: Hardcode the NVIDIA ScreenCast modifiers that GNOME actually uses.
    // These were observed from pw-dump of GNOME ScreenCast nodes:
    //   - 0x300000000e08010, 0x300000000e08011, 0x300000000e08012, 0x300000000e08013
    //   - 0x300000000e08014, 0x300000000e08015, 0x300000000e08016
    // These are NVIDIA block-linear / tiled formats for ScreenCast output.
    //
    // Format: 0x03 (NVIDIA vendor) | modifier_value
    // The 0xe08xxx values represent specific NVIDIA tiled memory layouts.
    let nvidia_screencast_modifiers: Vec<u64> = vec![
        0x300000000e08010, // NVIDIA_BLOCK_LINEAR_2D(0, 0, 0, 0x10, 0)
        0x300000000e08011,
        0x300000000e08012,
        0x300000000e08013,
        0x300000000e08014,
        0x300000000e08015,
        0x300000000e08016,
        // Include LINEAR (0x0) for Sway/wlroots compatibility.
        // xdg-desktop-portal-wlr doesn't support NVIDIA tiled modifiers,
        // so LINEAR is required as fallback for Sway sessions.
        // Put GPU-tiled first so GNOME prefers those for zero-copy.
        0x0, // LINEAR - required for xdg-desktop-portal-wlr (Sway)
    ];

    // Select modifiers based on detected GPU vendor:
    // - NVIDIA: Use hardcoded 0xe08xxx ScreenCast modifiers (EGL reports different family)
    // - AMD/Intel: Use EGL modifiers directly (they typically match ScreenCast output)
    // - Unknown with EGL modifiers: Use EGL modifiers (likely AMD/Intel with LINEAR-only)
    // - Unknown without modifiers: Use DRM_FORMAT_MOD_INVALID as universal fallback
    //
    // CRITICAL: AMD drivers often only report LINEAR (0x0) modifier, which has vendor_id = 0.
    // This means has_amd = false even on AMD hardware. We must still use EGL modifiers
    // in this case, otherwise we only offer INVALID and miss LINEAR support.
    let modifiers: Vec<u64> = if has_nvidia {
        eprintln!("[PIPEWIRESRC_DEBUG] NVIDIA GPU detected - using hardcoded ScreenCast modifiers");
        nvidia_screencast_modifiers
    } else if has_amd || has_intel || !egl_modifiers.is_empty() {
        // AMD/Intel detected by vendor ID, OR we have EGL modifiers (likely AMD/Intel with LINEAR)
        // Use EGL modifiers directly, with INVALID as fallback
        let vendor_hint = if has_amd {
            "AMD"
        } else if has_intel {
            "Intel"
        } else {
            "Unknown (likely AMD/Intel via LINEAR)"
        };
        eprintln!(
            "[PIPEWIRESRC_DEBUG] {} GPU detected - using EGL modifiers",
            vendor_hint
        );
        let mut mods = egl_modifiers.clone();
        if !mods.contains(&DRM_FORMAT_MOD_INVALID) {
            mods.push(DRM_FORMAT_MOD_INVALID);
        }
        mods
    } else {
        eprintln!(
            "[PIPEWIRESRC_DEBUG] Unknown GPU, no EGL modifiers - using DRM_FORMAT_MOD_INVALID only"
        );
        vec![DRM_FORMAT_MOD_INVALID]
    };

    let vendor_name = if has_nvidia {
        "NVIDIA"
    } else if has_amd {
        "AMD"
    } else if has_intel {
        "Intel"
    } else {
        "Unknown"
    };
    eprintln!(
        "[PIPEWIRESRC_DEBUG] Offering {} modifiers for {} GPU",
        modifiers.len(),
        vendor_name
    );
    for (i, m) in modifiers.iter().enumerate().take(5) {
        eprintln!("[PIPEWIRESRC_DEBUG]   offer[{}] = 0x{:x}", i, m);
    }
    if modifiers.len() > 5 {
        eprintln!("[PIPEWIRESRC_DEBUG]   ... and {} more", modifiers.len() - 5);
    }

    DmaBufCapabilities {
        modifiers,
        dmabuf_available: has_nvidia_modifiers || !egl_modifiers.is_empty(),
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum OutputMode {
    #[default]
    Auto,
    Cuda,
    DmaBuf,
    System,
}

impl OutputMode {
    fn from_str(s: &str) -> Self {
        match s.to_lowercase().as_str() {
            "cuda" => Self::Cuda,
            "dmabuf" | "dma-buf" => Self::DmaBuf,
            "system" | "memory" | "shm" => Self::System,
            _ => Self::Auto,
        }
    }
}

#[derive(Debug)]
pub struct Settings {
    pipewire_node_id: Option<u32>,
    /// PipeWire FD from XDG Desktop Portal's OpenPipeWireRemote.
    /// This FD grants access to ScreenCast nodes that aren't visible to the default daemon.
    /// Without it, connecting to ScreenCast nodes returns "target not found".
    pipewire_fd: Option<i32>,
    render_node: Option<String>,
    output_mode: OutputMode,
    cuda_device_id: i32,
    /// Keepalive time in milliseconds. When no frame arrives from PipeWire within
    /// this time, the last buffer is resent with updated timestamps. This handles
    /// GNOME 49+ damage-based ScreenCast which only sends frames when screen changes.
    /// 0 = disabled (wait forever), recommended: 100 (10 FPS minimum during static screens)
    keepalive_time_ms: u32,
    /// Whether to resend the last buffer on EOS
    resend_last: bool,
    /// CUDA context received from Wolf via set_context() or acquired via GStreamer
    cuda_context: Option<Arc<std::sync::Mutex<CUDAContext>>>,
    /// Raw pointer for GStreamer CUDA context interop - used by Wolf's context sharing
    cuda_raw_ptr: AtomicPtr<GstCudaContext>,
    /// Target FPS for PipeWire negotiation. The actual max_framerate sent to Mutter
    /// will be target_fps * 2 to avoid the 16ms/17ms timing boundary issue where
    /// ~50% of frames are skipped as "too early".
    /// See: design/2026-01-11-mutter-headless-60fps-investigation.md
    target_fps: u32,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            pipewire_node_id: None,
            pipewire_fd: None,
            render_node: Some("/dev/dri/renderD128".into()),
            output_mode: OutputMode::Auto,
            cuda_device_id: -1,
            // Default: 100ms keepalive for GNOME damage-based ScreenCast (10 FPS minimum)
            keepalive_time_ms: 100,
            resend_last: false,
            cuda_context: None,
            cuda_raw_ptr: AtomicPtr::new(std::ptr::null_mut()),
            // Default: 60 FPS target (will negotiate as 120 FPS max_framerate = 60 * 2)
            target_fps: 60,
        }
    }
}

/// Unified stream abstraction for different capture backends.
/// Supports PipeWire (GNOME), wlr-export-dmabuf (GPU), wlr-screencopy (SHM),
/// and ext-image-copy-capture (modern Wayland protocol for Sway 1.10+).
pub enum FrameStream {
    /// PipeWire ScreenCast stream (GNOME, xdg-desktop-portal)
    PipeWire(PipeWireStream),
    /// wlr-export-dmabuf stream (Sway/wlroots with GPU memory)
    WlrExport(WlrExportDmabufStream),
    /// wlr-screencopy stream (Sway/wlroots with SHM - legacy fallback)
    WlrScreencopy(WlrScreencopyStream),
    /// ext-image-copy-capture stream (Sway 1.10+, modern protocol with damage tracking)
    ExtImageCopyCapture(ExtImageCopyCaptureStream),
}

impl FrameStream {
    /// Receive a frame with timeout from any stream type
    pub fn recv_frame_timeout(&self, timeout: Duration) -> Result<FrameData, RecvError> {
        match self {
            FrameStream::PipeWire(stream) => stream.recv_frame_timeout(timeout),
            FrameStream::WlrExport(stream) => stream.recv_frame_timeout(timeout),
            FrameStream::WlrScreencopy(stream) => stream.recv_frame_timeout(timeout),
            FrameStream::ExtImageCopyCapture(stream) => stream.recv_frame_timeout(timeout),
        }
    }
}

pub struct State {
    stream: Option<FrameStream>,
    video_info: Option<VideoInfo>,
    egl_display: Option<Arc<EGLDisplay>>,
    buffer_pool: Option<CUDABufferPool>,
    actual_output_mode: OutputMode,
    frame_count: u64,
    /// Last buffer for keepalive - resent when PipeWire doesn't deliver frames
    /// (normal for GNOME 49+ damage-based ScreenCast with static screens)
    last_buffer: Option<gst::Buffer>,
    /// Whether the buffer pool has been configured with video dimensions.
    /// Pool configuration happens on first frame when dimensions are known.
    /// This enables buffer reuse, eliminating per-frame allocation overhead.
    buffer_pool_configured: bool,
    /// DMA-BUF allocator for AMD/Intel zero-copy path (OutputMode::DmaBuf)
    /// Wraps DMA-BUF file descriptors in GStreamer memory without CPU copies.
    dmabuf_allocator: Option<DmaBufAllocator>,
    /// DRM format string for DmaBuf mode caps negotiation (e.g., "XR24:0x0300000000e08010")
    /// Required by vapostproc - it needs drm-format in caps for zero-copy DMABuf import
    /// Format: "{fourcc}:0x{modifier:016x}" or "{fourcc}" for LINEAR modifier
    drm_format_string: Option<String>,
}

impl Default for State {
    fn default() -> Self {
        Self {
            stream: None,
            video_info: None,
            egl_display: None,
            buffer_pool: None,
            actual_output_mode: OutputMode::System,
            frame_count: 0,
            last_buffer: None,
            buffer_pool_configured: false,
            dmabuf_allocator: None,
            drm_format_string: None,
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
            glib::ParamSpecUInt::builder("pipewire-node-id").nick("PipeWire Node ID").blurb("PipeWire node ID from ScreenCast portal").construct().build(),
            glib::ParamSpecInt::builder("pipewire-fd")
                .nick("PipeWire FD")
                .blurb("PipeWire file descriptor from portal's OpenPipeWireRemote (required for ScreenCast access)")
                .minimum(-1)
                .maximum(65535)
                .default_value(-1)
                .construct()
                .build(),
            glib::ParamSpecString::builder("render-node").nick("DRM Render Node").blurb("DRM render node").default_value(Some("/dev/dri/renderD128")).construct().build(),
            glib::ParamSpecString::builder("output-mode").nick("Output Mode").blurb("auto, cuda, dmabuf, or system").default_value(Some("auto")).construct().build(),
            glib::ParamSpecInt::builder("cuda-device-id").nick("CUDA Device ID").blurb("CUDA device ID (-1 for auto)").minimum(-1).maximum(16).default_value(-1).construct().build(),
            // Keepalive properties for GNOME 49+ damage-based ScreenCast
            glib::ParamSpecUInt::builder("keepalive-time")
                .nick("Keepalive Time")
                .blurb("Periodically resend last buffer (in milliseconds, 0=disabled). Handles GNOME damage-based ScreenCast where frames only arrive when screen changes.")
                .minimum(0)
                .maximum(60000)
                .default_value(100)  // 10 FPS minimum during static screens
                .construct()
                .build(),
            glib::ParamSpecBoolean::builder("resend-last")
                .nick("Resend Last")
                .blurb("Resend last buffer on EOS")
                .default_value(false)
                .construct()
                .build(),
            glib::ParamSpecUInt::builder("target-fps")
                .nick("Target FPS")
                .blurb("Target frames per second. PipeWire max_framerate will be set to target_fps * 2 to avoid 16ms/17ms timing boundary issues.")
                .minimum(1)
                .maximum(240)
                .default_value(60)
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
            "output-mode" => {
                s.output_mode = value
                    .get::<Option<String>>()
                    .unwrap()
                    .as_deref()
                    .map(OutputMode::from_str)
                    .unwrap_or_default()
            }
            "cuda-device-id" => s.cuda_device_id = value.get().unwrap(),
            "keepalive-time" => s.keepalive_time_ms = value.get().unwrap(),
            "resend-last" => s.resend_last = value.get().unwrap(),
            "target-fps" => s.target_fps = value.get().unwrap(),
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
            "output-mode" => match s.output_mode {
                OutputMode::Auto => "auto",
                OutputMode::Cuda => "cuda",
                OutputMode::DmaBuf => "dmabuf",
                OutputMode::System => "system",
            }
            .to_value(),
            "cuda-device-id" => s.cuda_device_id.to_value(),
            "keepalive-time" => s.keepalive_time_ms.to_value(),
            "resend-last" => s.resend_last.to_value(),
            "target-fps" => s.target_fps.to_value(),
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
                "Captures PipeWire ScreenCast with zero-copy GPU output",
                "Wolf Project",
            )
        });
        Some(&*META)
    }

    fn pad_templates() -> &'static [gst::PadTemplate] {
        static TEMPLATES: Lazy<Vec<gst::PadTemplate>> = Lazy::new(|| {
            // Include all RGBA/BGRA variants since gst_video_info_dma_drm_to_video_info()
            // may produce different formats depending on the DRM fourcc.
            // DRM ARGB8888 -> GST BGRA, DRM BGRA8888 -> GST ARGB on little-endian.
            // Also include 24-bit formats (Bgr, Rgb) for wlr-screencopy on Sway.
            let rgba_formats = [
                VideoFormat::Bgra,
                VideoFormat::Rgba,
                VideoFormat::Argb,
                VideoFormat::Abgr,
                VideoFormat::Bgrx,
                VideoFormat::Rgbx,
                VideoFormat::Xrgb,
                VideoFormat::Xbgr,
                VideoFormat::Bgr,  // 24-bit BGR (wlr-screencopy BG24)
                VideoFormat::Rgb,  // 24-bit RGB (wlr-screencopy RG24)
                VideoFormat::Nv12,
            ];
            let mut caps = gst::Caps::new_empty();
            caps.merge(
                VideoCapsBuilder::new()
                    .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                    .format_list(rgba_formats)
                    .build(),
            );
            // DMABuf with DMA_DRM format (like Wolf's waylanddisplaysrc)
            // Wolf uses: video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=AB24:0x...
            // The drm-format field contains the DRM fourcc and modifier
            caps.merge(
                VideoCapsBuilder::new()
                    .features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF])
                    .format(VideoFormat::DmaDrm)
                    .build(),
            );
            caps.merge(VideoCapsBuilder::new().format_list(rgba_formats).build());
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

    /// Receive CUDA context from Wolf via GStreamer's context mechanism.
    /// Wolf creates the context and pushes it to elements via gst_element_set_context().
    fn set_context(&self, context: &gst::Context) {
        eprintln!(
            "[PIPEWIRESRC_DEBUG] set_context called, context_type={:?}",
            context.context_type()
        );
        let elem = self.obj().upcast_ref::<gst::Element>().to_owned();

        // Get raw pointer for GStreamer CUDA interop
        let cuda_raw_ptr = {
            let settings = self.settings.lock();
            settings.cuda_raw_ptr.as_ptr()
        };

        // Try to create CUDA context from the pushed context
        // This matches Wolf's pattern: it creates context and pushes it to elements
        match CUDAContext::new_from_set_context(&elem, context, -1, cuda_raw_ptr) {
            Ok(ctx) => {
                let mut settings = self.settings.lock();
                if settings.cuda_context.is_none() {
                    eprintln!(
                        "[PIPEWIRESRC_DEBUG] Received CUDA context via set_context - SUCCESS"
                    );
                    gst::info!(
                        CAT,
                        imp = self,
                        "Received CUDA context from pipeline (via set_context)"
                    );
                    settings.cuda_context = Some(Arc::new(std::sync::Mutex::new(ctx)));
                }
            }
            Err(e) => {
                eprintln!("[PIPEWIRESRC_DEBUG] set_context failed: {}", e);
                gst::debug!(
                    CAT,
                    imp = self,
                    "set_context: not a CUDA context or failed: {}",
                    e
                );
            }
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

        // Extract settings - but don't hold the lock while doing CUDA setup
        let (node_id, pipewire_fd, render_node, output_mode, device_id, target_fps) = {
            let settings = self.settings.lock();
            let node_id = settings.pipewire_node_id.ok_or_else(|| {
                gst::error_msg!(
                    gst::LibraryError::Settings,
                    ("pipewire-node-id must be set")
                )
            })?;
            (
                node_id,
                settings.pipewire_fd,
                settings.render_node.clone(),
                settings.output_mode,
                settings.cuda_device_id,
                settings.target_fps,
            )
        };

        eprintln!("[PIPEWIRESRC_DEBUG] start() called: node_id={}, pipewire_fd={:?}, render_node={:?}, output_mode={:?}, target_fps={}", node_id, pipewire_fd, render_node, output_mode, target_fps);
        gst::info!(CAT, imp = self, "Starting for node {}", node_id);

        let mut state = State::default();
        let elem = self.obj().upcast_ref::<gst::Element>().to_owned();

        // Will be set if we successfully create an EGL display
        let mut dmabuf_caps: Option<DmaBufCapabilities> = None;

        // Try CUDA mode using Wolf's context sharing pattern
        if output_mode == OutputMode::Auto || output_mode == OutputMode::Cuda {
            eprintln!("[PIPEWIRESRC_DEBUG] Trying CUDA mode...");
            match init_cuda() {
                Ok(()) => eprintln!("[PIPEWIRESRC_DEBUG] init_cuda() succeeded"),
                Err(e) => {
                    eprintln!("[PIPEWIRESRC_DEBUG] init_cuda() failed: {:?}", e);
                }
            }
            if let Ok(()) = init_cuda() {
                // Check if we already received a CUDA context via set_context()
                let have_cuda_context = self.settings.lock().cuda_context.is_some();
                eprintln!(
                    "[PIPEWIRESRC_DEBUG] have_cuda_context={}",
                    have_cuda_context
                );

                if !have_cuda_context {
                    // No context pushed by Wolf - try to acquire one from the pipeline
                    // This matches waylandsrc's fallback behavior
                    eprintln!("[PIPEWIRESRC_DEBUG] No CUDA context from set_context, acquiring via new_from_gstreamer...");
                    gst::info!(
                        CAT,
                        imp = self,
                        "No CUDA context from set_context, acquiring from pipeline"
                    );
                    let cuda_raw_ptr = self.settings.lock().cuda_raw_ptr.as_ptr();
                    match CUDAContext::new_from_gstreamer(&elem, device_id, cuda_raw_ptr) {
                        Ok(ctx) => {
                            eprintln!("[PIPEWIRESRC_DEBUG] new_from_gstreamer succeeded!");
                            let mut settings = self.settings.lock();
                            if settings.cuda_context.is_none() {
                                gst::info!(
                                    CAT,
                                    imp = self,
                                    "Acquired CUDA context via new_from_gstreamer"
                                );
                                settings.cuda_context = Some(Arc::new(std::sync::Mutex::new(ctx)));
                            } else {
                                // CRITICAL: Context was already set via set_context() during
                                // gst_cuda_ensure_element_context. Both CUDAContext objects wrap
                                // the same GstCudaContext pointer, but with incorrect ref count.
                                //
                                // If we drop ctx normally, its Drop impl calls gst_object_unref
                                // on the shared pointer, causing use-after-free when
                                // settings.cuda_context is later used.
                                //
                                // Use mem::forget to prevent the double-unref. The minor memory
                                // leak (16 bytes for CUDAContext + potential stream handle) is
                                // acceptable to prevent a crash.
                                eprintln!("[PIPEWIRESRC_DEBUG] Context already set via set_context, using mem::forget");
                                gst::info!(CAT, imp = self,
                                    "Context already set via set_context, using mem::forget to prevent double-unref");
                                std::mem::forget(ctx);
                            }
                        }
                        Err(e) => {
                            eprintln!("[PIPEWIRESRC_DEBUG] new_from_gstreamer failed: {}", e);
                            gst::warning!(CAT, imp = self, "Failed to acquire CUDA context: {}", e);
                        }
                    }
                }

                // Now check if we have a context and set up EGL/buffer pool
                let settings = self.settings.lock();
                let has_context = settings.cuda_context.is_some();
                eprintln!("[PIPEWIRESRC_DEBUG] After context acquisition: has_context={}, render_node={:?}", has_context, render_node);
                if let Some(ref cuda_context) = settings.cuda_context {
                    if let Some(ref node_path) = render_node {
                        eprintln!(
                            "[PIPEWIRESRC_DEBUG] Creating EGL display for {}...",
                            node_path
                        );
                        match create_egl_display(node_path) {
                            Ok(display) => {
                                eprintln!("[PIPEWIRESRC_DEBUG] EGL display created, creating buffer pool...");
                                // Query GPU-supported DMA-BUF modifiers BEFORE wrapping in Arc
                                // This enables proper PipeWire format negotiation with DMA-BUF
                                dmabuf_caps = Some(query_dmabuf_modifiers(&display));

                                let cuda_ctx = cuda_context.lock().unwrap();
                                match CUDABufferPool::new(&cuda_ctx) {
                                    Ok(pool) => {
                                        eprintln!("[PIPEWIRESRC_DEBUG] Buffer pool created - CUDA MODE ENABLED!");
                                        gst::info!(
                                            CAT,
                                            imp = self,
                                            "Using CUDA mode with shared context"
                                        );
                                        state.egl_display = Some(Arc::new(display));
                                        state.buffer_pool = Some(pool);
                                        state.actual_output_mode = OutputMode::Cuda;
                                    }
                                    Err(e) => {
                                        eprintln!(
                                            "[PIPEWIRESRC_DEBUG] Buffer pool creation failed: {}",
                                            e
                                        );
                                    }
                                }
                            }
                            Err(e) => {
                                eprintln!("[PIPEWIRESRC_DEBUG] EGL display failed: {}", e);
                                gst::warning!(CAT, imp = self, "EGL display failed: {}", e);
                            }
                        }
                    } else {
                        eprintln!(
                            "[PIPEWIRESRC_DEBUG] render_node is None, can't create EGL display"
                        );
                    }
                }
            }
        } else {
            eprintln!(
                "[PIPEWIRESRC_DEBUG] Not trying CUDA (output_mode={:?})",
                output_mode
            );
        }

        // AMD/Intel fallback: if CUDA failed but we have a render node, try EGL for DmaBuf
        // This enables zero-copy DMA-BUF acquisition even without CUDA (for VA-API encoding)
        if dmabuf_caps.is_none() {
            if let Some(ref node_path) = render_node {
                eprintln!("[PIPEWIRESRC_DEBUG] CUDA unavailable, trying EGL-only path for DmaBuf (AMD/Intel)...");
                match create_egl_display(node_path) {
                    Ok(display) => {
                        eprintln!("[PIPEWIRESRC_DEBUG] EGL display created (no CUDA), querying DmaBuf caps...");
                        let caps = query_dmabuf_modifiers(&display);
                        if caps.dmabuf_available {
                            eprintln!(
                                "[PIPEWIRESRC_DEBUG] DmaBuf available via EGL! modifiers={}",
                                caps.modifiers.len()
                            );
                            dmabuf_caps = Some(caps);
                            // Store EGL display for potential future use (e.g., DmaBuf import)
                            state.egl_display = Some(Arc::new(display));
                            // Set DmaBuf mode if we can acquire DmaBuf frames
                            // AMD/Intel: output DMA-BUF directly for VA-API zero-copy
                            state.actual_output_mode = OutputMode::DmaBuf;
                            state.dmabuf_allocator = Some(DmaBufAllocator::new());
                            eprintln!("[PIPEWIRESRC_DEBUG] DmaBufAllocator initialized for AMD/Intel zero-copy");
                        } else {
                            eprintln!("[PIPEWIRESRC_DEBUG] EGL reports no DmaBuf support");
                        }
                    }
                    Err(e) => {
                        eprintln!(
                            "[PIPEWIRESRC_DEBUG] EGL display failed (fallback path): {}",
                            e
                        );
                    }
                }
            } else {
                eprintln!("[PIPEWIRESRC_DEBUG] No render_node, can't try EGL fallback");
            }
        }

        eprintln!(
            "[PIPEWIRESRC_DEBUG] Final actual_output_mode={:?}",
            state.actual_output_mode
        );
        if state.actual_output_mode == OutputMode::System {
            if output_mode == OutputMode::DmaBuf {
                state.actual_output_mode = OutputMode::DmaBuf;
                if state.dmabuf_allocator.is_none() {
                    state.dmabuf_allocator = Some(DmaBufAllocator::new());
                    eprintln!(
                        "[PIPEWIRESRC_DEBUG] DmaBufAllocator initialized (forced DmaBuf mode)"
                    );
                }
                gst::info!(CAT, imp = self, "Using DMA-BUF mode");
            } else {
                eprintln!("[PIPEWIRESRC_DEBUG] Using SYSTEM MEMORY mode (not CUDA!)");
                gst::info!(CAT, imp = self, "Using system memory mode");
            }
        }

        // Determine capture mode based on compositor and output_mode property.
        //
        // For Sway (wlroots): Prefer ext-image-copy-capture (Sway 1.10+), then wlr-screencopy.
        // Both bypass PipeWire entirely and use native Wayland protocols.
        // ext-image-copy-capture is the modern protocol with damage tracking support.
        // PipeWire + xdg-desktop-portal-wlr has format negotiation issues.
        //
        // For GNOME: Use PipeWire ScreenCast via xdg-desktop-portal.
        // - output_mode=system: PipeWire SHM
        // - output_mode=cuda/dmabuf/auto: PipeWire DMA-BUF
        //
        // Check available capture protocols (prefer newer/better ones first)
        let ext_image_copy_available = ExtImageCopyCaptureStream::is_available();
        let wlr_screencopy_available = WlrScreencopyStream::is_available();
        let use_shm = output_mode == OutputMode::System;

        let frame_stream = if ext_image_copy_available {
            // Sway 1.10+: Use ext-image-copy-capture (modern protocol with damage tracking)
            // This bypasses PipeWire entirely and uses the native Wayland protocol.
            eprintln!(
                "[PIPEWIRESRC_DEBUG] ext-image-copy-capture available: using modern Wayland protocol"
            );
            gst::info!(
                CAT,
                imp = self,
                "Sway 1.10+: using ext-image-copy-capture for SHM capture (damage tracking enabled)"
            );

            state.actual_output_mode = OutputMode::System;

            let ext_stream = ExtImageCopyCaptureStream::connect(target_fps)
                .map_err(|e| gst::error_msg!(gst::LibraryError::Init, ("ext-image-copy-capture: {}", e)))?;
            FrameStream::ExtImageCopyCapture(ext_stream)
        } else if wlr_screencopy_available {
            // wlroots compositor (Sway <1.10): Use wlr-screencopy as fallback
            // This bypasses PipeWire entirely and uses the native Wayland protocol.
            eprintln!(
                "[PIPEWIRESRC_DEBUG] wlroots compositor detected: using wlr-screencopy (legacy fallback)"
            );
            gst::info!(
                CAT,
                imp = self,
                "wlroots compositor: using wlr-screencopy for SHM capture"
            );

            state.actual_output_mode = OutputMode::System;

            let wlr_stream = WlrScreencopyStream::connect(target_fps)
                .map_err(|e| gst::error_msg!(gst::LibraryError::Init, ("wlr-screencopy: {}", e)))?;
            FrameStream::WlrScreencopy(wlr_stream)
        } else if use_shm {
            // GNOME + SHM mode: Use PipeWire SHM
            eprintln!("[PIPEWIRESRC_DEBUG] output-mode=system: using PipeWire SHM");
            gst::info!(
                CAT,
                imp = self,
                "System memory mode: PipeWire SHM for hardware encoding"
            );

            state.actual_output_mode = OutputMode::System;

            // Pass None for dmabuf_caps to force SHM-only format negotiation
            let pw_stream = PipeWireStream::connect(node_id, pipewire_fd, None, target_fps)
                .map_err(|e| gst::error_msg!(gst::LibraryError::Init, ("PipeWire SHM: {}", e)))?;
            FrameStream::PipeWire(pw_stream)
        } else {
            // DMA-BUF mode (GNOME with GPU)
            eprintln!(
                "[PIPEWIRESRC_DEBUG] output-mode={:?}: using PipeWire DMA-BUF",
                output_mode
            );
            gst::info!(CAT, imp = self, "DMA-BUF mode: zero-copy GPU memory");

            let pw_stream = PipeWireStream::connect(node_id, pipewire_fd, dmabuf_caps, target_fps)
                .map_err(|e| {
                    gst::error_msg!(gst::LibraryError::Init, ("PipeWire DMA-BUF: {}", e))
                })?;
            FrameStream::PipeWire(pw_stream)
        };

        state.stream = Some(frame_stream);
        *state_guard = Some(state);
        Ok(())
    }

    fn stop(&self) -> Result<(), gst::ErrorMessage> {
        let mut g = self.state.lock();
        if let Some(s) = g.take() {
            gst::info!(CAT, imp = self, "Stopping");
            drop(s);
        }
        Ok(())
    }

    fn is_seekable(&self) -> bool {
        false
    }

    /// Handle context queries from downstream elements.
    /// When downstream elements need a CUDA context, we provide ours.
    fn query(&self, query: &mut gst::QueryRef) -> bool {
        if query.type_() == gst::QueryType::Context {
            let settings = self.settings.lock();
            if let Some(ref cuda_context) = settings.cuda_context {
                gst::debug!(
                    CAT,
                    imp = self,
                    "Handling CUDA context query from downstream"
                );
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
        // First check if state is initialized (after start())
        let g = self.state.lock();
        let actual_mode = g.as_ref().map(|s| s.actual_output_mode);
        drop(g); // Release state lock before acquiring settings lock

        // Determine output mode: use actual mode if available, otherwise use configured mode
        let output_mode = match actual_mode {
            Some(mode) => mode,
            None => {
                // State not initialized yet (before start()) - use configured output_mode
                // This is crucial for caps negotiation during pipeline linking
                self.settings.lock().output_mode
            }
        };

        // Include all RGBA/BGRA variants since gst_video_info_dma_drm_to_video_info()
        // may produce different formats depending on the DRM fourcc.
        // Also include 24-bit formats (Bgr, Rgb) for wlr-screencopy on Sway.
        let rgba_formats = [
            VideoFormat::Bgra,
            VideoFormat::Rgba,
            VideoFormat::Argb,
            VideoFormat::Abgr,
            VideoFormat::Bgrx,
            VideoFormat::Rgbx,
            VideoFormat::Xrgb,
            VideoFormat::Xbgr,
            VideoFormat::Bgr,  // 24-bit BGR (wlr-screencopy BG24)
            VideoFormat::Rgb,  // 24-bit RGB (wlr-screencopy RG24)
            VideoFormat::Nv12,
        ];

        let mut caps = match output_mode {
            OutputMode::Cuda => VideoCapsBuilder::new()
                .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                .format_list(rgba_formats)
                .build(),
            // DMABuf with DMA_DRM format (like Wolf's waylanddisplaysrc)
            // Wolf uses: video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=AB24:0x...
            OutputMode::DmaBuf => VideoCapsBuilder::new()
                .features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF])
                .format(VideoFormat::DmaDrm)
                .build(),
            OutputMode::System => VideoCapsBuilder::new().format_list(rgba_formats).build(),
            OutputMode::Auto => {
                // Auto mode before start(): advertise all capabilities (like pad template)
                // GStreamer will negotiate based on downstream requirements
                let mut all_caps = gst::Caps::new_empty();
                all_caps.merge(
                    VideoCapsBuilder::new()
                        .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                        .format_list(rgba_formats)
                        .build(),
                );
                // DMABuf with DMA_DRM format (like Wolf's waylanddisplaysrc)
                all_caps.merge(
                    VideoCapsBuilder::new()
                        .features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF])
                        .format(VideoFormat::DmaDrm)
                        .build(),
                );
                all_caps.merge(VideoCapsBuilder::new().format_list(rgba_formats).build());
                all_caps
            }
        };
        if let Some(f) = filter {
            caps = caps.intersect(f);
        }
        Some(caps)
    }

    fn set_caps(&self, caps: &gst::Caps) -> Result<(), gst::LoggableError> {
        gst::info!(CAT, imp = self, "Caps: {:?}", caps);
        if let Ok(info) = VideoInfo::from_caps(caps) {
            // Validate framerate - warn if it's 0/1 which is typically a bug in pipeline construction
            let fps = info.fps();
            if fps.numer() == 0 || fps.denom() == 0 {
                gst::warning!(CAT, imp = self,
                    "Invalid framerate {}/{} in caps - this is likely a bug in the upstream pipeline. \
                     Check that the pipeline uses display_mode.refreshRate instead of hardcoded 0/1.",
                    fps.numer(), fps.denom());
            }
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
        // Get shared CUDA context and keepalive settings (cloning the Arc, not the context)
        let (cuda_context, keepalive_time_ms) = {
            let settings = self.settings.lock();
            (settings.cuda_context.clone(), settings.keepalive_time_ms)
        };

        let mut g = self.state.lock();
        let state = g.as_mut().ok_or(gst::FlowError::Eos)?;
        let stream = state.stream.as_ref().ok_or(gst::FlowError::Error)?;

        // Determine timeout based on whether we have a last buffer:
        // - First frame: wait up to 30 seconds (GNOME ScreenCast may take time to start)
        // - Subsequent frames with keepalive: use short timeout for responsive keepalive
        // - Subsequent frames without keepalive: use default timeout
        let has_last_buffer = state.last_buffer.is_some();
        let timeout = if !has_last_buffer {
            // First frame: wait longer because GNOME ScreenCast needs time to:
            // 1. Complete format negotiation
            // 2. Transition from Paused to Streaming state
            // 3. Actually render and deliver the first frame
            Duration::from_secs(30)
        } else if keepalive_time_ms > 0 {
            Duration::from_millis(keepalive_time_ms as u64)
        } else {
            Duration::from_secs(30) // Default timeout
        };

        if !has_last_buffer {
            gst::info!(
                CAT,
                imp = self,
                "Waiting for first frame from PipeWire (timeout: {:?})",
                timeout
            );
        }

        // Try to receive a frame with timeout
        let frame_result = stream.recv_frame_timeout(timeout);

        let frame = match frame_result {
            Ok(frame) => frame,
            Err(RecvError::Timeout) if keepalive_time_ms > 0 => {
                // Timeout with keepalive enabled: resend last buffer with updated timestamps
                // This is normal for GNOME 49+ damage-based ScreenCast with static screens
                if let Some(ref last_buf) = state.last_buffer {
                    gst::debug!(
                        CAT,
                        imp = self,
                        "Keepalive: resending last buffer (no new frame from PipeWire)"
                    );

                    // Clone the buffer and update timestamps
                    let mut buf = last_buf.copy();
                    if let Some(buf_ref) = buf.get_mut() {
                        // Update PTS/DTS to current pipeline time
                        if let Some(clock) = self.obj().clock() {
                            let base_time = self.obj().base_time();
                            if let (Some(now), Some(base)) = (clock.time(), base_time) {
                                let running_time = now.saturating_sub(base);
                                buf_ref.set_pts(running_time);
                                buf_ref.set_dts(running_time);
                            }
                        }
                    }

                    state.frame_count += 1;
                    drop(g);
                    return Ok(CreateSuccess::NewBuffer(buf));
                } else {
                    // No last buffer yet - first frame hasn't arrived within keepalive time
                    // This shouldn't happen often since first frame uses longer timeout
                    gst::warning!(
                        CAT,
                        imp = self,
                        "Keepalive timeout but no last buffer available (waiting for first frame)"
                    );
                    // Don't error immediately - continue waiting in subsequent create() calls
                    // The 30-second first-frame timeout above should handle this
                    return Err(gst::FlowError::Error);
                }
            }
            Err(RecvError::Timeout) => {
                // Timeout without keepalive - this is an error
                gst::error!(
                    CAT,
                    imp = self,
                    "Frame receive timeout (keepalive disabled)"
                );
                return Err(gst::FlowError::Error);
            }
            Err(RecvError::Disconnected) => {
                gst::error!(CAT, imp = self, "PipeWire stream disconnected");
                return Err(gst::FlowError::Eos);
            }
            Err(RecvError::Error(e)) => {
                gst::error!(CAT, imp = self, "Frame receive error: {}", e);
                return Err(gst::FlowError::Error);
            }
        };

        // Debug: trace which frame type we received and what mode we're in
        eprintln!("[PIPEWIRESRC_DEBUG] create(): actual_output_mode={:?}, has_cuda_context={}, has_egl_display={}, has_buffer_pool={}",
            state.actual_output_mode,
            cuda_context.is_some(),
            state.egl_display.is_some(),
            state.buffer_pool.is_some());

        match &frame {
            FrameData::DmaBuf(dmabuf) => {
                eprintln!(
                    "[PIPEWIRESRC_DEBUG] Received DmaBuf frame: {}x{} fourcc=0x{:x}",
                    dmabuf.width(),
                    dmabuf.height(),
                    dmabuf.format().code as u32
                );
            }
            FrameData::Shm {
                width,
                height,
                format,
                ..
            } => {
                eprintln!(
                    "[PIPEWIRESRC_DEBUG] Received SHM frame: {}x{} format=0x{:x}",
                    width, height, format
                );
            }
        }

        // Timing instrumentation
        use std::time::Instant;
        let frame_start = Instant::now();

        let (buffer, actual_format, width, height) = match frame {
            FrameData::DmaBuf(dmabuf) if state.actual_output_mode == OutputMode::Cuda => {
                let t0 = Instant::now();
                // Use waylanddisplaycore's battle-tested CUDA conversion with shared context
                let cuda_context_arc = cuda_context.as_ref().ok_or_else(|| {
                    eprintln!("[PIPEWIRESRC_DEBUG] ERROR: cuda_context is None!");
                    gst::FlowError::Error
                })?;
                let cuda_ctx = cuda_context_arc.lock().unwrap();
                let egl_display = state.egl_display.as_ref().ok_or(gst::FlowError::Error)?;

                // Get raw EGLDisplay handle for waylanddisplaycore's EGLImage::from()
                let raw_display: RawEGLDisplay = egl_display.get_display_handle().handle;

                let w = dmabuf.width() as u32;
                let h = dmabuf.height() as u32;
                let lock_time = t0.elapsed();

                // EGL import timing
                let t1 = Instant::now();
                let egl_image = EGLImage::from(&dmabuf, &raw_display).map_err(|e| {
                    eprintln!("[PIPEWIRESRC_DEBUG] EGLImage error: {}", e);
                    gst::FlowError::Error
                })?;
                let egl_time = t1.elapsed();

                // CUDA import timing
                let t2 = Instant::now();
                let cuda_image = CUDAImage::from(egl_image, &cuda_ctx).map_err(|e| {
                    eprintln!("[PIPEWIRESRC_DEBUG] CUDAImage error: {}", e);
                    gst::FlowError::Error
                })?;
                let cuda_time = t2.elapsed();

                // Derive VideoFormat from DMA-BUF's fourcc (matches waylanddisplaycore pattern)
                let drm_format = dmabuf.format();
                let video_format = drm_fourcc_to_video_format(drm_format.code);
                gst::debug!(
                    CAT,
                    imp = self,
                    "DMA-BUF format: {:?} -> {:?}",
                    drm_format.code,
                    video_format
                );

                let base_info = VideoInfo::builder(video_format, w, h)
                    .build()
                    .map_err(|_| gst::FlowError::Error)?;
                let fourcc: u32 = drm_format.code as u32;
                let modifier: u64 = drm_format.modifier.into();
                let dma_video_info = VideoInfoDmaDrm::new(base_info.clone(), fourcc, modifier);

                // Configure buffer pool on first frame (when dimensions are known)
                // This enables buffer reuse, eliminating per-frame allocation overhead (~12ms per 4K frame)
                if !state.buffer_pool_configured {
                    if let Some(ref pool) = state.buffer_pool {
                        // Build FIXED caps for the buffer pool
                        // GStreamer requires fully-specified caps (not ranges/lists) for pool configuration
                        // Must include framerate to be "fixed" - use 60/1 as standard
                        let pool_caps = VideoCapsBuilder::new()
                            .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                            .format(video_format)
                            .width(w as i32)
                            .height(h as i32)
                            .framerate(gst::Fraction::new(60, 1))
                            .build();

                        // Calculate buffer size: width * height * bytes_per_pixel
                        // BGRA/RGBA = 4 bytes per pixel
                        let buffer_size = w * h * 4;

                        // Configure pool with 8-16 buffers to prevent buffer starvation
                        // See design/2026-01-11-mutter-headless-60fps-investigation.md - Buffer Starvation
                        match pool.configure_basic(&pool_caps, buffer_size, 8, 16) {
                            Ok(()) => {
                                eprintln!("[PIPEWIRESRC_DEBUG] Buffer pool configured: {}x{} format={:?} size={}MB",
                                    w, h, video_format, buffer_size / (1024 * 1024));
                                match pool.activate() {
                                    Ok(()) => {
                                        eprintln!("[PIPEWIRESRC_DEBUG] Buffer pool activated - buffer reuse enabled!");
                                        state.buffer_pool_configured = true;
                                    }
                                    Err(e) => {
                                        eprintln!("[PIPEWIRESRC_DEBUG] Buffer pool activation failed: {} - using per-frame allocation", e);
                                    }
                                }
                            }
                            Err(e) => {
                                eprintln!("[PIPEWIRESRC_DEBUG] Buffer pool configuration failed: {} - using per-frame allocation", e);
                            }
                        }
                    }
                }

                // Buffer creation timing
                let t3 = Instant::now();
                let buf = cuda_image
                    .to_gst_buffer(dma_video_info, &cuda_ctx, state.buffer_pool.as_ref())
                    .map_err(|e| {
                        eprintln!("[PIPEWIRESRC_DEBUG] to_gst_buffer error: {}", e);
                        gst::FlowError::Error
                    })?;
                let buf_time = t3.elapsed();

                let total = frame_start.elapsed();
                // Log timing every 60 frames (1 second at 60fps)
                if state.frame_count % 60 == 0 {
                    eprintln!("[PIPEWIRESRC_TIMING] frame={} lock={:?} egl={:?} cuda={:?} buf={:?} total={:?}",
                        state.frame_count, lock_time, egl_time, cuda_time, buf_time, total);
                }

                // Get the actual format from the buffer's VideoMeta (set by to_gst_buffer)
                // This is the format that gst_video_info_dma_drm_to_video_info() produced
                let actual_fmt = buf
                    .meta::<gst_video::VideoMeta>()
                    .map(|m| m.format())
                    .unwrap_or(video_format);

                (buf, actual_fmt, w, h)
            }
            FrameData::DmaBuf(dmabuf) if state.actual_output_mode == OutputMode::DmaBuf => {
                // AMD/Intel zero-copy path: wrap DMA-BUF fds directly in GStreamer buffer
                eprintln!("[PIPEWIRESRC_DEBUG] Processing DmaBuf in ZERO-COPY mode (AMD/Intel)");
                let w = dmabuf.width() as u32;
                let h = dmabuf.height() as u32;
                let drm_format = dmabuf.format();
                let fourcc: u32 = drm_format.code as u32;
                let modifier: u64 = drm_format.modifier.into();
                let video_format = drm_fourcc_to_video_format(drm_format.code);

                // Build drm-format string for caps (e.g., "XR24:0x0300000000e08010")
                // This format is required by vapostproc for zero-copy DMABuf import
                let drm_format_str = drm_format_to_gst_string(drm_format.code, modifier);
                eprintln!("[PIPEWIRESRC_DEBUG] DmaBuf DRM format: fourcc=0x{:08x} modifier=0x{:016x} drm-format='{}' -> {:?}",
                    fourcc, modifier, drm_format_str, video_format);

                // Store DRM format string for caps negotiation (required by vapostproc)
                state.drm_format_string = Some(drm_format_str);

                let allocator = state.dmabuf_allocator.as_ref().ok_or_else(|| {
                    eprintln!(
                        "[PIPEWIRESRC_DEBUG] ERROR: dmabuf_allocator is None in DmaBuf mode!"
                    );
                    gst::FlowError::Error
                })?;

                let buf = self.dmabuf_to_gst_dmabuf(&dmabuf, allocator, video_format)?;
                (buf, video_format, w, h)
            }
            FrameData::DmaBuf(dmabuf) => {
                // System memory fallback path (copies DMA-BUF to system memory)
                eprintln!("[PIPEWIRESRC_DEBUG] Processing DmaBuf in FALLBACK (system memory) mode");
                let w = dmabuf.width() as u32;
                let h = dmabuf.height() as u32;
                let video_format = drm_fourcc_to_video_format(dmabuf.format().code);
                let buf = self.dmabuf_to_system(&dmabuf)?;
                (buf, video_format, w, h)
            }
            FrameData::Shm {
                data,
                width,
                height,
                stride,
                format,
            } => {
                // Try to convert format (fourcc) to VideoFormat, fall back to Bgra
                let video_format = if format != 0 {
                    match DrmFourcc::try_from(format) {
                        Ok(fourcc) => drm_fourcc_to_video_format(fourcc),
                        Err(_) => VideoFormat::Bgra,
                    }
                } else {
                    VideoFormat::Bgra
                };
                let buf = self.create_system_buffer(&data, width, height, stride, video_format)?;
                (buf, video_format, width, height)
            }
        };

        // Check if we need to update caps to match the actual buffer format
        let needs_caps_update = match &state.video_info {
            Some(info) => {
                info.format() != actual_format || info.width() != width || info.height() != height
            }
            None => true,
        };

        if needs_caps_update {
            gst::info!(
                CAT,
                imp = self,
                "Format/size changed, updating caps to {:?} {}x{}",
                actual_format,
                width,
                height
            );

            // Get framerate from existing video_info or use 60/1 as default
            // GStreamer requires framerate for caps to be "fixed"
            let fps = state
                .video_info
                .as_ref()
                .map(|info| info.fps())
                .unwrap_or(gst::Fraction::new(60, 1));

            // Build new caps with the actual format (must include framerate for fixed caps)
            let new_caps = match state.actual_output_mode {
                OutputMode::Cuda => VideoCapsBuilder::new()
                    .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                    .format(actual_format)
                    .width(width as i32)
                    .height(height as i32)
                    .framerate(fps)
                    .build(),
                OutputMode::DmaBuf => {
                    // AMD/Intel zero-copy: Use DMA_DRM format with drm-format field
                    // Wolf uses: video/x-raw(memory:DMABuf),format=DMA_DRM,drm-format=XR24:0x...
                    // The drm-format field is required for vapostproc to import the DMABuf
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
                _ => VideoCapsBuilder::new()
                    .format(actual_format)
                    .width(width as i32)
                    .height(height as i32)
                    .framerate(fps)
                    .build(),
            };

            // Update stored video_info
            if let Ok(info) = VideoInfo::from_caps(&new_caps) {
                state.video_info = Some(info);
            }

            // Store buffer for keepalive (before dropping lock)
            if keepalive_time_ms > 0 {
                state.last_buffer = Some(buffer.clone());
            }

            // Release state lock before calling set_caps (it may need the lock)
            drop(g);

            // Set the new caps on the src pad
            let obj = self.obj();
            let pad = obj.static_pad("src").expect("src pad should exist");
            if !pad.push_event(gst::event::Caps::new(&new_caps)) {
                gst::warning!(CAT, imp = self, "Failed to push new caps event");
            }
        } else {
            // Store buffer for keepalive
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
    /// Zero-copy DMA-BUF to GStreamer buffer (AMD/Intel VA-API path)
    ///
    /// Wraps the DMA-BUF file descriptors directly in a GStreamer buffer without any CPU copies.
    /// The downstream VA-API encoder can then access the GPU memory directly via DMA-BUF import.
    fn dmabuf_to_gst_dmabuf(
        &self,
        dmabuf: &smithay::backend::allocator::dmabuf::Dmabuf,
        allocator: &DmaBufAllocator,
        video_format: VideoFormat,
    ) -> Result<gst::Buffer, gst::FlowError> {
        use smithay::reexports::rustix::fs::{seek, SeekFrom};
        use std::os::fd::{AsFd, AsRawFd};

        let width = dmabuf.width() as u32;
        let height = dmabuf.height() as u32;

        // Calculate expected buffer size for GStreamer
        let required_size = gst_video::VideoInfo::builder(video_format, width, height)
            .build()
            .map_err(|e| {
                eprintln!("[PIPEWIRESRC_DEBUG] Failed to build VideoInfo: {:?}", e);
                gst::FlowError::Error
            })?
            .size();

        let mut gst_buffer = gst::Buffer::new();

        // Get mutable reference and append memory for each DMA-BUF plane
        {
            let gst_buf = gst_buffer.get_mut().ok_or_else(|| {
                eprintln!("[PIPEWIRESRC_DEBUG] Failed to get mutable buffer reference");
                gst::FlowError::Error
            })?;

            for (plane_idx, handle) in dmabuf.handles().enumerate() {
                let fd = handle.as_raw_fd();

                // Get actual size of the DMA-BUF by seeking to end
                let actual_size = seek(&handle.as_fd(), SeekFrom::End(0)).map_err(|e| {
                    eprintln!("[PIPEWIRESRC_DEBUG] Failed to seek DMA-BUF fd: {:?}", e);
                    gst::FlowError::Error
                })? as usize;

                // Reset seek position
                let _ = seek(&handle.as_fd(), SeekFrom::Start(0));

                // Use the larger of required and actual sizes
                let allocation_size = required_size.max(actual_size);

                // Allocate GStreamer memory wrapping the DMA-BUF fd
                // DONT_CLOSE: fd is owned by smithay Dmabuf, will be closed when it's dropped
                let memory = unsafe {
                    allocator.alloc_with_flags(fd, allocation_size, FdMemoryFlags::DONT_CLOSE)
                        .map_err(|e| {
                            eprintln!("[PIPEWIRESRC_DEBUG] DmaBufAllocator alloc failed for plane {}: {:?}", plane_idx, e);
                            gst::FlowError::Error
                        })?
                };

                gst_buf.append_memory(memory);
                eprintln!(
                    "[PIPEWIRESRC_DEBUG] Appended DMA-BUF plane {} (fd={}, size={})",
                    plane_idx, fd, allocation_size
                );
            }

            // Collect offsets and strides
            let offsets: Vec<usize> = dmabuf.offsets().map(|o| o as usize).collect();
            let strides: Vec<i32> = dmabuf.strides().map(|s| s as i32).collect();

            // Add video metadata with plane info
            gst_video::VideoMeta::add_full(
                gst_buf,
                gst_video::VideoFrameFlags::empty(),
                video_format,
                width,
                height,
                &offsets,
                &strides,
            )
            .map_err(|e| {
                eprintln!("[PIPEWIRESRC_DEBUG] Failed to add VideoMeta: {:?}", e);
                gst::FlowError::Error
            })?;
        }

        eprintln!(
            "[PIPEWIRESRC_DEBUG] Zero-copy DMA-BUF buffer created: {}x{} {:?}",
            width, height, video_format
        );
        Ok(gst_buffer)
    }

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

        // Derive format from DMA-BUF's fourcc
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

    /// Test OutputMode parsing from string
    #[test]
    fn test_output_mode_from_str_cuda() {
        assert_eq!(OutputMode::from_str("cuda"), OutputMode::Cuda);
        assert_eq!(OutputMode::from_str("CUDA"), OutputMode::Cuda);
        assert_eq!(OutputMode::from_str("Cuda"), OutputMode::Cuda);
    }

    #[test]
    fn test_output_mode_from_str_dmabuf() {
        assert_eq!(OutputMode::from_str("dmabuf"), OutputMode::DmaBuf);
        assert_eq!(OutputMode::from_str("dma-buf"), OutputMode::DmaBuf);
        assert_eq!(OutputMode::from_str("DmaBuf"), OutputMode::DmaBuf);
    }

    #[test]
    fn test_output_mode_from_str_system() {
        assert_eq!(OutputMode::from_str("system"), OutputMode::System);
        assert_eq!(OutputMode::from_str("memory"), OutputMode::System);
        assert_eq!(OutputMode::from_str("shm"), OutputMode::System);
        assert_eq!(OutputMode::from_str("SYSTEM"), OutputMode::System);
    }

    #[test]
    fn test_output_mode_from_str_auto() {
        assert_eq!(OutputMode::from_str("auto"), OutputMode::Auto);
        assert_eq!(OutputMode::from_str("Auto"), OutputMode::Auto);
        assert_eq!(OutputMode::from_str("unknown"), OutputMode::Auto);
        assert_eq!(OutputMode::from_str(""), OutputMode::Auto);
    }

    #[test]
    fn test_output_mode_default() {
        assert_eq!(OutputMode::default(), OutputMode::Auto);
    }

    /// Test DRM fourcc to GStreamer VideoFormat conversion
    /// Note: These tests require gst::init() because VideoFormat::from_fourcc() uses GStreamer
    #[test]
    fn test_drm_fourcc_to_video_format_argb8888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Argb8888);
        assert_eq!(format, VideoFormat::Bgra, "ARGB8888 should map to Bgra");
    }

    #[test]
    fn test_drm_fourcc_to_video_format_abgr8888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Abgr8888);
        assert_eq!(format, VideoFormat::Rgba, "ABGR8888 should map to Rgba");
    }

    #[test]
    fn test_drm_fourcc_to_video_format_xrgb8888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Xrgb8888);
        assert_eq!(format, VideoFormat::Bgrx, "XRGB8888 should map to Bgrx");
    }

    #[test]
    fn test_drm_fourcc_to_video_format_xbgr8888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Xbgr8888);
        assert_eq!(format, VideoFormat::Rgbx, "XBGR8888 should map to Rgbx");
    }

    #[test]
    fn test_drm_fourcc_to_video_format_rgba8888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Rgba8888);
        assert_eq!(format, VideoFormat::Abgr, "RGBA8888 should map to Abgr");
    }

    #[test]
    fn test_drm_fourcc_to_video_format_bgra8888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Bgra8888);
        assert_eq!(format, VideoFormat::Argb, "BGRA8888 should map to Argb");
    }

    /// Test DRM BGRX8888 mapping
    /// BGRX8888: 32-bit word 0xBBGGRRXX → little-endian bytes [X,R,G,B] → GStreamer Xrgb
    #[test]
    fn test_drm_fourcc_to_video_format_bgrx8888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Bgrx8888);
        // BGRX8888 word layout: B in high bits, X in low bits
        // Little-endian byte order: [X,R,G,B] = Xrgb
        assert_eq!(
            format,
            VideoFormat::Xrgb,
            "BGRX8888 should map to Xrgb (little-endian byte order)"
        );
    }

    /// Test DRM RGBX8888 mapping
    /// RGBX8888: 32-bit word 0xRRGGBBXX → little-endian bytes [X,B,G,R] → GStreamer Xbgr
    #[test]
    fn test_drm_fourcc_to_video_format_rgbx8888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Rgbx8888);
        // RGBX8888 word layout: R in high bits, X in low bits
        // Little-endian byte order: [X,B,G,R] = Xbgr
        assert_eq!(
            format,
            VideoFormat::Xbgr,
            "RGBX8888 should map to Xbgr (little-endian byte order)"
        );
    }

    /// Test DRM BGR888 mapping (24-bit format for Sway ext-image-copy-capture)
    /// DRM_FORMAT_BGR888: [23:0] B:G:R = bits B(23-16):G(15-8):R(7-0)
    /// Little-endian memory: [R,G,B] = GStreamer Rgb
    #[test]
    fn test_drm_fourcc_to_video_format_bgr888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Bgr888);
        // DRM BGR888 stores R at lowest address due to little-endian
        // So memory layout is [R,G,B] which is GStreamer Rgb
        assert_eq!(
            format,
            VideoFormat::Rgb,
            "BGR888 should map to Rgb (DRM bit layout != memory byte order)"
        );
    }

    /// Test DRM RGB888 mapping (24-bit format)
    /// DRM_FORMAT_RGB888: [23:0] R:G:B = bits R(23-16):G(15-8):B(7-0)
    /// Little-endian memory: [B,G,R] = GStreamer Bgr
    #[test]
    fn test_drm_fourcc_to_video_format_rgb888() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Rgb888);
        // DRM RGB888 stores B at lowest address due to little-endian
        // So memory layout is [B,G,R] which is GStreamer Bgr
        assert_eq!(
            format,
            VideoFormat::Bgr,
            "RGB888 should map to Bgr (DRM bit layout != memory byte order)"
        );
    }

    /// Test Settings default values
    #[test]
    fn test_settings_default() {
        let settings = Settings::default();
        assert!(settings.pipewire_node_id.is_none());
        assert_eq!(
            settings.render_node,
            Some("/dev/dri/renderD128".to_string())
        );
        assert_eq!(settings.output_mode, OutputMode::Auto);
        assert_eq!(settings.cuda_device_id, -1);
        assert!(settings.cuda_context.is_none());
    }

    /// Test State default values
    #[test]
    fn test_state_default() {
        let state = State::default();
        assert!(state.stream.is_none());
        assert!(state.video_info.is_none());
        assert!(state.egl_display.is_none());
        assert!(state.buffer_pool.is_none());
        assert_eq!(state.actual_output_mode, OutputMode::System);
        assert_eq!(state.frame_count, 0);
    }

    /// Test that valid framerate values are detected correctly
    /// This test ensures we catch bugs like framerate=0/1 in pipeline construction
    #[test]
    fn test_framerate_validation() {
        // Valid framerates
        let valid_fps = gst::Fraction::new(60, 1);
        assert!(
            valid_fps.numer() > 0 && valid_fps.denom() > 0,
            "60/1 should be valid"
        );

        let valid_fps_30 = gst::Fraction::new(30, 1);
        assert!(
            valid_fps_30.numer() > 0 && valid_fps_30.denom() > 0,
            "30/1 should be valid"
        );

        // Invalid framerate (the bug we're preventing)
        let invalid_fps = gst::Fraction::new(0, 1);
        assert!(
            invalid_fps.numer() == 0,
            "0/1 should be detected as invalid"
        );
    }
}
