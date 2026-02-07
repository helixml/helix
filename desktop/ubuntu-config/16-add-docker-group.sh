#!/usr/bin/env bash
# Add retro user to docker socket's group at runtime
# The docker socket is mounted from sandbox container with a specific GID.
# We need to add retro to THAT group (by GID), not a newly created docker group.
#
# CRITICAL: This script MUST succeed - docker access is required for agents

set -e  # Exit on any error

# Check if retro user exists (should be created by 10-setup_user.sh)
if ! id retro >/dev/null 2>&1; then
    echo "**** FATAL: retro user does not exist! ****"
    echo "**** This init script must run AFTER 10-setup_user.sh ****"
    return 1
fi

# Get the GID of the docker socket (mounted from sandbox)
if [ ! -S /var/run/docker.sock ]; then
    echo "**** WARNING: Docker socket not found, skipping docker group setup ****"
    return 0
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
    return 1
fi

# Also handle host docker socket for privileged mode (Helix-in-Helix development)
# The host docker socket is mounted at /var/run/host-docker.sock when privileged mode is enabled
if [ -S /var/run/host-docker.sock ]; then
    HOST_SOCKET_GID=$(stat -c "%g" /var/run/host-docker.sock)
    echo "**** Host docker socket found, GID: $HOST_SOCKET_GID ****"

    # Only add to group if it's different from the main docker socket
    if [ "$HOST_SOCKET_GID" != "$SOCKET_GID" ]; then
        # Check if a group with this GID exists, if not create one
        if ! getent group "$HOST_SOCKET_GID" >/dev/null 2>&1; then
            echo "**** Creating host-docker group with GID $HOST_SOCKET_GID ****"
            groupadd -g "$HOST_SOCKET_GID" host-docker
        else
            EXISTING_GROUP=$(getent group "$HOST_SOCKET_GID" | cut -d: -f1)
            echo "**** Group with GID $HOST_SOCKET_GID already exists: $EXISTING_GROUP ****"
        fi

        # Add retro to the host socket's group
        echo "**** Adding retro user to host docker GID $HOST_SOCKET_GID ****"
        usermod -aG "$HOST_SOCKET_GID" retro

        if id retro | grep -q "$HOST_SOCKET_GID"; then
            echo "**** SUCCESS: retro is now in host docker group $HOST_SOCKET_GID ****"
        else
            echo "**** WARNING: Failed to add retro to host docker group $HOST_SOCKET_GID ****"
        fi
    else
        echo "**** Host docker socket has same GID as main socket, no additional setup needed ****"
    fi
else
    echo "**** Host docker socket not found (privileged mode not enabled) ****"
fi
