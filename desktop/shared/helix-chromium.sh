#!/bin/sh
# Chromium launcher (installed as /usr/bin/chromium on arm64; chromium.real is the
# browser). --password-store=basic avoids the "Choose password for new keyring"
# dialog in containers.

# On a virtio-gpu (Helix for Mac: Venus on MoltenVK), the GL driver is virgl, whose
# contexts lack the robustness Chromium's GPU process requires, so Chromium falls
# back to software rendering. Zink (GL on Vulkan) over Venus provides it.
# optimal_keys stops Zink from emulating last-vertex provoking vertex (MoltenVK has
# no VK_EXT_provoking_vertex) with a geometry shader on every draw; the cost is that
# flat-shaded varyings take the first vertex's value.
if [ -z "${MESA_LOADER_DRIVER_OVERRIDE:-}" ]; then
    for node in /sys/class/drm/renderD*; do
        driver=$(basename "$(readlink -f "$node/device/driver")" 2>/dev/null)
        case "$driver" in
            virtio_gpu|virtio-pci)
                export MESA_LOADER_DRIVER_OVERRIDE=zink ZINK_DEBUG=optimal_keys,quiet
                break
                ;;
        esac
    done
fi

exec /usr/bin/chromium.real --password-store=basic --disable-dev-shm-usage "$@"
