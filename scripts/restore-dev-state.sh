#!/usr/bin/env bash
# Reapply a snapshot taken by scripts/backup-dev-state.sh to the local dev stack.
#
# DESTRUCTIVE: the main database, the kodit code-index database and the whole
# /filestore volume are replaced by the contents of the snapshot.
#
# The api container is stopped and started again — never recreated — because the
# dev api container's environment intentionally diverges from .env and
# `docker compose up` would clobber it.
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/dev-state-common.sh"

BACKUP_DIR=""
WITH_KODIT=1
WITH_ENV=1
ASSUME_YES=0

usage() {
  cat <<EOF
Usage: $(basename "$0") [backup-dir] [options]

  backup-dir     Snapshot to restore (default: dev-state-backups/latest)
  --no-kodit     Leave the vectorchord-kodit code-index database untouched
  --no-env       Leave .env untouched
  -y, --yes      Do not prompt for confirmation
  -h, --help     Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-kodit) WITH_KODIT=0; shift ;;
    --no-env)   WITH_ENV=0; shift ;;
    -y|--yes)   ASSUME_YES=1; shift ;;
    -h|--help)  usage; exit 0 ;;
    -*)         usage >&2; die "unknown argument: $1" ;;
    *)          [[ -z "$BACKUP_DIR" ]] || die "more than one backup directory given"; BACKUP_DIR="$1"; shift ;;
  esac
done

cd "$REPO_ROOT"

[[ -n "$BACKUP_DIR" ]] || BACKUP_DIR="$DEFAULT_BACKUP_ROOT/latest"
[[ -d "$BACKUP_DIR" ]] || die "no such backup directory: $BACKUP_DIR"
BACKUP_DIR="$(cd "$BACKUP_DIR" && pwd)"

for f in postgres.dump filestore.tar.gz manifest.json; do
  [[ -f "$BACKUP_DIR/$f" ]] || die "backup is incomplete, missing $f in $BACKUP_DIR"
done

PG_ID="$(require_container postgres)"
API_ID="$(require_container api)"
FILESTORE_VOL="$(volume_at "$API_ID" /filestore)"
[[ -n "$FILESTORE_VOL" ]] || die "could not resolve the /filestore volume from the api container"
HELPER_IMAGE="$(helper_image)"

KODIT_ID=""
if [[ "$WITH_KODIT" == "1" && -f "$BACKUP_DIR/kodit.dump" ]]; then
  KODIT_ID="$(container_id vectorchord-kodit)"
  [[ -n "$KODIT_ID" ]] || warn "snapshot has kodit.dump but vectorchord-kodit is not running; skipping it"
fi

cat <<EOF

About to restore: $BACKUP_DIR
$(sed 's/^/  /' "$BACKUP_DIR/manifest.json")

This will DESTROY and replace:
  - database 'postgres' in container $(docker inspect "$PG_ID" --format '{{.Name}}' | tr -d /)
$( [[ -n "$KODIT_ID" ]] && echo "  - database 'kodit' in container $(docker inspect "$KODIT_ID" --format '{{.Name}}' | tr -d /)" )
  - every file in the docker volume $FILESTORE_VOL
$( [[ "$WITH_ENV" == "1" && -f "$BACKUP_DIR/env" ]] && echo "  - $REPO_ROOT/.env (the current one is saved as .env.pre-restore.<timestamp>)" )

EOF
confirm "Proceed?"

# Stop everything that holds a connection to the main database so DROP DATABASE
# can take the lock. `stop`, never `down`/`up` — see the header comment.
STOPPED_SERVICES=()
for svc in api postgres-mcp; do
  id="$(container_id "$svc")"
  if [[ -n "$id" && "$(docker inspect "$id" --format '{{.State.Running}}')" == "true" ]]; then
    STOPPED_SERVICES+=("$svc")
  fi
