#!/bin/bash
# One-time conversion of a control-plane host from the single `api` service to
# blue/green (docker-compose.bluegreen.yaml + nginx). Run ON the host from the
# stack directory, after the release it names is already deployed in place:
#
#   scripts/bluegreen-cutover.sh --image REPO:TAG --nginx-conf FILE \
#       --health-host NAME [--compose-base FILE] [--yes]
#
#   --image         the image api-blue starts on: the release the host runs now
#                   (London: ghcr.io/helixml/controlplane:2.12.70; Meta:
#                   registry.helixml.tech/helix/controlplane:<sha7>-linux-amd64)
#   --nginx-conf    the host's new /etc/nginx/nginx.conf (helixml/infra), which
#                   includes /etc/nginx/helix-active/{stream,http}.conf and
#                   takes over the public API ports
#   --health-host   the host's public name (app.helix.ml, meta.helix.ml)
#   --compose-base  the compose file the colours extend (default
#                   docker-compose.yaml; Meta: docker-compose.dev.yaml)
#   --yes           skip the confirmation after the config diff
#
# The API is down from stopping `api` until nginx serves api-blue (the colour's
# boot, typically under a minute): the old process embeds NATS on the
# JetStream directory the external NATS takes over, so they never run at once.
# Every later deploy is zero-downtime (scripts/lib/bluegreen.sh). On failure
# the previous .env, nginx.conf and `api` are restored.
set -euo pipefail

usage() { sed -n '2,23p' "$0" >&2; exit 2; }
IMAGE='' NGINX_CONF='' HEALTH_HOST='' BASE=docker-compose.yaml YES=false
while [ $# -gt 0 ]; do
  case $1 in
    --image) IMAGE=$2; shift 2 ;;
    --nginx-conf) NGINX_CONF=$2; shift 2 ;;
    --health-host) HEALTH_HOST=$2; shift 2 ;;
    --compose-base) BASE=$2; shift 2 ;;
    --yes) YES=true; shift ;;
    *) usage ;;
  esac
done
[ -n "$IMAGE" ] && [ -n "$NGINX_CONF" ] && [ -n "$HEALTH_HOST" ] || usage
[[ "$IMAGE" == *:* ]] || { echo "--image needs a tag" >&2; exit 2; }

# shellcheck source-path=SCRIPTDIR source=lib/bluegreen.sh
. "$(dirname "$0")/lib/bluegreen.sh"
BG_SUDO=${BG_SUDO-sudo}
# shellcheck disable=SC2034 # read by lib/bluegreen.sh
BG_HEALTH_HOST=$HEALTH_HOST
die() { echo "cutover: ERROR: $*" >&2; exit 1; }
log() { echo "cutover: $*"; }

[ -f .env ] && [ -f "$BASE" ] && [ -f docker-compose.bluegreen.yaml ] ||
  die "run from the stack directory, which needs .env, $BASE and docker-compose.bluegreen.yaml"
! bg_enabled || die ".env already has HELIX_ACTIVE_COLOUR: this host is blue/green"
[ -z "$(bg_env_get COMPOSE_FILE)" ] || die ".env sets COMPOSE_FILE; merge it into the new value by hand"
NGINX_CONF=$(realpath "$NGINX_CONF")
grep -q 'helix-active/stream.conf' "$NGINX_CONF" ||
  die "$NGINX_CONF does not include /etc/nginx/helix-active/stream.conf"

# The host's compose files today, in the order docker compose loads them.
files=("$BASE")
case $BASE in
  docker-compose.yaml) [ ! -f docker-compose.override.yaml ] || files+=(docker-compose.override.yaml) ;;
  docker-compose.dev.yaml) [ "$(bg_env_get HELIX_VHOST_TLS_MODE)" != auto ] || files+=(docker-compose.tls.yaml) ;;
esac
old_files=$(IFS=:; echo "${files[*]}")
new_files="$old_files:docker-compose.bluegreen.yaml"
repo=${IMAGE%:*} tag=${IMAGE##*:}

# --- read-only: what changes for the API -----------------------------------
tmp=$(mktemp -d)
render() { # render COMPOSE_FILE SERVICE: the service's resolved config as JSON
  $BG_SUDO env COMPOSE_FILE="$1" HELIX_COMPOSE_BASE="$BASE" HELIX_CONTROLPLANE_IMAGE="$repo" \
    HELIX_VERSION_BLUE="$tag" docker compose --profile bluegreen config --format json |
    python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin)["services"][sys.argv[1]], indent=1, sort_keys=True))' "$2"
}
render "$old_files" api > "$tmp/api.json"
render "$new_files" api-blue > "$tmp/api-blue.json"
log "api (now) -> api-blue (resolved compose config; secrets are not printed):"
python3 - "$tmp/api.json" "$tmp/api-blue.json" <<'EOF'
import json, sys
old, new = (json.load(open(p)) for p in sys.argv[1:3])
for key in sorted(set(old) | set(new)):
    if key == "environment":
        oe, ne = old.get(key, {}), new.get(key, {})
        for k in sorted(set(oe) | set(ne)):
            if oe.get(k) != ne.get(k):
                show = lambda v: "<unset>" if v is None else ("<changed>" if any(s in k for s in ("TOKEN", "KEY", "SECRET", "PASSWORD")) else v)
                print(f"  env {k}: {show(oe.get(k))} -> {show(ne.get(k))}")
    elif old.get(key) != new.get(key):
        print(f"  {key}: {json.dumps(old.get(key))} -> {json.dumps(new.get(key))}")
