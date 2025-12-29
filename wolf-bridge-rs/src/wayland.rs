//! Wayland client for Wolf's compositor
//!
//! Connects to Wolf's Wayland display and submits frames via DMA-BUF
//! (zero-copy) or SHM (fallback).

use anyhow::{Context, Result};
use std::os::unix::io::RawFd;
use tracing::{debug, info, warn};
use wayland_client::{
    protocol::{wl_buffer, wl_compositor, wl_registry, wl_shm, wl_shm_pool, wl_surface},
    Connection, Dispatch, EventQueue, Proxy, QueueHandle,
};
use wayland_protocols::xdg::shell::client::{xdg_surface, xdg_toplevel, xdg_wm_base};
use wayland_protocols::wp::linux_dmabuf::zv1::client::{
    zwp_linux_buffer_params_v1, zwp_linux_dmabuf_v1,
};

/// Wayland client state
pub struct WaylandClient {
    conn: Connection,
    queue: EventQueue<WaylandState>,
    state: WaylandState,
}

struct WaylandState {
    compositor: Option<wl_compositor::WlCompositor>,
    shm: Option<wl_shm::WlShm>,
    dmabuf: Option<zwp_linux_dmabuf_v1::ZwpLinuxDmabufV1>,
    xdg_wm_base: Option<xdg_wm_base::XdgWmBase>,
    surface: Option<wl_surface::WlSurface>,
    xdg_surface: Option<xdg_surface::XdgSurface>,
    toplevel: Option<xdg_toplevel::XdgToplevel>,
    current_buffer: Option<wl_buffer::WlBuffer>,
    configured: bool,
    frame_pending: bool,
    dmabuf_formats: Vec<u32>,
}

impl WaylandClient {
    /// Connect to Wolf's Wayland display
    pub fn connect(display_name: &str) -> Result<Self> {
        let conn = Connection::connect_to_env()
            .or_else(|_| {
                std::env::set_var("WAYLAND_DISPLAY", display_name);
                Connection::connect_to_env()
            })
            .context("Failed to connect to Wayland display")?;

        let display = conn.display();
        let mut queue = conn.new_event_queue();
        let qh = queue.handle();

        let mut state = WaylandState {
            compositor: None,
            shm: None,
            dmabuf: None,
            xdg_wm_base: None,
            surface: None,
            xdg_surface: None,
            toplevel: None,
            current_buffer: None,
            configured: false,
            frame_pending: false,
            dmabuf_formats: Vec::new(),
        };

        // Get registry
        let _registry = display.get_registry(&qh, ());
        queue.roundtrip(&mut state)?;

        // Verify required globals
        if state.compositor.is_none() {
            anyhow::bail!("No wl_compositor found");
        }
        if state.xdg_wm_base.is_none() {
            anyhow::bail!("No xdg_wm_base found");
        }
        if state.shm.is_none() {
            anyhow::bail!("No wl_shm found");
        }

        // DMA-BUF is optional
        if state.dmabuf.is_some() {
            queue.roundtrip(&mut state)?;
            info!("DMA-BUF supported ({} formats)", state.dmabuf_formats.len());
        } else {
            warn!("DMA-BUF not available, using SHM fallback");
        }

        // Create surface
        let compositor = state.compositor.as_ref().unwrap();
        let surface = compositor.create_surface(&qh, ());
        state.surface = Some(surface.clone());

        // Create xdg_surface
        let xdg_wm_base = state.xdg_wm_base.as_ref().unwrap();
        let xdg_surface = xdg_wm_base.get_xdg_surface(&surface, &qh, ());
        state.xdg_surface = Some(xdg_surface.clone());

        // Create toplevel
        let toplevel = xdg_surface.get_toplevel(&qh, ());
        toplevel.set_title("Desktop".to_string());
        toplevel.set_app_id("wolf-bridge".to_string());
        toplevel.set_fullscreen(None);
        state.toplevel = Some(toplevel);

        // Commit and wait for configure
        surface.commit();
        queue.roundtrip(&mut state)?;

        while !state.configured {
            queue.blocking_dispatch(&mut state)?;
        }

        info!("Wayland surface created and configured");

        Ok(Self { conn, queue, state })
    }

    /// Check if DMA-BUF is supported
    pub fn has_dmabuf(&self) -> bool {
        self.state.dmabuf.is_some()
    }

