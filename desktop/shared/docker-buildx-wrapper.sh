#!/bin/bash
# Docker wrapper that transparently adds --load for remote buildx builders.
#
# Problem: When BUILDX_BUILDER points to a remote builder, 'docker build -t'
# builds the image remotely but doesn't load it into the local daemon.
# Users expect 'docker build -t foo .' to make 'foo' available locally.
#
# Solution: This wrapper detects 'docker build' with -t and no explicit output,
# and adds --load automatically when a remote builder is active.
#
# Installed at /usr/local/bin/docker (ahead of /usr/bin/docker in PATH).

REAL_DOCKER=/usr/bin/docker

# Fast path: only intercept 'build' subcommand
if [ "${1:-}" != "build" ] || [ -z "${BUILDX_BUILDER:-}" ]; then
    exec "$REAL_DOCKER" "$@"
fi

# Check if builder is remote (cache result for performance)
if [ -z "${_DOCKER_WRAPPER_IS_REMOTE:-}" ]; then
    _DOCKER_WRAPPER_IS_REMOTE=$("$REAL_DOCKER" buildx inspect "$BUILDX_BUILDER" 2>/dev/null | grep -cm1 "^Driver:.*remote" || echo "0")
    export _DOCKER_WRAPPER_IS_REMOTE
fi

if [ "$_DOCKER_WRAPPER_IS_REMOTE" != "1" ]; then
    exec "$REAL_DOCKER" "$@"
fi

# Parse args to check for -t and --output/--load/--push
has_tag=false
has_output=false
for arg in "$@"; do
    case "$arg" in
        -t|--tag) has_tag=true ;;
        --output|--output=*|--load|--push) has_output=true ;;
    esac
done

if $has_tag && ! $has_output; then
    exec "$REAL_DOCKER" "$@" --load
else
    exec "$REAL_DOCKER" "$@"
fi
