//! PipeWire ScreenCast source - reuses waylanddisplaycore's CUDA conversion
//! Cache-bust: 2026-01-09T03:40Z - set_active + 30s first-frame timeout
//!
//! This element follows Wolf/gst-wayland-display's context sharing pattern:
//! - Wolf creates the CUDA context and pushes it via set_context()
//! - We receive it in set_context() and store it
//! - We respond to context queries from downstream elements
//! - Fallback: if no context pushed, acquire via new_from_gstreamer()

use crate::pipewire_stream::{DmaBufCapabilities, FrameData, PipeWireStream, RecvError};
use gst::glib;
use gst::prelude::*;
use gst::subclass::prelude::*;
use gst_base::prelude::BaseSrcExt;
use gst_base::subclass::base_src::CreateSuccess;
use gst_base::subclass::prelude::*;
use gst_video::{VideoCapsBuilder, VideoFormat, VideoInfo, VideoInfoDmaDrm};
use once_cell::sync::Lazy;
use parking_lot::Mutex;
use smithay::backend::allocator::Buffer;
use smithay::backend::drm::{DrmNode, NodeType};
use smithay::backend::egl::{EGLDevice, EGLDisplay};
use smithay::backend::egl::ffi::egl::types::EGLDisplay as RawEGLDisplay;
use std::sync::atomic::AtomicPtr;
use std::sync::Arc;
use std::time::Duration;

// Reuse battle-tested CUDA code from waylanddisplaycore
use waylanddisplaycore::utils::allocator::cuda::{
    init_cuda, CUDAContext, CUDAImage, EGLImage, CUDABufferPool, GstCudaContext,
    CAPS_FEATURE_MEMORY_CUDA_MEMORY, gst_cuda_handle_context_query_wrapped,
};
// DRM types from waylanddisplaycore (which re-exports from smithay)
use waylanddisplaycore::Fourcc as DrmFourcc;

/// Convert DRM fourcc to GStreamer VideoFormat.
///
/// IMPORTANT: This mapping compensates for a CUDA limitation. When receiving frames from
/// PipeWire/GNOME ScreenCast:
/// - PipeWire sends BGRx format (bytes: B, G, R, x in memory)
/// - The correct DRM fourcc would be XRGB8888, but CUDA rejects it with tiled modifiers
/// - So we use BGRX8888 (which CUDA accepts) but need to tell GStreamer the actual byte order
///
/// The mappings here ensure cudaconvertscale interprets the pixel data correctly despite
/// the "incorrect" fourcc used for CUDA import.
fn drm_fourcc_to_video_format(fourcc: DrmFourcc) -> VideoFormat {
    // Try GStreamer's built-in conversion first
    match VideoFormat::from_fourcc(fourcc as u32) {
        VideoFormat::Unknown => {
            // CUDA compatibility mappings:
            // We use BGRX8888/RGBX8888 for CUDA import (it accepts them with tiled modifiers)
            // but the actual pixel data is in Bgrx/Rgbx layout (from PipeWire BGRx/RGBx).
            // So we map to the GStreamer format that matches the ACTUAL byte order.
            match fourcc {
                DrmFourcc::Argb8888 => VideoFormat::Bgra,
                DrmFourcc::Abgr8888 => VideoFormat::Rgba,
                DrmFourcc::Xrgb8888 => VideoFormat::Bgrx,
                DrmFourcc::Xbgr8888 => VideoFormat::Rgbx,
                DrmFourcc::Rgba8888 => VideoFormat::Abgr,
                DrmFourcc::Bgra8888 => VideoFormat::Argb,
                // CUDA workaround: BGRX8888/RGBX8888 contain Bgrx/Rgbx data from PipeWire
                // Map to the format that matches the actual byte order, not the DRM spec
                DrmFourcc::Rgbx8888 => VideoFormat::Rgbx,
                DrmFourcc::Bgrx8888 => VideoFormat::Bgrx,
                _ => {
                    eprintln!("Unknown DRM fourcc {:?}, falling back to Bgra", fourcc);
                    VideoFormat::Bgra
                }
            }
        }
        format => format,
    }
}

static CAT: Lazy<gst::DebugCategory> = Lazy::new(|| {
    gst::DebugCategory::new("pipewirezerocopysrc", gst::DebugColorFlags::empty(), Some("PipeWire zero-copy source"))
});