    /// Dispatch pending Wayland events
    pub fn dispatch(&mut self) -> Result<()> {
        self.queue.dispatch_pending(&mut self.state)?;
        self.queue.flush()?;
        Ok(())
    }

    /// Submit a DMA-BUF frame
    pub fn submit_dmabuf(
        &mut self,
        fd: RawFd,
        width: u32,
        height: u32,
        stride: u32,
        format: u32,
        modifier: u64,
    ) -> Result<()> {
        let dmabuf = match &self.state.dmabuf {
            Some(d) => d,
            None => anyhow::bail!("DMA-BUF not supported"),
        };

        if self.state.frame_pending {
            return Ok(()); // Skip frame
        }

        let qh = self.queue.handle();

        // Create DMA-BUF params
        let params = dmabuf.create_params(&qh, ());
        params.add(
            fd,
            0, // plane index
            0, // offset
            stride,
            (modifier >> 32) as u32,
            (modifier & 0xffffffff) as u32,
        );
        params.create(width as i32, height as i32, format, 0);

        self.queue.roundtrip(&mut self.state)?;

        // Note: Buffer creation is async, handled in the event handler
        // For now, we commit the surface
        if let Some(surface) = &self.state.surface {
            if let Some(buffer) = &self.state.current_buffer {
                surface.attach(Some(buffer), 0, 0);
                surface.damage_buffer(0, 0, width as i32, height as i32);
                surface.commit();
                self.state.frame_pending = true;
            }
        }

        Ok(())
    }

    /// Submit a SHM frame (fallback path)
    pub fn submit_shm(
        &mut self,
        data: &[u8],
        width: u32,
        height: u32,
        stride: u32,
        format: u32,
    ) -> Result<()> {
        if self.state.frame_pending {
            return Ok(()); // Skip frame
        }

        let shm = match &self.state.shm {
            Some(s) => s,
            None => anyhow::bail!("SHM not available"),
        };

        let size = (stride * height) as usize;
        if data.len() < size {
            anyhow::bail!("Data too small for frame");
        }

        // Create SHM file
        let fd = create_shm_file(size)?;

        // Map and copy data
        unsafe {
            let ptr = nix::sys::mman::mmap(
                None,
                std::num::NonZeroUsize::new(size).unwrap(),
                nix::sys::mman::ProtFlags::PROT_READ | nix::sys::mman::ProtFlags::PROT_WRITE,
                nix::sys::mman::MapFlags::MAP_SHARED,
                Some(&fd),
                0,
            )?;
            std::ptr::copy_nonoverlapping(data.as_ptr(), ptr.as_ptr() as *mut u8, size);
            nix::sys::mman::munmap(ptr, size)?;
        }

        let qh = self.queue.handle();

        // Create pool and buffer
        let pool = shm.create_pool(fd.as_raw_fd(), size as i32, &qh, ());
        let wl_format = match format {
            0x34325258 => wl_shm::Format::Xrgb8888, // DRM_FORMAT_XRGB8888
            0x34325241 => wl_shm::Format::Argb8888, // DRM_FORMAT_ARGB8888
            _ => wl_shm::Format::Argb8888,
        };

        let buffer = pool.create_buffer(
            0,
            width as i32,
            height as i32,
            stride as i32,
            wl_format,
            &qh,
            (),
        );

        // Destroy old buffer
        if let Some(old) = self.state.current_buffer.take() {
            old.destroy();
        }
        self.state.current_buffer = Some(buffer.clone());

        // Attach and commit
        if let Some(surface) = &self.state.surface {
            surface.attach(Some(&buffer), 0, 0);
            surface.damage_buffer(0, 0, width as i32, height as i32);
            surface.commit();
            self.state.frame_pending = true;
        }

        pool.destroy();

        Ok(())
    }
}

fn create_shm_file(size: usize) -> Result<std::os::fd::OwnedFd> {
    use nix::fcntl::OFlag;
    use nix::sys::stat::Mode;
    use std::ffi::CString;

    let name = CString::new(format!("/wolf-bridge-{}", std::process::id()))?;

    let fd = nix::sys::mman::shm_open(
        name.as_c_str(),
        OFlag::O_RDWR | OFlag::O_CREAT | OFlag::O_EXCL,
        Mode::S_IRUSR | Mode::S_IWUSR,
    )?;

    nix::sys::mman::shm_unlink(name.as_c_str())?;
    nix::unistd::ftruncate(&fd, size as i64)?;

    Ok(fd)
}

// Wayland event handlers

