use smithay::backend::SwapBuffersError;
use smithay::backend::drm::CreateDrmNodeError;
pub use smithay::reexports::calloop::channel::{Channel, Sender, channel};

#[cfg(feature = "cuda")]
use crate::utils::allocator::cuda::CUDABufferPool;
use crate::utils::device::gpu::GPUDevice;
pub use smithay::backend::allocator::{
    Format as DrmFormat, Fourcc, Modifier as DrmModifier, Vendor as DrmVendor, format::FormatSet,
};
pub use smithay::backend::input::{ButtonState, KeyState};
use smithay::utils::{Logical, Point};
use std::ffi::{CString, c_char, c_void};
use std::str::FromStr;
use std::sync::mpsc::{self, Receiver, SyncSender};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;
use utils::RenderTarget;

pub(crate) mod comp;
#[cfg(test)]
mod tests;
pub mod utils;
pub(crate) mod wayland;

pub use crate::utils::video_info::GstVideoInfo;

pub enum Command {
    InputDevice(String),
    VideoInfo(GstVideoInfo),
    Buffer(
        SyncSender<Result<gst::Buffer, SwapBuffersError>>,
        Option<Tracer>,
    ),
    #[cfg(feature = "cuda")]
    UpdateCUDABufferPool(Arc<Mutex<Option<CUDABufferPool>>>),
    KeyboardInput(u32, KeyState),
    PointerMotion(Point<f64, Logical>),
    PointerMotionAbsolute(Point<f64, Logical>),
    PointerButton(u32, ButtonState),
    PointerAxis(f64, f64),
    GetSupportedDmaFormats(SyncSender<FormatSet>),
    GetRenderDevice(SyncSender<Option<GPUDevice>>),
    TouchDown(u32, Point<f64, Logical>),
    TouchUp(u32),
    TouchMotion(u32, Point<f64, Logical>),
    TouchCancel,
    TouchFrame,
    Quit,
}

#[derive(Clone)]
pub struct Tracer {
    start_fn: extern "C" fn(*const c_char) -> *mut c_void,
    end_fn: extern "C" fn(*mut c_void),
}

pub struct Trace {
    ctx: *mut c_void,
    end_fn: extern "C" fn(*mut c_void),
}

impl Tracer {
    pub fn new(
        start_fn: extern "C" fn(*const c_char) -> *mut c_void,
        end_fn: extern "C" fn(*mut c_void),
    ) -> Self {
        Tracer { start_fn, end_fn }
    }

    pub fn trace(&self, name: &str) -> Trace {
        let trace_name = CString::new(name).unwrap();
        let ctx = (self.start_fn)(trace_name.as_ptr());
        Trace::new(ctx, self.end_fn)
    }
}

impl Trace {
    pub fn new(ctx: *mut c_void, end_fn: extern "C" fn(*mut c_void)) -> Self {
        Trace { ctx, end_fn }
    }
}

impl Drop for Trace {
    fn drop(&mut self) {
        (self.end_fn)(self.ctx);
    }
}

pub struct WaylandDisplay {
    thread_handle: Option<JoinHandle<()>>,
    command_tx: Sender<Command>,

    pub tracer: Option<Tracer>,
    pub devices: MaybeRecv<Vec<CString>>,
    pub envs: MaybeRecv<Vec<CString>>,
}

pub enum MaybeRecv<T: Clone> {
    Rx(Receiver<T>),
    Value(T),
}

impl<T: Clone> MaybeRecv<T> {
    pub fn get(&mut self) -> &T {
        match self {
            MaybeRecv::Rx(recv) => {
                let value = recv.recv().unwrap();
                *self = MaybeRecv::Value(value.clone());
                self.get()
            }
            MaybeRecv::Value(val) => val,
        }
    }
}

impl WaylandDisplay {
    pub fn new(render_node: Option<String>) -> Result<WaylandDisplay, CreateDrmNodeError> {
        let (channel_tx, channel_rx) = std::sync::mpsc::sync_channel(0);
        let (devices_tx, devices_rx) = std::sync::mpsc::channel();
        let (envs_tx, envs_rx) = std::sync::mpsc::channel();
        let render_target = RenderTarget::from_str(
            &render_node.unwrap_or_else(|| String::from("/dev/dri/renderD128")),
        )?;

        let thread_handle = std::thread::spawn(move || {
            if let Err(err) = std::panic::catch_unwind(|| {
                // calloops channel is not "UnwindSafe", but the std channel is... *sigh* lets workaround it creatively
                let (command_tx, command_src) = smithay::reexports::calloop::channel::channel();
                channel_tx.send(command_tx).unwrap();
                comp::init(command_src, render_target, devices_tx, envs_tx);
            }) {
                tracing::error!(?err, "Compositor thread panic'ed!");
            }
        });
        let command_tx = channel_rx.recv().unwrap();

        Ok(WaylandDisplay {
            thread_handle: Some(thread_handle),
            command_tx,
            tracer: None,
            devices: MaybeRecv::Rx(devices_rx),
            envs: MaybeRecv::Rx(envs_rx),
        })
    }

