#!/usr/bin/env bash
# Add retro user to docker group at runtime
# The retro user is created by GOW base-app at container startup,
# so we can't add them to the docker group at build time.

if getent group docker >/dev/null 2>&1; then
    # Add retro user to docker group
    usermod -aG docker retro 2>/dev/null || true
    echo "**** Added retro user to docker group ****"
else
    echo "**** Docker group does not exist, skipping ****"
fi
