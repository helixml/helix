#!/usr/bin/env bash
# Unit tests for install.sh's postgres-pin block (also used by ./stack and
# Helix for Mac): existing installs keep the PostgreSQL major their data was
# created with; fresh installs get the compose default. Docker and sudo are
# stubbed, so this runs anywhere bash does.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

block() { sed -n '/^# BEGIN postgres-pin/,/^# END postgres-pin/p' "$ROOT/install.sh"; }

# Load only the functions between the markers; install.sh itself runs an
# installer at top level.
eval "$(block)"

# Read by the functions loaded above.
# shellcheck disable=SC2034
ENVIRONMENT=linux
# shellcheck disable=SC2034
DOCKER_CMD=fake_docker
FAKE_VOLUMES="$WORK/volumes"   # one directory per volume, named as docker would
CALLS="$WORK/calls"            # docker run / sudo calls, one per line
FAKE_LOCAL_IMAGES=""           # `docker image ls postgres` output
FAKE_HIDDEN_MOUNTPOINT=""      # report a mountpoint the user cannot read

mkdir -p "$WORK/bin"
printf '#!/bin/sh\necho "sudo $*" >> "%s"\nexit 1\n' "$CALLS" > "$WORK/bin/sudo"
chmod +x "$WORK/bin/sudo"
PATH="$WORK/bin:$PATH"

# fake_docker: `volume ls` matches the compose labels (project_volume naming),
# `volume inspect` reports the volume directory as its mountpoint, and
# `run --mount type=volume,src=X` reads X's PG_VERSION.
fake_docker() {
    case "$1 $2" in
        "volume ls")
            local project="" vol=""
            while [ $# -gt 0 ]; do
                case "$1" in
                    label=com.docker.compose.project=*) project="${1#label=com.docker.compose.project=}" ;;
                    label=com.docker.compose.volume=*) vol="${1#label=com.docker.compose.volume=}" ;;
                esac
                shift
            done
            if [ -d "$FAKE_VOLUMES/${project}_${vol}" ]; then echo "${project}_${vol}"; fi
            return 0 ;;
        "volume inspect")
            local name="${*: -1}"
            [ -d "$FAKE_VOLUMES/$name" ] || return 1
            if [ -n "$FAKE_HIDDEN_MOUNTPOINT" ]; then
                echo "/var/lib/docker/volumes/$name/_data"
            else
                echo "$FAKE_VOLUMES/$name"
            fi ;;
        "image ls") [ -z "$FAKE_LOCAL_IMAGES" ] || echo "$FAKE_LOCAL_IMAGES" ;;
        "run --rm")
            local mount="$4" image="$7"
            echo "docker run $image" >> "$CALLS"
            case "$mount" in
                type=volume,src=*) mount="${mount#type=volume,src=}"; cat "$FAKE_VOLUMES/${mount%%,*}/PG_VERSION" ;;
                *) return 1 ;;
            esac ;;
        *) echo "unexpected docker call: $*" >&2; return 1 ;;
    esac
}

fails=0
check() { # name expected actual
    if [ "$2" = "$3" ]; then
        echo "ok   - $1"
    else
        echo "FAIL - $1: expected [$2], got [$3]"
        fails=$((fails + 1))
    fi
}

new_install_dir() { # name
    rm -rf "$FAKE_VOLUMES" "$CALLS"
    mkdir -p "$FAKE_VOLUMES"
    touch "$CALLS"
    FAKE_LOCAL_IMAGES=""
    FAKE_HIDDEN_MOUNTPOINT=""
    INSTALL_DIR="$WORK/$1"
    rm -rf "$INSTALL_DIR"
    mkdir -p "$INSTALL_DIR"
    printf 'POSTGRES_ADMIN_PASSWORD=x\n' > "$INSTALL_DIR/.env"
}

pg12_volume() { # volume name
    mkdir -p "$FAKE_VOLUMES/$1"
    echo "${2:-12}" > "$FAKE_VOLUMES/$1/PG_VERSION"
}