    pub fn new_with_channel(
        render_node: Option<String>,
        command_tx: Sender<Command>,
        commands_rx: Channel<Command>,
    ) -> Result<WaylandDisplay, CreateDrmNodeError> {
        let (devices_tx, devices_rx) = std::sync::mpsc::channel();
        let (envs_tx, envs_rx) = std::sync::mpsc::channel();
        let render_target = RenderTarget::from_str(
            &render_node.unwrap_or_else(|| String::from("/dev/dri/renderD128")),
        )?;

        let thread_handle = std::thread::spawn(move || {
            comp::init(commands_rx, render_target, devices_tx, envs_tx);
        });

        Ok(WaylandDisplay {
            thread_handle: Some(thread_handle),
            command_tx,
            tracer: None,
            devices: MaybeRecv::Rx(devices_rx),
            envs: MaybeRecv::Rx(envs_rx),
        })
    }

    pub fn devices(&mut self) -> impl Iterator<Item = &str> {
        self.devices
            .get()
            .iter()
            .map(|string| string.to_str().unwrap())
    }

    pub fn env_vars(&mut self) -> impl Iterator<Item = &str> {
        self.envs
            .get()
            .iter()
            .map(|string| string.to_str().unwrap())
    }

    pub fn add_input_device(&self, path: impl Into<String>) {
        let _ = self.command_tx.send(Command::InputDevice(path.into()));
    }

    pub fn set_video_info(&self, info: GstVideoInfo) {
        let _ = self.command_tx.send(Command::VideoInfo(info));
    }

    pub fn keyboard_input(&self, key: u32, pressed: bool) {
        let state = if pressed {
            KeyState::Pressed
        } else {
            KeyState::Released
        };
        let _ = self.command_tx.send(Command::KeyboardInput(key, state));
    }

    pub fn pointer_motion(&self, x: f64, y: f64) {
        let _ = self.command_tx.send(Command::PointerMotion((x, y).into()));
    }

    pub fn pointer_motion_absolute(&self, x: f64, y: f64) {
        let _ = self
            .command_tx
            .send(Command::PointerMotionAbsolute((x, y).into()));
    }

    pub fn pointer_button(&self, button: u32, pressed: bool) {
        let state = if pressed {
            ButtonState::Pressed
        } else {
            ButtonState::Released
        };
        let _ = self.command_tx.send(Command::PointerButton(button, state));
    }

    pub fn pointer_axis(&self, x: f64, y: f64) {
        let _ = self.command_tx.send(Command::PointerAxis(x, y));
    }

    pub fn touch_down(&self, id: u32, rel_x: f64, rel_y: f64) {
        let _ = self
            .command_tx
            .send(Command::TouchDown(id, (rel_x, rel_y).into()));
    }

    pub fn touch_up(&self, id: u32) {
        let _ = self.command_tx.send(Command::TouchUp(id));
    }

    pub fn touch_motion(&self, id: u32, rel_x: f64, rel_y: f64) {
        let _ = self
            .command_tx
            .send(Command::TouchMotion(id, (rel_x, rel_y).into()));
    }

    pub fn touch_cancel(&self) {
        let _ = self.command_tx.send(Command::TouchCancel);
    }

    pub fn touch_frame(&self) {
        let _ = self.command_tx.send(Command::TouchFrame);
    }

    pub fn frame(&self) -> Result<gst::Buffer, gst::FlowError> {
        let (buffer_tx, buffer_rx) = mpsc::sync_channel(0);
        if let Err(err) = self
            .command_tx
            .send(Command::Buffer(buffer_tx, self.tracer.clone()))
        {
            tracing::warn!(?err, "Failed to send buffer command.");
            return Err(gst::FlowError::Eos);
        }

        match buffer_rx.recv() {
            Ok(Ok(buffer)) => Ok(buffer),
            Ok(Err(err)) => match err {
                SwapBuffersError::AlreadySwapped => unreachable!(),
                SwapBuffersError::ContextLost(_) => Err(gst::FlowError::Eos),
                SwapBuffersError::TemporaryFailure(_) => Err(gst::FlowError::Error),
            },
            Err(err) => {
                tracing::warn!(?err, "Failed to recv buffer ack.");
                Err(gst::FlowError::Error)
            }
        }
    }

    pub fn get_supported_dma_formats(&self) -> FormatSet {
        let (buffer_tx, buffer_rx) = mpsc::sync_channel(0);
        let _ = self
            .command_tx
            .send(Command::GetSupportedDmaFormats(buffer_tx));
        buffer_rx.recv().unwrap()
    }

    pub fn get_render_device(&self) -> Option<GPUDevice> {
        let (buffer_tx, buffer_rx) = mpsc::sync_channel(0);
        let _ = self.command_tx.send(Command::GetRenderDevice(buffer_tx));
        buffer_rx.recv().unwrap()
    }
}

impl Drop for WaylandDisplay {
    fn drop(&mut self) {
        if let Err(err) = self.command_tx.send(Command::Quit) {
            tracing::warn!("Failed to send stop command: {}", err);
            return;
        };
        if self.thread_handle.take().unwrap().join().is_err() {
            tracing::warn!("Failed to join compositor thread");
        };
    }
}
