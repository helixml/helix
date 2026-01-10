use crate::comp::{ClientState, NixInterface, State};
use crate::tests::client::WaylandClient;
use crate::utils::RenderTarget;
use crate::utils::allocator::{GsBufferType, GsGlesbuffer};
use crate::utils::tests::INIT;
use calloop::generic::Generic;
use calloop::{EventLoop, Interest, Mode, PostAction};
use gst_video::VideoInfo;
use smithay::backend::renderer::damage::OutputDamageTracker;
use smithay::output;
use smithay::output::{Output, PhysicalProperties, Subpixel};
use smithay::reexports::input::Libinput;
use smithay::reexports::wayland_server::Display;
use smithay::wayland::socket::ListeningSocketSource;
use std::os::unix::net::UnixStream;
use std::sync::Arc;
use std::sync::atomic::Ordering;
use std::time::Duration;

pub struct Fixture {
    pub client: WaylandClient,
    pub server: State,
    server_event_loop: EventLoop<'static, State>,
}

impl Fixture {
    pub fn new() -> Self {
        INIT.call_once(|| {
            gst::init().expect("Failed to initialize GStreamer");
        });

        let libinput_context = Libinput::new_from_path(NixInterface);
        let event_loop = EventLoop::<State>::try_new().expect("Unable to create event_loop");
        let display = Display::<State>::new().unwrap();
        let dh = display.handle();

        let server_state = State::new(
            &RenderTarget::Software,
            &dh,
            &libinput_context,
            event_loop.handle(),
        );

        let source = ListeningSocketSource::new_auto().unwrap();
        let socket_name = source.socket_name().to_string_lossy().into_owned();
        tracing::info!(?socket_name, "Listening on wayland socket.");
        server_state
            .handle
            .insert_source(source, |client_stream, _, state| {
                if let Err(err) = state
                    .dh
                    .insert_client(client_stream, Arc::new(ClientState::default()))
                {
                    tracing::error!(?err, "Error adding wayland client.");
                };
            })
            .expect("Failed to init wayland socket source");

        server_state
            .handle
            .insert_source(
                Generic::new(display, Interest::READ, Mode::Level),
                |_, display, state| {
                    // Safety: we don't drop the display
                    unsafe {
                        display.get_mut().dispatch_clients(state).unwrap();
                    }
                    Ok(PostAction::Continue)
                },
            )
            .unwrap();

        // Setup Wayland client
        let runtime_dir = std::env::var("XDG_RUNTIME_DIR").unwrap();
        let wclient = WaylandClient::new(
            UnixStream::connect(format!("{}/{}", runtime_dir, socket_name)).unwrap(),
        );

        let mut f = Fixture {
            client: wclient,
            server: server_state,
            server_event_loop: event_loop,
        };

        f.create_server_output();
        f.round_trip();

        f
    }

    fn create_server_output(&mut self) {
        let output = self.server.output.get_or_insert_with(|| {
            let output = Output::new(
                "HEADLESS-1".into(),
                PhysicalProperties {
                    make: "Virtual".into(),
                    model: "Wolf".into(),
                    size: (0, 0).into(),
                    subpixel: Subpixel::Unknown,
                },
            );
            output.create_global::<State>(&self.server.dh);
            output
        });
        let mode = output::Mode {
            size: (320, 240).into(),
            refresh: 1000, // 1 FPS
        };
        output.change_current_state(Some(mode), None, None, None);
        output.set_preferred(mode);
        let dtr = OutputDamageTracker::from_output(&output);

        self.server.space.map_output(&output, (0, 0));
        self.server.dtr = Some(dtr);
        self.server
            .set_pointer_location((mode.size.w as f64 / 2.0, mode.size.h as f64 / 2.0).into());

        let video_info = VideoInfo::builder(
            gst_video::VideoFormat::Rgba,
            mode.size.w as u32,
            mode.size.h as u32,
        )
        .build()
        .unwrap();
        let allocator = GsGlesbuffer::new(&mut self.server.renderer, video_info.clone())
            .expect("Failed to create GsGlesbuffer");
        self.server.output_buffer = Some(GsBufferType::RAW(allocator));
        self.server.video_info = Some(video_info);
    }

    pub fn round_trip(&mut self) {
        let data = self.client.send_sync();
        let mut run_times = 100;
        while !data.done.load(Ordering::Relaxed) && run_times > 0 {
            self.update_server();
            self.update_client();

            std::thread::sleep(Duration::from_millis(10));
            run_times -= 1;
        }
        if run_times == 0 {
            panic!("Timeout establishing connection to wayland server!");
        }
    }

    pub fn update_server(&mut self) {
        self.server
            .dh
            .flush_clients()
            .expect("Failed to flush clients");
        self.server.space.refresh();
        self.server_event_loop
            .dispatch(Duration::ZERO, &mut self.server)
            .unwrap();
    }

    pub fn update_client(&mut self) {
        self.client.dispatch();
    }

    pub fn create_window(&mut self, width: u16, height: u16) {
        self.client.create_window();
        self.round_trip();

        self.client.setup_window(width, height);
        self.round_trip();
        self.round_trip();

        // I don't know why this isn't triggered, but I've spent hours trying to debug why the
        // window size was kept at (0,0). Without this `under` in input.rs would never work
        for window in self.server.space.elements() {
            window.on_commit();
        }
    }
}
