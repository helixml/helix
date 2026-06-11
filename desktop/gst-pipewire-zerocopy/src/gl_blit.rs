//! OBS + wolf synthesis for NVIDIA capture.
//!
//! The problem: importing Mutter's external PipeWire dma-buf into CUDA *per
//! frame* (`cuGraphicsEGLRegisterImage`) spikes to tens of ms under GPU
//! contention and cannot be cached (a cached registration of an external,
//! producer-written buffer serves stale/reordered frames).
//!
//! The fix, taken from the two high-performance references:
//!   - OBS (PipeWire capture + NVENC): imports the external dma-buf as a *GL
//!     texture* every frame — cheap (`eglCreateImage` + GL bind, no CUDA
//!     register) — then composites into a framebuffer OBS owns.
//!   - wolf / gst-wayland-display: renders into ONE persistent buffer it owns
//!     whose CUDA registration is created ONCE and reused every frame.
//!
//! So: per frame we GL-import the capture (cheap) and blit it into a persistent
//! buffer we own (`GsCUDABuf`, registered to CUDA once at creation); then map
//! that buffer for nvenc. The expensive CUDA register happens once, not 60×/s,
//! and we never cache-register an external buffer.
//!
//! Thread affinity: a `GlesRenderer` is GL-context-bound, so a `GlBlitter` is
//! created on and confined to the PipeWire loop thread (held as a local there,
//! captured by the local process closure — never sent across threads).

use std::sync::{Arc, Mutex};

use smithay::backend::allocator::dmabuf::Dmabuf;
use smithay::backend::allocator::Buffer as _;
use smithay::backend::drm::DrmNode;
use smithay::backend::egl::{EGLContext, EGLDisplay};
use smithay::backend::renderer::gles::GlesRenderer;
use smithay::backend::renderer::{Color32F, Frame, ImportDma, Renderer};
use smithay::utils::{Physical, Point, Rectangle, Size, Transform};

use gst_video::{VideoCapsBuilder, VideoFormat, VideoInfo, VideoInfoDmaDrm};

use waylanddisplaycore::utils::allocator::cuda::{
    CUDAContext, CUDABufferPool, CAPS_FEATURE_MEMORY_CUDA_MEMORY,
};
use waylanddisplaycore::utils::allocator::{GsBuffer, GsBufferType, GsCUDABuf};

use crate::pipewire_stream::FrameData;

/// Per-thread GL blitter that feeds wolf's persistent-buffer CUDA path from our
/// PipeWire capture. Lives on the PipeWire loop thread only.
pub struct GlBlitter {
    renderer: GlesRenderer,
    cuda_context: Arc<Mutex<CUDAContext>>,
    render_node: DrmNode,
    /// Persistent owned output buffer, registered to CUDA once. Created lazily
    /// on the first frame (when we know the capture dimensions/format).
    output: Option<GsBufferType>,
    out_w: u32,
    out_h: u32,
}

impl GlBlitter {
    /// Build a GL renderer sharing `egl_display` (the same display used for the
    /// CUDA buffer's EGLImage, so render + CUDA-map are coherent).
    pub fn new(
        egl_display: &Arc<EGLDisplay>,
        cuda_context: Arc<Mutex<CUDAContext>>,
        render_node: DrmNode,
    ) -> Result<Self, String> {
        let context =
            EGLContext::new(egl_display).map_err(|e| format!("EGLContext::new: {:?}", e))?;
        let renderer =
            unsafe { GlesRenderer::new(context) }.map_err(|e| format!("GlesRenderer::new: {:?}", e))?;
        Ok(Self {
            renderer,
            cuda_context,
            render_node,
            output: None,
            out_w: 0,
            out_h: 0,
        })
    }

