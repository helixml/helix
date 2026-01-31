#!/usr/bin/env bash
# GOW init script: Configure device permissions

set -e

gow_log "**** Configure devices ****"

# Fix GPU device permissions for non-root users
# In Docker-in-Docker setups, GPU devices may be mounted as root:root
# We need to make them accessible to the retro user for Vulkan/CUDA/EGL
gow_log "Fixing GPU device permissions..."
for gpu_dev in /dev/dri/card* /dev/dri/renderD*; do
    if [ -c "$gpu_dev" ]; then
        # Get current group - if it's root, we need to fix it
        current_group=$(stat -c "%G" "$gpu_dev")
        if [ "$current_group" = "root" ]; then
            # Change to video group if it exists, otherwise make world-accessible
            if getent group video >/dev/null 2>&1; then
                chgrp video "$gpu_dev"
                chmod 660 "$gpu_dev"
                gow_log "Changed $gpu_dev to video group"
            else
                chmod 666 "$gpu_dev"
                gow_log "Made $gpu_dev world-accessible (no video group)"
            fi
        else
            gow_log "$gpu_dev already has group: $current_group"
        fi
    fi
done

# Add retro to video and render groups if they exist
if getent group video >/dev/null 2>&1; then
    usermod -aG video "${UNAME:-retro}" 2>/dev/null || true
    gow_log "Added ${UNAME:-retro} to video group"
fi
if getent group render >/dev/null 2>&1; then
    usermod -aG render "${UNAME:-retro}" 2>/dev/null || true
    gow_log "Added ${UNAME:-retro} to render group"
fi

gow_log "Exec device groups"
# Make sure we're in the right groups to use all the required devices
# We're actually relying on word splitting for this call, so disable the
# warning from shellcheck
# shellcheck disable=SC2086
/opt/gow/ensure-groups ${GOW_REQUIRED_DEVICES:-/dev/uinput /dev/input/event*}

gow_log "DONE"