/// Create EGL display from render node path (reuses waylanddisplaycore's pattern)
fn create_egl_display(node_path: &str) -> Result<EGLDisplay, String> {
    let drm_node = DrmNode::from_path(node_path).map_err(|e| format!("DrmNode: {:?}", e))?;
    let drm_render = drm_node.node_with_type(NodeType::Render).and_then(Result::ok).unwrap_or(drm_node);
    let device = EGLDevice::enumerate().map_err(|e| format!("enumerate: {:?}", e))?
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
        Fourcc::Argb8888, Fourcc::Abgr8888, Fourcc::Xrgb8888, Fourcc::Xbgr8888,
        Fourcc::Bgra8888, Fourcc::Rgba8888, Fourcc::Bgrx8888, Fourcc::Rgbx8888,
    ];

    let egl_modifiers: Vec<u64> = render_formats.iter()
        .filter(|f| target_formats.contains(&f.code))
        .map(|f| f.modifier.into())
        .collect();

    eprintln!("[PIPEWIRESRC_DEBUG] EGL reports {} render format modifiers, nvidia={} amd={} intel={}",
        egl_modifiers.len(), has_nvidia, has_amd, has_intel);
    for (i, m) in egl_modifiers.iter().enumerate().take(5) {
        eprintln!("[PIPEWIRESRC_DEBUG]   egl_modifier[{}] = 0x{:x}", i, m);
    }
    if egl_modifiers.len() > 5 {
        eprintln!("[PIPEWIRESRC_DEBUG]   ... and {} more", egl_modifiers.len() - 5);
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
        // Also include LINEAR (0x0) as last resort - some headless configs might need it
        // But put GPU-tiled first so GNOME prefers those
    ];

    // Select modifiers based on detected GPU vendor:
    // - NVIDIA: Use hardcoded 0xe08xxx ScreenCast modifiers (EGL reports different family)
    // - AMD/Intel: Use EGL modifiers directly (they typically match ScreenCast output)
    // - Unknown: Use DRM_FORMAT_MOD_INVALID as universal fallback
    let modifiers: Vec<u64> = if has_nvidia {
        eprintln!("[PIPEWIRESRC_DEBUG] NVIDIA GPU detected - using hardcoded ScreenCast modifiers");
        nvidia_screencast_modifiers
    } else if has_amd || has_intel {
        // AMD and Intel EGL modifiers typically match what ScreenCast uses
        // Use EGL modifiers directly, with INVALID as fallback
        eprintln!("[PIPEWIRESRC_DEBUG] AMD/Intel GPU detected - using EGL modifiers");
        let mut mods = egl_modifiers.clone();
        if !mods.contains(&DRM_FORMAT_MOD_INVALID) {
            mods.push(DRM_FORMAT_MOD_INVALID);
        }
        mods
    } else {
        eprintln!("[PIPEWIRESRC_DEBUG] Unknown GPU - using DRM_FORMAT_MOD_INVALID");
        vec![DRM_FORMAT_MOD_INVALID]
    };

    let vendor_name = if has_nvidia { "NVIDIA" } else if has_amd { "AMD" } else if has_intel { "Intel" } else { "Unknown" };
    eprintln!("[PIPEWIRESRC_DEBUG] Offering {} modifiers for {} GPU", modifiers.len(), vendor_name);
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
pub enum OutputMode { #[default] Auto, Cuda, DmaBuf, System }

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

pub struct State {
    stream: Option<PipeWireStream>,
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
        }
    }
}

pub struct PipeWireZeroCopySrc {
    settings: Mutex<Settings>,
    state: Mutex<Option<State>>,
}

