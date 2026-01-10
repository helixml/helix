#![allow(dead_code)]
#![allow(non_camel_case_types)]
#![allow(non_snake_case)]

use crate::utils::allocator::cuda::CUDAContext;
use gst::ffi as gst_ffi;
use gst::ffi::{GstContext, GstElement, GstQuery};
use gst::glib::ffi as glib_ffi;
use gst_video::VideoInfoDmaDrm;
use gst_video::ffi::GstVideoInfo;
use gst_video::glib::gobject_ffi;
use gst_video::glib::translate::{FromGlibPtrNone, ToGlibPtr};
use libloading::{Library, Symbol};
use smithay::backend::egl::ffi::egl::types::{EGLDisplay, EGLImageKHR, EGLint};
use std::ffi::c_void;
use std::os::raw::{c_int, c_uint};
use std::ptr;
use std::sync::{Arc, OnceLock};

pub type GstCudaContext = *mut c_void;

#[macro_export]
macro_rules! cuda_call {
    ($expression:expr) => {{
        let result = unsafe { $expression };
        if result != CUDA_SUCCESS {
            Err(format!("CUDA error: {}", cuda_result_to_string(result)))
        } else {
            Ok(())
        }
    }};
}

type GstCudaStream = *mut c_void;
pub(crate) type GstBufferPool = *mut c_void;
pub(crate) type GstCudaStreamHandle = *mut c_void;
#[repr(C)]
struct GstCudaMemory {
    mem: gst_ffi::GstMemory,
    context: GstCudaContext,
    info: GstVideoInfo,
    priv_: *mut c_void,              // GstCudaMemoryPrivate pointer
    _gst_reserved: [*mut c_void; 4], // GST_PADDING (see gstconfig.h)
}

impl GstCudaMemory {
    /// Safely borrow the GstVideoInfo from a GstMemory pointer
    fn from_gst_memory<'a>(mem: *const gst_ffi::GstMemory) -> Option<&'a Self> {
        if mem.is_null() {
            return None;
        }
        Some(unsafe { &*(mem as *const GstCudaMemory) })
    }

    /// Get a safe VideoInfo wrapper
    pub fn video_info_safe(&self) -> gst_video::VideoInfo {
        unsafe { gst_video::VideoInfo::from_glib_none(&self.info as *const _) }
    }
}

#[repr(C)]
pub(crate) struct CUeglFrame {
    pub(crate) frame: CUeglFrameUnion,
    pub(crate) width: c_uint,
    pub(crate) height: c_uint,
    pub(crate) depth: c_uint,
    pub(crate) pitch: c_uint,
    pub(crate) plane_count: c_uint,
    pub(crate) num_channels: c_uint,
    // Followings are ENUMS
    pub(crate) frame_type: c_uint,
    pub(crate) egl_color_format: c_uint,
    pub(crate) cu_format: c_uint,
}

#[repr(C)]
pub(crate) union CUeglFrameUnion {
    pub p_array: [CUarray; MAX_PLANES],
    pub p_pitch: [*mut c_void; MAX_PLANES],
}

const MAX_PLANES: usize = 3;

// CUDA driver API types
type CUcontext = *mut c_void;
type CUstream = *mut c_void;
type CUdeviceptr = u64;
type CUarray = *mut c_void;
pub(crate) type CUgraphicsResource = *mut c_void;
type CUresult = c_uint;

// CUDA constants
pub(crate) const CUDA_SUCCESS: CUresult = 0;

// EGL constants
pub(crate) const EGL_NO_IMAGE_KHR: EGLImageKHR = ptr::null_mut();
pub(crate) const EGL_LINUX_DMA_BUF_EXT: u32 = 0x3270;
pub(crate) const EGL_DMA_BUF_PLANE0_FD_EXT: EGLint = 0x3272;
pub(crate) const EGL_DMA_BUF_PLANE0_OFFSET_EXT: EGLint = 0x3273;
pub(crate) const EGL_DMA_BUF_PLANE0_PITCH_EXT: EGLint = 0x3274;
pub(crate) const EGL_DMA_BUF_PLANE0_MODIFIER_LO_EXT: EGLint = 0x3443;
pub(crate) const EGL_DMA_BUF_PLANE0_MODIFIER_HI_EXT: EGLint = 0x3444;
pub(crate) const EGL_DMA_BUF_PLANE1_FD_EXT: EGLint = 0x3275;
pub(crate) const EGL_DMA_BUF_PLANE1_OFFSET_EXT: EGLint = 0x3276;
pub(crate) const EGL_DMA_BUF_PLANE1_PITCH_EXT: EGLint = 0x3277;
pub(crate) const EGL_DMA_BUF_PLANE1_MODIFIER_LO_EXT: EGLint = 0x3445;
pub(crate) const EGL_DMA_BUF_PLANE1_MODIFIER_HI_EXT: EGLint = 0x3446;
pub(crate) const EGL_WIDTH: EGLint = 0x3057;
pub(crate) const EGL_HEIGHT: EGLint = 0x3056;
pub(crate) const EGL_LINUX_DRM_FOURCC_EXT: EGLint = 0x3271;
pub(crate) const EGL_NONE: EGLint = 0x3038;

