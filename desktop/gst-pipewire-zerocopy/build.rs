fn main() {
    gst_plugin_version_helper::info();

    // Link against GStreamer CUDA library for gst_cuda_stream_new and other CUDA functions
    // The waylanddisplaycore crate uses these functions via FFI bindings
    println!("cargo:rustc-link-lib=gstcuda-1.0");
}
