// Package stackscript holds the shell scripts Helix for Mac runs inside its VM.
package stackscript

// postgresPinShell is install.sh's postgres-pin block, verbatim
// (scripts/test-install-postgres-pin.sh fails when the copies differ).
//
// An app update swaps the VM root disk, which brings a docker-compose file
// that defaults to PostgreSQL 17, while Docker volumes persist on ZFS. A
// PostgreSQL 12 volume does not start under 17, so Start pins the
// data's major in .env on every boot, before docker compose up: install.sh
// only ran when the image was baked, and .env can come from the new root disk.
const postgresPinShell = `# BEGIN postgres-pin
# This block is shared verbatim with ./stack and Helix for Mac
# (for-mac/stackscript/postgres_pin.go); scripts/test-install-postgres-pin.sh tests it and
# checks the copies match.
#
# docker-compose.yaml defaults new installs to PostgreSQL 17. A data directory
# created by an older major will not start under 17: PostgreSQL refuses it
# ("database files are incompatible with server") and leaves it untouched.
# Installs that already hold older data therefore keep their major, pinned as
# HELIX_POSTGRES_IMAGE in .env, until the operator upgrades it with the
# dump/restore in docs/postgres-upgrade.md.
#
# Callers set INSTALL_DIR (the compose project directory), DOCKER_CMD and
# ENVIRONMENT. Nothing here prompts for a password: root-only paths are read
# with "sudo -n" or through a container.
HELIX_POSTGRES_DEFAULT_MAJOR=17
HELIX_POSTGRES_LEGACY_IMAGE=postgres:12.22-alpine

# env_file_value FILE KEY: the last uncommented value of KEY in FILE.
env_file_value() {
    [ -f "$1" ] || return 0
    grep -E "^[[:space:]]*$2[[:space:]]*=" "$1" 2>/dev/null \
        | tail -n1 | cut -d= -f2- | tr -d '"' | tr -d "'" | xargs || true
}

# compose_project_name: the project name docker compose derives for
# $INSTALL_DIR (lowercase, [a-z0-9_-] only, no leading _ or -), which prefixes
# its named volumes.
compose_project_name() {
    local name="${COMPOSE_PROJECT_NAME:-$(env_file_value "$INSTALL_DIR/.env" COMPOSE_PROJECT_NAME)}"
    name="${name:-$(basename "$INSTALL_DIR")}"
    echo "$name" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9_-' | sed 's/^[_-]*//'
}

# pg_version_via_container MOUNT: PG_VERSION read through a throwaway
# container, for data the current user cannot read (Docker Desktop keeps
# volumes inside its VM). Uses a postgres image that is already present, or
# the one the stack is about to pull anyway.
pg_version_via_container() {
    local image
    image=$($DOCKER_CMD image ls --format '{{.Repository}}:{{.Tag}}' postgres 2>/dev/null | grep -v '<none>' | head -n1 || true)
    image="${image:-postgres:$HELIX_POSTGRES_DEFAULT_MAJOR-alpine}"
    $DOCKER_CMD run --rm --mount "$1,dst=/pgdata,readonly" --entrypoint cat "$image" /pgdata/PG_VERSION 2>/dev/null || true
}

# read_pg_version DIR MOUNT: PG_VERSION in data directory DIR, or nothing
# when it has none. MOUNT is the --mount source for the container fallback.
read_pg_version() {
    local file="$1/PG_VERSION" known="$1"
    if [ -r "$file" ]; then
        cat "$file" 2>/dev/null
        return 0
    fi
    # A bind path that is missing, and visibly so without root (the closest
    # existing directory is searchable), is a fresh install. A volume's
    # mountpoint may live in another filesystem (Docker Desktop's VM).
    case "$2" in
        type=bind,*)
            while [ ! -e "$known" ] && [ "$known" != / ]; do
                known=$(dirname "$known")
            done
            if [ ! -e "$file" ] && [ -x "$known" ]; then
                return 0
            fi
            ;;
    esac
    if [ "$ENVIRONMENT" != "gitbash" ] && sudo -n cat "$file" 2>/dev/null; then
        return 0
    fi
    pg_version_via_container "$2"
}

# postgres_data_major: the PostgreSQL major recorded in the existing data
# directory (PG_VERSION), or nothing for a fresh install.
postgres_data_major() {
    local data volume mountpoint project
    data=$(env_file_value "$INSTALL_DIR/.env" POSTGRES_DATA)
    # A literal ~ from .env, expanded the way compose expands it.
    # shellcheck disable=SC2088
    case "$data" in
        "~"|"~/"*) data="$HOME${data#"~"}" ;;
        ./*|../*) data="$INSTALL_DIR/$data" ;;
    esac
    case "$data" in
        /*) read_pg_version "$data" "type=bind,src=$data" | tr -d '[:space:]'; return 0 ;;
    esac

    data="${data:-helix-postgres-db}"
    project=$(compose_project_name)
    volume=$($DOCKER_CMD volume ls -q \
        --filter "label=com.docker.compose.project=$project" \
        --filter "label=com.docker.compose.volume=$data" 2>/dev/null | head -n1 || true)
    if [ -z "$volume" ] && $DOCKER_CMD volume inspect "${project}_$data" >/dev/null 2>&1; then
        volume="${project}_$data"
    fi
    [ -n "$volume" ] || return 0

    mountpoint=$($DOCKER_CMD volume inspect -f '{{ .Mountpoint }}' "$volume" 2>/dev/null || true)
    read_pg_version "${mountpoint:-/nonexistent}" "type=volume,src=$volume" | tr -d '[:space:]'
}

# pin_postgres_for_existing_data: keep an existing install on the PostgreSQL
# major its data was created with. Never overrides an operator's own
# HELIX_POSTGRES_IMAGE.
pin_postgres_for_existing_data() {
    local env_file="$INSTALL_DIR/.env" major image pin
    if [ -n "${HELIX_POSTGRES_IMAGE:-}" ] || [ -n "$(env_file_value "$env_file" HELIX_POSTGRES_IMAGE)" ]; then
        return 0
    fi
    major=$(postgres_data_major)
    case "$major" in ''|*[!0-9]*) return 0 ;; esac
    [ "$major" -lt "$HELIX_POSTGRES_DEFAULT_MAJOR" ] || return 0

    if [ "$major" = 12 ]; then
        image="$HELIX_POSTGRES_LEGACY_IMAGE"
    else
        image="postgres:${major}-alpine"
    fi
    pin=$(printf '\n# Existing PostgreSQL %s data: stay on %s until it is upgraded\n# (https://github.com/helixml/helix/blob/main/docs/postgres-upgrade.md).\nHELIX_POSTGRES_IMAGE=%s\n' "$major" "$major" "$image")
    if [ -w "$env_file" ] || [ "$ENVIRONMENT" = "gitbash" ]; then
        echo "$pin" >> "$env_file" || return 1
    else
        if ! echo "$pin" | sudo -n tee -a "$env_file" >/dev/null; then
            echo "Could not write HELIX_POSTGRES_IMAGE=$image to $env_file; add it by hand." >&2
            return 1
        fi
    fi
    echo "Existing PostgreSQL $major data found: set HELIX_POSTGRES_IMAGE=$image in $env_file"
}

# warn_postgres_eol: point installs still pinned to PostgreSQL 12 (end of
# life) at the upgrade guide.
warn_postgres_eol() {
    local image="${HELIX_POSTGRES_IMAGE:-$(env_file_value "$INSTALL_DIR/.env" HELIX_POSTGRES_IMAGE)}"
    case "${image:-postgres:$HELIX_POSTGRES_DEFAULT_MAJOR}" in
        postgres:12*|*/postgres:12*)
            echo
            echo "┌───────────────────────────────────────────────────────────────────────────"
            echo "│ ⚠️  Helix's database runs PostgreSQL 12, which is end-of-life and no longer"
            echo "│    receives security fixes. Upgrading is a one-time dump and restore that"
            echo "│    keeps your current data volume as a rollback:"
            echo "│    https://github.com/helixml/helix/blob/main/docs/postgres-upgrade.md"
            echo "└───────────────────────────────────────────────────────────────────────────"
            ;;
    esac
}
# END postgres-pin
`
