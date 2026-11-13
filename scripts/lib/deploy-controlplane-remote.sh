#!/bin/bash
# Runs ON the control-plane host, fed over `ssh host bash -s` by
# scripts/deploy-prod.sh, which prepends these assignments:
#   STACK_DIR VERSION SNAP HEALTH_HOST COMPOSE_SYNC RECREATE_POSTGRES REPO_COMPOSE_B64
#
# Plan (read-only): diff the stack's docker-compose.yaml against the one being
# deployed, resolve each service's image before and after, refuse unsafe
# database changes, and confirm every image the deploy pulls is pullable. Only
# then mutate: snapshot the DB, optionally install the new compose file, bump
# HELIX_VERSION, pull and converge every non-database service, health-check.
# Any failure after the snapshot restores the exact prior .env and compose file
# and reconverges the previous services.
#
# Everything runs inside main() with stdin from /dev/null, so bash has read the
# whole script before any command can consume the ssh stdin it arrives on.

# Database services hold data a recreate can strand: they are created or
# recreated only with RECREATE_POSTGRES=true, and never across a PostgreSQL
# major. A service is a database by name, or by a PostgreSQL-family image.
DB_SERVICES="postgres vectorchord-kodit"

main() {
  set -euo pipefail
  : "${STACK_DIR:?}" "${VERSION:?}" "${SNAP:?}" "${HEALTH_HOST:?}" "${REPO_COMPOSE_B64:?}"

  die() { echo "controlplane: ERROR: $*" >&2; exit 1; }
  image_of() { awk -v s="$1" '$1 == s { print $2 }' "$2"; }
  is_db() {
    local img
    case " $DB_SERVICES " in *" $1 "*) return 0 ;; esac
    for img in "$(image_of "$1" "$tmp/old")" "$(image_of "$1" "$tmp/new")"; do
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

  # Once the snapshot is taken, every non-zero exit lands here.
  on_exit() {
    local status=$?
    if [ "$applying" = "true" ] && [ "$status" -ne 0 ]; then
      rollback
    fi
    rm -r "$tmp"
    exit "$status"
  }

  rollback() {
    set +e
    echo "controlplane: deploy failed after the snapshot - rolling back to ${old_version:-<none>}"
    sudo cp -p "$tmp/compose.prev" docker-compose.yaml &&
      sudo cp -p "$tmp/env.prev" .env &&
      echo "controlplane: restored the previous docker-compose.yaml and .env"
    sudo docker compose up -d --no-deps "${restore[@]}"
    echo "DB snapshot for manual restore if a migration was incompatible: $SNAP"
  }

  cd "$STACK_DIR"
  [ -f docker-compose.yaml ] || die "$STACK_DIR/docker-compose.yaml not found"
  sudo grep -qE '^HELIX_VERSION=' .env || die "$STACK_DIR/.env has no HELIX_VERSION line"

  tmp=$(mktemp -d)
  applying=false
  trap on_exit EXIT
  printf '%s' "$REPO_COMPOSE_B64" | base64 -d > "$tmp/docker-compose.yaml"

  local override=()
  [ -f docker-compose.override.yaml ] && override=(-f docker-compose.override.yaml)
  service_images sudo docker compose --project-directory . -f docker-compose.yaml "${override[@]}" > "$tmp/old"
  service_images sudo env HELIX_VERSION="$VERSION" docker compose --project-directory . \
    -f "$tmp/docker-compose.yaml" "${override[@]}" > "$tmp/new"
  if [ ! -s "$tmp/old" ] || [ ! -s "$tmp/new" ]; then die "could not resolve service images"; fi

  old_version=$(sudo grep -E '^HELIX_VERSION=' .env | head -1 | cut -d= -f2- | tr -d '"')
  echo "controlplane: ${old_version:-<none>} -> $VERSION"

  local drift=false
  sudo cmp -s docker-compose.yaml "$tmp/docker-compose.yaml" || drift=true

  # --- image-pin diff ---------------------------------------------------------
  local svc old new old_major new_major db_changed=()
  echo "controlplane: image pins (current -> deploying):"
  while read -r svc; do
    old=$(image_of "$svc" "$tmp/old")
    new=$(image_of "$svc" "$tmp/new")
    if [ "$old" = "$new" ]; then
      echo "  $svc: $old (unchanged)"
    elif [ -z "$new" ]; then
      echo "  $svc: $old -> <removed from compose; its container is left running>"
    else
      echo "  $svc: ${old:-<new service>} -> $new"
      if is_db "$svc"; then
        db_changed+=("$svc")
        if [ -n "$old" ]; then
          old_major=$(pg_major "$old")
          new_major=$(pg_major "$new")
          [ "$old_major" = "$new_major" ] ||
            die "$svc would change PostgreSQL major ($old -> $new); that is a dump/restore (docs/postgres-upgrade.md), never a deploy"
        fi
      fi
    fi
  done < <(cut -d' ' -f1 "$tmp/old" "$tmp/new" | sort -u)

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
  sudo docker exec helix-postgres-1 psql -U postgres -c 'CHECKPOINT;' > /dev/null 2>&1 || true
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
