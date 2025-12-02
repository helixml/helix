#!/bin/bash
# Docker CLI wrapper that resolves symlinks in bind mount paths
# This is needed for Hydra (nested dockerd) to work correctly.
#
# Problem: When running docker inside a dev container with Hydra enabled,
# the Docker CLI sends paths like /home/retro/work/foo to Hydra's dockerd.
# But /home/retro/work is a symlink to /data/workspaces/spec-tasks/{id},
# and Hydra's dockerd runs in the sandbox container which doesn't have
# that symlink - it only has the real /data/workspaces/... path.
#
# Solution: Resolve symlinks in -v/--volume arguments before passing to docker.

DOCKER_REAL="/usr/bin/docker.real"

# Function to resolve a path through symlinks
resolve_path() {
    local path="$1"
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