done
if [[ ${#STOPPED_SERVICES[@]} -gt 0 ]]; then
  log "Stopping ${STOPPED_SERVICES[*]}"
  compose stop "${STOPPED_SERVICES[@]}" >/dev/null
fi

restart_stopped() {
  if [[ ${#STOPPED_SERVICES[@]} -gt 0 ]]; then
    log "Starting ${STOPPED_SERVICES[*]}"
    compose start "${STOPPED_SERVICES[@]}" >/dev/null
  fi
}
trap restart_stopped EXIT

restore_db() {
  local cid="$1" dbname="$2" dumpfile="$3"
  log "Restoring database '$dbname'"
  docker exec "$cid" psql -U postgres -d template1 -v ON_ERROR_STOP=1 -qc \
    "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$dbname' AND pid <> pg_backend_pid();" >/dev/null
  docker cp "$dumpfile" "$cid:/tmp/helix-restore.dump"
  docker exec "$cid" pg_restore -U postgres -d template1 \
    --create --clean --if-exists --no-owner --no-privileges --exit-on-error /tmp/helix-restore.dump
  docker exec "$cid" rm -f /tmp/helix-restore.dump
}

restore_db "$PG_ID" postgres "$BACKUP_DIR/postgres.dump"
if [[ -n "$KODIT_ID" ]]; then
  restore_db "$KODIT_ID" kodit "$BACKUP_DIR/kodit.dump"
fi

log "Restoring filestore volume ($FILESTORE_VOL)"
docker run --rm \
  -v "$FILESTORE_VOL":/filestore \
  -v "$BACKUP_DIR":/backup:ro \
  --entrypoint sh "$HELPER_IMAGE" \
  -c 'set -e
      find /filestore -mindepth 1 -maxdepth 1 -exec rm -rf {} \;
      tar xzf /backup/filestore.tar.gz -C /filestore'

if [[ "$WITH_ENV" == "1" && -f "$BACKUP_DIR/env" ]]; then
  if [[ -f .env ]] && cmp -s .env "$BACKUP_DIR/env"; then
    log ".env already matches the snapshot"
  else
    if [[ -f .env ]]; then
      cp .env ".env.pre-restore.$(date -u +%Y-%m-%dT%H-%M-%SZ)"
    fi
    cp "$BACKUP_DIR/env" .env
    log ".env restored (previous copy kept as .env.pre-restore.*)"
    warn "the api container keeps its original environment across stop/start;"
    warn "run 'docker compose -f docker-compose.dev.yaml up -d api' yourself if the .env changes must take effect"
  fi
fi
if [[ -f "$BACKUP_DIR/env.usercreds" ]]; then
  cp "$BACKUP_DIR/env.usercreds" .env.usercreds
fi

restart_stopped
trap - EXIT

log "Waiting for the api to come back"
for _ in $(seq 1 60); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/api/v1/config || true)"
  [[ "$code" == "200" ]] && break
  sleep 2
done
log "api /api/v1/config -> ${code:-no response}"

if [[ -f "$BACKUP_DIR/row-counts.txt" ]]; then
  log "Verifying row counts against the snapshot"
  docker exec "$PG_ID" psql -U postgres -d postgres -tAqc "$ROW_COUNT_SQL" > /tmp/helix-restore-rowcounts.txt
  if diff -u "$BACKUP_DIR/row-counts.txt" /tmp/helix-restore-rowcounts.txt > /tmp/helix-restore-rowdiff.txt; then
    log "row counts match exactly ($(wc -l < "$BACKUP_DIR/row-counts.txt") tables)"
  else
    warn "row counts differ from the snapshot (the api may have written on startup):"
    sed 's/^/  /' /tmp/helix-restore-rowdiff.txt >&2
  fi
fi

cat <<EOF

Restore complete.

Note: sandbox / desktop containers are not part of a snapshot. Rows in
'sandboxes', 'sandbox_instances' and 'sessions' may reference containers that no
longer exist; start a new session to get a live one.
EOF
