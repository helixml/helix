//! PipeWire stream consumer
//!
//! Connects to a PipeWire screen-cast stream and receives frames
//! as either DMA-BUF (zero-copy) or SHM (fallback).

use anyhow::{Context, Result};
use pipewire::{
    prelude::*,
    spa::{
        self,
        param::video::{VideoFormat, VideoInfoRaw},
        pod::Pod,
        utils::Direction,
    },
    stream::{Stream, StreamFlags, StreamState},
    Context as PwContext, MainLoop,
};
use std::cell::RefCell;
use std::os::unix::io::RawFd;
use std::rc::Rc;
use std::sync::mpsc::{self, Receiver, Sender};
use tracing::{debug, info, warn};

/// Frame received from PipeWire
pub enum Frame {
    /// DMA-BUF frame (zero-copy GPU buffer)
    DmaBuf {
        fd: RawFd,
        width: u32,
        height: u32,
        stride: u32,
        format: u32,
        modifier: u64,
    },
    /// SHM frame (CPU memory, requires copy)
    Shm {
        data: Vec<u8>,
        width: u32,
        height: u32,
        stride: u32,
        format: u32,
    },
}

/// PipeWire stream wrapper
pub struct PipeWireStream {
    main_loop: MainLoop,
    frame_rx: Receiver<Frame>,
    _stream: Stream,
}

struct StreamState {
    frame_tx: Sender<Frame>,
    width: u32,
    height: u32,
    format: u32,
    stride: u32,
    modifier: u64,
}

impl PipeWireStream {
    /// Connect to a PipeWire screen-cast stream by node ID
    pub fn connect(node_id: u32, width: u32, height: u32) -> Result<Self> {
        pipewire::init();

        let main_loop = MainLoop::new(None).context("Failed to create PipeWire main loop")?;
        let context = PwContext::new(&main_loop).context("Failed to create PipeWire context")?;
        let core = context.connect(None).context("Failed to connect to PipeWire")?;

        let (frame_tx, frame_rx) = mpsc::channel();

        let state = Rc::new(RefCell::new(StreamState {
            frame_tx,
            width,
            height,
            format: spa::param::video::VideoFormat::BGRx as u32,
            stride: width * 4,
            modifier: 0,
        }));

        // Create stream
        let props = pipewire::properties! {
            *pipewire::keys::MEDIA_TYPE => "Video",
            *pipewire::keys::MEDIA_CATEGORY => "Capture",
            *pipewire::keys::MEDIA_ROLE => "Screen",
        };

        let stream = Stream::new(&core, "wolf-bridge", props)
            .context("Failed to create PipeWire stream")?;

        // Set up stream event handlers
        let state_clone = state.clone();
        let _listener = stream
            .add_local_listener_with_user_data(state_clone)
            .state_changed(|stream, state_data, old, new| {
                debug!("Stream state: {:?} -> {:?}", old, new);
                if matches!(new, StreamState::Streaming) {
                    info!("PipeWire stream now streaming");
                }
            })
            .param_changed(|stream, state_data, id, param| {
                if id != spa::param::ParamType::Format.as_raw() {
                    return;
                }

                if let Some(param) = param {
                    // Parse video format
                    if let Ok(info) = VideoInfoRaw::parse(param) {
                        let mut state = state_data.borrow_mut();
                        state.width = info.size().width;
                        state.height = info.size().height;
                        state.format = spa_video_format_to_drm(info.format());
                        state.stride = state.width * 4; // Assume 4 bytes per pixel

                        if let Some(modifier) = info.modifier() {
                            state.modifier = modifier;
                        }

                        info!(
                            "Stream format: {}x{}, format={:#x}, modifier={:#x}",
                            state.width, state.height, state.format, state.modifier
                        );
                    }
                }
            })
            .process(|stream, state_data| {
                process_frame(stream, state_data);
            })
            .register()
            .context("Failed to register stream listener")?;

        // Build format params
        let params = build_format_params(width, height);
        let params_ref: Vec<&Pod> = params.iter().collect();

        // Connect to the stream
        stream
            .connect(
                Direction::Input,
                Some(node_id),
                StreamFlags::AUTOCONNECT | StreamFlags::MAP_BUFFERS,
                &mut params_ref.into_iter(),
            )
            .context("Failed to connect to PipeWire stream")?;

        info!("Connecting to PipeWire node {}...", node_id);

        Ok(Self {
            main_loop,
            frame_rx,
            _stream: stream,
        })
    }

    /// Try to get the next frame (non-blocking)
    pub fn try_dequeue_frame(&mut self) -> Result<Option<Frame>> {
        // Process PipeWire events
        self.main_loop.iterate(std::time::Duration::from_millis(1));

        // Check for frames
        match self.frame_rx.try_recv() {
            Ok(frame) => Ok(Some(frame)),
            Err(mpsc::TryRecvError::Empty) => Ok(None),
            Err(mpsc::TryRecvError::Disconnected) => {
                anyhow::bail!("PipeWire stream disconnected")
            }
        }
    }
}

fn process_frame(stream: &Stream, state_data: &Rc<RefCell<StreamState>>) {
    let mut buffer = match stream.dequeue_buffer() {
        Some(b) => b,
        None => return,
    };

    let state = state_data.borrow();
    let datas = buffer.datas_mut();

    if datas.is_empty() {
        return;
    }

    let data = &datas[0];

    // Check for DMA-BUF
    if data.type_() == spa::buffer::DataType::DmaBuf {
        if let Some(fd) = data.as_raw().fd {
            let stride = data.chunk().stride() as u32;
            let stride = if stride == 0 { state.stride } else { stride };

            let frame = Frame::DmaBuf {
                fd: fd as RawFd,
                width: state.width,
                height: state.height,
                stride,
                format: state.format,
                modifier: state.modifier,
            };

            if state.frame_tx.send(frame).is_err() {
                warn!("Failed to send frame");
            }
        }
    } else if let Some(ptr) = data.data() {
        // SHM buffer
        let chunk = data.chunk();
        let offset = chunk.offset() as usize;
        let stride = chunk.stride() as u32;
        let stride = if stride == 0 { state.stride } else { stride };
        let size = (stride * state.height) as usize;

        if ptr.len() >= offset + size {
            let frame = Frame::Shm {
                data: ptr[offset..offset + size].to_vec(),
                width: state.width,
                height: state.height,
                stride,
                format: state.format,
            };

            if state.frame_tx.send(frame).is_err() {
                warn!("Failed to send frame");
            }
        }
    }
}

fn build_format_params(width: u32, height: u32) -> Vec<Vec<u8>> {
    // Build SPA pod for format negotiation
    // This is simplified - in production you'd use spa-pod-builder properly
    vec![]
}

/// Convert SPA video format to DRM fourcc
fn spa_video_format_to_drm(format: VideoFormat) -> u32 {
    match format {
        VideoFormat::BGRx | VideoFormat::BGRA => 0x34325241, // DRM_FORMAT_ARGB8888
        VideoFormat::RGBx | VideoFormat::RGBA => 0x34324241, // DRM_FORMAT_ABGR8888
        VideoFormat::xRGB | VideoFormat::ARGB => 0x34325842, // DRM_FORMAT_BGRA8888
        VideoFormat::xBGR | VideoFormat::ABGR => 0x34324152, // DRM_FORMAT_RGBA8888
        VideoFormat::RGB => 0x34324752, // DRM_FORMAT_RGB888
        VideoFormat::BGR => 0x34324742, // DRM_FORMAT_BGR888
        _ => 0x34325258, // DRM_FORMAT_XRGB8888 (fallback)
    }
}
