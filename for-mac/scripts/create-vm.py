#!/usr/bin/env python3
"""Create a UTM 5.0 VM configuration for Helix Ubuntu with Venus support."""

import os
import plistlib
import shutil
import subprocess
import uuid

VM_NAME = "helix-ubuntu"
ISO_PATH = os.path.expanduser("~/Library/Application Support/Helix/iso/ubuntu-25.10-desktop-arm64.iso")
UTM_DOCUMENTS = os.path.expanduser("~/Library/Containers/com.utmapp.UTM/Data/Documents")

def create_vm(vm_name=VM_NAME, iso_path=ISO_PATH, memory_mb=8192, cpu_count=4, disk_gb=64):
    """Create a UTM 5.0 compatible VM configuration."""

    vm_path = os.path.join(UTM_DOCUMENTS, f"{vm_name}.utm")

    if os.path.exists(vm_path):
        print(f"VM already exists at {vm_path}")
        response = input("Delete and recreate? [y/N] ")
        if response.lower() != 'y':
            return None
        shutil.rmtree(vm_path)

    os.makedirs(vm_path)
    os.makedirs(os.path.join(vm_path, "Data"))
    os.makedirs(os.path.join(vm_path, "Images"))

    # Generate UUIDs
    vm_uuid = str(uuid.uuid4()).upper()
    cd_uuid = str(uuid.uuid4()).upper()
    disk_uuid = str(uuid.uuid4()).upper()

    # Create disk image
    disk_filename = f"{disk_uuid}.qcow2"
    disk_path = os.path.join(vm_path, "Images", disk_filename)

    qemu_img = "/Applications/UTM.app/Contents/Resources/qemu/qemu-img"
    if not os.path.exists(qemu_img):
        qemu_img = "qemu-img"

    result = subprocess.run([qemu_img, "create", "-f", "qcow2", disk_path, f"{disk_gb}G"],
                           capture_output=True, text=True)
    if result.returncode != 0:
        print(f"Warning: Failed to create disk image: {result.stderr}")

    # UTM 5.0 config format
    config = {
        "Backend": "QEMU",
        "ConfigurationVersion": 4,
        "Display": [
            {
                "DownscalingFilter": "Linear",
                "DynamicResolution": True,
                "Hardware": "virtio-gpu-gl-pci",  # OpenGL/Venus acceleration
                "NativeResolution": False,
                "UpscalingFilter": "Nearest",
            }
        ],
        "Drive": [
            {
                "Identifier": cd_uuid,
                "ImageType": "CD",
                "Interface": "USB",
                "InterfaceVersion": 1,
                "ReadOnly": True,
            },
            {
                "Identifier": disk_uuid,
                "ImageName": disk_filename,
                "ImageType": "Disk",
                "Interface": "VirtIO",
                "InterfaceVersion": 1,
                "ReadOnly": False,
            }
        ],
        "Information": {
            "Icon": "linux",
            "IconCustom": False,
            "Name": vm_name,
            "UUID": vm_uuid,
        },
        "Input": {
            "MaximumUsbShare": 3,
            "UsbBusSupport": "3.0",
            "UsbSharing": False,
        },
        "Network": [
            {
                "Hardware": "virtio-net-pci",
                "IsolateFromHost": False,
                "MacAddress": f"52:42:{uuid.uuid4().hex[:2].upper()}:{uuid.uuid4().hex[:2].upper()}:{uuid.uuid4().hex[:2].upper()}:{uuid.uuid4().hex[:2].upper()}",
                "Mode": "Shared",
                "PortForward": [],
            }
        ],
        "QEMU": {
            "AdditionalArguments": [],
            "BalloonDevice": False,
            "DebugLog": False,
            "Hypervisor": True,  # Use Apple HVF
            "PS2Controller": False,
            "RNGDevice": True,
            "RTCLocalTime": False,
            "TPMDevice": False,
            "TSO": False,
            "UEFIBoot": True,
        },
        "Serial": [],
        "Sharing": {
            "ClipboardSharing": True,
            "DirectoryShareMode": "VirtFS",
            "DirectoryShareReadOnly": False,
        },
        "Sound": [
            {
                "Hardware": "intel-hda",
            }
        ],
        "System": {
            "Architecture": "aarch64",
            "CPU": "default",
            "CPUCount": cpu_count,
            "CPUFlagsAdd": [],
            "CPUFlagsRemove": [],
            "ForceMulticore": False,
            "JITCacheSize": 0,
            "MemorySize": memory_mb,
            "Target": "virt",
        },
    }

    # Write config
    config_path = os.path.join(vm_path, "config.plist")
    with open(config_path, "wb") as f:
        plistlib.dump(config, f)

    # Create symlink to ISO in Data directory (UTM expects this)
    if iso_path and os.path.exists(iso_path):
        iso_link = os.path.join(vm_path, "Data", os.path.basename(iso_path))
        if not os.path.exists(iso_link):
            os.symlink(iso_path, iso_link)

    print(f"VM created at: {vm_path}")
    print(f"  UUID: {vm_uuid}")
    print(f"  Memory: {memory_mb} MB")
    print(f"  CPUs: {cpu_count}")
    print(f"  Disk: {disk_gb} GB")
    print(f"  GPU: virtio-gpu-gl-pci (Venus/OpenGL)")
    print()
    print("Restart UTM to see the new VM.")

    return vm_path

if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description="Create a UTM VM for Helix")
    parser.add_argument("--name", default=VM_NAME, help="VM name")
    parser.add_argument("--iso", default=ISO_PATH, help="Path to ISO")
    parser.add_argument("--memory", type=int, default=8192, help="Memory in MB")
    parser.add_argument("--cpus", type=int, default=4, help="Number of CPUs")
    parser.add_argument("--disk", type=int, default=64, help="Disk size in GB")
    args = parser.parse_args()

    create_vm(args.name, args.iso, args.memory, args.cpus, args.disk)
