#!/bin/bash
# Docker CLI wrapper that translates paths from user-friendly to container paths.
# This is needed for Hydra (nested dockerd) to work correctly.
# See: https://github.com/helixml/helix/issues/1405
#
# Problem: When running docker inside a dev container with Hydra enabled,
# the Docker CLI sends paths like /home/retro/work/foo to Hydra's dockerd.
# But Hydra's dockerd runs in the sandbox container, which can only access
# the actual workspace path (/data/workspaces/spec-tasks/{id}), not
# the user-friendly path (/home/retro/work).
#
# Solution: Translate /home/retro/work paths to the actual WORKSPACE_DIR path.
# The WORKSPACE_DIR env var is set by Wolf executor to the actual data path.
#
# Updated 2025-12-13: Now uses WORKSPACE_DIR env var instead of symlink resolution.
# This is required because /home/retro/work is now a bind mount (not a symlink).

DOCKER_REAL="/usr/bin/docker.real"

# User-friendly path that's bind-mounted inside the dev container
USER_PATH="/home/retro/work"

# Function to translate a path from user-friendly to actual workspace path
resolve_path() {
    local path="$1"

    # If WORKSPACE_DIR is set and path starts with /home/retro/work,
    # translate to the actual workspace path
    if [[ -n "$WORKSPACE_DIR" && "$path" == "$USER_PATH"* ]]; then
        # Replace /home/retro/work prefix with WORKSPACE_DIR
        local relative="${path#$USER_PATH}"
        echo "${WORKSPACE_DIR}${relative}"
        return
    fi

    # Fallback to symlink resolution for other paths or when WORKSPACE_DIR not set
    if [[ -e "$path" || -L "$path" ]]; then
        readlink -f "$path" 2>/dev/null || echo "$path"
    else
        # Path doesn't exist yet - resolve parent directory
        local dir=$(dirname "$path")
        local base=$(basename "$path")
        if [[ -e "$dir" || -L "$dir" ]]; then
            local resolved_dir=$(readlink -f "$dir" 2>/dev/null || echo "$dir")
            echo "${resolved_dir}/${base}"
        else
            echo "$path"
        fi
    fi
}

# Function to process a volume argument
# Handles formats: /src:/dst, /src:/dst:ro, /src:/dst:rw,z, etc.
process_volume_arg() {
    local vol="$1"

    # Split on first colon to get source path
    # But be careful: Windows paths have colons too (not applicable here, but good to note)
    local src="${vol%%:*}"
    local rest="${vol#*:}"

    # Skip named volumes (no slashes, no dots at start)
    # Named volumes look like: myvolume:/path or volume_name:/path
    if [[ "$src" != /* && "$src" != .* && ! -e "$src" && ! -L "$src" ]]; then
        echo "$vol"
        return
    fi

    # Process absolute paths, relative paths (./foo, ../foo), and existing paths
    local resolved_src=$(resolve_path "$src")
    if [[ "$resolved_src" != "$src" ]]; then
        echo "${resolved_src}:${rest}"
        return
    fi

    # Return unchanged
    echo "$vol"
}

# Build new argument list with resolved paths
args=()
i=0
while [[ $# -gt 0 ]]; do
    arg="$1"
    shift

    case "$arg" in
        -v|--volume)
            # Next argument is the volume spec
            if [[ $# -gt 0 ]]; then
                vol="$1"
                shift
                resolved_vol=$(process_volume_arg "$vol")
                args+=("$arg" "$resolved_vol")
            else
                args+=("$arg")
            fi
            ;;
        -v=*|--volume=*)
            # Volume spec is part of the argument
            prefix="${arg%%=*}="
            vol="${arg#*=}"
            resolved_vol=$(process_volume_arg "$vol")
            args+=("${prefix}${resolved_vol}")
            ;;
        --mount)
            # --mount uses key=value pairs, source= or src= contains the path
            if [[ $# -gt 0 ]]; then
                mount_spec="$1"
                shift
                # Process source= or src= in the mount spec
                if [[ "$mount_spec" == *"source="* ]] || [[ "$mount_spec" == *"src="* ]]; then
                    # Extract and resolve the source path
                    new_mount=""
                    IFS=',' read -ra parts <<< "$mount_spec"
                    for part in "${parts[@]}"; do
                        if [[ "$part" == source=* ]]; then
                            src="${part#source=}"
                            resolved_src=$(resolve_path "$src")
                            new_mount="${new_mount}source=${resolved_src},"
                        elif [[ "$part" == src=* ]]; then
                            src="${part#src=}"
                            resolved_src=$(resolve_path "$src")
                            new_mount="${new_mount}src=${resolved_src},"
                        else
                            new_mount="${new_mount}${part},"
                        fi
                    done
                    # Remove trailing comma
                    mount_spec="${new_mount%,}"
                fi
                args+=("$arg" "$mount_spec")
            else
                args+=("$arg")
            fi
            ;;
        *)
            args+=("$arg")
            ;;
    esac
done

exec "$DOCKER_REAL" "${args[@]}"
