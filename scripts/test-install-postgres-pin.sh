#!/usr/bin/env bash
# Unit tests for install.sh's postgres-pin block: existing installs keep the
# PostgreSQL major their data was created with; fresh installs get the
# compose default. Docker is stubbed, so this runs anywhere bash does.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Load only the functions between the markers; install.sh itself runs an
# installer at top level.
eval "$(sed -n '/^# BEGIN postgres-pin/,/^# END postgres-pin/p' "$ROOT/install.sh")"

# Read by the install.sh functions loaded above.
# shellcheck disable=SC2034
ENVIRONMENT=linux
# shellcheck disable=SC2034
DOCKER_CMD=fake_docker
FAKE_VOLUMES="$WORK/volumes"   # one directory per volume, named as docker would

# fake_docker: `volume ls` matches the compose labels (project_volume naming),
# `volume inspect` reports the volume directory as its mountpoint.
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
            [ -d "$FAKE_VOLUMES/${project}_${vol}" ] && echo "${project}_${vol}"
            return 0 ;;
        "volume inspect")
            local name="${*: -1}"
            [ -d "$FAKE_VOLUMES/$name" ] || return 1
            echo "$FAKE_VOLUMES/$name" ;;
        "run --rm") return 1 ;;
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
    rm -rf "$FAKE_VOLUMES"
    mkdir -p "$FAKE_VOLUMES"
    INSTALL_DIR="$WORK/$1"
    rm -rf "$INSTALL_DIR"
    mkdir -p "$INSTALL_DIR"
    printf 'POSTGRES_ADMIN_PASSWORD=x\n' > "$INSTALL_DIR/.env"
}

pinned() { env_file_value "$INSTALL_DIR/.env" HELIX_POSTGRES_IMAGE; }
pin_count() { grep -c '^HELIX_POSTGRES_IMAGE=' "$INSTALL_DIR/.env" || true; }

unset HELIX_POSTGRES_IMAGE COMPOSE_PROJECT_NAME POSTGRES_DATA

# Fresh install: no volume, nothing pinned.
new_install_dir HelixML
pin_postgres_for_existing_data >/dev/null
check "fresh install is not pinned" "" "$(pinned)"
check "fresh install gets no EOL warning" "" "$(warn_postgres_eol)"

# Existing PG12 named volume (project name from the install dir, lowercased).
new_install_dir HelixML
mkdir -p "$FAKE_VOLUMES/helixml_helix-postgres-db"
echo 12 > "$FAKE_VOLUMES/helixml_helix-postgres-db/PG_VERSION"
check "reads PG_VERSION from the compose volume" "12" "$(postgres_data_major)"
pin_postgres_for_existing_data >/dev/null
check "PG12 volume is pinned to 12" "postgres:12.22-alpine" "$(pinned)"
pin_postgres_for_existing_data >/dev/null
check "pinning is idempotent" "1" "$(pin_count)"
case "$(warn_postgres_eol)" in
    *end-of-life*) check "pinned PG12 install is warned" yes yes ;;
    *) check "pinned PG12 install is warned" yes no ;;
esac

# COMPOSE_PROJECT_NAME in .env selects the volume prefix.
new_install_dir other
echo "COMPOSE_PROJECT_NAME=Helix-Prod" >> "$INSTALL_DIR/.env"
mkdir -p "$FAKE_VOLUMES/helix-prod_helix-postgres-db"
echo 12 > "$FAKE_VOLUMES/helix-prod_helix-postgres-db/PG_VERSION"
pin_postgres_for_existing_data >/dev/null
check "COMPOSE_PROJECT_NAME volume is pinned" "postgres:12.22-alpine" "$(pinned)"

# Data already on the default major.
new_install_dir HelixML
mkdir -p "$FAKE_VOLUMES/helixml_helix-postgres-db"
echo 17 > "$FAKE_VOLUMES/helixml_helix-postgres-db/PG_VERSION"
pin_postgres_for_existing_data >/dev/null
check "PG17 data is not pinned" "" "$(pinned)"

# Another older major keeps its own major.
new_install_dir HelixML
mkdir -p "$FAKE_VOLUMES/helixml_helix-postgres-db"
echo 15 > "$FAKE_VOLUMES/helixml_helix-postgres-db/PG_VERSION"
pin_postgres_for_existing_data >/dev/null
check "PG15 data is pinned to 15" "postgres:15-alpine" "$(pinned)"

# Bind-mounted data directory (absolute and relative POSTGRES_DATA).
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
echo "POSTGRES_DATA=$WORK/missing" >> "$INSTALL_DIR/.env"
pin_postgres_for_existing_data >/dev/null
check "empty bind mount is not pinned" "" "$(pinned)"

# An operator's own choice always wins.
new_install_dir HelixML
mkdir -p "$FAKE_VOLUMES/helixml_helix-postgres-db"
echo 12 > "$FAKE_VOLUMES/helixml_helix-postgres-db/PG_VERSION"
echo "HELIX_POSTGRES_IMAGE=registry.example.com/postgres:12.22-alpine" >> "$INSTALL_DIR/.env"
pin_postgres_for_existing_data >/dev/null
check "existing HELIX_POSTGRES_IMAGE is kept" "registry.example.com/postgres:12.22-alpine" "$(pinned)"
check "existing HELIX_POSTGRES_IMAGE is not duplicated" "1" "$(pin_count)"

new_install_dir HelixML
mkdir -p "$FAKE_VOLUMES/helixml_helix-postgres-db"
echo 12 > "$FAKE_VOLUMES/helixml_helix-postgres-db/PG_VERSION"
HELIX_POSTGRES_IMAGE=postgres:12.22-alpine pin_postgres_for_existing_data >/dev/null
check "HELIX_POSTGRES_IMAGE from the shell is respected" "" "$(pinned)"

# The pin must match the compose default it protects against.
compose_default=$(sed -nE 's/.*\$\{HELIX_POSTGRES_IMAGE:-postgres:([0-9]+)[^}]*\}.*/\1/p' "$ROOT/docker-compose.yaml")
check "install.sh default major matches docker-compose.yaml" "$compose_default" "$HELIX_POSTGRES_DEFAULT_MAJOR"

if [ "$fails" -gt 0 ]; then
    echo "$fails test(s) failed"
    exit 1
fi
echo "All postgres-pin tests passed"