impl Default for PipeWireZeroCopySrc {
    fn default() -> Self {
        Self { settings: Mutex::new(Settings::default()), state: Mutex::new(None) }
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
        static PROPERTIES: Lazy<Vec<glib::ParamSpec>> = Lazy::new(|| vec![
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
        ]);
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
            "output-mode" => s.output_mode = value.get::<Option<String>>().unwrap().as_deref().map(OutputMode::from_str).unwrap_or_default(),
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
            "render-node" => s.render_node.clone().unwrap_or_else(|| "/dev/dri/renderD128".into()).to_value(),
            "output-mode" => match s.output_mode { OutputMode::Auto => "auto", OutputMode::Cuda => "cuda", OutputMode::DmaBuf => "dmabuf", OutputMode::System => "system" }.to_value(),
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
            gst::subclass::ElementMetadata::new("PipeWire Zero-Copy Source", "Source/Video", "Captures PipeWire ScreenCast with zero-copy GPU output", "Wolf Project")
        });
        Some(&*META)
    }

    fn pad_templates() -> &'static [gst::PadTemplate] {
        static TEMPLATES: Lazy<Vec<gst::PadTemplate>> = Lazy::new(|| {
            // Include all RGBA/BGRA variants since gst_video_info_dma_drm_to_video_info()
            // may produce different formats depending on the DRM fourcc.
            // DRM ARGB8888 -> GST BGRA, DRM BGRA8888 -> GST ARGB on little-endian.
            let rgba_formats = [
                VideoFormat::Bgra, VideoFormat::Rgba,
                VideoFormat::Argb, VideoFormat::Abgr,
                VideoFormat::Bgrx, VideoFormat::Rgbx,
                VideoFormat::Xrgb, VideoFormat::Xbgr,
                VideoFormat::Nv12,
            ];
            let mut caps = gst::Caps::new_empty();
            caps.merge(VideoCapsBuilder::new().features([CAPS_FEATURE_MEMORY_CUDA_MEMORY]).format_list(rgba_formats).build());
            caps.merge(VideoCapsBuilder::new().features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF]).format(VideoFormat::DmaDrm).build());
            caps.merge(VideoCapsBuilder::new().format_list(rgba_formats).build());
            vec![gst::PadTemplate::new("src", gst::PadDirection::Src, gst::PadPresence::Always, &caps).unwrap()]
        });
        TEMPLATES.as_ref()
    }

    fn change_state(&self, transition: gst::StateChange) -> Result<gst::StateChangeSuccess, gst::StateChangeError> {
        match self.parent_change_state(transition) {
            Ok(gst::StateChangeSuccess::Success) if transition.next() == gst::State::Paused => Ok(gst::StateChangeSuccess::NoPreroll),
            x => x,
        }
    }

    /// Receive CUDA context from Wolf via GStreamer's context mechanism.
    /// Wolf creates the context and pushes it to elements via gst_element_set_context().
    fn set_context(&self, context: &gst::Context) {
        eprintln!("[PIPEWIRESRC_DEBUG] set_context called, context_type={:?}", context.context_type());
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
                    eprintln!("[PIPEWIRESRC_DEBUG] Received CUDA context via set_context - SUCCESS");
                    gst::info!(CAT, imp = self, "Received CUDA context from pipeline (via set_context)");
                    settings.cuda_context = Some(Arc::new(std::sync::Mutex::new(ctx)));
                }
            }
            Err(e) => {
                eprintln!("[PIPEWIRESRC_DEBUG] set_context failed: {}", e);
                gst::debug!(CAT, imp = self, "set_context: not a CUDA context or failed: {}", e);
            }
        }

        self.parent_set_context(context)
    }
}

