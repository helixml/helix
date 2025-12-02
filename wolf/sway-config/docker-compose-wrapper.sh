#!/bin/bash
# Docker Compose wrapper that resolves symlinks in volume paths
# This is needed for Hydra (nested dockerd) to work correctly.
#
# Note: When invoked as a Docker CLI plugin, the first argument is "compose"
# which we need to preserve and pass through.

COMPOSE_REAL="/usr/libexec/docker/cli-plugins/docker-compose.real"

# Docker CLI plugin protocol: first arg is the plugin name ("compose")
# We need to preserve it and process the remaining args
PLUGIN_NAME="$1"
shift

# Function to resolve a path through symlinks
resolve_path() {
    local path="$1"
    # Expand ~ to home directory
    path="${path/#\~/$HOME}"

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

# Process a compose file and resolve volume paths
# Creates a modified copy with resolved paths
process_compose_file() {
    local file="$1"
    local output="$2"
    local dir=$(dirname "$(readlink -f "$file")")

    # Read file and process volume lines
    while IFS= read -r line || [[ -n "$line" ]]; do
        # Check if this looks like a volume mount line: "- /path:/path" or "- ./path:/path"
        if [[ "$line" =~ ^([[:space:]]*-[[:space:]]*)([\"\'"]?)([^:\"\']+):(.+)$ ]]; then
            prefix="${BASH_REMATCH[1]}"
            quote="${BASH_REMATCH[2]}"
            src="${BASH_REMATCH[3]}"
            rest="${BASH_REMATCH[4]}"

            # Remove trailing quote from rest if present
            rest="${rest%\"}"
            rest="${rest%\'}"

            # Resolve the source path
            resolved="$src"

            if [[ "$src" == ./* || "$src" == ../* ]]; then
                # Relative path - resolve from compose file directory
                resolved=$(cd "$dir" && resolve_path "$src")
            elif [[ "$src" == ~* || "$src" == /* ]]; then
                # Absolute or home path
                resolved=$(resolve_path "$src")
            fi
            # else: named volume, leave unchanged

            if [[ "$resolved" != "$src" ]]; then
                echo "${prefix}${quote}${resolved}:${rest}${quote}"
            else
                echo "$line"
            fi
        else
            echo "$line"
        fi
    done < "$file" > "$output"
}

# Find compose files from arguments
find_compose_files() {
    local i=1
    while [[ $i -le $# ]]; do
        local arg="${!i}"
        case "$arg" in
            -f|--file)
                ((i++))
                if [[ $i -le $# ]]; then
                    echo "${!i}"
                fi
                ;;
            -f=*|--file=*)
                echo "${arg#*=}"
                ;;
        esac
        ((i++))
    done
}

# Check if this is a command that uses compose files
needs_processing() {
    for arg in "$@"; do
        case "$arg" in
            up|down|start|stop|restart|run|exec|build|pull|push|logs|ps|create|convert)
                return 0
                ;;
            --)
                return 1
                ;;
        esac
    done
    return 1
}

# Main logic
if needs_processing "$@"; then
    # Get list of compose files
    mapfile -t compose_files < <(find_compose_files "$@")

    # If no -f specified, check for default files
    if [[ ${#compose_files[@]} -eq 0 ]]; then
        for default in "compose.yaml" "compose.yml" "docker-compose.yaml" "docker-compose.yml"; do
            if [[ -f "$default" ]]; then
                compose_files+=("$default")
                break
            fi
        done
    fi

    if [[ ${#compose_files[@]} -gt 0 ]]; then
        # Process each compose file - create temp file in same directory
        declare -A file_map
        declare -a tmp_files=()

        for file in "${compose_files[@]}"; do
            if [[ -f "$file" ]]; then
                file_dir=$(dirname "$file")
                file_base=$(basename "$file")
                # Create temp file in same directory with .hydra-resolved prefix
                tmp_file="${file_dir}/.hydra-resolved.${file_base}"
                process_compose_file "$file" "$tmp_file"
                file_map["$file"]="$tmp_file"
                tmp_files+=("$tmp_file")
            fi
        done

        # Clean up temp files on exit
        cleanup_tmp_files() {
            for f in "${tmp_files[@]}"; do
                rm -f "$f"
            done
        }
        trap cleanup_tmp_files EXIT

        # Rebuild args with modified file paths
        new_args=()
        skip_next=0
        found_file_arg=0

        for arg in "$@"; do
            if [[ $skip_next -eq 1 ]]; then
                skip_next=0
                # This is a file path after -f
                if [[ -n "${file_map[$arg]}" ]]; then
                    new_args+=("${file_map[$arg]}")
                else
                    new_args+=("$arg")
                fi
                continue
            fi

            case "$arg" in
                -f|--file)
                    new_args+=("$arg")
                    skip_next=1
                    found_file_arg=1
                    ;;
                -f=*|--file=*)
                    prefix="${arg%%=*}="
                    orig="${arg#*=}"
                    if [[ -n "${file_map[$orig]}" ]]; then
                        new_args+=("${prefix}${file_map[$orig]}")
                    else
                        new_args+=("$arg")
                    fi
                    found_file_arg=1
                    ;;
                *)
                    new_args+=("$arg")
                    ;;
            esac
        done

        # If no -f was specified but we found a default file, add it
        if [[ $found_file_arg -eq 0 && ${#file_map[@]} -gt 0 ]]; then
            # Get first mapped file
            for orig in "${!file_map[@]}"; do
                new_args=("-f" "${file_map[$orig]}" "${new_args[@]}")
                break
            done
        fi

        exec "$COMPOSE_REAL" "$PLUGIN_NAME" "${new_args[@]}"
    fi
fi

# Fall through - no processing needed or no compose files
exec "$COMPOSE_REAL" "$PLUGIN_NAME" "$@"
