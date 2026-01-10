//! PipeWire ScreenCast source element with zero-copy CUDA output
//!
//! This module provides the `pipewirezerocopysrc` GStreamer element that:
//! 1. Connects to PipeWire ScreenCast to receive DMA-BUF frames
//! 2. Converts DMA-BUFs to CUDA buffers using EGL interop
//! 3. Outputs video/x-raw(memory:CUDAMemory) for hardware encoding

use gst::glib;
use gst::prelude::*;

mod imp;

glib::wrapper! {
    pub struct PipeWireZeroCopySrc(ObjectSubclass<imp::PipeWireZeroCopySrc>) @extends gst_base::PushSrc, gst_base::BaseSrc, gst::Element, gst::Object;
}

pub fn register(plugin: &gst::Plugin) -> Result<(), glib::BoolError> {
    gst::Element::register(
        Some(plugin),
        "pipewirezerocopysrc",
        gst::Rank::NONE,
        PipeWireZeroCopySrc::static_type(),
    )
}
