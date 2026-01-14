#!/bin/bash
# Detect and configure the correct GPU render node based on GPU_VENDOR
#
# On multi-GPU systems (like Lambda Labs), there may be multiple render nodes:
# - /dev/dri/renderD128 = virtio-gpu (virtual, useless for encoding)
# - /dev/dri/renderD129 = actual NVIDIA/AMD GPU
#
# This script finds the render node that matches GPU_VENDOR by checking
# the driver symlink in /sys/class/drm/.
#
# Exports:
#   HELIX_RENDER_NODE  - The detected render node path (e.g., /dev/dri/renderD129)
#   HELIX_DRM_CARD     - The corresponding card device (e.g., /dev/dri/card1)
#   WLR_DRM_DEVICES    - For Sway: colon-separated list of DRM devices to use
#   LIBVA_DRIVER_NAME  - VA-API driver name for AMD/Intel (radeonsi, iHD)
#
# Usage: source /usr/local/bin/detect-render-node.sh

detect_render_node() {
    local gpu_vendor="${GPU_VENDOR:-}"
    local target_driver=""
    local detected_node=""
    local detected_card=""

    # Determine which kernel driver to look for based on GPU_VENDOR
    case "$gpu_vendor" in
        nvidia)
            target_driver="nvidia"
            ;;
        amd)
            target_driver="amdgpu"
            ;;
        intel)
            target_driver="i915"
            ;;
        none|"")
            echo "[render-node] Software rendering mode (GPU_VENDOR=${gpu_vendor:-unset})"
            export HELIX_RENDER_NODE="SOFTWARE"
            export LIBGL_ALWAYS_SOFTWARE=1
            export MESA_GL_VERSION_OVERRIDE=4.5
            return 0
            ;;
        *)
            echo "[render-node] WARNING: Unknown GPU_VENDOR: $gpu_vendor, defaulting to software rendering"
            export HELIX_RENDER_NODE="SOFTWARE"
            export LIBGL_ALWAYS_SOFTWARE=1
            export MESA_GL_VERSION_OVERRIDE=4.5
            return 0
            ;;
    esac

    # Find render node AND card device matching the target driver
    # renderD* = render-only node (for GPU compute/encoding)
    # card* = full DRM node (for display output, required by compositors)
    if [ -d "/sys/class/drm" ]; then
        for render_node in /dev/dri/renderD*; do
            if [ -e "$render_node" ]; then
                node_name=$(basename "$render_node")
                driver_link="/sys/class/drm/$node_name/device/driver"

                if [ -L "$driver_link" ]; then
                    driver=$(readlink "$driver_link" | grep -o '[^/]*$')
                    if [ "$driver" = "$target_driver" ]; then
                        detected_node="$render_node"
                        echo "[render-node] Found $gpu_vendor GPU at $render_node (driver: $driver)"

                        # Find corresponding card device (same PCI device)
                        # renderD128 → card0, renderD129 → card1, etc. (but this varies)
                        # Safer: check all cardN devices for same PCI device
                        pci_path=$(readlink -f "/sys/class/drm/$node_name/device")
                        for card in /dev/dri/card*; do
                            if [ -e "$card" ]; then
                                card_name=$(basename "$card")
                                card_pci=$(readlink -f "/sys/class/drm/$card_name/device")
                                if [ "$pci_path" = "$card_pci" ]; then
                                    detected_card="$card"
                                    echo "[render-node] Found corresponding card device: $card"
                                    break
                                fi
                            fi
                        done
                        break
                    fi
                fi
            fi
        done
    fi

    # Fallback: auto-detect best available GPU if specified driver not found
    # This handles cases where GPU_VENDOR doesn't match reality (e.g., nvidia-smi
    # exists but no NVIDIA GPU available, or multi-GPU system with wrong vendor set)
    if [ -z "$detected_node" ]; then
        echo "[render-node] WARNING: Could not find $target_driver driver, auto-detecting best available GPU..."

        # Priority order: nvidia > amdgpu > i915 > first available
        local priority_drivers="nvidia amdgpu i915"
        for try_driver in $priority_drivers; do
            for render_node in /dev/dri/renderD*; do
                if [ -e "$render_node" ]; then
                    node_name=$(basename "$render_node")
                    driver_link="/sys/class/drm/$node_name/device/driver"
                    if [ -L "$driver_link" ]; then
                        driver=$(readlink "$driver_link" | grep -o '[^/]*$')
                        if [ "$driver" = "$try_driver" ]; then
                            detected_node="$render_node"
                            echo "[render-node] Auto-detected: $render_node (driver: $driver)"

                            # Find corresponding card device
                            pci_path=$(readlink -f "/sys/class/drm/$node_name/device")
                            for card in /dev/dri/card*; do
                                if [ -e "$card" ]; then
                                    card_name=$(basename "$card")
                                    card_pci=$(readlink -f "/sys/class/drm/$card_name/device")
                                    if [ "$pci_path" = "$card_pci" ]; then
                                        detected_card="$card"
                                        echo "[render-node] Auto-detected card: $card"
                                        break
                                    fi
                                fi
                            done

                            # Update gpu_vendor to match detected driver for VA-API config
                            case "$driver" in
                                nvidia) gpu_vendor="nvidia" ;;
                                amdgpu) gpu_vendor="amd" ;;
                                i915)   gpu_vendor="intel" ;;
                            esac
                            break 2  # Exit both loops
                        fi
                    fi
                fi
            done
        done

        # If still nothing found, try first available render node
        if [ -z "$detected_node" ] && [ -e "/dev/dri/renderD128" ]; then
            detected_node="/dev/dri/renderD128"
            detected_card="/dev/dri/card0"
            echo "[render-node] Fallback: using $detected_node (unknown driver)"
        fi

        if [ -z "$detected_node" ]; then
            # No render nodes found - this is OK for NVIDIA (uses NVENC via CUDA, not VA-API)
            echo "[render-node] WARNING: No render nodes found in /dev/dri/ (OK for NVIDIA NVENC)"
            return 0
        fi
    fi

    export HELIX_RENDER_NODE="$detected_node"

    # Set card device for compositors
    if [ -n "$detected_card" ]; then
        export HELIX_DRM_CARD="$detected_card"

        # WLR_DRM_DEVICES for Sway: first device is rendering GPU
        # Format: /dev/dri/card1:/dev/dri/card0 means render on card1, display fallback to card0
        # For single-GPU or when we want exclusive use, just specify the one card
        export WLR_DRM_DEVICES="$detected_card"
        echo "[render-node] Set WLR_DRM_DEVICES=$detected_card for Sway"

        # Configure Mutter GPU selection by writing directly to udev database
        # Mutter uses the mutter-device-preferred-primary tag to select primary GPU
        # See: https://github.com/GNOME/mutter/blob/main/doc/multi-gpu.md
        #
        # In containers without udevd, we can't use udev rules. Instead, we write
        # directly to /run/udev/data/c<major>:<minor> which libgudev reads.
        # We must tag the RENDER NODE (not card device) as that's what Mutter checks.
        if [ -n "$detected_node" ]; then
            mkdir -p /run/udev/data

            # Get major:minor for the render node
            if [ -c "$detected_node" ]; then
                MAJOR=$(stat -c %t "$detected_node")
                MINOR=$(stat -c %T "$detected_node")
                # Convert hex to decimal
                MAJOR_DEC=$((16#$MAJOR))
                MINOR_DEC=$((16#$MINOR))

                UDEV_DB_FILE="/run/udev/data/c${MAJOR_DEC}:${MINOR_DEC}"
                echo "G:mutter-device-preferred-primary" > "$UDEV_DB_FILE"
                echo "[render-node] Created udev database entry for Mutter: $UDEV_DB_FILE"
            fi
        fi
    fi

    # Set VA-API driver name for AMD/Intel
    # This ensures libva uses the correct driver on multi-GPU systems
    case "$gpu_vendor" in
        amd)
            # radeonsi is the Mesa VA-API driver for AMD
            export LIBVA_DRIVER_NAME="radeonsi"
            echo "[render-node] Set LIBVA_DRIVER_NAME=radeonsi for AMD VA-API"
            ;;
        intel)
            # iHD is the Intel Media Driver (newer), i965 is legacy
            # Try iHD first (Intel Gen8+), fall back to i965
            if [ -f "/usr/lib/x86_64-linux-gnu/dri/iHD_drv_video.so" ]; then
                export LIBVA_DRIVER_NAME="iHD"
                echo "[render-node] Set LIBVA_DRIVER_NAME=iHD for Intel VA-API"
            else
                export LIBVA_DRIVER_NAME="i965"
                echo "[render-node] Set LIBVA_DRIVER_NAME=i965 for Intel VA-API (legacy)"
            fi
            ;;
        nvidia)
            # NVIDIA doesn't use VA-API for encoding (uses NVENC)
            # But nvidia-vaapi-driver exists for decode
            if [ -f "/usr/lib/x86_64-linux-gnu/dri/nvidia_drv_video.so" ]; then
                export LIBVA_DRIVER_NAME="nvidia"
                echo "[render-node] Set LIBVA_DRIVER_NAME=nvidia for NVIDIA VA-API decode"
            fi
            ;;
    esac

    return 0
}

# Run detection when sourced
detect_render_node
