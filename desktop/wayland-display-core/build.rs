use pkg_config;

fn main() {
    // Check if the cuda feature is enabled
    #[cfg(feature = "cuda")]
    {
        // Link GStreamer CUDA library
        if let Err(e) = pkg_config::Config::new()
            .atleast_version("1.24")
            .probe("gstreamer-cuda-1.0")
        {
            eprintln!(
                "Warning: gstreamer-cuda-1.0 not found via pkg-config: {}",
                e
            );
        }
        // Note: CUDA driver functions (cuStreamSynchronize, cuMemcpy2DAsync) are loaded
        // dynamically at runtime via libloading, no link-time dependency on libcuda.so
    }

    // Rerun if build script changes
    println!("cargo:rerun-if-changed=build.rs");
}
