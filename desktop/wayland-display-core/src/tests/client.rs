use std::fs::File;
use std::os::fd::AsFd;
use std::os::unix::net::UnixStream;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use wayland_backend::client::Backend;
use wayland_client::protocol::wl_callback::WlCallback;
use wayland_client::protocol::wl_display::WlDisplay;
use wayland_client::protocol::{wl_callback, wl_pointer, wl_region};
use wayland_client::{
    Connection, Dispatch, EventQueue, QueueHandle, WEnum, delegate_noop,
    protocol::{
        wl_buffer, wl_compositor, wl_keyboard, wl_registry, wl_seat, wl_shm, wl_shm_pool,
        wl_surface,
    },
};
use wayland_protocols::{
    wp::{
        pointer_constraints::zv1::{
            client::zwp_confined_pointer_v1, client::zwp_locked_pointer_v1::ZwpLockedPointerV1,
            client::zwp_pointer_constraints_v1,
            client::zwp_pointer_constraints_v1::ZwpPointerConstraintsV1,
        },
        relative_pointer::zv1::client::zwp_relative_pointer_manager_v1::ZwpRelativePointerManagerV1,
        relative_pointer::zv1::client::zwp_relative_pointer_v1,
        relative_pointer::zv1::client::zwp_relative_pointer_v1::ZwpRelativePointerV1,
        viewporter::client::wp_viewport::WpViewport,
        viewporter::client::wp_viewporter::WpViewporter,
    },
    xdg::shell::client::{xdg_surface, xdg_toplevel, xdg_wm_base},
};

pub struct WaylandClient {
    conn: Connection,
    display: WlDisplay,
    queue: EventQueue<State>,
    qh: QueueHandle<State>,
    state: State,
}

#[derive(Debug)]
pub enum MouseEvents {
    Pointer(wl_pointer::Event),
    Relative(zwp_relative_pointer_v1::Event),
}

struct State {
    qh: QueueHandle<State>,

    compositor: Option<wl_compositor::WlCompositor>,
    buffer: Option<wl_buffer::WlBuffer>,
    wm_base: Option<xdg_wm_base::XdgWmBase>,
    viewporter: Option<WpViewporter>,
    seat: Option<wl_seat::WlSeat>,
    pointer_constraints: Option<ZwpPointerConstraintsV1>,
    relative_pointer_manager: Option<ZwpRelativePointerManagerV1>,

    pointer: Option<wl_pointer::WlPointer>,
    pointer_confined: bool,
    keyboard: Option<wl_keyboard::WlKeyboard>,
    windows: Vec<Window>,
    pub mouse_events: Vec<MouseEvents>,
}

#[derive(Debug, Clone, Default)]
pub struct Configure {
    pub size: (i32, i32),
    pub bounds: Option<(i32, i32)>,
    pub states: Vec<xdg_toplevel::State>,
}

#[derive(Default)]
pub struct SyncData {
    pub done: AtomicBool,
}

impl Dispatch<wl_registry::WlRegistry, ()> for State {
    fn event(
        state: &mut Self,
        registry: &wl_registry::WlRegistry,
        event: wl_registry::Event,
        _: &(),
        _: &Connection,
        qh: &QueueHandle<Self>,
    ) {
        if let wl_registry::Event::Global {
            name,
            interface,
            version,
        } = event
        {
            tracing::trace!("{:?} {:?}", name, interface);
            match &interface[..] {
                "wl_compositor" => {
                    let compositor =
                        registry.bind::<wl_compositor::WlCompositor, _, _>(name, version, qh, ());
                    state.compositor = Some(compositor);
                }
                "wl_shm" => {
                    let shm = registry.bind::<wl_shm::WlShm, _, _>(name, version, qh, ());

                    let (init_w, init_h) = (320, 240);

                    let mut file = tempfile::tempfile().unwrap();
                    draw(&mut file, (init_w, init_h));
                    let pool = shm.create_pool(file.as_fd(), (init_w * init_h * 4) as i32, qh, ());
                    let buffer = pool.create_buffer(
                        0,
                        init_w as i32,
                        init_h as i32,
                        (init_w * 4) as i32,
                        wl_shm::Format::Argb8888,
                        qh,
                        (),
                    );
                    state.buffer = Some(buffer.clone());
                }
                "wl_seat" => {
                    state.seat =
                        Some(registry.bind::<wl_seat::WlSeat, _, _>(name, version, qh, ()));
                }
                "xdg_wm_base" => {
                    let wm_base =
                        registry.bind::<xdg_wm_base::XdgWmBase, _, _>(name, version, qh, ());
                    state.wm_base = Some(wm_base);
                }
                "wp_viewporter" => {
                    state.viewporter =
                        Some(registry.bind::<WpViewporter, _, _>(name, version, qh, ()));
                }
                "zwp_pointer_constraints_v1" => {
                    state.pointer_constraints =
                        Some(registry.bind::<ZwpPointerConstraintsV1, _, _>(name, version, qh, ()))
                }
                "zwp_relative_pointer_manager_v1" => {
                    state.relative_pointer_manager = Some(
                        registry.bind::<ZwpRelativePointerManagerV1, _, _>(name, version, qh, ()),
                    );
                }
                _ => {}
            }
        }
    }
}

