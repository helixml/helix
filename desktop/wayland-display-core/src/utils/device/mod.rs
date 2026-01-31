pub mod gpu;

#[derive(Debug, Clone, Eq, PartialEq, Hash)]
pub enum PCIVendor {
    Unknown = 0x0000,
    Intel = 0x8086,
    NVIDIA = 0x10de,
    AMD = 0x1002,
}
impl PCIVendor {
    pub fn as_str(&self) -> &'static str {
        match self {
            PCIVendor::Intel => "Intel",
            PCIVendor::NVIDIA => "NVIDIA",
            PCIVendor::AMD => "AMD",
            PCIVendor::Unknown => "Unknown",
        }
    }
}
impl From<u32> for PCIVendor {
    fn from(vendor_id: u32) -> Self {
        match vendor_id {
            0x8086 => PCIVendor::Intel,
            0x10de => PCIVendor::NVIDIA,
            0x1002 => PCIVendor::AMD,
            _ => PCIVendor::Unknown,
        }
    }
}
impl std::fmt::Display for PCIVendor {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.as_str())
    }
}
