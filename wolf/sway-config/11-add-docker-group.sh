#!/usr/bin/env bash
# Add retro user to docker group at runtime
# The retro user is created by GOW base-app at container startup,
# so we can't add them to the docker group at build time.
#
# CRITICAL: This script MUST succeed - docker access is required for agents

set -e  # Exit on any error

# Check if docker group exists
if ! getent group docker >/dev/null 2>&1; then
    echo "**** Docker group does not exist, creating it ****"
    groupadd docker
fi

# Check if retro user exists (should be created by 10-setup_user.sh)
if ! id retro >/dev/null 2>&1; then
    echo "**** FATAL: retro user does not exist! ****"
    echo "**** This init script must run AFTER 10-setup_user.sh ****"
    exit 1
fi

# Add retro to docker group
echo "**** Adding retro user to docker group ****"
usermod -aG docker retro

# Verify it worked - FAIL if it didn't
if getent group docker | grep -q retro; then
    echo "**** SUCCESS: retro is now in docker group ****"
else
    echo "**** FATAL: usermod succeeded but retro is NOT in docker group! ****"
    echo "**** Docker group contents: $(getent group docker) ****"
    exit 1
fi
