#!/usr/bin/env bash
# Snapshot the full state of the local dev stack (docker-compose.dev.yaml) so it
# can be reapplied later with scripts/restore-dev-state.sh.
#
# Captured:
#   postgres.dump      main application DB (users, orgs, bots, projects, sessions, api keys)
#   kodit.dump         vectorchord-kodit code-index DB (skip with --no-kodit)
#   filestore.tar.gz   /filestore volume (git repos, avatars, NATS jetstream, workspaces)
#   env                .env  (secrets — the backup dir is gitignored)
#   env.usercreds      .env.usercreds, if present
#   sandbox-versions.txt, manifest.json, row-counts.txt
#
# NOT captured (ephemeral / rebuildable): running sandbox + desktop containers,
# sandbox-data, sandbox-docker-storage, hydra-data, registry-data, Go caches.
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/dev-state-common.sh"

OUT_DIR=""
WITH_KODIT=1
QUIESCE=0

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

  -o, --output DIR   Write the snapshot here (default: dev-state-backups/<timestamp>)
      --no-kodit     Skip the vectorchord-kodit code-index database
      --quiesce      Stop the api container for the duration of the dump so the
                     DB and the filestore are captured at the same instant
  -h, --help         Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output) OUT_DIR="$2"; shift 2 ;;
    --no-kodit)  WITH_KODIT=0; shift ;;
    --quiesce)   QUIESCE=1; shift ;;
    -h|--help)   usage; exit 0 ;;
    *)           usage >&2; die "unknown argument: $1" ;;
  esac
done

cd "$REPO_ROOT"

STAMP="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
[[ -n "$OUT_DIR" ]] || OUT_DIR="$DEFAULT_BACKUP_ROOT/$STAMP"
mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"

PG_ID="$(require_container postgres)"
API_ID="$(require_container api)"
FILESTORE_VOL="$(volume_at "$API_ID" /filestore)"
[[ -n "$FILESTORE_VOL" ]] || die "could not resolve the /filestore volume from the api container"
HELPER_IMAGE="$(helper_image)"

restore_api=0
cleanup() {
  if [[ "$restore_api" == "1" ]]; then
    log "Restarting api"
    compose start api >/dev/null
  fi
}
trap cleanup EXIT

if [[ "$QUIESCE" == "1" ]]; then
  log "Stopping api for a consistent snapshot"
  compose stop api >/dev/null
  restore_api=1
fi

log "Snapshot directory: $OUT_DIR"

log "Dumping main database (postgres)"
docker exec "$PG_ID" pg_dump -U postgres -Fc --no-owner --no-privileges postgres > "$OUT_DIR/postgres.dump"

log "Recording row counts"
docker exec "$PG_ID" psql -U postgres -d postgres -tAqc "$ROW_COUNT_SQL" > "$OUT_DIR/row-counts.txt"

KODIT_ID=""
if [[ "$WITH_KODIT" == "1" ]]; then
  KODIT_ID="$(container_id vectorchord-kodit)"
  if [[ -n "$KODIT_ID" ]]; then
    log "Dumping code-index database (kodit)"
    docker exec "$KODIT_ID" pg_dump -U postgres -Fc --no-owner --no-privileges kodit > "$OUT_DIR/kodit.dump"
  else
    warn "vectorchord-kodit is not running; skipping the code-index database"
  fi
fi

log "Archiving filestore volume ($FILESTORE_VOL)"
docker run --rm \
  -v "$FILESTORE_VOL":/filestore:ro \
  -v "$OUT_DIR":/backup \
  --entrypoint sh "$HELPER_IMAGE" \
  -c "tar czf /backup/filestore.tar.gz -C /filestore . && chown $(id -u):$(id -g) /backup/filestore.tar.gz"


log "Copying config"
[[ -f .env ]] && cp .env "$OUT_DIR/env" || warn ".env not found"
[[ -f .env.usercreds ]] && cp .env.usercreds "$OUT_DIR/env.usercreds" || true
[[ -f sandbox-versions.txt ]] && cp sandbox-versions.txt "$OUT_DIR/sandbox-versions.txt" || true

log "Writing manifest"
pg_version="$(docker exec "$PG_ID" postgres --version | awk '{print $3}')"
cat > "$OUT_DIR/manifest.json" <<EOF
{
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "host": "$(hostname)",
  "repo_root": "$REPO_ROOT",
  "git_branch": "$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)",
  "git_commit": "$(git rev-parse HEAD 2>/dev/null || echo unknown)",
  "git_dirty": $(if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then echo true; else echo false; fi),
  "compose_project": "$(docker inspect "$API_ID" --format '{{index .Config.Labels "com.docker.compose.project"}}')",
  "postgres_version": "$pg_version",
  "filestore_volume": "$FILESTORE_VOL",
  "includes_kodit": $(if [[ -f "$OUT_DIR/kodit.dump" ]]; then echo true; else echo false; fi),
  "quiesced": $(if [[ "$QUIESCE" == "1" ]]; then echo true; else echo false; fi)
}
EOF

if [[ -L "$DEFAULT_BACKUP_ROOT/latest" || -e "$DEFAULT_BACKUP_ROOT/latest" ]]; then
  rm -f "$DEFAULT_BACKUP_ROOT/latest"
fi
mkdir -p "$DEFAULT_BACKUP_ROOT"
ln -s "$OUT_DIR" "$DEFAULT_BACKUP_ROOT/latest"

log "Done. Contents:"
ls -lh "$OUT_DIR" | sed '1d'
printf '\nRestore with:\n  ./scripts/restore-dev-state.sh %s\n' "$OUT_DIR"
