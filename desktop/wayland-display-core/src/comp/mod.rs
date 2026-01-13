use super::{Command, DrmFormat, GstVideoInfo};
use gst_video::VideoInfo;
use smithay::backend::allocator::format::FormatSet;
use smithay::backend::input::AxisSource;
use smithay::backend::input::TouchSlot;
use smithay::backend::renderer::ImportEgl;
use smithay::backend::renderer::gles::GlesRenderer;
use smithay::reexports::gbm::BufferObjectFlags;
use smithay::wayland::dmabuf::DmabufFeedbackBuilder;
use smithay::wayland::presentation::Refresh;
use smithay::wayland::single_pixel_buffer::SinglePixelBufferState;
use smithay::{
    backend::{
        allocator::{Fourcc, dmabuf::Dmabuf},
        drm::DrmNode,
        libinput::LibinputInputBackend,
        renderer::{
            Bind,
            damage::{Error as DTRError, OutputDamageTracker},
            element::memory::{MemoryBuffer, MemoryRenderBuffer},
        },
    },
    desktop::{
        PopupManager, Space, Window,
        utils::{
            OutputPresentationFeedback, send_frames_surface_tree,
            surface_presentation_feedback_flags_from_states, surface_primary_scanout_output,
            update_surface_primary_scanout_output,
        },
    },
    input::{Seat, SeatState, keyboard::XkbConfig, pointer::CursorImageStatus},
    output::{Mode as OutputMode, Output, PhysicalProperties, Subpixel},
    reexports::{
        calloop::{
            EventLoop, Interest, LoopHandle, Mode, PostAction,
            channel::{Channel, Event},
            generic::Generic,
            timer::{TimeoutAction, Timer},
        },
        input::Libinput,
        wayland_protocols::wp::presentation_time::server::wp_presentation_feedback,
        wayland_server::{
            Display, DisplayHandle,
            backend::{ClientData, ClientId, DisconnectReason, GlobalId},
        },
    },
    utils::{Clock, Logical, Monotonic, Physical, Point, Rectangle, Size, Transform},
    wayland::{
        compositor::{CompositorClientState, CompositorState, with_states},
        dmabuf::{DmabufGlobal, DmabufState},
        output::OutputManagerState,
        pointer_constraints::PointerConstraintsState,
        presentation::PresentationState,
        relative_pointer::RelativePointerManagerState,
        selection::data_device::DataDeviceState,
        shell::xdg::{SurfaceCachedState, XdgShellState, XdgToplevelSurfaceData},
        shm::ShmState,
        socket::ListeningSocketSource,
        viewporter::ViewporterState,
    },
};
use std::sync::Mutex;
use std::{
    collections::HashSet,
    ffi::CString,
    sync::{Arc, mpsc::Sender},
    time::{Duration, Instant},
};
use tracing::debug;

mod focus;
mod input;
mod rendering;

pub use self::focus::*;
pub use self::input::*;
pub use self::rendering::*;
#[cfg(feature = "cuda")]
use crate::utils::allocator::GsCUDABuf;
use crate::utils::allocator::{
    GsBuffer, GsBufferType, GsDmaBuf, GsGlesbuffer, VideoInfoTypes, gst_video_format_to_drm_fourcc,
    gst_video_format_to_drm_modifier, new_gbm_device,
};
use crate::utils::device::gpu::GPUDevice;
use crate::utils::renderer::setup_renderer;
use crate::{utils::RenderTarget, wayland::protocols::wl_drm::create_drm_global};

#[derive(Debug, Default)]
pub struct ClientState {
    pub compositor_state: CompositorClientState,
}

impl ClientData for ClientState {
    fn initialized(&self, _client_id: ClientId) {}
    fn disconnected(&self, _client_id: ClientId, _reason: DisconnectReason) {}
}

#[allow(dead_code)]
pub struct State {
    pub handle: LoopHandle<'static, State>,
    should_quit: bool,
    pub(crate) clock: Clock<Monotonic>,

    // render
    pub(crate) dtr: Option<OutputDamageTracker>,
    pub(crate) output_buffer: Option<GsBufferType>,
    render_node: Option<DrmNode>,
    pub renderer: GlesRenderer,
    dmabuf_global: Option<(DmabufGlobal, GlobalId)>,
    last_render: Option<Instant>,

