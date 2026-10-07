#!/bin/bash
# Chromium launcher (installed as /usr/bin/chromium on arm64; chromium.real is the
# browser). --password-store=basic avoids the "Choose password for new keyring"
# dialog in containers.

args=(--password-store=basic --disable-dev-shm-usage)

# On a virtio-gpu (Helix for Mac: Venus on MoltenVK) Chromium renders with ANGLE's
# Vulkan backend straight on Venus. The GPU process still initialises through
# native GL, and virgl's GL contexts lack the robustness it requires, so it gets
# Zink (GL on Vulkan) for that; optimal_keys keeps Zink off geometry-shader
# emulation, which MoltenVK can't compile. ANGLE-on-GL over Zink renders
# anti-aliased paths wrongly, so the Vulkan backend isn't optional.
angle_features=Vulkan,VulkanFromANGLE,DefaultANGLEVulkan
virtio=false
for node in /sys/class/drm/renderD*; do
    case "$(basename "$(readlink -f "$node/device/driver")" 2>/dev/null)" in
        virtio_gpu|virtio-pci) virtio=true; break ;;
    esac
done
if $virtio; then
    [ -n "${MESA_LOADER_DRIVER_OVERRIDE:-}" ] || export MESA_LOADER_DRIVER_OVERRIDE=zink ZINK_DEBUG=optimal_keys,quiet
    args+=(--use-angle=vulkan)
    # Chromium honours only the last --enable-features, so merge into the caller's.
    merged=false
    for arg in "$@"; do
        case "$arg" in --enable-features=*) merged=true ;; esac
    done
    $merged || args+=("--enable-features=$angle_features")
fi

for arg in "$@"; do
    case "$arg" in
        --enable-features=*) $virtio && arg="$arg,$angle_features" ;;
    esac
    args+=("$arg")
done

exec /usr/bin/chromium.real "${args[@]}"
