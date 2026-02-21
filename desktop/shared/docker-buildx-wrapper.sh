#!/bin/bash
# Docker wrapper that transparently routes 'docker build' through buildx
# and adds --load for remote builders.
#
# Problem: Docker 29.x's 'docker build' ignores the default buildx builder
# and always uses the local daemon's built-in BuildKit. When a shared remote
# BuildKit is configured as the default builder, 'docker build' bypasses it
# entirely — defeating cross-session cache sharing.
#
# Solution: This wrapper rewrites 'docker build' to 'docker buildx build'
# (which honors the default builder) and adds --load when a remote builder
# is active so images are loaded into the local daemon.
#
# Installed at /usr/local/bin/docker (ahead of /usr/bin/docker in PATH).

REAL_DOCKER=/usr/bin/docker

# Fast path: only intercept 'build' subcommand
if [ "${1:-}" != "build" ]; then
    exec "$REAL_DOCKER" "$@"
fi

# Determine the active builder's driver (remote vs docker)
# Check explicit BUILDX_BUILDER first, then fall back to default builder
if [ -z "${_DOCKER_WRAPPER_DRIVER:-}" ]; then
    if [ -n "${BUILDX_BUILDER:-}" ]; then
        _DOCKER_WRAPPER_DRIVER=$("$REAL_DOCKER" buildx inspect "$BUILDX_BUILDER" 2>/dev/null | grep -m1 "^Driver:" | awk '{print $2}')
    else
        # Inspect the default builder (marked with * in buildx ls)
        _DOCKER_WRAPPER_DRIVER=$("$REAL_DOCKER" buildx inspect 2>/dev/null | grep -m1 "^Driver:" | awk '{print $2}')
    fi
    _DOCKER_WRAPPER_DRIVER="${_DOCKER_WRAPPER_DRIVER:-docker}"
    export _DOCKER_WRAPPER_DRIVER
fi

# Remove 'build' from args — we'll use 'buildx build' instead
shift

if [ "$_DOCKER_WRAPPER_DRIVER" = "remote" ]; then
    # Remote builder: check if --load is needed
    has_tag=false
    has_output=false
    for arg in "$@"; do
        case "$arg" in
            -t|--tag) has_tag=true ;;
            --output|--output=*|--load|--push) has_output=true ;;
        esac
    done

    if $has_tag && ! $has_output; then
        exec "$REAL_DOCKER" buildx build "$@" --load
    fi
fi

# Use 'buildx build' to honor the default builder
exec "$REAL_DOCKER" buildx build "$@"