unsafe extern "C" {
    // CUDA Driver API
    fn CuMemcpy2DAsync(pCopy: *const CUDA_MEMCPY2D, stream: CUstream) -> CUresult;
}

// Add dynamic function pointer types for CUDA-EGL interop
pub(crate) type CuGraphicsEGLRegisterImageFn = unsafe extern "C" fn(
    pCudaResource: *mut CUgraphicsResource,
    image: EGLImageKHR,
    flags: c_uint,
) -> CUresult;

pub(crate) type CuGraphicsUnregisterResourceFn =
    unsafe extern "C" fn(resource: CUgraphicsResource) -> CUresult;

pub(crate) type CuGraphicsResourceGetMappedEglFrameFn = unsafe extern "C" fn(
    pEglFrame: *mut CUeglFrame,
    resource: CUgraphicsResource,
    index: c_uint,
    mipLevel: c_uint,
) -> CUresult;

// Structure to hold dynamically loaded EGL interop functions
pub(crate) struct CudaEglFunctions {
    // Keep the library handle alive so the symbols remain valid
    _lib: Arc<Library>,
    pub register_image: CuGraphicsEGLRegisterImageFn,
    pub unregister_resource: CuGraphicsUnregisterResourceFn,
    pub get_mapped_frame: CuGraphicsResourceGetMappedEglFrameFn,
}

impl CudaEglFunctions {
    /// Load CUDA-EGL interop functions dynamically from the CUDA library
    pub fn load() -> Result<Self, String> {
        unsafe {
            // Try to open the CUDA library
            let lib = Library::new("libcuda.so.1")
                .or_else(|_| Library::new("libcuda.so"))
                .map_err(|e| format!("Failed to open CUDA library: {}", e))?;

            // Load cuGraphicsEGLRegisterImage
            let register_image: Symbol<CuGraphicsEGLRegisterImageFn> = lib
                .get(b"cuGraphicsEGLRegisterImage")
                .map_err(|e| format!("Failed to load cuGraphicsEGLRegisterImage: {}", e))?;
            let register_image = *register_image;

            // Load cuGraphicsUnregisterResource
            let unregister_resource: Symbol<CuGraphicsUnregisterResourceFn> = lib
                .get(b"cuGraphicsUnregisterResource")
                .map_err(|e| format!("Failed to load cuGraphicsUnregisterResource: {}", e))?;
            let unregister_resource = *unregister_resource;

            // Load cuGraphicsResourceGetMappedEglFrame
            let get_mapped_frame: Symbol<CuGraphicsResourceGetMappedEglFrameFn> = lib
                .get(b"cuGraphicsResourceGetMappedEglFrame")
                .map_err(|e| {
                    format!("Failed to load cuGraphicsResourceGetMappedEglFrame: {}", e)
                })?;
            let get_mapped_frame = *get_mapped_frame;

            Ok(CudaEglFunctions {
                _lib: Arc::new(lib),
                register_image,
                unregister_resource,
                get_mapped_frame,
            })
        }
    }

    /// Convenience method to register an EGL image
    pub unsafe fn register_egl_image(
        &self,
        resource: *mut CUgraphicsResource,
        image: EGLImageKHR,
        flags: c_uint,
    ) -> Result<(), String> {
        let result = unsafe { (self.register_image)(resource, image, flags) };
        if result == CUDA_SUCCESS {
            Ok(())
        } else {
            Err(cuda_result_to_string(result).into())
        }
    }

    /// Convenience method to unregister a resource
    pub unsafe fn unregister_resource(&self, resource: CUgraphicsResource) -> Result<(), String> {
        let result = unsafe { (self.unregister_resource)(resource) };
        if result == CUDA_SUCCESS {
            Ok(())
        } else {
            Err(cuda_result_to_string(result).into())
        }
    }

