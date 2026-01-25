#!/usr/bin/env bash
# Sway init script: Fix GPU device permissions
#
# In Docker-in-Docker setups, GPU devices may be mounted as root:root
# We need to make them accessible to non-root users for Vulkan/CUDA/EGL

set -e

source /opt/gow/bash-lib/utils.sh

gow_log "**** Fixing GPU device permissions ****"

# Fix GPU device permissions for non-root users
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

gow_log "DONE"
