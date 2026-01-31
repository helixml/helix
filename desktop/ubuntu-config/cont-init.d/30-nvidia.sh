#!/bin/bash
# GOW init script: Configure NVIDIA GPU drivers

set -e

source /opt/gow/bash-lib/utils.sh

# Check if our custom volume is mounted
if [ -d /usr/nvidia ]; then
    gow_log "Nvidia driver volume detected"
    ldconfig

    if [ -d /usr/nvidia/share/vulkan/icd.d ]; then
        gow_log "[nvidia] Add Vulkan ICD"
        mkdir -p /usr/share/vulkan/icd.d/
        cp /usr/nvidia/share/vulkan/icd.d/* /usr/share/vulkan/icd.d/
    fi

    if [ -d /usr/nvidia/share/egl/egl_external_platform.d ]; then
        gow_log "[nvidia] Add EGL external platform"
        mkdir -p /usr/share/egl/egl_external_platform.d/
        cp /usr/nvidia/share/egl/egl_external_platform.d/* /usr/share/egl/egl_external_platform.d/
    fi

    if [ -d /usr/nvidia/share/glvnd/egl_vendor.d ]; then
        gow_log "[nvidia] Add egl-vendor"
        mkdir -p /usr/share/glvnd/egl_vendor.d/
        cp /usr/nvidia/share/glvnd/egl_vendor.d/* /usr/share/glvnd/egl_vendor.d/
    fi

    if [ -d /usr/nvidia/lib/gbm ]; then
        gow_log "[nvidia] Add gbm backend"
        mkdir -p /usr/lib/x86_64-linux-gnu/gbm/
        cp /usr/nvidia/lib/gbm/* /usr/lib/x86_64-linux-gnu/gbm/
    fi
# Check if there's libnvidia-allocator.so.1
elif [ -e /usr/lib/x86_64-linux-gnu/libnvidia-allocator.so.1 ]; then
    gow_log "Nvidia driver detected, assuming it's using the nvidia driver volume"
    ldconfig

    # Create a symlink to the nvidia-drm_gbm.so (if not present)
    if [ ! -e /usr/lib/x86_64-linux-gnu/gbm/nvidia-drm_gbm.so ]; then
        gow_log "Creating symlink to nvidia-drm_gbm.so"
        mkdir -p /usr/lib/x86_64-linux-gnu/gbm
        ln -sv ../libnvidia-allocator.so.1 /usr/lib/x86_64-linux-gnu/gbm/nvidia-drm_gbm.so
    fi

    # Create json config files
    if [ ! -f /usr/share/glvnd/egl_vendor.d/10_nvidia.json ]; then
        gow_log "Creating json 10_nvidia.json file"
        mkdir -p /usr/share/glvnd/egl_vendor.d/
        echo '{
            "file_format_version" : "1.0.0",
            "ICD": {
                "library_path": "libEGL_nvidia.so.0"
            }
        }' > /usr/share/glvnd/egl_vendor.d/10_nvidia.json
    fi

    if [ ! -f /usr/share/vulkan/icd.d/nvidia_icd.json ]; then
        gow_log "Creating json nvidia_icd.json file"
        mkdir -p /usr/share/vulkan/icd.d/
        echo '{
            "file_format_version" : "1.0.0",
            "ICD": {
                "library_path": "libGLX_nvidia.so.0",
                "api_version" : "1.3.242"
            }
        }' > /usr/share/vulkan/icd.d/nvidia_icd.json
    fi

    if [ ! -f /usr/share/egl/egl_external_platform.d/15_nvidia_gbm.json ]; then
        gow_log "Creating json 15_nvidia_gbm.json file"
        mkdir -p /usr/share/egl/egl_external_platform.d/
        echo '{
            "file_format_version" : "1.0.0",
            "ICD": {
                "library_path": "libnvidia-egl-gbm.so.1"
            }
        }' > /usr/share/egl/egl_external_platform.d/15_nvidia_gbm.json
    fi

    if [ ! -f /usr/share/egl/egl_external_platform.d/10_nvidia_wayland.json ]; then
        gow_log "Creating json 10_nvidia_wayland.json file"
        mkdir -p /usr/share/egl/egl_external_platform.d/
        echo '{
            "file_format_version" : "1.0.0",
            "ICD": {
                "library_path": "libnvidia-egl-wayland.so.1"
            }
        }' > /usr/share/egl/egl_external_platform.d/10_nvidia_wayland.json
    fi
fi