    /// Convenience method to get mapped EGL frame
    pub unsafe fn get_mapped_egl_frame(
        &self,
        resource: CUgraphicsResource,
        index: c_uint,
        mip_level: c_uint,
    ) -> Result<CUeglFrame, String> {
        let mut frame: CUeglFrame = unsafe { std::mem::zeroed() };
        let result = unsafe { (self.get_mapped_frame)(&mut frame, resource, index, mip_level) };
        if result == CUDA_SUCCESS {
            Ok(frame)
        } else {
            Err(cuda_result_to_string(result).into())
        }
    }
}

static CUDA_EGL_FUNCTIONS: OnceLock<CudaEglFunctions> = OnceLock::new();

pub fn init_cuda_egl() -> Result<(), String> {
    CUDA_EGL_FUNCTIONS
        .get_or_init(|| CudaEglFunctions::load().expect("Failed to load CUDA-EGL functions"));
    Ok(())
}

pub fn get_cuda_egl_functions() -> Result<&'static CudaEglFunctions, String> {
    CUDA_EGL_FUNCTIONS.get().ok_or_else(|| {
        "CUDA EGL functions not initialized. Call init_cuda_egl() first.".to_string()
    })
}

fn gst_dma_video_info_to_video_info(
    dma_video_info: &VideoInfoDmaDrm,
) -> Result<GstVideoInfo, String> {
    let mut video_info: GstVideoInfo = unsafe { std::mem::zeroed() };
    unsafe { gst_video::ffi::gst_video_info_init(&mut video_info) };

    let result = unsafe {
        gst_video::ffi::gst_video_info_dma_drm_to_video_info(
            dma_video_info.to_glib_none().0,
            &mut video_info,
        )
    };
    if result == glib_ffi::GFALSE {
        return Err("Failed to convert DMA-BUF video info to GStreamer video info".into());
    }

    Ok(video_info)
}

// EGLImage extension function pointers
pub(crate) type PFN_eglCreateImageKHR = unsafe extern "C" fn(
    dpy: EGLDisplay,
    ctx: *mut c_void,
    target: u32,
    buffer: *mut c_void,
    attrib_list: *const EGLint,
) -> EGLImageKHR;

pub(crate) type PFN_eglDestroyImageKHR =
    unsafe extern "C" fn(dpy: EGLDisplay, image: EGLImageKHR) -> c_int;

// CUDA memcpy2D structure
#[repr(C)]
struct CUDA_MEMCPY2D {
    pub srcXInBytes: usize,
    pub srcY: usize,
    pub srcMemoryType: c_uint,
    pub srcHost: *const c_void,
    pub srcDevice: CUdeviceptr,
    pub srcArray: CUarray,
    pub srcPitch: usize,
    pub dstXInBytes: usize,
    pub dstY: usize,
    pub dstMemoryType: c_uint,
    pub dstHost: *mut c_void,
    pub dstDevice: CUdeviceptr,
    pub dstArray: CUarray,
    pub dstPitch: usize,
    pub WidthInBytes: usize,
    pub Height: usize,
}

#[allow(dead_code)]
const CU_MEMORYTYPE_HOST: c_uint = 1;
#[allow(dead_code)]
const CU_MEMORYTYPE_DEVICE: c_uint = 2;
#[allow(dead_code)]
const CU_MEMORYTYPE_ARRAY: c_uint = 3;
#[allow(dead_code)]
const CU_MEMORYTYPE_UNIFIED: c_uint = 4;

