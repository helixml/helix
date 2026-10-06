#!/usr/bin/env bash
# Shared helpers for backup-dev-state.sh / restore-dev-state.sh.
# Sourced, not executed.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$REPO_ROOT/docker-compose.dev.yaml"
DEFAULT_BACKUP_ROOT="$REPO_ROOT/dev-state-backups"

# Row-count snapshot query (exact counts for every table in the public schema).
ROW_COUNT_SQL="
SELECT table_name || '=' || (xpath('/row/c/text()', x))[1]::text
FROM (
  SELECT table_name,
         query_to_xml(format('SELECT count(*) AS c FROM %I.%I', table_schema, table_name),
                      false, true, '') AS x
  FROM information_schema.tables
  WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
) t
ORDER BY table_name;"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[fatal]\033[0m %s\n' "$*" >&2; exit 1; }

compose() { docker compose -f "$COMPOSE_FILE" "$@"; }

# Resolve a compose service to a container id. Empty if the service is not created.
container_id() {
  compose ps -aq "$1" 2>/dev/null | head -n1
}

require_container() {
  local svc="$1" id
  id="$(container_id "$svc")"
  [[ -n "$id" ]] || die "compose service '$svc' has no container. Start the stack first: ./stack start"
  printf '%s' "$id"
}

# Name of the named volume bound at $2 inside container $1.
volume_at() {
  docker inspect "$1" --format \
    "{{range .Mounts}}{{if eq .Destination \"$2\"}}{{.Name}}{{end}}{{end}}" 2>/dev/null
}

# A small image guaranteed to be present because the stack runs it, used for
# volume tar/untar so we never depend on pulling alpine.
helper_image() {
  docker inspect "$(require_container postgres)" --format '{{.Config.Image}}'
}

confirm() {
  local prompt="$1"
  if [[ "${ASSUME_YES:-0}" == "1" ]]; then
    log "--yes given, skipping confirmation"
    return 0
  fi
  [[ -t 0 ]] || die "not a tty and --yes not given; refusing to run destructively"
  read -r -p "$prompt [type 'yes' to continue] " reply
  [[ "$reply" == "yes" ]] || die "aborted by user"
}
