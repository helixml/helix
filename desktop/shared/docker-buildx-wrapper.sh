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
# When HELIX_REGISTRY is set, uses push/pull instead of tarball --load for
# layer-level deduplication (~0.6s vs ~9.5s for a 1-layer change in 7.73GB image).
#
# Installed at /usr/local/bin/docker (ahead of /usr/bin/docker in PATH).

REAL_DOCKER=/usr/bin/docker

# --- Compose build interception ---
# docker compose calls buildx bake internally (Go API, not CLI), bypassing
# the smart --load logic below. Intercept 'docker compose ... build' and
# decompose into individual 'docker buildx build' commands that go through
# smart --load. Non-build compose commands pass through unchanged.
if [ "${1:-}" = "compose" ]; then
    # Scan for "build" subcommand
    _has_build=false
    for _a in "$@"; do
        [ "$_a" = "build" ] && _has_build=true && break
    done

    if $_has_build; then
        # Check builder driver (same logic as below, cached across calls)
        if [ -z "${_DOCKER_WRAPPER_DRIVER:-}" ]; then
            if [ -n "${BUILDX_BUILDER:-}" ]; then
                _DOCKER_WRAPPER_DRIVER=$("$REAL_DOCKER" buildx inspect "$BUILDX_BUILDER" 2>/dev/null | grep -m1 "^Driver:" | awk '{print $2}')
            else
                _DOCKER_WRAPPER_DRIVER=$("$REAL_DOCKER" buildx inspect 2>/dev/null | grep -m1 "^Driver:" | awk '{print $2}')
            fi
            _DOCKER_WRAPPER_DRIVER="${_DOCKER_WRAPPER_DRIVER:-docker}"
            export _DOCKER_WRAPPER_DRIVER
        fi

        if [ "$_DOCKER_WRAPPER_DRIVER" = "remote" ]; then
            # Parse: compose_flags (before "build"), build_flags & services (after "build")
            _compose_flags=()
            _build_flags=()
            _services=()
            _phase="compose"
            _skip=false
            shift  # skip "compose"
            for _a in "$@"; do
                if $_skip; then
                    [ "$_phase" = "compose" ] && _compose_flags+=("$_a") || _build_flags+=("$_a")
                    _skip=false
                    continue
                fi
                if [ "$_phase" = "compose" ]; then
                    if [ "$_a" = "build" ]; then
                        _phase="build"
                        continue
                    fi
                    _compose_flags+=("$_a")
                    case "$_a" in -f|--file|-p|--project-name|--project-directory|--env-file) _skip=true ;; esac
                else
                    case "$_a" in
                        --no-cache|--pull|--quiet|-q) _build_flags+=("$_a") ;;
                        --progress=*) _build_flags+=("$_a") ;;
                        --progress) _build_flags+=("$_a"); _skip=true ;;
                        -*) ;;  # ignore other compose build flags
                        *) _services+=("$_a") ;;
                    esac
                fi
            done

            # Get compose config as JSON
            _cfg=$("$REAL_DOCKER" compose "${_compose_flags[@]}" config --format json 2>/dev/null)
            if [ $? -ne 0 ] || [ -z "$_cfg" ]; then
                echo "[docker-wrapper] Failed to parse compose config, falling back"
                exec "$REAL_DOCKER" compose "${_compose_flags[@]}" build "${_build_flags[@]}" "${_services[@]}"
            fi

            _proj=$(echo "$_cfg" | jq -r '.name // empty')

            # Get services with build sections, optionally filtered
            if [ ${#_services[@]} -gt 0 ]; then
                _filter=$(printf '"%s",' "${_services[@]}")
                _filter="[${_filter%,}]"
                mapfile -t _svc_list < <(echo "$_cfg" | jq -r --argjson f "$_filter" \
                    '.services | to_entries[] | select(.value.build != null) | select(.key as $k | $f | index($k)) | .key')
            else
                mapfile -t _svc_list < <(echo "$_cfg" | jq -r \
                    '.services | to_entries[] | select(.value.build != null) | .key')
            fi

            if [ ${#_svc_list[@]} -eq 0 ]; then
                echo "[docker-wrapper] No services with build sections"
                exit 0
            fi

            echo "[docker-wrapper] compose build: ${#_svc_list[@]} service(s) via smart --load"
            for _svc in "${_svc_list[@]}"; do
                _sj=$(echo "$_cfg" | jq --arg n "$_svc" '.services[$n]')
                _ctx=$(echo "$_sj" | jq -r '.build.context // "."')
                _df=$(echo "$_sj" | jq -r '.build.dockerfile // "Dockerfile"')
                _tgt=$(echo "$_sj" | jq -r '.build.target // empty')
                _img=$(echo "$_sj" | jq -r '.image // empty')
                [ -z "$_img" ] && _img="${_proj}-${_svc}"

                _args=(-t "$_img" -f "$_ctx/$_df")
                [ -n "$_tgt" ] && _args+=(--target "$_tgt")
                while IFS= read -r _ba; do
                    [ -n "$_ba" ] && _args+=(--build-arg "$_ba")
                done < <(echo "$_sj" | jq -r '.build.args // {} | to_entries[] | "\(.key)=\(.value)"')
                _args+=("${_build_flags[@]}" "$_ctx")

                echo "[docker-wrapper]   $_svc → $_img"
                "$0" buildx build "${_args[@]}"
                _rc=$?
                if [ $_rc -ne 0 ]; then
                    echo "[docker-wrapper] Failed: $_svc (exit $_rc)"
                    exit 1
                fi
            done
            exit 0
        fi
    fi

    # Not a compose build or not remote builder — pass through
    exec "$REAL_DOCKER" "$@"
fi

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

# load_via_registry: push image to shared registry, pull into local daemon.
# Only transfers changed layers (~0.6s for a 1-layer change in 7.73GB image)
# vs tarball --load which transfers the entire image (~9.5s).
load_via_registry() {
    local reg_tag="${HELIX_REGISTRY}/buildcache/${tags[0]}"
    # Strip -t/--tag flags from args (they conflict with --output name=)
    local build_args=()
    local skip_next=false
    for arg in "$@"; do
        if $skip_next; then
            skip_next=false
            continue
        fi
        case "$arg" in
            -t|--tag) skip_next=true ;;
            *) build_args+=("$arg") ;;
        esac
    done
    # Build and push to registry with the registry-prefixed tag
    "$REAL_DOCKER" buildx build "${build_args[@]}" \
        --output "type=image,name=$reg_tag,push=true" --provenance=false
    local rc=$?
    if [ $rc -ne 0 ]; then
        echo "[docker-wrapper] Registry push failed, falling back to tarball --load"
        exec "$REAL_DOCKER" buildx build "$@" --load
    fi
    # Pull from registry (layer-level dedup — only downloads changed layers)
    "$REAL_DOCKER" pull "$reg_tag" || {
        echo "[docker-wrapper] Registry pull failed, falling back to tarball --load"
        exec "$REAL_DOCKER" buildx build "$@" --load
    }
    # Re-tag to the original tag names
    for tag in "${tags[@]}"; do
        "$REAL_DOCKER" tag "$reg_tag" "$tag"
    done
    echo "[docker-wrapper] Loaded via registry (layer-level dedup)"
}

# Check if all tagged images exist in local daemon
all_local=true
for tag in "${tags[@]}"; do
    if [ -z "$("$REAL_DOCKER" images -q "$tag" 2>/dev/null)" ]; then
        all_local=false
        break
    fi
done

if ! $all_local; then
    # Image not in local daemon — must load it
    if [ -n "${HELIX_REGISTRY:-}" ]; then
        load_via_registry "$@"
        exit $?
    fi
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
    # Couldn't determine digest — fall back to loading
    if [ -n "${HELIX_REGISTRY:-}" ]; then
        load_via_registry "$@"
        exit $?
    fi
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
    if [ -n "${HELIX_REGISTRY:-}" ]; then
        load_via_registry "$@"
        exit $?
    fi
    exec "$REAL_DOCKER" buildx build "$@" --load
fi

echo "[docker-wrapper] Image unchanged (${new_id:0:19}), skipping load"
exit 0
