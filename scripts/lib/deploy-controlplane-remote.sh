#!/bin/bash
# Runs ON the control-plane host, fed over `ssh host bash -s` by
# scripts/deploy-prod.sh, which prepends these assignments:
#   STACK_DIR VERSION SNAP HEALTH_HOST COMPOSE_SYNC RECREATE_POSTGRES REPO_COMPOSE_B64
#
# Plan (read-only): diff the stack's docker-compose.yaml against the one being
# deployed, compare each service's running image with the one it will get,
# refuse unsafe database changes, and confirm every image the deploy pulls is
# pullable. Only
# then mutate: snapshot the DB, optionally install the new compose file, bump
# HELIX_VERSION, pull and converge every non-database service, health-check.
# Any failure after the snapshot restores the exact prior .env and compose file
# and reconverges the previous services.
#
# Everything runs inside main() with stdin from /dev/null, so bash has read the
# whole script before any command can consume the ssh stdin it arrives on.

# Database services hold data a recreate can strand: they are created or
# recreated only with RECREATE_POSTGRES=true, and their target image must match
# the major of the data they run on (PG_VERSION), whatever .env says. A service
# is a database by name, or by a PostgreSQL-family image.
DB_SERVICES="postgres vectorchord-kodit"

main() {
  set -euo pipefail
  : "${STACK_DIR:?}" "${VERSION:?}" "${SNAP:?}" "${HEALTH_HOST:?}" "${REPO_COMPOSE_B64:?}"

  die() { echo "controlplane: ERROR: $*" >&2; exit 1; }
  image_of() { awk -v s="$1" '$1 == s { print $2 }' "$2"; }
  # $tmp/running holds "service container image" for each running container.
  running_cid() { awk -v s="$1" '$1 == s { print $2 }' "$tmp/running"; }
  running_image() { awk -v s="$1" '$1 == s { print $3 }' "$tmp/running"; }
  is_db() {
    local img
    case " $DB_SERVICES " in *" $1 "*) return 0 ;; esac
    for img in "$(image_of "$1" "$tmp/old")" "$(image_of "$1" "$tmp/new")" "$(running_image "$1")"; do
      img=${img##*/}
      case "${img%%[:@]*}" in postgres | postgresql | pgvector | vchord* | vectorchord*) return 0 ;; esac
    done
    return 1
  }
  held_db() { is_db "$1" && [ "$RECREATE_POSTGRES" != "true" ]; }

  # "service image" lines from compose's canonical YAML. The rendered config
  # carries interpolated secrets, so it is only ever piped, never printed.
  service_images() {
    "$@" config | awk '
      /^[^ ]/            { in_services = ($0 == "services:"); next }
      in_services && /^  [^ ]/ { svc = $1; sub(/:$/, "", svc); next }
      in_services && /^    image: / { img = $0; sub(/^    image: /, "", img); print svc, img }
    ' | sort
  }

  pg_major() {
    local tag=${1##*:}
    case "$tag" in
      [0-9]*) echo "${tag%%[!0-9]*}" ;;
      pg[0-9]*) tag=${tag#pg}; echo "${tag%%[!0-9]*}" ;;
      *) die "cannot read a PostgreSQL major from image $1" ;;
    esac
  }

  # ghcr.io/<owner>/<name>:<tag> is GitHub package <owner>/<name>.
  describe_image() {
    local ref=${1%%@*}
    ref=${ref%:*}
    case "$ref" in
      ghcr.io/*/*) echo "$1 (GitHub package ${ref#ghcr.io/}: private, missing tag, or this host is not logged in to ghcr.io)" ;;
      *) echo "$1" ;;
    esac
  }

  # Once the snapshot is taken, every non-zero exit lands here, including a
  # HUP/TERM/INT/PIPE from a dropped SSH session (see the signal traps below).
  on_exit() {
    local status=$?
    if [ "$applying" = "true" ] && [ "$status" -ne 0 ]; then
      if ! rollback; then
        echo "controlplane: previous docker-compose.yaml and .env kept in $tmp" >&2
        exit 43
      fi
    fi
    rm -r "$tmp"
    exit "$status"
  }

  # Returns non-zero when the host could not be put back; the files are then
  # left as they are rather than reconverged against a half-restored config.
  rollback() {
    set +e
    trap '' HUP TERM INT PIPE
    echo "controlplane: deploy failed after the snapshot - rolling back to ${old_version:-<none>}"
    if ! { sudo cp -p "$tmp/compose.prev" docker-compose.yaml && sudo cp -p "$tmp/env.prev" .env; }; then
      echo "controlplane: ROLLBACK FAILED - could not restore docker-compose.yaml/.env; services left untouched (snapshot $SNAP)" >&2
      return 1
    fi
    echo "controlplane: restored the previous docker-compose.yaml and .env"
    if ! sudo docker compose up -d --no-deps "${restore[@]}"; then
      echo "controlplane: ROLLBACK FAILED - previous services did not start (snapshot $SNAP)" >&2
      return 1
    fi
    echo "DB snapshot for manual restore if a migration was incompatible: $SNAP"
  }

  cd "$STACK_DIR"
  [ -f docker-compose.yaml ] || die "$STACK_DIR/docker-compose.yaml not found"
  sudo grep -qE '^HELIX_VERSION=' .env || die "$STACK_DIR/.env has no HELIX_VERSION line"

  tmp=$(mktemp -d)
  applying=false
  trap on_exit EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 141' PIPE
  trap 'exit 143' TERM
  printf '%s' "$REPO_COMPOSE_B64" | base64 -d > "$tmp/docker-compose.yaml"

  local override=() current
  [ -f docker-compose.override.yaml ] && override=(-f docker-compose.override.yaml)
  current=(sudo docker compose --project-directory . -f docker-compose.yaml "${override[@]}")
  service_images "${current[@]}" > "$tmp/old"
  service_images sudo env HELIX_VERSION="$VERSION" docker compose --project-directory . \
    -f "$tmp/docker-compose.yaml" "${override[@]}" > "$tmp/new"
  if [ ! -s "$tmp/old" ] || [ ! -s "$tmp/new" ]; then die "could not resolve service images"; fi

  # What runs now, not what the host's compose renders: .env may have been
  # edited since the containers were created.
  local svc cid
  : > "$tmp/running"
  while read -r svc _; do
    cid=$("${current[@]}" ps -q "$svc" | head -1)
    [ -n "$cid" ] || continue
    printf '%s %s %s\n' "$svc" "$cid" "$(sudo docker inspect --format '{{.Config.Image}}' "$cid")" >> "$tmp/running"
  done < "$tmp/old"

  old_version=$(sudo grep -E '^HELIX_VERSION=' .env | head -1 | cut -d= -f2- | tr -d '"')
  echo "controlplane: ${old_version:-<none>} -> $VERSION"

  local drift=false
  sudo cmp -s docker-compose.yaml "$tmp/docker-compose.yaml" || drift=true

  # --- image-pin diff: running -> deploying ------------------------------------
  local cur new rendered note data target db_changed=()
  echo "controlplane: image pins (running -> deploying):"
  while read -r svc; do
    cur=$(running_image "$svc")
    new=$(image_of "$svc" "$tmp/new")
    rendered=$(image_of "$svc" "$tmp/old")
    note=""
    [ -z "$cur" ] || [ -z "$rendered" ] || [ "$rendered" = "$cur" ] || note=" [host compose now renders $rendered]"
    if [ -z "$new" ]; then
      echo "  $svc: ${cur:-<not running>} -> <removed from compose; its container is left running>$note"
      continue
    elif [ "$cur" = "$new" ]; then
      echo "  $svc: $cur (unchanged)$note"
    else
      echo "  $svc: ${cur:-<not running>} -> $new$note"
      # A stopped database the host already defines is held (or refused below).
      if is_db "$svc" && { [ -n "$cur" ] || [ -z "$rendered" ]; }; then db_changed+=("$svc"); fi
    fi
    is_db "$svc" || continue
    # The data decides the major: a running database must stay on it.
    if [ -n "$cur" ]; then
      cid=$(running_cid "$svc")
      data=$(sudo docker exec "$cid" sh -c 'cat "$PGDATA/PG_VERSION"') ||
        die "cannot read PG_VERSION from $svc ($cid)"
      target=$(pg_major "$new")
      [ "$data" = "$target" ] ||
        die "$svc data is PostgreSQL $data but would run $new; a major change is a dump/restore (docs/postgres-upgrade.md), never a deploy"
    elif [ -n "$rendered" ] && [ "$RECREATE_POSTGRES" = "true" ]; then
      die "$svc is defined on this host but not running, so its data major is unknown; start it on its current image before deploying with RECREATE_POSTGRES=true"
    fi
  done < <(cut -d' ' -f1 "$tmp/old" "$tmp/new" "$tmp/running" | sort -u)

  if [ "$drift" = "true" ]; then
    echo "controlplane: $STACK_DIR/docker-compose.yaml differs from the $VERSION docker-compose.yaml" \
      "($(diff docker-compose.yaml "$tmp/docker-compose.yaml" | grep -c '^[<>]' || true) lines)"
    [ "$COMPOSE_SYNC" = "true" ] ||
      die "compose drift: review the image pins above, then re-run with COMPOSE_SYNC=true to install the $VERSION compose file"
  fi
  if [ "${#db_changed[@]}" -gt 0 ] && [ "$RECREATE_POSTGRES" != "true" ]; then
    die "database service image change or creation for ${db_changed[*]}; re-run with RECREATE_POSTGRES=true (it runs after the ZFS snapshot)"
  fi

  # Converge every service except the held databases (compose recreates only
  # those whose image or configuration changed); roll back to the previous set.
  local targets=()
  restore=()
  while read -r svc _; do
    held_db "$svc" || targets+=("$svc")
  done < "$tmp/new"
  while read -r svc _; do
    held_db "$svc" || restore+=("$svc")
  done < "$tmp/old"

  # --- every image the deploy pulls must be pullable before anything changes ---
  local unpullable=()
  for svc in "${targets[@]}"; do
    new=$(image_of "$svc" "$tmp/new")
    sudo docker manifest inspect "$new" > /dev/null 2>&1 || unpullable+=("$(describe_image "$new")")
  done
  if [ "${#unpullable[@]}" -gt 0 ]; then
    printf 'controlplane: not pullable on this host:\n' >&2
    printf '  %s\n' "${unpullable[@]}" >&2
    die "aborting before any change"
  fi

  # --- mutations ----------------------------------------------------------------
  # Byte-for-byte copies (owner and mode kept) for the rollback path.
  sudo cp -p .env "$tmp/env.prev"
  sudo cp -p docker-compose.yaml "$tmp/compose.prev"
  # Crash-consistent DB snapshot for rollback (CHECKPOINT first to minimise WAL replay).
  local pg_cid
  pg_cid=$(running_cid postgres)
  [ -z "$pg_cid" ] || sudo docker exec "$pg_cid" psql -U postgres -c 'CHECKPOINT;' > /dev/null 2>&1 || true
  sudo zfs snapshot "$SNAP"
  applying=true

  if [ "$drift" = "true" ]; then
    local backup
    backup="docker-compose.yaml.bak-$(date +%Y%m%d-%H%M%S)"
    sudo cp -p docker-compose.yaml "$backup"
    # Writing through the existing inode keeps its owner and mode.
    # shellcheck disable=SC2024 # the new file is ours to read; sudo is for the write
    sudo tee docker-compose.yaml < "$tmp/docker-compose.yaml" > /dev/null
    echo "controlplane: installed the $VERSION docker-compose.yaml (previous kept as $STACK_DIR/$backup)"
  fi

  # The api and chrome services share this secret; only this host needs it,
  # so it is generated here and never leaves the box. Appending keeps .env's mode.
  if grep -q 'CHROME_LAUNCHER_TOKEN' docker-compose.yaml && ! sudo grep -qE '^CHROME_LAUNCHER_TOKEN="?[^"]' .env; then
    sudo sed -i -E '/^CHROME_LAUNCHER_TOKEN=("")?$/d' .env
    sudo sh -c 'printf "CHROME_LAUNCHER_TOKEN=%s\n" "$(od -An -tx1 -N32 /dev/urandom | tr -d " \n")" >> .env'
    echo "controlplane: generated CHROME_LAUNCHER_TOKEN in $STACK_DIR/.env"
  fi

  sudo sed -i "s/^HELIX_VERSION=.*/HELIX_VERSION=\"$VERSION\"/" .env

  echo "controlplane: converging ${targets[*]}"
  for svc in "${targets[@]}"; do
    sudo docker compose pull "$svc"
  done
  sudo docker compose up -d --no-deps "${targets[@]}"

  local ok=0 code
  for _ in $(seq 1 36); do
    code=$(curl -s -o /dev/null -w '%{http_code}' "https://$HEALTH_HOST/" --resolve "$HEALTH_HOST:443:127.0.0.1" --max-time 10 || echo 000)
    if [ "$code" = "200" ]; then ok=1; break; fi
    sleep 5
  done
  if [ "$ok" != "1" ]; then
    echo "HEALTHCHECK FAILED"
    exit 42
  fi
  applying=false
  echo "controlplane healthy on $VERSION"
}

main < /dev/null