unsafe extern "C" {
    // gstcudaloader
    pub(crate) fn gst_cuda_load_library() -> glib_ffi::gboolean;

    // GstCudaContext functions
    pub(crate) fn gst_cuda_context_new(device_id: c_int) -> *mut GstCudaContext;
    fn gst_cuda_context_get_handle(context: *mut GstCudaContext) -> CUcontext;
    fn gst_cuda_context_push(context: *mut GstCudaContext) -> glib_ffi::gboolean;
    fn gst_cuda_context_pop(pctx: *mut CUcontext) -> glib_ffi::gboolean;
    pub(crate) fn gst_cuda_ensure_element_context(
        element: *mut GstElement,
        device_id: c_int,
        cuda_ctx: *mut *mut GstCudaContext,
    ) -> glib_ffi::gboolean;

    pub(crate) fn gst_cuda_handle_set_context(
        element: *mut GstElement,
        context: *mut GstContext,
        device_id: c_int,
        cuda_ctx: *mut *mut GstCudaContext,
    ) -> glib_ffi::gboolean;

    // GstCudaStream functions
    pub(crate) fn gst_cuda_stream_new(context: *mut GstCudaContext) -> GstCudaStreamHandle;
    pub(crate) fn gst_cuda_stream_ref(stream: GstCudaStreamHandle);
    pub(crate) fn gst_cuda_stream_unref(stream: GstCudaStreamHandle);
    pub(crate) fn gst_clear_cuda_stream(stream: GstCudaStreamHandle);

    // GstCudaMemory functions
    fn gst_cuda_allocator_alloc(
        allocator: *mut gst_ffi::GstAllocator,
        context: *mut GstCudaContext,
        stream: GstCudaStream,
        info: *const gst_video::ffi::GstVideoInfo,
    ) -> *mut gst_ffi::GstMemory;

    fn gst_cuda_allocator_alloc_wrapped(
        allocator: *mut gst_ffi::GstAllocator,
        context: *mut GstCudaContext,
        stream: GstCudaStream,
        info: *const gst_video::ffi::GstVideoInfo,
        dev_ptr: *mut CUdeviceptr,
        user_data: *mut c_void,
        notify: Option<unsafe extern "C" fn(*mut c_void)>,
    ) -> *mut gst_ffi::GstMemory;

    fn gst_is_cuda_memory(mem: *mut gst_ffi::GstMemory) -> glib_ffi::gboolean;

    pub(crate) fn gst_cuda_memory_init_once() -> c_void;

    pub(crate) fn gst_cuda_buffer_pool_new(context: *mut GstCudaContext) -> GstBufferPool;
    pub(crate) fn gst_cuda_buffer_pool_get_type() -> glib_ffi::GType;
    pub(crate) fn gst_buffer_pool_config_set_cuda_stream(
        config: *mut gst_ffi::GstStructure,
        stream: GstCudaStreamHandle,
    );

    fn gst_cuda_stream_get_handle(stream: GstCudaStream) -> CUstream;

    fn gst_cuda_memory_get_stream(mem: *mut gst_ffi::GstMemory) -> GstCudaStream;

    pub(crate) fn gst_cuda_handle_context_query(
        element: *mut GstElement,
        query: *mut GstQuery,
        gst_cuda_context: *mut GstCudaContext,
    ) -> glib_ffi::gboolean;
}

pub(crate) struct CudaContextGuard;

impl CudaContextGuard {
    pub fn new(cuda_context: &CUDAContext) -> Result<Self, String> {
        if unsafe { gst_cuda_context_push(cuda_context.ptr) } == glib_ffi::GFALSE {
            return Err("Failed to push CUDA context".into());
        }
        Ok(CudaContextGuard)
    }
}

impl Drop for CudaContextGuard {
    fn drop(&mut self) {
        unsafe {
            gst_cuda_context_pop(ptr::null_mut());
        }
    }
}

pub(crate) const GST_BUFFER_POOL_OPTION_VIDEO_META: &[u8] = b"GstBufferPoolOptionVideoMeta\0";
const GST_MAP_CUDA: u32 = gst_ffi::GST_MAP_FLAG_LAST << 1;

pub(crate) fn gst_is_cuda_buffer_pool(obj: *mut gst::ffi::GstBufferPool) -> bool {
    unsafe {
        gobject_ffi::g_type_check_instance_is_a(
            obj as *mut gobject_ffi::GTypeInstance,
            gst_cuda_buffer_pool_get_type(),
        ) != 0
    }
}

fn alloc_cuda_buffer(
    cuda_context: &CUDAContext,
    video_info: &VideoInfoDmaDrm,
) -> Result<gst::Buffer, Box<dyn std::error::Error>> {
    let mut gst_video_info = gst_dma_video_info_to_video_info(video_info)?;

    // Use the stream from the context if available
    let stream = cuda_context
        .stream()
        .as_ref()
        .map(|s| s.stream)
        .unwrap_or(unsafe { std::mem::zeroed() });

    let gst_memory = unsafe {
        gst_cuda_allocator_alloc(
            ptr::null_mut(),
            cuda_context.ptr,
            stream,
            &mut gst_video_info,
        )
    };
    if gst_memory.is_null() {
        return Err("Failed to allocate GST CUDA memory".into());
    }

    let mut buffer = gst::Buffer::new();
    let buffer_ref = buffer.get_mut().unwrap();
    buffer_ref.append_memory(unsafe { gst::Memory::from_glib_full(gst_memory) });

    Ok(buffer)
}