EOF
log "pulling $IMAGE"
$BG_SUDO docker pull -q "$IMAGE" > /dev/null || die "cannot pull $IMAGE on this host"
if [ "$YES" != true ]; then
  read -r -p "cutover: the API goes down until api-blue serves. Continue? [y/N] " answer
  [ "$answer" = y ] || die "aborted"
fi

# --- mutations ------------------------------------------------------------------
nginx_conf=$($BG_SUDO nginx -V 2>&1 | sed -n 's/.*--conf-path=\([^ ]*\).*/\1/p')
nginx_conf=${nginx_conf:-/etc/nginx/nginx.conf}
stamp=$(date +%Y%m%d-%H%M%S)
$BG_SUDO cp -p .env "$tmp/env.prev"
$BG_SUDO cp -p .env ".env.pre-bluegreen-$stamp"
$BG_SUDO cp -p "$nginx_conf" "$tmp/nginx.conf.prev"
$BG_SUDO cp -p "$nginx_conf" "$nginx_conf.pre-bluegreen-$stamp"
log "backups: $PWD/.env.pre-bluegreen-$stamp, $nginx_conf.pre-bluegreen-$stamp"

# Meta's nginx is installed but stopped: the API bound :80/:443 itself.
nginx_load() {
  if $BG_SUDO pgrep -x nginx > /dev/null; then
    $BG_SUDO nginx -s reload
  elif [ -d /run/systemd/system ]; then
    $BG_SUDO systemctl enable --now nginx
  else
    $BG_SUDO nginx
  fi
}
stage=none
on_exit() {
  local status=$?
  if [ "$status" -ne 0 ] && [ "$stage" != none ]; then
    set +e
    echo "cutover: failed - restoring the single api" >&2
    [ "$stage" != stopped ] || bg_dc stop -t 60 api-blue nats
    $BG_SUDO cp -p "$tmp/nginx.conf.prev" "$nginx_conf" && $BG_SUDO nginx -t && nginx_load
    $BG_SUDO cp -p "$tmp/env.prev" .env
    $BG_SUDO env COMPOSE_FILE="$old_files" docker compose up -d --no-deps api && echo "cutover: the single api is back" >&2
  fi
  rm -rf "$tmp"
  exit "$status"
}
trap on_exit EXIT

stage='env'
bg_env_set COMPOSE_FILE "$new_files"
bg_env_set HELIX_COMPOSE_BASE "$BASE"
bg_env_set HELIX_CONTROLPLANE_IMAGE "$repo"
bg_env_set HELIX_VERSION_BLUE "$tag"
bg_env_set HELIX_ACTIVE_COLOUR blue
bg_dc pull --quiet nats
log "running migrations with $IMAGE (the running api keeps serving)"
timeout "$BG_MIGRATE_TIMEOUT" $BG_SUDO docker compose run --rm --no-deps -T api-blue --migrate-only

# nginx's new config, checked before anything stops (nginx -t binds nothing).
$BG_SUDO mkdir -p "$BG_NGINX_DIR"
bg_render_nginx blue "$tmp"
$BG_SUDO cp "$tmp/stream.conf" "$tmp/http.conf" "$BG_NGINX_DIR/"
$BG_SUDO nginx -t -c "$NGINX_CONF" || die "$NGINX_CONF does not pass nginx -t"

log "stopping the single api - the API is down from here"
down_at=$(date +%s)
stage=stopped
$BG_SUDO env COMPOSE_FILE="$old_files" docker compose stop -t 30 api
$BG_SUDO env COMPOSE_FILE="$old_files" docker compose rm -f api > /dev/null
bg_dc up -d --no-deps nats
bg_dc up -d --no-deps api-blue
bg_wait_ready blue || { bg_dc logs --tail 80 api-blue >&2; die "api-blue did not become ready"; }
$BG_SUDO cp "$NGINX_CONF" "$nginx_conf"
$BG_SUDO nginx -t
nginx_load
BG_WATCH_SECONDS=10 bg_watch blue || die "nginx does not reach api-blue"
stage='done'
log "api-blue is live on $tag; API was down for $(($(date +%s) - down_at))s"
log "every deploy from now on is blue/green"
