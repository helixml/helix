#!/usr/bin/env bash

helix_validate_sandbox_instance_id() {
    local instance_id="$1"

    if [[ -z "${instance_id//[[:space:]]/}" || "$instance_id" == *$'\n'* || "$instance_id" == *$'\r'* ]]; then
        echo "ERROR: sandbox instance ID must be one non-empty line" >&2
        return 1
    fi
}

helix_resolve_sandbox_instance_id() {
    local instance_id_file="${SANDBOX_INSTANCE_ID_FILE:-}"
    local configured_instance_id="${SANDBOX_INSTANCE_ID:-}"
    local instance_id_from_pod="${SANDBOX_INSTANCE_ID_FROM_POD:-false}"
    local persisted_instance_id
    local line_count
    local instance_id
    local instance_id_dir
    local temp_file
    local old_umask

    if [[ -z "$instance_id_file" ]]; then
        return 0
    fi

    if [[ -e "$instance_id_file" ]]; then
        if [[ ! -f "$instance_id_file" || ! -r "$instance_id_file" ]]; then
            echo "ERROR: sandbox instance ID file is not a readable regular file: $instance_id_file" >&2
            return 1
        fi

        line_count=$(wc -l < "$instance_id_file") || return 1
        persisted_instance_id=$(<"$instance_id_file") || return 1
        if [[ "$line_count" -ne 1 ]] || ! helix_validate_sandbox_instance_id "$persisted_instance_id"; then
            echo "ERROR: sandbox instance ID file is empty or corrupt: $instance_id_file" >&2
            return 1
        fi
        if [[ "$instance_id_from_pod" != "true" && -n "$configured_instance_id" && "$configured_instance_id" != "$persisted_instance_id" ]]; then
            echo "ERROR: configured sandbox instance ID does not match persisted ID in $instance_id_file" >&2
            return 1
        fi

        export SANDBOX_INSTANCE_ID="$persisted_instance_id"
        return 0
    fi

    instance_id="${configured_instance_id:-${HOSTNAME:-}}"
    helix_validate_sandbox_instance_id "$instance_id" || return 1

    instance_id_dir=${instance_id_file%/*}
    if [[ "$instance_id_dir" == "$instance_id_file" ]]; then
        instance_id_dir=.
    fi
    if [[ ! -d "$instance_id_dir" ]]; then
        echo "ERROR: sandbox instance ID directory does not exist: $instance_id_dir" >&2
        return 1
    fi

    old_umask=$(umask)
    umask 077
    temp_file=$(mktemp "${instance_id_file}.tmp.XXXXXX") || {
        umask "$old_umask"
        echo "ERROR: failed to create sandbox instance ID file in $instance_id_dir" >&2
        return 1
    }
    if ! printf '%s\n' "$instance_id" > "$temp_file"; then
        rm -f "$temp_file"
        umask "$old_umask"
        echo "ERROR: failed to persist sandbox instance ID to $instance_id_file" >&2
        return 1
    fi
    if ! ln "$temp_file" "$instance_id_file"; then
        rm -f "$temp_file"
        umask "$old_umask"
        if [[ -e "$instance_id_file" ]]; then
            helix_resolve_sandbox_instance_id
            return $?
        fi
        echo "ERROR: failed to persist sandbox instance ID to $instance_id_file" >&2
        return 1
    fi
    rm -f "$temp_file"
    umask "$old_umask"

    export SANDBOX_INSTANCE_ID="$instance_id"
}