    // management
    pub output: Option<Output>,
    pub video_info: Option<VideoInfo>,
    pub seat: Seat<Self>,
    pub space: Space<Window>,
    pub popups: PopupManager,
    pub(crate) pointer_location: Point<f64, Logical>,
    pub(crate) pointer_absolute_location: Point<f64, Logical>,
    last_pointer_movement: Instant,
    cursor_element: MemoryRenderBuffer,
    pub cursor_state: CursorImageStatus,
    surpressed_keys: HashSet<u32>,
    pub pending_windows: Vec<Window>,
    input_context: Libinput,

    // wayland state
    pub dh: DisplayHandle,
    pub compositor_state: CompositorState,
    pub data_device_state: DataDeviceState,
    pub dmabuf_state: DmabufState,
    output_state: OutputManagerState,
    presentation_state: PresentationState,
    relative_ptr_state: RelativePointerManagerState,
    pointer_constraints_state: PointerConstraintsState,
    pub seat_state: SeatState<Self>,
    pub shell_state: XdgShellState,
    pub shm_state: ShmState,
    viewporter_state: ViewporterState,
    cursor_event_count: i32,
    pub single_pixel_buffer_state: SinglePixelBufferState,
}

impl State {
    pub fn new(
        render_target: &RenderTarget,
        dh: &DisplayHandle,
        input_context: &Libinput,
        event_loop_handle: LoopHandle<'static, State>,
    ) -> Self {
        let clock = Clock::new();

        // init state
        let compositor_state = CompositorState::new_v6::<State>(&dh);
        let data_device_state = DataDeviceState::new::<State>(&dh);
        let mut dmabuf_state = DmabufState::new();
        let output_state = OutputManagerState::new_with_xdg_output::<State>(&dh);
        let presentation_state = PresentationState::new::<State>(&dh, clock.id() as _);
        let relative_ptr_state = RelativePointerManagerState::new::<State>(&dh);
        let pointer_constraints_state = PointerConstraintsState::new::<State>(&dh);
        let mut seat_state = SeatState::new();
        let shell_state = XdgShellState::new::<State>(&dh);
        let viewporter_state = ViewporterState::new::<State>(&dh);
        let single_pixel_buffer_state = SinglePixelBufferState::new::<Self>(&dh);

        let render_node: Option<DrmNode> = render_target.clone().into();

        let mut renderer = setup_renderer(render_node);

        let shm_state = ShmState::new::<State>(&dh, vec![]);
        let dmabuf_global = if let RenderTarget::Hardware(node) = render_target {
            let formats = Bind::<Dmabuf>::supported_formats(&renderer)
                .expect("Failed to query formats")
                .into_iter()
                .collect::<Vec<_>>();

            let dmabuf_default_feedback =
                DmabufFeedbackBuilder::new(node.dev_id(), formats.clone()).build();

            let dmabuf_global = if let Ok(default_feedback) = dmabuf_default_feedback {
                dmabuf_state.create_global_with_default_feedback::<State>(&dh, &default_feedback)
            } else {
                tracing::warn!("Failed to create default feedback for dmabuf, falling back to v3");
                dmabuf_state.create_global::<State>(&dh, formats.clone())
            };

            match renderer.bind_wl_display(&dh) {
                Ok(_) => tracing::info!("EGL hardware-acceleration enabled"),
                Err(err) => tracing::info!(?err, "Failed to initialize EGL hardware-acceleration"),
            }

            // wl_drm (mesa protocol, so we don't need EGL_WL_bind_display)
            let wl_drm_global = create_drm_global::<State>(
                &dh,
                node.dev_path().expect("Failed to determine DrmNode path?"),
                formats.clone(),
                &dmabuf_global,
            );

            Some((dmabuf_global, wl_drm_global))
        } else {
            None
        };

        let cursor_element = MemoryRenderBuffer::from_memory(
            MemoryBuffer::from_slice(CURSOR_DATA_BYTES, Fourcc::Abgr8888, (64, 64)),
            1,
            Transform::Normal,
            None,
        );

        let space = Space::default();

        let mut seat = seat_state.new_wl_seat(&dh, "seat-0");
        seat.add_keyboard(XkbConfig::default(), 200, 25)
            .expect("Failed to add keyboard to seat");
        seat.add_pointer();
        seat.add_touch();

        State {
            handle: event_loop_handle,
            should_quit: false,
            clock,

            renderer,
            dtr: None,
            output_buffer: None,
            render_node,
            dmabuf_global,
            video_info: None,
            last_render: None,

            space,
            popups: PopupManager::default(),
            seat,
            output: None,
            pointer_location: (0., 0.).into(),
            pointer_absolute_location: (0., 0.).into(),
            last_pointer_movement: Instant::now(),
            cursor_element,
            cursor_state: CursorImageStatus::default_named(),
            cursor_event_count: 0,
            surpressed_keys: HashSet::new(),
            pending_windows: Vec::new(),
            input_context: input_context.clone(),

            dh: dh.clone(),
            compositor_state,
            data_device_state,
            dmabuf_state,
            output_state,
            presentation_state,
            relative_ptr_state,
            pointer_constraints_state,
            seat_state,
            shell_state,
            shm_state,
            viewporter_state,
            single_pixel_buffer_state,
        }
    }
}