delegate_noop!(State: ignore wl_compositor::WlCompositor);
delegate_noop!(State: ignore wl_surface::WlSurface);
delegate_noop!(State: ignore wl_shm::WlShm);
delegate_noop!(State: ignore wl_shm_pool::WlShmPool);
delegate_noop!(State: ignore wl_buffer::WlBuffer);
delegate_noop!(State: ignore wl_region::WlRegion);
delegate_noop!(State: ignore WpViewporter);
delegate_noop!(State: ignore WpViewport);
delegate_noop!(State: ignore ZwpPointerConstraintsV1);
delegate_noop!(State: ignore ZwpLockedPointerV1);
delegate_noop!(State: ignore ZwpRelativePointerManagerV1);

impl WaylandClient {
    pub fn new(w_socket: UnixStream) -> Self {
        let backend = Backend::connect(w_socket).unwrap();
        let connection = Connection::from_backend(backend);
        let queue = connection.new_event_queue();
        let qh = queue.handle();

        let display = connection.display();
        let _registry = display.get_registry(&qh, ());

        let state = State {
            qh: qh.clone(),

            compositor: None,
            buffer: None,
            wm_base: None,
            viewporter: None,
            seat: None,
            pointer_constraints: None,
            relative_pointer_manager: None,

            pointer: None,
            pointer_confined: false,
            keyboard: None,
            windows: Vec::new(),
            mouse_events: Vec::new(),
        };

        WaylandClient {
            conn: connection,
            display,
            queue,
            qh,
            state,
        }
    }

    pub fn dispatch(&mut self) {
        self.conn.flush().expect("conn.flush()");
        self.queue.dispatch_pending(&mut self.state).unwrap();
        let _e = self
            .conn
            .prepare_read()
            .map(|guard| guard.read())
            .unwrap_or(Ok(0));
        // even if read_events returns an error, some messages may need dispatching
        self.queue.dispatch_pending(&mut self.state).unwrap();
    }

    pub fn send_sync(&self) -> Arc<SyncData> {
        let data = Arc::new(SyncData::default());
        self.display.sync(&self.qh, data.clone());
        data
    }

    pub fn create_window(&mut self) {
        self.state.create_window();
    }

    pub fn setup_window(&mut self, width: u16, height: u16) {
        let window = self.state.windows.last_mut().unwrap();
        window.set_title("Hello World!");
        window.attach_new_buffer(self.state.buffer.as_ref().unwrap());
        window.set_size(width, height);
        window.ack_last_and_commit();
    }

    pub fn get_client_events(&mut self) -> &mut Vec<MouseEvents> {
        self.state.mouse_events.as_mut()
    }

    /// Call this to start receiving Relative events in `get_client_events()`
    pub fn get_relative_pointer(&mut self) -> ZwpRelativePointerV1 {
        let qh = self.qh.clone();
        let pointer = self.state.pointer.as_ref().unwrap();
        self.state
            .relative_pointer_manager
            .as_ref()
            .unwrap()
            .get_relative_pointer(pointer, &qh, ())
    }

    /// Requests and acquire a [pointer lock](https://wayland.app/protocols/pointer-constraints-unstable-v1#zwp_pointer_constraints_v1:request:lock_pointer)
    ///
    /// Note that while a pointer is locked, the wl_pointer objects of the corresponding seat
    /// will not emit any wl_pointer.motion events, but relative motion events will still be emitted
    /// via wp_relative_pointer objects of the same seat. Use `get_relative_pointer()` to receive them
    pub fn lock_pointer(&mut self) -> ZwpLockedPointerV1 {
        let qh = self.qh.clone();
        let pointer = self.state.pointer.as_ref().unwrap().clone();
        let window = self.state.windows.last_mut().unwrap();

        self.state
            .pointer_constraints
            .as_ref()
            .unwrap()
            .lock_pointer(
                &window.surface,
                &pointer,
                None,
                zwp_pointer_constraints_v1::Lifetime::Oneshot,
                &qh,
                (),
            )
    }

