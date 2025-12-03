#!/usr/bin/env bash
# Add retro user to docker socket's group at runtime
# The docker socket is mounted from sandbox container with a specific GID.
# We need to add retro to THAT group (by GID), not a newly created docker group.
#
# CRITICAL: This script is SOURCED by entrypoint.sh, not executed directly!
# Therefore: NEVER use 'exit' - use 'return' instead, or it kills the entrypoint!

set -e  # Exit on any error

# Check if retro user exists (should be created by 10-setup_user.sh)
if ! id retro >/dev/null 2>&1; then
    echo "**** FATAL: retro user does not exist! ****"
    echo "**** This init script must run AFTER 10-setup_user.sh ****"
    return 1 2>/dev/null || exit 1  # return if sourced, exit if run directly
fi

# Get the GID of the docker socket (mounted from sandbox)
if [ ! -S /var/run/docker.sock ]; then
    echo "**** WARNING: Docker socket not found, skipping docker group setup ****"
    return 0 2>/dev/null || exit 0  # return if sourced, exit if run directly
fi

SOCKET_GID=$(stat -c "%g" /var/run/docker.sock)
echo "**** Docker socket GID: $SOCKET_GID ****"

# Check if a group with this GID exists, if not create one
if ! getent group "$SOCKET_GID" >/dev/null 2>&1; then
    # Remove any existing docker group with wrong GID first
    if getent group docker >/dev/null 2>&1; then
        echo "**** Removing existing docker group (wrong GID) ****"
        groupdel docker
    fi
    echo "**** Creating docker group with GID $SOCKET_GID ****"
    groupadd -g "$SOCKET_GID" docker
else
    EXISTING_GROUP=$(getent group "$SOCKET_GID" | cut -d: -f1)
    echo "**** Group with GID $SOCKET_GID already exists: $EXISTING_GROUP ****"
fi

# Add retro to the socket's group (by GID)
echo "**** Adding retro user to GID $SOCKET_GID ****"
usermod -aG "$SOCKET_GID" retro

# Verify it worked
if id retro | grep -q "$SOCKET_GID"; then
    echo "**** SUCCESS: retro is now in group $SOCKET_GID ****"
else
    echo "**** FATAL: usermod succeeded but retro is NOT in group $SOCKET_GID! ****"
    echo "**** retro groups: $(id retro) ****"
    return 1 2>/dev/null || exit 1  # return if sourced, exit if run directly
fi