pub(crate) fn init(
    command_src: Channel<Command>,
    render: impl Into<RenderTarget>,
    devices_tx: Sender<Vec<CString>>,
    envs_tx: Sender<Vec<CString>>,
) {
    let render_target = render.into();
    let _ = devices_tx.send(render_target.clone().as_devices());
    let render_node: Option<DrmNode> = render_target.clone().into();

    let mut event_loop = EventLoop::<State>::try_new().expect("Unable to create event_loop");

    let display = Display::<State>::new().unwrap();
    // init input backend
    let libinput_context = Libinput::new_from_path(NixInterface);
    let input_context = libinput_context.clone();
    let libinput_backend = LibinputInputBackend::new(libinput_context);

    let mut state = State::new(
        &render_target,
        &display.handle(),
        &input_context,
        event_loop.handle(),
    );

    // init event loop
    state
        .handle
        .insert_source(libinput_backend, move |event, _, state| {
            state.process_input_event(event)
        })
        .unwrap();

    state
        .handle
        .insert_source(command_src, move |event, _, state| {
            match event {
                Event::Msg(Command::VideoInfo(video_info)) => {
                    // Only change the output if it's not running already
                    // TODO: properly support automatic resolution switching with DMA buffers
                    if state.output.is_some() {
                        tracing::info!(
                            "Output already running, ignoring newly negotiated video info"
                        );
                        return;
                    }
                    let base_info: VideoInfo = video_info.clone().into();
                    debug!(
                        "Requested video format: {} .to_fourcc() = {}",
                        base_info.format(),
                        base_info.format().to_fourcc()
                    );
                    let size: Size<i32, Physical> =
                        (base_info.width() as i32, base_info.height() as i32).into();
                    let framerate = base_info.fps();
                    let duration = Duration::from_secs_f64(
                        framerate.numer() as f64 / framerate.denom() as f64,
                    );

                    // init wayland objects
                    let output = state.output.get_or_insert_with(|| {
                        let output = Output::new(
                            "HEADLESS-1".into(),
                            PhysicalProperties {
                                make: "Virtual".into(),
                                model: "Wolf".into(),
                                size: (0, 0).into(),
                                subpixel: Subpixel::Unknown,
                            },
                        );
                        output.create_global::<State>(&state.dh);
                        output
                    });
                    let mode = OutputMode {
                        size: size.into(),
                        refresh: (duration.as_secs_f64() * 1000.0).round() as i32,
                    };
                    output.change_current_state(Some(mode), None, None, None);
                    output.set_preferred(mode);
                    let dtr = OutputDamageTracker::from_output(&output);

                    state.space.map_output(&output, (0, 0));
                    state.dtr = Some(dtr);
                    let position = (size.w as f64 / 2.0, size.h as f64 / 2.0).into();
                    state.pointer_location = position;
                    state.pointer_absolute_location = position;
                    state.video_info = Some(video_info.clone().into());
                    match render_target {
                        RenderTarget::Hardware(_) => match video_info {
                            GstVideoInfo::RAW(base_info) => {
                                let allocator = GsGlesbuffer::new(&mut state.renderer, base_info)
                                    .expect("Failed to create GsGlesbuffer");
                                state.output_buffer = Some(GsBufferType::RAW(allocator));
                            }
                            GstVideoInfo::DMA(base_info) => {
                                let allocator = GsDmaBuf::new(render_node.unwrap(), base_info)
                                    .expect("Failed to create GsDmaBuf");
                                state.output_buffer = Some(GsBufferType::DMA(allocator));
                            }
                            #[cfg(feature = "cuda")]
                            GstVideoInfo::CUDA(base_info) => {
                                let egl_display = state
                                    .renderer
                                    .egl_context()
                                    .display()
                                    .get_display_handle()
                                    .handle;
                                let allocator = GsCUDABuf::new(
                                    render_node.unwrap(),
                                    base_info.cuda_context,
                                    base_info.video_info,
                                    Arc::new(Mutex::new(None)),
                                    &egl_display,
                                )
                                .expect("Failed to create GsCUDABuf");
                                state.output_buffer = Some(GsBufferType::CUDA(allocator));
                            }
                        },
                        RenderTarget::Software => {
                            let allocator =
                                GsGlesbuffer::new(&mut state.renderer, base_info.clone())
                                    .expect("Failed to create GsGlesbuffer");
                            state.output_buffer = Some(GsBufferType::RAW(allocator));
                        }
                    }

                    let new_size = size
                        .to_f64()
                        .to_logical(output.current_scale().fractional_scale())
                        .to_i32_round();
                    for window in state.space.elements() {
                        let toplevel = window.toplevel().unwrap();
                        let max_size = Rectangle::from_size(
                            with_states(toplevel.wl_surface(), |states| {
                                states
                                    .data_map
                                    .get::<XdgToplevelSurfaceData>()
                                    .map(|_attrs| {
                                        states
                                            .cached_state
                                            .get::<SurfaceCachedState>()
                                            .current()
                                            .max_size
                                    })
                            })
                            .unwrap_or(new_size),
                        );

                        let new_size = max_size
                            .intersection(Rectangle::from_size(new_size))
                            .map(|rect| rect.size);
                        toplevel.with_pending_state(|state| state.size = new_size);
                        toplevel.send_configure();
                    }
                }
                Event::Msg(Command::InputDevice(path)) => {
                    tracing::info!(path, "Adding input device.");
                    state.input_context.path_add_device(&path);
                }
                Event::Msg(Command::Buffer(buffer_sender, tracer)) => {
                    let wait = if let Some(last_render) = state.last_render {
                        let base_info = state.video_info.as_ref().unwrap().clone();
                        let framerate = base_info.fps();
                        let duration = Duration::from_secs_f64(
                            framerate.denom() as f64 / framerate.numer() as f64,
                        );
                        let time_passed = Instant::now().duration_since(last_render);
                        if time_passed < duration {
                            Some(duration - time_passed)
                        } else {
                            None
                        }
                    } else {
                        None
                    };

                    let render = move |state: &mut State, now: Instant| {
                        let _span = match tracer {
                            Some(ref tracer) => Some(tracer.trace("render")),
                            None => None,
                        };
                        if let Err(_) = match state.create_frame() {
                            Ok((buf, render_result)) => {
                                render_result
                                    .sync
                                    .wait()
                                    .expect("Error during render_result.sync"); // we need to wait before giving a hardware buffer to gstreamer or we might not be done writing to it
                                let res = buffer_sender.send(Ok(buf));
                                let rendered_states = &render_result.states;
                                let rendered_damage = render_result.damage.is_some();

                                if let Some(output) = state.output.as_ref() {
                                    let mut output_presentation_feedback =
                                        OutputPresentationFeedback::new(output);
                                    for window in state.space.elements() {
                                        window.with_surfaces(|surface, states| {
                                            update_surface_primary_scanout_output(
                                                surface,
                                                output,
                                                states,
                                                rendered_states,
                                                |next_output, _, _, _| next_output,
                                            );
                                        });
                                        window.send_frame(
                                            output,
                                            state.clock.now(),
                                            Some(Duration::ZERO),
                                            |_, _| Some(output.clone()),
                                        );
                                        window.take_presentation_feedback(
                                            &mut output_presentation_feedback,
                                            surface_primary_scanout_output,
                                            |surface, _| {
                                                surface_presentation_feedback_flags_from_states(
                                                    surface,
                                                    rendered_states,
                                                )
                                            },
                                        );
                                    }
                                    if rendered_damage {
                                        output_presentation_feedback.presented(
                                            state.clock.now(),
                                            output
                                                .current_mode()
                                                .map(|mode| {
                                                    Refresh::fixed(Duration::from_secs_f64(
                                                        1_000f64 / mode.refresh as f64,
                                                    ))
                                                })
                                                .unwrap_or(Refresh::Unknown),
                                            0,
                                            wp_presentation_feedback::Kind::Vsync,
                                        );
                                    }
                                    if let CursorImageStatus::Surface(wl_surface) =
                                        &state.cursor_state
                                    {
                                        send_frames_surface_tree(
                                            wl_surface,
                                            output,
                                            state.clock.now(),
                                            None,
                                            |_, _| Some(output.clone()),
                                        )
                                    }
                                }

                                state.last_render = Some(now);
                                res
                            }
                            Err(err) => {
                                tracing::error!(?err, "Rendering failed.");
                                buffer_sender.send(Err(match err {
                                    DTRError::OutputNoMode(_) => unreachable!(),
                                    DTRError::Rendering(err) => err.into(),
                                }))
                            }
                        } {
                            state.should_quit = true;
                        }
                    };

                    match wait {
                        Some(duration) => {
                            if let Err(err) = state.handle.insert_source(
                                Timer::from_duration(duration),
                                move |now, _, data| {
                                    render(data, now);
                                    TimeoutAction::Drop
                                },
                            ) {
                                tracing::error!(?err, "Event loop error.");
                                state.should_quit = true;
                            };
                        }
                        None => render(state, Instant::now()),
                    };
                }
                #[cfg(feature = "cuda")]
                Event::Msg(Command::UpdateCUDABufferPool(pool)) => {
                    tracing::info!("Updating CUDA buffer pool");
                    if let Some(GsBufferType::CUDA(ref mut cuda_buf)) = state.output_buffer {
                        cuda_buf.buffer_pool = pool;
                    }
                }
                Event::Msg(Command::Quit) | Event::Closed => {
                    state.should_quit = true;
                }
                Event::Msg(Command::KeyboardInput(scancode, key_state)) => {
                    let time: Duration = state.clock.now().into();
                    let keycode = state.scancode_to_keycode(scancode);
                    state.keyboard_input(time.as_millis() as u32, keycode, key_state);
                }
                Event::Msg(Command::PointerMotion(position)) => {
                    let time: Duration = state.clock.now().into();
                    state.pointer_motion(
                        time.as_millis() as u32,
                        time.as_nanos() as u64,
                        position,
                        position,
                    );
                }
                Event::Msg(Command::PointerMotionAbsolute(position)) => {
                    let time: Duration = state.clock.now().into();
                    state.pointer_motion_absolute(time.as_millis() as u32, position);
                }
                Event::Msg(Command::PointerButton(btn_code, btn_state)) => {
                    let time: Duration = state.clock.now().into();
                    state.pointer_button(time.as_millis() as u32, btn_code, btn_state);
                }
                Event::Msg(Command::PointerAxis(horizontal_amount, vertical_amount)) => {
                    let time: Duration = state.clock.now().into();
                    state.pointer_axis(
                        time.as_millis() as u32,
                        AxisSource::Wheel,
                        horizontal_amount * 3.0 / 120.0,
                        vertical_amount * 3.0 / 120.0,
                        Some(horizontal_amount),
                        Some(vertical_amount),
                    );
                }
                Event::Msg(Command::GetSupportedDmaFormats(sender)) => {
                    let formats = Bind::<Dmabuf>::supported_formats(&state.renderer);
                    let supported_formats = match &state.output_buffer {
                        None => match state.render_node {
                            // If there's no output_buffer, we'll return all supported DMA formats
                            Some(node) => {
                                let gbm_dev =
                                    new_gbm_device(node).expect("Failed to create gbm device");
                                formats
                                    .unwrap_or_default()
                                    .iter()
                                    .filter(|f| {
                                        gbm_dev.is_format_supported(
                                            f.code,
                                            BufferObjectFlags::RENDERING,
                                        )
                                    })
                                    .map(|f| *f)
                                    .collect()
                            }
                            None => FormatSet::default(),
                        },
                        Some(output_buffer) => {
                            // If we already have negotiated an output buffer,
                            // that's the only format that we are going to support
                            match output_buffer.get_video_info() {
                                VideoInfoTypes::VideoInfo(_) => FormatSet::default(),
                                VideoInfoTypes::VideoInfoDmaDrm(video_info) => {
                                    let fourcc = gst_video_format_to_drm_fourcc(&video_info);
                                    let modifier = gst_video_format_to_drm_modifier(&video_info);
                                    let drm_format = DrmFormat {
                                        code: fourcc.expect(
                                            "Failed to convert gst_video_format to drm_fourcc",
                                        ),
                                        modifier: modifier.expect(
                                            "Failed to convert gst_video_format to drm_modifier",
                                        ),
                                    };
                                    FormatSet::from_iter([drm_format])
                                }
                            }
                        }
                    };
                    debug!("Supported dma formats: {:?}", supported_formats);
                    let _ = sender.send(supported_formats);
                }
                Event::Msg(Command::GetRenderDevice(sender)) => {
                    let render_device: Option<GPUDevice> = match &state.render_node {
                        Some(node) => {
                            let result = GPUDevice::try_from(*node);
                            match result {
                                Ok(device) => Some(device),
                                Err(err) => {
                                    tracing::warn!("Error during GetRenderDevice: {}", err);
                                    None
                                }
                            }
                        }
                        None => None,
                    };
                    debug!("Render device requested: {:?}", render_device);
                    if let Err(err) = sender.send(render_device) {
                        tracing::warn!(?err, "Failed to send render device.");
                    }
                }
                Event::Msg(Command::TouchDown(id, rel_position)) => {
                    let time: Duration = state.clock.now().into();
                    let logical_position = state
                        .relative_touch_to_logical(rel_position)
                        .expect("Failed to convert relative touch position to logical coordinates");
                    state.touch_down(
                        time.as_millis() as u32,
                        TouchSlot::from(Some(id)),
                        logical_position,
                    );
                }
                Event::Msg(Command::TouchUp(id)) => {
                    let time: Duration = state.clock.now().into();
                    state.touch_up(time.as_millis() as u32, TouchSlot::from(Some(id)));
                }
                Event::Msg(Command::TouchMotion(id, rel_position)) => {
                    let time: Duration = state.clock.now().into();
                    let logical_position = state
                        .relative_touch_to_logical(rel_position)
                        .expect("Failed to convert relative touch position to logical coordinates");
                    state.touch_motion(
                        time.as_millis() as u32,
                        TouchSlot::from(Some(id)),
                        logical_position,
                    );
                }
                Event::Msg(Command::TouchCancel) => {
                    state.touch_cancel();
                }
                Event::Msg(Command::TouchFrame) => {
                    state.touch_frame();
                }
            };
        })
        .unwrap();

    let source = ListeningSocketSource::new_auto().unwrap();
    let socket_name = source.socket_name().to_string_lossy().into_owned();
    tracing::info!(?socket_name, "Listening on wayland socket.");
    event_loop
        .handle()
        .insert_source(source, |client_stream, _, state| {
            if let Err(err) = state
                .dh
                .insert_client(client_stream, Arc::new(ClientState::default()))
            {
                tracing::error!(?err, "Error adding wayland client.");
            };
        })
        .expect("Failed to init wayland socket source");

    event_loop
        .handle()
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

    let env_vars = vec![CString::new(format!("WAYLAND_DISPLAY={}", socket_name)).unwrap()];
    if let Err(err) = envs_tx.send(env_vars) {
        tracing::warn!(?err, "Failed to post environment to application.");
    }

    let signal = event_loop.get_signal();
    if let Err(err) = event_loop.run(None, &mut state, |state| {
        state.dh.flush_clients().expect("Failed to flush clients");
        state.space.refresh();
        state.popups.cleanup();

        if state.should_quit {
            signal.stop();
        }
    }) {
        tracing::error!(?err, "Event loop broke.");
    }
}