    /// Request and acquire a [pointer confinement region](https://wayland.app/protocols/pointer-constraints-unstable-v1#zwp_pointer_constraints_v1:request:confine_pointer)
    pub fn confine_pointer(
        &mut self,
        x: i32,
        y: i32,
        width: i32,
        height: i32,
    ) -> zwp_confined_pointer_v1::ZwpConfinedPointerV1 {
        let qh = self.qh.clone();
        let pointer = self.state.pointer.as_ref().unwrap().clone();
        let window = self.state.windows.last_mut().unwrap();
        let region = self
            .state
            .compositor
            .as_ref()
            .unwrap()
            .create_region(&qh, ());
        region.add(x, y, width, height);

        self.state
            .pointer_constraints
            .as_ref()
            .unwrap()
            .confine_pointer(
                &window.surface,
                &pointer,
                Some(&region),
                zwp_pointer_constraints_v1::Lifetime::Persistent,
                &qh,
                (),
            )
    }

    pub fn is_confined(&self) -> bool {
        self.state.pointer_confined
    }
}

impl State {
    pub fn create_window(&mut self) {
        let compositor = self.compositor.as_ref().unwrap();
        let xdg_wm_base = self.wm_base.as_ref().unwrap();
        let viewporter = self.viewporter.as_ref().unwrap();

        let surface = compositor.create_surface(&self.qh, ());
        let xdg_surface = xdg_wm_base.get_xdg_surface(&surface, &self.qh, ());
        let xdg_toplevel = xdg_surface.get_toplevel(&self.qh, ());
        let viewport = viewporter.get_viewport(&surface, &self.qh, ());

        let window = Window {
            surface,
            xdg_surface,
            xdg_toplevel,
            viewport,
            pending_configure: Configure::default(),
            configures_received: Vec::new(),
            close_requested: false,
        };

        window.commit();

        self.windows.push(window);
    }
}

fn draw(tmp: &mut File, (buf_x, buf_y): (u32, u32)) {
    use std::{cmp::min, io::Write};
    let mut buf = std::io::BufWriter::new(tmp);
    for y in 0..buf_y {
        for x in 0..buf_x {
            let a = 0xFF;
            let r = min(((buf_x - x) * 0xFF) / buf_x, ((buf_y - y) * 0xFF) / buf_y);
            let g = min((x * 0xFF) / buf_x, ((buf_y - y) * 0xFF) / buf_y);
            let b = min(((buf_x - x) * 0xFF) / buf_x, (y * 0xFF) / buf_y);
            buf.write_all(&[b as u8, g as u8, r as u8, a as u8])
                .unwrap();
        }
    }
    buf.flush().unwrap();
}

impl State {}

pub struct Window {
    pub surface: wl_surface::WlSurface,
    pub xdg_surface: xdg_surface::XdgSurface,
    pub xdg_toplevel: xdg_toplevel::XdgToplevel,
    pub viewport: WpViewport,
    pub pending_configure: Configure,
    pub configures_received: Vec<(u32, Configure)>,
    pub close_requested: bool,
}

impl Window {
    pub fn commit(&self) {
        self.surface.commit();
    }

    pub fn ack_last(&self) {
        let serial = self.configures_received.last().unwrap().0;
        self.xdg_surface.ack_configure(serial);
    }

    pub fn ack_last_and_commit(&self) {
        self.ack_last();
        self.commit();
    }

    pub fn attach_new_buffer(&self, buffer: &wl_buffer::WlBuffer) {
        self.surface.attach(Some(buffer), 0, 0);
    }

    pub fn set_size(&self, w: u16, h: u16) {
        self.viewport.set_destination(i32::from(w), i32::from(h));
    }

    pub fn set_title(&self, title: &str) {
        self.xdg_toplevel.set_title(title.to_owned());
    }
}