pub(crate) fn acquire_or_alloc_buffer(
    buffer_pool: Option<GstBufferPool>,
    cuda_context: &CUDAContext,
    video_info: &VideoInfoDmaDrm,
) -> Result<gst::Buffer, Box<dyn std::error::Error>> {
    if let Some(pool) = buffer_pool {
        // Use the pool if available
        let mut gst_buffer: *mut gst_ffi::GstBuffer = ptr::null_mut();
        let result = unsafe {
            gst::ffi::gst_buffer_pool_acquire_buffer(
                pool as *mut gst::ffi::GstBufferPool,
                &mut gst_buffer,
                ptr::null_mut(),
            )
        };

        if result != gst_ffi::GST_FLOW_OK {
            tracing::info!("Failed to acquire buffer from pool: {:?}", result);
            return alloc_cuda_buffer(cuda_context, video_info);
        }

        if gst_buffer.is_null() {
            return Err("Acquired buffer is null".into());
        }

        Ok(unsafe { gst::Buffer::from_glib_full(gst_buffer) })
    } else {
        // Fallback to direct allocation
        tracing::info!("No buffer pool available, allocating directly");
        alloc_cuda_buffer(cuda_context, video_info)
    }
}

pub(crate) fn copy_to_gst_buffer(
    egl_frame: CUeglFrame,
    gst_buffer: &mut gst::Buffer,
    cuda_context: &CUDAContext,
) -> Result<gst_video::VideoInfo, Box<dyn std::error::Error>> {
    let stream_handle = cuda_context
        .stream()
        .as_ref()
        .map(|s| unsafe { gst_cuda_stream_get_handle(s.stream) })
        .unwrap_or(unsafe { std::mem::zeroed() });

    // Get memory from the buffer
    let gst_memory = unsafe { gst_ffi::gst_buffer_peek_memory(gst_buffer.as_mut_ptr(), 0) };

    if gst_memory.is_null() {
        return Err("Failed to get memory from buffer".into());
    }

    let video_info = GstCudaMemory::from_gst_memory(gst_memory)
        .ok_or("Failed to get GstCudaMemory")?
        .video_info_safe();

    // Map the GStreamer memory
    let mut map_info: gst_ffi::GstMapInfo = unsafe { std::mem::zeroed() };
    let map_success = unsafe {
        gst_ffi::gst_memory_map(
            gst_memory,
            &mut map_info,
            gst_ffi::GST_MAP_WRITE | GST_MAP_CUDA,
        )
    };

    if map_success == glib_ffi::GFALSE {
        return Err("Failed to map GStreamer CUDA memory".into());
    }

    let dst_device_ptr = map_info.data as CUdeviceptr;

    let _cuda_context_guard = CudaContextGuard::new(cuda_context)?;

    // Copy from EGL frame to GStreamer memory for each plane
    for plane in 0..egl_frame.plane_count as usize {
        let mut copy_params: CUDA_MEMCPY2D = unsafe { std::mem::zeroed() };

        // Set up source (from EGL frame)
        match egl_frame.frame_type {
            // Array type
            0 => {
                copy_params.srcMemoryType = CU_MEMORYTYPE_ARRAY;
                copy_params.srcArray = unsafe { egl_frame.frame.p_array[plane] };
            }
            // Pitched pointer type
            1 => {
                copy_params.srcMemoryType = CU_MEMORYTYPE_DEVICE;
                copy_params.srcDevice = unsafe { egl_frame.frame.p_pitch[plane] as CUdeviceptr };
                copy_params.srcPitch = egl_frame.pitch as usize;
            }
            _ => {
                return Err("Unsupported EGL frame type".into());
            }
        }

        copy_params.dstMemoryType = CU_MEMORYTYPE_DEVICE;
        copy_params.dstDevice = dst_device_ptr + video_info.offset()[plane] as u64;
        copy_params.dstPitch = video_info.stride()[plane] as usize;
        copy_params.WidthInBytes = video_info.comp_width(plane as u8) as usize
            * video_info.comp_pstride(plane as u8) as usize;
        copy_params.Height = video_info.comp_height(plane as u8) as usize;

        cuda_call!(CuMemcpy2DAsync(&copy_params, stream_handle))?;
    }

    // Safe to unmap without synchronization as the copy is async
    unsafe { gst_ffi::gst_memory_unmap(gst_memory, &mut map_info) };
    Ok(video_info)
}

pub(crate) fn cuda_result_to_string(result: CUresult) -> &'static str {
    match result {
        CUDA_SUCCESS => "CUDA_SUCCESS",
        1 => "CUDA_ERROR_INVALID_VALUE",
        2 => "CUDA_ERROR_OUT_OF_MEMORY",
        3 => "CUDA_ERROR_NOT_INITIALIZED",
        4 => "CUDA_ERROR_DEINITIALIZED",
        100 => "CUDA_ERROR_NO_DEVICE",
        101 => "CUDA_ERROR_INVALID_DEVICE",
        200 => "CUDA_ERROR_INVALID_IMAGE",
        201 => "CUDA_ERROR_INVALID_CONTEXT",
        _ => "CUDA_ERROR_UNKNOWN",
    }
}
