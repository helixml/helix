#!/bin/bash
# Docker wrapper that transparently routes builds through buildx and uses
# smart --load for remote builders (skips image export when unchanged).
#
# Problem: Docker 29.x's 'docker build' ignores the default buildx builder
# and always uses the local daemon's built-in BuildKit. When a shared remote
# BuildKit is configured, 'docker build' bypasses it — defeating cache sharing.
#
# Solution: This wrapper intercepts both 'docker build' and 'docker buildx build',
# routes them through the configured buildx builder, and for remote builders
# applies smart --load: probes the image digest first (~0.5s), then only does
# the expensive --load (tarball export) when the image actually changed.
#
# Installed at /usr/local/bin/docker (ahead of /usr/bin/docker in PATH).

REAL_DOCKER=/usr/bin/docker

# Detect which form: 'docker build ...' or 'docker buildx build ...'
if [ "${1:-}" = "build" ]; then
    # 'docker build ...' → rewrite to 'docker buildx build ...'
    shift
elif [ "${1:-}" = "buildx" ] && [ "${2:-}" = "build" ]; then
    # 'docker buildx build ...' → intercept and apply smart --load
    shift 2
else
    # Not a build command — pass through unchanged
    exec "$REAL_DOCKER" "$@"
fi

# At this point, "$@" contains the build args (everything after 'build')

# Determine the active builder's driver (remote vs docker)
# Cache the result to avoid repeated 'buildx inspect' calls
if [ -z "${_DOCKER_WRAPPER_DRIVER:-}" ]; then
    if [ -n "${BUILDX_BUILDER:-}" ]; then
        _DOCKER_WRAPPER_DRIVER=$("$REAL_DOCKER" buildx inspect "$BUILDX_BUILDER" 2>/dev/null | grep -m1 "^Driver:" | awk '{print $2}')
    else
        _DOCKER_WRAPPER_DRIVER=$("$REAL_DOCKER" buildx inspect 2>/dev/null | grep -m1 "^Driver:" | awk '{print $2}')
    fi
    _DOCKER_WRAPPER_DRIVER="${_DOCKER_WRAPPER_DRIVER:-docker}"
    export _DOCKER_WRAPPER_DRIVER
fi

if [ "$_DOCKER_WRAPPER_DRIVER" != "remote" ]; then
    # Non-remote builder: just use buildx build directly (no --load needed)
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

# Image exists locally. Quick build with --output type=image to check if
# anything changed. This exports the manifest to BuildKit's internal store
# (fast, no tarball transfer) and writes the config digest to iidfile.
# --provenance=false ensures the iidfile contains the config digest (not a
# manifest list), matching what 'docker images --no-trunc -q' returns.
iid_file=$(mktemp /tmp/buildx-iid-XXXXXX)
"$REAL_DOCKER" buildx build "$@" --output type=image --provenance=false --iidfile "$iid_file"
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
