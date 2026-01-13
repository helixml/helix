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
    }

    // Rerun if build script changes
    println!("cargo:rerun-if-changed=build.rs");
}
