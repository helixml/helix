#!/bin/bash
# Docker wrapper that transparently routes 'docker build' through buildx
# and uses smart --load for remote builders (skips export when image unchanged).
#
# Problem: Docker 29.x's 'docker build' ignores the default buildx builder
# and always uses the local daemon's built-in BuildKit. When a shared remote
# BuildKit is configured as the default builder, 'docker build' bypasses it
# entirely — defeating cross-session cache sharing.
#
# Solution: This wrapper rewrites 'docker build' to 'docker buildx build'
# (which honors the default builder) and uses smart --load for remote builders:
# builds without --load first (~5s) to check if the image changed, then only
# does the expensive --load (~655s for 7.7GB images) when something actually
# changed. This makes cached rebuilds near-instant.
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

if [ "$_DOCKER_WRAPPER_DRIVER" != "remote" ]; then
    # Non-remote builder: just use buildx build directly
    exec "$REAL_DOCKER" buildx build "$@"
fi

# Remote builder: extract tags and check for explicit output flags
has_tag=false
has_output=false
tags=()
next_is_tag=false
for arg in "$@"; do
    if $next_is_tag; then
        tags+=("$arg")
        next_is_tag=false
        continue
    fi
    case "$arg" in
        -t|--tag) has_tag=true; next_is_tag=true ;;
        --output|--output=*|--load|--push) has_output=true ;;
    esac
done

# If user specified explicit output or no tag, pass through
if $has_output || ! $has_tag; then
    exec "$REAL_DOCKER" buildx build "$@"
fi

# Check if all tagged images exist in local daemon
all_local=true
for tag in "${tags[@]}"; do
    if [ -z "$("$REAL_DOCKER" images -q "$tag" 2>/dev/null)" ]; then
        all_local=false
        break
    fi
done

if ! $all_local; then
    # Image not in local daemon — must build with --load
    exec "$REAL_DOCKER" buildx build "$@" --load
fi

# Image exists locally. Quick build WITHOUT --load to check if anything changed.
# BuildKit runs the full build (all cached = ~5s) and writes the image digest
# to iidfile WITHOUT the expensive export step (~655s for large images).
iid_file=$(mktemp /tmp/buildx-iid-XXXXXX)
"$REAL_DOCKER" buildx build "$@" --iidfile "$iid_file"
rc=$?
if [ $rc -ne 0 ]; then
    rm -f "$iid_file"
    exit $rc
fi

new_id=""
[ -f "$iid_file" ] && new_id=$(cat "$iid_file")
rm -f "$iid_file"

if [ -z "$new_id" ]; then
    # Couldn't determine digest — fall back to --load
    exec "$REAL_DOCKER" buildx build "$@" --load
fi

# Compare buildx digest with local daemon's image ID
need_load=false
for tag in "${tags[@]}"; do
    local_id=$("$REAL_DOCKER" images --no-trunc -q "$tag" 2>/dev/null | head -1)
    if [ "$new_id" != "$local_id" ]; then
        need_load=true
        break
    fi
done

if $need_load; then
    echo "[docker-wrapper] Image changed (new: ${new_id:0:19}), loading into daemon..."
    exec "$REAL_DOCKER" buildx build "$@" --load
fi

echo "[docker-wrapper] Image unchanged (${new_id:0:19}), skipping load"
exit 0