pinned() { env_file_value "$INSTALL_DIR/.env" HELIX_POSTGRES_IMAGE; }
pin_count() { grep -c '^HELIX_POSTGRES_IMAGE=' "$INSTALL_DIR/.env" || true; }
calls() { tr '\n' ';' < "$CALLS"; }

unset HELIX_POSTGRES_IMAGE COMPOSE_PROJECT_NAME POSTGRES_DATA

# Fresh install: no volume, nothing pinned, no sudo, no container.
new_install_dir HelixML
pin_postgres_for_existing_data >/dev/null
check "fresh install is not pinned" "" "$(pinned)"
check "fresh install gets no EOL warning" "" "$(warn_postgres_eol)"
check "fresh install runs no sudo or container" "" "$(calls)"

# Existing PG12 named volume (project name from the install dir, lowercased).
new_install_dir HelixML
pg12_volume helixml_helix-postgres-db
check "reads PG_VERSION from the compose volume" "12" "$(postgres_data_major)"
pin_postgres_for_existing_data >/dev/null
check "PG12 volume is pinned to 12" "postgres:12.22-alpine" "$(pinned)"
pin_postgres_for_existing_data >/dev/null
check "pinning is idempotent" "1" "$(pin_count)"
case "$(warn_postgres_eol)" in
    *end-of-life*) check "pinned PG12 install is warned" yes yes ;;
    *) check "pinned PG12 install is warned" yes no ;;
esac

# Project names follow compose: lowercase, [a-z0-9_-], no leading _ or -.
new_install_dir _-Helix.ML
check "project name strips leading _ and -" "helixml" "$(compose_project_name)"
pg12_volume helixml_helix-postgres-db
pin_postgres_for_existing_data >/dev/null
check "volume under a normalized project name is pinned" "postgres:12.22-alpine" "$(pinned)"

# COMPOSE_PROJECT_NAME in .env selects the volume prefix.
new_install_dir other
echo "COMPOSE_PROJECT_NAME=Helix-Prod" >> "$INSTALL_DIR/.env"
pg12_volume helix-prod_helix-postgres-db
pin_postgres_for_existing_data >/dev/null
check "COMPOSE_PROJECT_NAME volume is pinned" "postgres:12.22-alpine" "$(pinned)"

# Root-only mountpoint: sudo -n fails, so read through a container using a
# postgres image that is already present, else the default image.
new_install_dir HelixML
pg12_volume helixml_helix-postgres-db
FAKE_HIDDEN_MOUNTPOINT=1
FAKE_LOCAL_IMAGES="postgres:12.22-alpine"
pin_postgres_for_existing_data >/dev/null
check "unreadable volume is pinned" "postgres:12.22-alpine" "$(pinned)"
check "unreadable volume: sudo -n, then a local postgres image" \
    "sudo -n cat /var/lib/docker/volumes/helixml_helix-postgres-db/_data/PG_VERSION;docker run postgres:12.22-alpine;" "$(calls)"

new_install_dir HelixML
pg12_volume helixml_helix-postgres-db
FAKE_HIDDEN_MOUNTPOINT=1
pin_postgres_for_existing_data >/dev/null
check "no local postgres image: reads with the default image" \
    "sudo -n cat /var/lib/docker/volumes/helixml_helix-postgres-db/_data/PG_VERSION;docker run postgres:17-alpine;" "$(calls)"

# Data already on the default major.
new_install_dir HelixML
pg12_volume helixml_helix-postgres-db 17
pin_postgres_for_existing_data >/dev/null
check "PG17 data is not pinned" "" "$(pinned)"

# Another older major keeps its own major.
new_install_dir HelixML
pg12_volume helixml_helix-postgres-db 15
pin_postgres_for_existing_data >/dev/null
check "PG15 data is pinned to 15" "postgres:15-alpine" "$(pinned)"

# Bind-mounted data directory: absolute, relative and ~ paths.
new_install_dir HelixML
mkdir -p "$WORK/pgdata"
echo 12 > "$WORK/pgdata/PG_VERSION"
echo "POSTGRES_DATA=$WORK/pgdata" >> "$INSTALL_DIR/.env"
pin_postgres_for_existing_data >/dev/null
check "absolute POSTGRES_DATA is pinned" "postgres:12.22-alpine" "$(pinned)"