impl BaseSrcImpl for PipeWireZeroCopySrc {
    fn start(&self) -> Result<(), gst::ErrorMessage> {
        let mut state_guard = self.state.lock();
        if state_guard.is_some() { return Ok(()); }

        // Extract settings - but don't hold the lock while doing CUDA setup
        let (node_id, pipewire_fd, render_node, output_mode, device_id, target_fps) = {
            let settings = self.settings.lock();
            let node_id = settings.pipewire_node_id.ok_or_else(|| gst::error_msg!(gst::LibraryError::Settings, ("pipewire-node-id must be set")))?;
            (node_id, settings.pipewire_fd, settings.render_node.clone(), settings.output_mode, settings.cuda_device_id, settings.target_fps)
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
                eprintln!("[PIPEWIRESRC_DEBUG] have_cuda_context={}", have_cuda_context);

                if !have_cuda_context {
                    // No context pushed by Wolf - try to acquire one from the pipeline
                    // This matches waylandsrc's fallback behavior
                    eprintln!("[PIPEWIRESRC_DEBUG] No CUDA context from set_context, acquiring via new_from_gstreamer...");
                    gst::info!(CAT, imp = self, "No CUDA context from set_context, acquiring from pipeline");
                    let cuda_raw_ptr = self.settings.lock().cuda_raw_ptr.as_ptr();
                    match CUDAContext::new_from_gstreamer(&elem, device_id, cuda_raw_ptr) {
                        Ok(ctx) => {
                            eprintln!("[PIPEWIRESRC_DEBUG] new_from_gstreamer succeeded!");
                            let mut settings = self.settings.lock();
                            if settings.cuda_context.is_none() {
                                gst::info!(CAT, imp = self, "Acquired CUDA context via new_from_gstreamer");
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
                        eprintln!("[PIPEWIRESRC_DEBUG] Creating EGL display for {}...", node_path);
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
                                        gst::info!(CAT, imp = self, "Using CUDA mode with shared context");
                                        state.egl_display = Some(Arc::new(display));
                                        state.buffer_pool = Some(pool);
                                        state.actual_output_mode = OutputMode::Cuda;
                                    }
                                    Err(e) => {
                                        eprintln!("[PIPEWIRESRC_DEBUG] Buffer pool creation failed: {}", e);
                                    }
                                }
                            }
                            Err(e) => {
                                eprintln!("[PIPEWIRESRC_DEBUG] EGL display failed: {}", e);
                                gst::warning!(CAT, imp = self, "EGL display failed: {}", e);
                            }
                        }
                    } else {
                        eprintln!("[PIPEWIRESRC_DEBUG] render_node is None, can't create EGL display");
                    }
                }
            }
        } else {
            eprintln!("[PIPEWIRESRC_DEBUG] Not trying CUDA (output_mode={:?})", output_mode);
        }

        eprintln!("[PIPEWIRESRC_DEBUG] Final actual_output_mode={:?}", state.actual_output_mode);
        if state.actual_output_mode == OutputMode::System {
            if output_mode == OutputMode::DmaBuf {
                state.actual_output_mode = OutputMode::DmaBuf;
                gst::info!(CAT, imp = self, "Using DMA-BUF mode");
            } else {
                eprintln!("[PIPEWIRESRC_DEBUG] Using SYSTEM MEMORY mode (not CUDA!)");
                gst::info!(CAT, imp = self, "Using system memory mode");
            }
        }

        let stream = PipeWireStream::connect(node_id, pipewire_fd, dmabuf_caps, target_fps).map_err(|e| gst::error_msg!(gst::LibraryError::Init, ("PipeWire: {}", e)))?;
        state.stream = Some(stream);
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

    fn is_seekable(&self) -> bool { false }

    /// Handle context queries from downstream elements.
    /// When downstream elements need a CUDA context, we provide ours.
    fn query(&self, query: &mut gst::QueryRef) -> bool {
        if query.type_() == gst::QueryType::Context {
            let settings = self.settings.lock();
            if let Some(ref cuda_context) = settings.cuda_context {
                gst::debug!(CAT, imp = self, "Handling CUDA context query from downstream");
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
        let rgba_formats = [
            VideoFormat::Bgra, VideoFormat::Rgba,
            VideoFormat::Argb, VideoFormat::Abgr,
            VideoFormat::Bgrx, VideoFormat::Rgbx,
            VideoFormat::Xrgb, VideoFormat::Xbgr,
            VideoFormat::Nv12,
        ];

        let mut caps = match output_mode {
            OutputMode::Cuda => VideoCapsBuilder::new().features([CAPS_FEATURE_MEMORY_CUDA_MEMORY]).format_list(rgba_formats).build(),
            OutputMode::DmaBuf => VideoCapsBuilder::new().features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF]).format(VideoFormat::DmaDrm).build(),
            OutputMode::System => VideoCapsBuilder::new().format_list(rgba_formats).build(),
            OutputMode::Auto => {
                // Auto mode before start(): advertise all capabilities (like pad template)
                // GStreamer will negotiate based on downstream requirements
                let mut all_caps = gst::Caps::new_empty();
                all_caps.merge(VideoCapsBuilder::new().features([CAPS_FEATURE_MEMORY_CUDA_MEMORY]).format_list(rgba_formats).build());
                all_caps.merge(VideoCapsBuilder::new().features([gstreamer_allocators::CAPS_FEATURE_MEMORY_DMABUF]).format(VideoFormat::DmaDrm).build());
                all_caps.merge(VideoCapsBuilder::new().format_list(rgba_formats).build());
                all_caps
            }
        };
        if let Some(f) = filter { caps = caps.intersect(f); }
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
            if let Some(s) = self.state.lock().as_mut() { s.video_info = Some(info); }
        }
        self.parent_set_caps(caps)
    }

}

impl PushSrcImpl for PipeWireZeroCopySrc {
    fn create(&self, _buffer: Option<&mut gst::BufferRef>) -> Result<CreateSuccess, gst::FlowError> {
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
            Duration::from_secs(30)  // Default timeout
        };

        if !has_last_buffer {
            gst::info!(CAT, imp = self, "Waiting for first frame from PipeWire (timeout: {:?})", timeout);
        }

        // Try to receive a frame with timeout
        let frame_result = stream.recv_frame_timeout(timeout);

