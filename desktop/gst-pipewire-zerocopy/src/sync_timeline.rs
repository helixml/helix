//! PipeWire explicit synchronization (SPA_META_SyncTimeline) support.
//!
//! Why this exists: on NVIDIA there is no implicit dma-buf sync, so when we
//! GL-sample a buffer Mutter just handed us, Mutter's GPU write may not be
//! finished → we encode stale/partial pixels for one frame (the "stale-frame
//! flash" under heavy compositor load). The fix is to wait for Mutter's EXPLICIT
//! acquire fence before sampling, and signal the release point when we're done so
//! Mutter can reuse the slot — exactly what OBS does for NVIDIA.
//!
//! The wait must be GPU-side (`eglWaitSync` via smithay's `EGLFence`), NOT a CPU
//! wait, because the capture runs on the single PipeWire loop thread; a CPU wait
//! there delays buffer recycling and throttles Mutter. `EGLFence::wait()` is
//! server-side: the GPU stream waits, the CPU returns immediately.
//!
//! Gated behind `HELIX_EXPLICIT_SYNC` (default OFF) so it can be enabled per
//! session for testing without risking other users on a shared host. When off,
//! we do NOT negotiate SyncTimeline, so Mutter never waits on a release point and
//! behaviour is identical to today.

use std::os::fd::{FromRawFd, OwnedFd};

// SPA constants (spa/buffer/meta.h, spa/buffer/buffer.h)
pub const SPA_META_SYNC_TIMELINE: u32 = 9;
pub const SPA_DATA_SYNC_OBJ: u32 = 5;
/// sizeof(struct spa_meta_sync_timeline) = two u64 = 16 bytes.
pub const SPA_META_SYNC_TIMELINE_SIZE: i32 = 16;

/// struct spa_meta_sync_timeline { uint64_t acquire_point; uint64_t release_point; }
#[repr(C)]
#[derive(Clone, Copy, Debug)]
pub struct SpaMetaSyncTimeline {
    pub acquire_point: u64,
    pub release_point: u64,
}

// libdrm syncobj API (stable C functions in libdrm's xf86drm.h).
#[link(name = "drm")]
extern "C" {
    fn drmSyncobjCreate(fd: i32, flags: u32, handle: *mut u32) -> i32;
    fn drmSyncobjDestroy(fd: i32, handle: u32) -> i32;
    fn drmSyncobjFDToHandle(fd: i32, obj_fd: i32, handle: *mut u32) -> i32;
    fn drmSyncobjExportSyncFile(fd: i32, handle: u32, sync_file_fd: *mut i32) -> i32;
    fn drmSyncobjTransfer(
        fd: i32,
        dst_handle: u32,
        dst_point: u64,
        src_handle: u32,
        src_point: u64,
        flags: u32,
    ) -> i32;
    fn drmSyncobjTimelineSignal(
        fd: i32,
        handles: *const u32,
        points: *const u64,
        handle_count: u32,
    ) -> i32;
}

/// True if explicit sync is enabled via env. Read once.
pub fn enabled() -> bool {
    use std::sync::OnceLock;
    static ON: OnceLock<bool> = OnceLock::new();
    *ON.get_or_init(|| {
        std::env::var("HELIX_EXPLICIT_SYNC")
            .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
            .unwrap_or(false)
    })
}

/// Open the render node to drive syncobj ioctls. Returns a raw fd (kept for the
/// process lifetime — we leak it intentionally; one fd per stream).
pub fn open_drm_render_fd() -> Option<i32> {
    use std::os::fd::IntoRawFd;
    for path in ["/dev/dri/renderD128", "/dev/dri/renderD129"] {
        if let Ok(f) = std::fs::OpenOptions::new().read(true).write(true).open(path) {
            return Some(f.into_raw_fd());
        }
    }
    None
}

/// Convert Mutter's acquire timeline point into a sync_file fd we can hand to
/// `EGLFence::import` for a GPU-side wait. Steps: import the syncobj fd → a handle,
/// create a temp binary syncobj, transfer the timeline point into it, export it as
/// a sync_file. Returns None on any failure (caller then skips the wait).
///
/// # Safety
/// `drm_fd` must be a valid DRM fd; `acquire_obj_fd` a valid syncobj fd.
pub unsafe fn export_acquire_sync_file(
    drm_fd: i32,
    acquire_obj_fd: i32,
    acquire_point: u64,
) -> Option<OwnedFd> {
    let mut acq_handle: u32 = 0;
    if drmSyncobjFDToHandle(drm_fd, acquire_obj_fd, &mut acq_handle) != 0 || acq_handle == 0 {
        return None;
    }
    let mut tmp: u32 = 0;
    if drmSyncobjCreate(drm_fd, 0, &mut tmp) != 0 || tmp == 0 {
        drmSyncobjDestroy(drm_fd, acq_handle);
        return None;
    }
    // Copy the acquire timeline point into the binary (point 0) syncobj.
    let mut sync_file_fd: i32 = -1;
    if drmSyncobjTransfer(drm_fd, tmp, 0, acq_handle, acquire_point, 0) == 0 {
        let _ = drmSyncobjExportSyncFile(drm_fd, tmp, &mut sync_file_fd);
    }
    drmSyncobjDestroy(drm_fd, tmp);
    drmSyncobjDestroy(drm_fd, acq_handle);
    if sync_file_fd >= 0 {
        Some(OwnedFd::from_raw_fd(sync_file_fd))
    } else {
        None
    }
}

/// Signal Mutter's release point so it may reuse the buffer slot. We call this
/// after our synchronous CUDA copy has read the buffer, so an immediate CPU
/// signal is correct (we are provably done reading). Without this, Mutter blocks
/// forever waiting for the release once SyncTimeline is negotiated.
///
/// # Safety
/// `drm_fd` valid; `release_obj_fd` a valid syncobj fd.
pub unsafe fn signal_release(drm_fd: i32, release_obj_fd: i32, release_point: u64) {
    let mut rel_handle: u32 = 0;
    if drmSyncobjFDToHandle(drm_fd, release_obj_fd, &mut rel_handle) != 0 || rel_handle == 0 {
        return;
    }
    let handles = [rel_handle];
    let points = [release_point];
    drmSyncobjTimelineSignal(drm_fd, handles.as_ptr(), points.as_ptr(), 1);
    drmSyncobjDestroy(drm_fd, rel_handle);
}