impl Dispatch<wl_registry::WlRegistry, ()> for WaylandState {
    fn event(
        state: &mut Self,
        registry: &wl_registry::WlRegistry,
        event: wl_registry::Event,
        _: &(),
        _: &Connection,
        qh: &QueueHandle<Self>,
    ) {
        if let wl_registry::Event::Global { name, interface, version } = event {
            match interface.as_str() {
                "wl_compositor" => {
                    state.compositor = Some(registry.bind(name, version.min(4), qh, ()));
                }
                "wl_shm" => {
                    state.shm = Some(registry.bind(name, 1, qh, ()));
                }
                "xdg_wm_base" => {
                    state.xdg_wm_base = Some(registry.bind(name, 1, qh, ()));
                }
                "zwp_linux_dmabuf_v1" => {
                    state.dmabuf = Some(registry.bind(name, version.min(3), qh, ()));
                }
                _ => {}
            }
        }
    }
}

impl Dispatch<wl_compositor::WlCompositor, ()> for WaylandState {
    fn event(_: &mut Self, _: &wl_compositor::WlCompositor, _: wl_compositor::Event, _: &(), _: &Connection, _: &QueueHandle<Self>) {}
}

impl Dispatch<wl_shm::WlShm, ()> for WaylandState {
    fn event(_: &mut Self, _: &wl_shm::WlShm, _: wl_shm::Event, _: &(), _: &Connection, _: &QueueHandle<Self>) {}
}

impl Dispatch<wl_shm_pool::WlShmPool, ()> for WaylandState {
    fn event(_: &mut Self, _: &wl_shm_pool::WlShmPool, _: wl_shm_pool::Event, _: &(), _: &Connection, _: &QueueHandle<Self>) {}
}

impl Dispatch<wl_surface::WlSurface, ()> for WaylandState {
    fn event(_: &mut Self, _: &wl_surface::WlSurface, _: wl_surface::Event, _: &(), _: &Connection, _: &QueueHandle<Self>) {}
}

impl Dispatch<wl_buffer::WlBuffer, ()> for WaylandState {
    fn event(
        state: &mut Self,
        _: &wl_buffer::WlBuffer,
        event: wl_buffer::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        if let wl_buffer::Event::Release = event {
            state.frame_pending = false;
        }
    }
}

impl Dispatch<xdg_wm_base::XdgWmBase, ()> for WaylandState {
    fn event(
        _: &mut Self,
        wm_base: &xdg_wm_base::XdgWmBase,
        event: xdg_wm_base::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        if let xdg_wm_base::Event::Ping { serial } = event {
            wm_base.pong(serial);
        }
    }
}

impl Dispatch<xdg_surface::XdgSurface, ()> for WaylandState {
    fn event(
        state: &mut Self,
        surface: &xdg_surface::XdgSurface,
        event: xdg_surface::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        if let xdg_surface::Event::Configure { serial } = event {
            surface.ack_configure(serial);
            state.configured = true;
        }
    }
}

impl Dispatch<xdg_toplevel::XdgToplevel, ()> for WaylandState {
    fn event(_: &mut Self, _: &xdg_toplevel::XdgToplevel, _: xdg_toplevel::Event, _: &(), _: &Connection, _: &QueueHandle<Self>) {}
}

impl Dispatch<zwp_linux_dmabuf_v1::ZwpLinuxDmabufV1, ()> for WaylandState {
    fn event(
        state: &mut Self,
        _: &zwp_linux_dmabuf_v1::ZwpLinuxDmabufV1,
        event: zwp_linux_dmabuf_v1::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        if let zwp_linux_dmabuf_v1::Event::Format { format } = event {
            state.dmabuf_formats.push(format);
        }
    }
}

impl Dispatch<zwp_linux_buffer_params_v1::ZwpLinuxBufferParamsV1, ()> for WaylandState {
    fn event(
        state: &mut Self,
        params: &zwp_linux_buffer_params_v1::ZwpLinuxBufferParamsV1,
        event: zwp_linux_buffer_params_v1::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        match event {
            zwp_linux_buffer_params_v1::Event::Created { buffer } => {
                if let Some(old) = state.current_buffer.take() {
                    old.destroy();
                }
                state.current_buffer = Some(buffer);
                params.destroy();
            }
            zwp_linux_buffer_params_v1::Event::Failed => {
                warn!("DMA-BUF buffer creation failed");
                params.destroy();
            }
            _ => {}
        }
    }
}
