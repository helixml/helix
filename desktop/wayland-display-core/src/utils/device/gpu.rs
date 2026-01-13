use crate::utils::device::PCIVendor;
use smithay::backend::drm::DrmNode;
use std::error::Error;
use std::fs;
use std::os::unix::fs::MetadataExt;
use std::path::PathBuf;

#[derive(Debug, Clone, PartialEq)]
pub struct GPUDevice {
    pci_vendor: PCIVendor,
    device_name: String,
}
impl GPUDevice {
    pub fn pci_vendor(&self) -> &PCIVendor {
        &self.pci_vendor
    }

    pub fn device_name(&self) -> &str {
        &self.device_name
    }
}
impl TryFrom<DrmNode> for GPUDevice {
    type Error = Box<dyn Error>;
    fn try_from(drm_node: DrmNode) -> Result<Self, Self::Error> {
        get_gpu_device(drm_node.dev_path().unwrap().to_str().unwrap())
    }
}
impl std::fmt::Display for GPUDevice {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "GPUDevice {{ pci_vendor: {}, device_name: {} }}",
            self.pci_vendor, self.device_name
        )
    }
}

pub fn get_gpu_device(path: &str) -> Result<GPUDevice, Box<dyn Error>> {
    let card = get_card_from_render_node(path)?;
    let vendor_str = fs::read_to_string(format!("/sys/class/drm/{}/device/vendor", card))?;
    let vendor_str = vendor_str.trim_start_matches("0x").trim_end_matches('\n');
    let vendor = u32::from_str_radix(&vendor_str, 16)?;

    let device_id = fs::read_to_string(format!("/sys/class/drm/{}/device/device", card))?;
    let device_id = device_id.trim_start_matches("0x").trim_end_matches('\n');

    // Look up in hwdata PCI database
    let device_name = match fs::read_to_string("/usr/share/hwdata/pci.ids") {
        Ok(pci_ids) => parse_pci_ids(&pci_ids, vendor_str, device_id).unwrap_or("".to_owned()),
        Err(e) => {
            tracing::warn!("Failed to read /usr/share/hwdata/pci.ids: {}", e);
            "".to_owned()
        }
    };

    Ok(GPUDevice {
        pci_vendor: PCIVendor::try_from(vendor)?,
        device_name,
    })
}

fn parse_pci_ids(pci_data: &str, vendor_id: &str, device_id: &str) -> Option<String> {
    let mut current_vendor = String::new();
    let vendor_id = vendor_id.to_lowercase();
    let device_id = device_id.to_lowercase();

    for line in pci_data.lines() {
        // Skip comments and empty lines
        if line.starts_with('#') || line.is_empty() {
            continue;
        }

        // Check for vendor lines (no leading whitespace)
        if !line.starts_with(['\t', ' ']) {
            let mut parts = line.splitn(2, ' ');
            if let (Some(vendor), Some(_)) = (parts.next(), parts.next()) {
                current_vendor = vendor.to_lowercase();
            }
            continue;
        }

        // Check for device lines (leading whitespace)
        let line = line.trim_start();
        let mut parts = line.splitn(2, ' ');
        if let (Some(dev_id), Some(desc)) = (parts.next(), parts.next()) {
            if dev_id.to_lowercase() == device_id && current_vendor == vendor_id {
                return Some(desc.trim().to_owned());
            }
        }
    }

    None
}

fn get_card_from_render_node(render_path: &str) -> std::io::Result<String> {
    // Get the device's sysfs path
    let metadata = fs::metadata(render_path)?;
    let rdev = metadata.rdev();
    let major = gnu_dev_major(rdev);
    let minor = gnu_dev_minor(rdev);

    // The sysfs path for the device
    let sys_path = format!("/sys/dev/char/{}:{}", major, minor);

    // Read the device symlink to get the actual device
    let device_link = PathBuf::from(&sys_path).join("device");
    let device_real = fs::canonicalize(device_link)?;

    // Now find card* entries under the same device
    let drm_path = device_real.join("drm");
    for entry in fs::read_dir(drm_path)? {
        let entry = entry?;
        let name = entry.file_name().to_string_lossy().to_string();
        if name.starts_with("card") && !name.contains('-') {
            return Ok(name);
        }
    }

    Err(std::io::Error::new(
        std::io::ErrorKind::NotFound,
        "No card found",
    ))
}

fn gnu_dev_major(dev: u64) -> u32 {
    let mut major = 0;
    major |= ((dev >> 8) & 0xfff) as u32;
    major |= ((dev >> 32) & 0xfffff000) as u32;
    major
}

fn gnu_dev_minor(dev: u64) -> u32 {
    let mut minor = 0;
    minor |= (dev & 0xff) as u32;
    minor |= ((dev >> 12) & 0xffffff00) as u32;
    minor
}