    /// Import the capture dma-buf as a GL texture, blit it into our persistent
    /// CUDA buffer, and return a CUDAMemory GstBuffer for nvenc.
    pub fn process(&mut self, dmabuf: &Dmabuf, pts_ns: i64) -> Result<FrameData, String> {
        let w = dmabuf.width();
        let h = dmabuf.height();
        let drm_format = dmabuf.format();
        let video_format = crate::pipewire_stream::drm_fourcc_to_video_format(drm_format.code);
        let fourcc_u32 = drm_format.code as u32;
        let modifier_u64: u64 = drm_format.modifier.into();

        // (Re)create the persistent output buffer if missing or resized.
        if self.output.is_none() || self.out_w != w || self.out_h != h {
            self.output = Some(self.create_output(w, h, video_format, fourcc_u32, modifier_u64)?);
            self.out_w = w;
            self.out_h = h;
        }

        // Cheap per-frame import of the external capture buffer (OBS-style).
        let texture = self
            .renderer
            .import_dmabuf(dmabuf, None)
            .map_err(|e| format!("import_dmabuf: {:?}", e))?;

        let size: Size<i32, Physical> = (w as i32, h as i32).into();
        let full = Rectangle::<i32, Physical>::from_size(size);

        // Bind on a CLONE of the buffer handle (GsBufferType is Arc-backed) so the
        // mutable bind borrow and the immutable to_gs_buffer borrow don't conflict
        // — this mirrors wolf's create_frame.
        let mut bind_handle = self.output.as_ref().expect("output set above").clone();
        let mut fb = bind_handle
            .bind(&mut self.renderer)
            .map_err(|e| format!("bind output: {:?}", e))?;
        {
            let mut frame = self
                .renderer
                .render(&mut fb, size, Transform::Normal)
                .map_err(|e| format!("render: {:?}", e))?;
            // Clear first — the persistent buffer is reused every frame, and
            // render_texture_at blends src-over. Without this, frames accumulate
            // and saturate to white.
            frame
                .clear(Color32F::BLACK, &[full])
                .map_err(|e| format!("clear: {:?}", e))?;
            frame
                .render_texture_at(
                    &texture,
                    Point::<i32, Physical>::from((0, 0)),
                    1,
                    1.0,
                    Transform::Normal,
                    &[full],
                    &[],
                    1.0,
                )
                .map_err(|e| format!("render_texture_at: {:?}", e))?;
            let _sync = frame.finish().map_err(|e| format!("finish: {:?}", e))?;
        }

        // Map our persistent buffer's cached CUDA registration → GstBuffer.
        let buffer = self
            .output
            .as_ref()
            .expect("output set above")
            .to_gs_buffer(&mut fb, &mut self.renderer)
            .map_err(|e| format!("to_gs_buffer: {}", e))?;

        Ok(FrameData::CudaBuffer {
            buffer,
            width: w,
            height: h,
            format: video_format,
            pts_ns,
        })
    }

    fn create_output(
        &mut self,
        w: u32,
        h: u32,
        video_format: VideoFormat,
        fourcc_u32: u32,
        modifier_u64: u64,
    ) -> Result<GsBufferType, String> {
        let base = VideoInfo::builder(video_format, w, h)
            .build()
            .map_err(|e| format!("VideoInfo build: {:?}", e))?;
        let video_info = VideoInfoDmaDrm::new(base, fourcc_u32, modifier_u64);

        // Same raw EGL display as the renderer, so the output buffer's EGLImage
        // (and its CUDA registration) is coherent with the GL render target.
        let raw_display = self
            .renderer
            .egl_context()
            .display()
            .get_display_handle()
            .handle;

        // Configured CUDA buffer pool so to_gst_buffer ACQUIRES a reused buffer
        // each frame instead of allocating a fresh one (the per-frame alloc was
        // fragmenting GPU memory and degrading the stream to single-digit fps).
        let pool = {
            let ctx = self.cuda_context.lock().unwrap();
            let pool = CUDABufferPool::new(&ctx).map_err(|e| format!("CUDABufferPool::new: {}", e))?;
            let caps = VideoCapsBuilder::new()
                .features([CAPS_FEATURE_MEMORY_CUDA_MEMORY])
                .format(video_format)
                .width(w as i32)
                .height(h as i32)
                .framerate(gst::Fraction::new(60, 1))
                .build();
            pool.configure_basic(&caps, w * h * 4, 8, 16)
                .map_err(|e| format!("pool configure: {}", e))?;
            pool.activate().map_err(|e| format!("pool activate: {}", e))?;
            pool
        };

        let buf = GsCUDABuf::new(
            self.render_node,
            self.cuda_context.clone(),
            video_info,
            Arc::new(Mutex::new(Some(pool))),
            &raw_display,
        )
        .ok_or_else(|| "GsCUDABuf::new returned None".to_string())?;
        eprintln!(
            "[GL_BLIT] persistent output buffer created: {}x{} {:?}",
            w, h, video_format
        );
        Ok(GsBufferType::CUDA(buf))
    }
}