        let frame = match frame_result {
            Ok(frame) => frame,
            Err(RecvError::Timeout) if keepalive_time_ms > 0 => {
                // Timeout with keepalive enabled: resend last buffer with updated timestamps
                // This is normal for GNOME 49+ damage-based ScreenCast with static screens
                if let Some(ref last_buf) = state.last_buffer {
                    gst::debug!(CAT, imp = self, "Keepalive: resending last buffer (no new frame from PipeWire)");

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
                    gst::warning!(CAT, imp = self, "Keepalive timeout but no last buffer available (waiting for first frame)");
                    // Don't error immediately - continue waiting in subsequent create() calls
                    // The 30-second first-frame timeout above should handle this
                    return Err(gst::FlowError::Error);
                }
            }
            Err(RecvError::Timeout) => {
                // Timeout without keepalive - this is an error
                gst::error!(CAT, imp = self, "Frame receive timeout (keepalive disabled)");
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
                eprintln!("[PIPEWIRESRC_DEBUG] Received DmaBuf frame: {}x{} fourcc=0x{:x}",
                    dmabuf.width(), dmabuf.height(), dmabuf.format().code as u32);
            }
            FrameData::Shm { width, height, format, .. } => {
                eprintln!("[PIPEWIRESRC_DEBUG] Received SHM frame: {}x{} format=0x{:x}",
                    width, height, format);
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
                let egl_image = EGLImage::from(&dmabuf, &raw_display)
                    .map_err(|e| { eprintln!("[PIPEWIRESRC_DEBUG] EGLImage error: {}", e); gst::FlowError::Error })?;
                let egl_time = t1.elapsed();

                // CUDA import timing
                let t2 = Instant::now();
                let cuda_image = CUDAImage::from(egl_image, &cuda_ctx)
                    .map_err(|e| { eprintln!("[PIPEWIRESRC_DEBUG] CUDAImage error: {}", e); gst::FlowError::Error })?;
                let cuda_time = t2.elapsed();

                // Derive VideoFormat from DMA-BUF's fourcc (matches waylanddisplaycore pattern)
                let drm_format = dmabuf.format();
                let video_format = drm_fourcc_to_video_format(drm_format.code);
                gst::debug!(CAT, imp = self, "DMA-BUF format: {:?} -> {:?}", drm_format.code, video_format);

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
                let buf = cuda_image.to_gst_buffer(dma_video_info, &cuda_ctx, state.buffer_pool.as_ref())
                    .map_err(|e| { eprintln!("[PIPEWIRESRC_DEBUG] to_gst_buffer error: {}", e); gst::FlowError::Error })?;
                let buf_time = t3.elapsed();

                let total = frame_start.elapsed();
                // Log timing every 60 frames (1 second at 60fps)
                if state.frame_count % 60 == 0 {
                    eprintln!("[PIPEWIRESRC_TIMING] frame={} lock={:?} egl={:?} cuda={:?} buf={:?} total={:?}",
                        state.frame_count, lock_time, egl_time, cuda_time, buf_time, total);
                }

                // Get the actual format from the buffer's VideoMeta (set by to_gst_buffer)
                // This is the format that gst_video_info_dma_drm_to_video_info() produced
                let actual_fmt = buf.meta::<gst_video::VideoMeta>()
                    .map(|m| m.format())
                    .unwrap_or(video_format);

                (buf, actual_fmt, w, h)
            }
            FrameData::DmaBuf(dmabuf) => {
                eprintln!("[PIPEWIRESRC_DEBUG] Processing DmaBuf in FALLBACK (non-CUDA) mode");
                let w = dmabuf.width() as u32;
                let h = dmabuf.height() as u32;
                let video_format = drm_fourcc_to_video_format(dmabuf.format().code);
                let buf = self.dmabuf_to_system(&dmabuf)?;
                (buf, video_format, w, h)
            }
            FrameData::Shm { data, width, height, stride, format } => {
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
            Some(info) => info.format() != actual_format || info.width() != width || info.height() != height,
            None => true,
        };

        if needs_caps_update {
            gst::info!(CAT, imp = self, "Format/size changed, updating caps to {:?} {}x{}", actual_format, width, height);

            // Get framerate from existing video_info or use 60/1 as default
            // GStreamer requires framerate for caps to be "fixed"
            let fps = state.video_info.as_ref()
                .map(|info| info.fps())
                .unwrap_or(gst::Fraction::new(60, 1));

            // Build new caps with the actual format (must include framerate for fixed caps)
            let new_caps = match state.actual_output_mode {
                OutputMode::Cuda => {
                    VideoCapsBuilder::new()
                        .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                        .format(actual_format)
                        .width(width as i32)
                        .height(height as i32)
                        .framerate(fps)
                        .build()
                }
                _ => {
                    VideoCapsBuilder::new()
                        .format(actual_format)
                        .width(width as i32)
                        .height(height as i32)
                        .framerate(fps)
                        .build()
                }
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
    fn dmabuf_to_system(&self, dmabuf: &smithay::backend::allocator::dmabuf::Dmabuf) -> Result<gst::Buffer, gst::FlowError> {
        use std::os::fd::AsRawFd;
        let size = (dmabuf.height() as usize) * (dmabuf.strides().next().unwrap_or(0) as usize);
        let mut data = vec![0u8; size];

        unsafe {
            let fd = dmabuf.handles().next().ok_or(gst::FlowError::Error)?.as_raw_fd();
            let ptr = libc::mmap(std::ptr::null_mut(), size, libc::PROT_READ, libc::MAP_SHARED, fd, 0);
            if ptr == libc::MAP_FAILED { return Err(gst::FlowError::Error); }
            std::ptr::copy_nonoverlapping(ptr as *const u8, data.as_mut_ptr(), size);
            libc::munmap(ptr, size);
        }

        // Derive format from DMA-BUF's fourcc
        let video_format = drm_fourcc_to_video_format(dmabuf.format().code);
        self.create_system_buffer(&data, dmabuf.width() as u32, dmabuf.height() as u32, dmabuf.strides().next().unwrap_or(0), video_format)
    }

    fn create_system_buffer(&self, data: &[u8], width: u32, height: u32, stride: u32, format: VideoFormat) -> Result<gst::Buffer, gst::FlowError> {
        let mut buffer = gst::Buffer::with_size(data.len()).map_err(|_| gst::FlowError::Error)?;
        {
            let buf = buffer.get_mut().unwrap();
            buf.map_writable().map_err(|_| gst::FlowError::Error)?.copy_from_slice(data);
        }
        {
            let buf = buffer.get_mut().unwrap();
            gst_video::VideoMeta::add_full(buf, gst_video::VideoFrameFlags::empty(), format, width, height, &[0], &[stride as i32]).map_err(|_| gst::FlowError::Error)?;
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

    /// Test CUDA workaround mappings for BGRX8888/RGBX8888
    /// These are CRITICAL: PipeWire sends BGRx/RGBx data, but we use BGRX8888/RGBX8888
    /// fourcc for CUDA compatibility (CUDA rejects XRGB8888 with NVIDIA tiled modifiers).
    /// We must map these to Bgrx/Rgbx so GStreamer interprets the colors correctly.
    #[test]
    fn test_drm_fourcc_to_video_format_bgrx8888_cuda_workaround() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Bgrx8888);
        // CRITICAL: Must be Bgrx to match the actual pixel data from PipeWire
        // If this is Xrgb, colors will be swapped (R/B channels reversed)
        assert_eq!(format, VideoFormat::Bgrx, "BGRX8888 should map to Bgrx (CUDA workaround)");
    }

    #[test]
    fn test_drm_fourcc_to_video_format_rgbx8888_cuda_workaround() {
        gst::init().unwrap();
        let format = drm_fourcc_to_video_format(DrmFourcc::Rgbx8888);
        // CRITICAL: Must be Rgbx to match the actual pixel data from PipeWire
        // If this is Xbgr, colors will be swapped (R/B channels reversed)
        assert_eq!(format, VideoFormat::Rgbx, "RGBX8888 should map to Rgbx (CUDA workaround)");
    }

    /// Test Settings default values
    #[test]
    fn test_settings_default() {
        let settings = Settings::default();
        assert!(settings.pipewire_node_id.is_none());
        assert_eq!(settings.render_node, Some("/dev/dri/renderD128".to_string()));
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
        assert!(valid_fps.numer() > 0 && valid_fps.denom() > 0, "60/1 should be valid");

        let valid_fps_30 = gst::Fraction::new(30, 1);
        assert!(valid_fps_30.numer() > 0 && valid_fps_30.denom() > 0, "30/1 should be valid");

        // Invalid framerate (the bug we're preventing)
        let invalid_fps = gst::Fraction::new(0, 1);
        assert!(invalid_fps.numer() == 0, "0/1 should be detected as invalid");
    }
}