impl Dispatch<WlCallback, Arc<SyncData>> for State {
    fn event(
        _state: &mut Self,
        _proxy: &WlCallback,
        event: <WlCallback as wayland_client::Proxy>::Event,
        data: &Arc<SyncData>,
        _conn: &Connection,
        _qhandle: &QueueHandle<Self>,
    ) {
        match event {
            wl_callback::Event::Done { .. } => data.done.store(true, Ordering::Relaxed),
            _ => unreachable!(),
        }
    }
}

impl Dispatch<xdg_wm_base::XdgWmBase, ()> for State {
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

impl Dispatch<xdg_surface::XdgSurface, ()> for State {
    fn event(
        state: &mut Self,
        xdg_surface: &xdg_surface::XdgSurface,
        event: xdg_surface::Event,
        _: &(),
        _: &Connection,
        _qh: &QueueHandle<Self>,
    ) {
        match event {
            xdg_surface::Event::Configure { serial } => {
                let window = state
                    .windows
                    .iter_mut()
                    .find(|w| w.xdg_surface == *xdg_surface)
                    .unwrap();
                let configure = window.pending_configure.clone();
                window.configures_received.push((serial, configure));
            }
            _ => unreachable!(),
        }
    }
}

impl Dispatch<xdg_toplevel::XdgToplevel, ()> for State {
    fn event(
        state: &mut Self,
        xdg_toplevel: &xdg_toplevel::XdgToplevel,
        event: xdg_toplevel::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        let window = state
            .windows
            .iter_mut()
            .find(|w| w.xdg_toplevel == *xdg_toplevel)
            .unwrap();

        match event {
            xdg_toplevel::Event::Configure {
                width,
                height,
                states,
            } => {
                let configure = &mut window.pending_configure;
                configure.size = (width, height);
                configure.states = states
                    .chunks_exact(4)
                    .flat_map(TryInto::<[u8; 4]>::try_into)
                    .map(u32::from_ne_bytes)
                    .flat_map(xdg_toplevel::State::try_from)
                    .collect();
            }
            xdg_toplevel::Event::Close => {
                window.close_requested = true;
            }
            xdg_toplevel::Event::ConfigureBounds { width, height } => {
                window.pending_configure.bounds = Some((width, height));
            }
            xdg_toplevel::Event::WmCapabilities { .. } => (),
            _ => unreachable!(),
        }
    }
}

impl Dispatch<wl_seat::WlSeat, ()> for State {
    fn event(
        state: &mut Self,
        seat: &wl_seat::WlSeat,
        event: wl_seat::Event,
        _: &(),
        _: &Connection,
        qh: &QueueHandle<Self>,
    ) {
        if let wl_seat::Event::Capabilities {
            capabilities: WEnum::Value(capabilities),
        } = event
        {
            if capabilities.contains(wl_seat::Capability::Keyboard) {
                state.keyboard = Some(seat.get_keyboard(qh, ()));
            }
            if capabilities.contains(wl_seat::Capability::Pointer) {
                state.pointer = Some(seat.get_pointer(qh, ()));
            }
        }
    }
}

impl Dispatch<wl_keyboard::WlKeyboard, ()> for State {
    fn event(
        _state: &mut Self,
        _: &wl_keyboard::WlKeyboard,
        event: wl_keyboard::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        tracing::debug!("{:?}", event);
    }
}

impl Dispatch<wl_pointer::WlPointer, ()> for State {
    fn event(
        state: &mut Self,
        _: &wl_pointer::WlPointer,
        event: wl_pointer::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        tracing::debug!("{:?}", event);
        state.mouse_events.push(MouseEvents::Pointer(event));
    }
}

impl Dispatch<ZwpRelativePointerV1, ()> for State {
    fn event(
        state: &mut Self,
        _: &ZwpRelativePointerV1,
        event: zwp_relative_pointer_v1::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        tracing::debug!("{:?}", event);
        state.mouse_events.push(MouseEvents::Relative(event));
    }
}

impl Dispatch<zwp_confined_pointer_v1::ZwpConfinedPointerV1, ()> for State {
    fn event(
        state: &mut Self,
        _: &zwp_confined_pointer_v1::ZwpConfinedPointerV1,
        event: zwp_confined_pointer_v1::Event,
        _: &(),
        _: &Connection,
        _: &QueueHandle<Self>,
    ) {
        tracing::debug!("{:?}", event);
        match event {
            zwp_confined_pointer_v1::Event::Confined => {
                state.pointer_confined = true;
            }
            zwp_confined_pointer_v1::Event::Unconfined => {
                state.pointer_confined = false;
            }
            _ => {}
        }
    }
}
