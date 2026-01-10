pub mod allocator;
pub mod device;
pub mod renderer;
pub mod video_info;

mod target;

pub use self::target::*;

pub mod tests {
    use std::sync::Once;
    pub static INIT: Once = Once::new();

    #[cfg(test)]
    pub fn test_init() -> () {
        INIT.call_once(|| {
            tracing_subscriber::fmt::try_init().ok();
            gst::init().expect("Failed to initialize GStreamer");
        });
    }
}
