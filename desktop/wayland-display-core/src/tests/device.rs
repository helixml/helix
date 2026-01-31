use crate::utils::device::PCIVendor;
use crate::utils::device::gpu::get_gpu_device;
use test_log::test;

#[test]
fn test_get_gpu_device() {
    for path in vec![
        "/dev/dri/card0",
        "/dev/dri/renderD128",
        "/dev/dri/by-path/pci-0000:2d:00.0-render",
        "/dev/dri/card1",
        "/dev/dri/renderD129",
        "/dev/dri/by-path/pci-0000:04:00.0-render",
    ] {
        match get_gpu_device(path) {
            Ok(device) => {
                tracing::info!("Found GPU: {}", device);
                assert!(!device.device_name().is_empty(), "Device name is empty");
                assert_ne!(
                    *device.pci_vendor(),
                    PCIVendor::Unknown,
                    "Unknown PCI vendor"
                );
            }
            Err(e) => {
                tracing::error!("Failed to get GPU device for path {}: {}", path, e);
                continue;
            }
        };
    }
}