new_install_dir HelixML
mkdir -p "$INSTALL_DIR/data/pg"
echo 12 > "$INSTALL_DIR/data/pg/PG_VERSION"
echo "POSTGRES_DATA=./data/pg" >> "$INSTALL_DIR/.env"
pin_postgres_for_existing_data >/dev/null
check "relative POSTGRES_DATA is pinned" "postgres:12.22-alpine" "$(pinned)"

new_install_dir HelixML
mkdir -p "$WORK/home/pgdata"
echo 12 > "$WORK/home/pgdata/PG_VERSION"
echo "POSTGRES_DATA=~/pgdata" >> "$INSTALL_DIR/.env"
HOME="$WORK/home" pin_postgres_for_existing_data >/dev/null
check "~ POSTGRES_DATA is read as a path" "postgres:12.22-alpine" "$(pinned)"

new_install_dir HelixML
echo "POSTGRES_DATA=$WORK/missing/pgdata" >> "$INSTALL_DIR/.env"
pin_postgres_for_existing_data >/dev/null
check "missing bind mount is not pinned" "" "$(pinned)"
check "missing bind mount runs no sudo or container" "" "$(calls)"

new_install_dir HelixML
mkdir -p "$WORK/emptydata"
echo "POSTGRES_DATA=$WORK/emptydata" >> "$INSTALL_DIR/.env"
pin_postgres_for_existing_data >/dev/null
check "empty bind mount runs no sudo or container" "" "$(calls)"

# An operator's own choice always wins.
new_install_dir HelixML
pg12_volume helixml_helix-postgres-db
echo "HELIX_POSTGRES_IMAGE=registry.example.com/postgres:12.22-alpine" >> "$INSTALL_DIR/.env"
pin_postgres_for_existing_data >/dev/null
check "existing HELIX_POSTGRES_IMAGE is kept" "registry.example.com/postgres:12.22-alpine" "$(pinned)"
check "existing HELIX_POSTGRES_IMAGE is not duplicated" "1" "$(pin_count)"

new_install_dir HelixML
pg12_volume helixml_helix-postgres-db
HELIX_POSTGRES_IMAGE=postgres:12.22-alpine pin_postgres_for_existing_data >/dev/null
check "HELIX_POSTGRES_IMAGE from the shell is respected" "" "$(pinned)"

# ./stack runs the same block for the dev checkout, with its .env.
new_install_dir devcheckout
echo "COMPOSE_PROJECT_NAME=-Dev" >> "$INSTALL_DIR/.env"
pg12_volume dev_helix-postgres-db
cp "$ROOT/install.sh" "$INSTALL_DIR/install.sh"
(
    # shellcheck disable=SC2317 # called by the eval'd block
    docker() { fake_docker "$@"; }
    eval "$(sed -n '/^function pin_dev_postgres_major/,/^}/p' "$ROOT/stack")"
    # shellcheck disable=SC2034 # read by pin_dev_postgres_major
    DIR="$INSTALL_DIR"
    set -euo pipefail
    IFS=$'\n\t'
    pin_dev_postgres_major >/dev/null
)
check "./stack pins with COMPOSE_PROJECT_NAME from .env" "postgres:12.22-alpine" "$(pinned)"

# The copies of the block and the defaults stay in step.
go_copy=$(awk '/^const postgresPinShell = `/{sub(/^const postgresPinShell = `/, ""); on=1} on{ if (/^`$/) exit; print }' \
    "$ROOT/for-mac/stackscript/postgres_pin.go")
check "Helix for Mac copy matches install.sh" "$(block)" "$go_copy"
compose_default=$(sed -nE 's/.*\$\{HELIX_POSTGRES_IMAGE:-postgres:([0-9]+)[^}]*\}.*/\1/p' "$ROOT/docker-compose.yaml")
check "install.sh default major matches docker-compose.yaml" "$compose_default" "$HELIX_POSTGRES_DEFAULT_MAJOR"

if [ "$fails" -gt 0 ]; then
    echo "$fails test(s) failed"
    exit 1
fi
echo "All postgres-pin tests passed"
