# shellcheck shell=bash disable=SC2086 # $BG_SUDO is "sudo" or empty: unquoted on purpose
# Blue/green API switch, sourced by the remote deploy scripts
# (deploy-controlplane-remote.sh for London, deploy-meta-remote.sh for Meta).
# design/2026-11-20-bluegreen-deploys.md.
#
# The API runs as api-blue or api-green (docker-compose.bluegreen.yaml). nginx
# owns the public ports and forwards them to the active colour through
# $BG_NGINX_DIR/{stream,http}.conf, which only this file writes. .env records
# HELIX_ACTIVE_COLOUR and each colour's HELIX_VERSION_<COLOUR>.
#
# bg_deploy VERSION:
#   1. run the new release's migrations in a one-off container (expand-only:
#      the running colour keeps working on the migrated schema),
#   2. start the idle colour on VERSION and wait until it answers readyz over
#      HTTP and TLS,
#   3. point nginx at it, then watch the public edge serve it for
#      $BG_WATCH_SECONDS,
#   4. drain the old colour (SIGTERM: websockets close at once and reconnect
#      to the new colour, requests get 45s) and remove its container.
# bg_rollback undoes whichever of those steps ran: the old colour serves
# again and the new one is removed.
#
# The caller sets, before sourcing or calling:
#   BG_SUDO          "sudo" or "" (prefix for docker, nginx and file writes)
#   BG_HEALTH_HOST   the host's public name (TLS SNI for the checks)
# and runs from the stack directory, whose .env names the compose files
# (COMPOSE_FILE) and colours.

BG_NGINX_DIR=${BG_NGINX_DIR:-/etc/nginx/helix-active}
BG_READY_TIMEOUT=${BG_READY_TIMEOUT:-180}
BG_WATCH_SECONDS=${BG_WATCH_SECONDS:-30}
BG_MIGRATE_TIMEOUT=${BG_MIGRATE_TIMEOUT:-900}
BG_DRAIN_SECONDS=${BG_DRAIN_SECONDS:-60}
BG_POLL_SECONDS=${BG_POLL_SECONDS:-2}
# Container ports nginx forwards; each colour publishes them on 127.0.0.1 at
# port + 10000 (blue) or + 20000 (green).
BG_PORTS="80 443 2224 8080 9110"

# Progress of bg_deploy, read by bg_rollback:
#   "" nothing changed; started: the new colour exists; switched: nginx points
#   at it; removed: the old colour is gone.
BG_STAGE=""
BG_OLD=""
BG_NEW=""

bg_log() { echo "bluegreen: $*"; }
bg_die() { echo "bluegreen: ERROR: $*" >&2; exit 1; }

bg_dc() { $BG_SUDO docker compose "$@"; }

bg_env_get() {
  $BG_SUDO grep -E "^$1=" .env | tail -1 | cut -d= -f2- | tr -d '"' || true
}

# Replaces KEY's line, or appends it; .env keeps its owner and mode.
bg_env_set() {
  if $BG_SUDO grep -qE "^$1=" .env; then
    $BG_SUDO sed -i "s|^$1=.*|$1=\"$2\"|" .env
  else
    printf '%s="%s"\n' "$1" "$2" | $BG_SUDO tee -a .env > /dev/null
  fi
}

bg_enabled() { $BG_SUDO grep -qE '^HELIX_ACTIVE_COLOUR=' .env; }

bg_other() { if [ "$1" = blue ]; then echo green; else echo blue; fi; }

bg_upper() { echo "$1" | tr '[:lower:]' '[:upper:]'; }

bg_port() { # bg_port COLOUR CONTAINER_PORT
  case $1 in
    blue) echo $(($2 + 10000)) ;;
    green) echo $(($2 + 20000)) ;;
    *) bg_die "unknown colour '$1'" ;;
  esac
}

# The colour whose readyz answered 200 (from X-Helix-Instance), or "".
bg_instance() { # bg_instance CURL_ARGS...
  curl -sS -o /dev/null -D - --max-time 5 "$@" 2> /dev/null |
    tr -d '\r' | awk '
      NR == 1 { ok = ($2 == "200") }
      tolower($1) == "x-helix-instance:" && ok { print $2 }
    '
}

bg_ready_http() { # the colour's own plain-HTTP port
  [ "$(bg_instance "http://127.0.0.1:$(bg_port "$1" 8080)/api/v1/readyz")" = "$1" ]
}

# Whether the API terminates TLS itself (certmagic). Without it the colours
# have no :443 and the edge is checked through nginx's :8080.
bg_tls() { [ "$(bg_env_get HELIX_VHOST_TLS_MODE)" = auto ]; }

bg_ready_tls() { # the colour's own TLS port, with the public name's certificate
  local port
  bg_tls || return 0
  port=$(bg_port "$1" 443)
  [ "$(bg_instance --resolve "$BG_HEALTH_HOST:$port:127.0.0.1" "https://$BG_HEALTH_HOST:$port/api/v1/readyz")" = "$1" ]
}

bg_edge() { # the colour the public edge (nginx) reaches
  if bg_tls; then
    bg_instance --resolve "$BG_HEALTH_HOST:443:127.0.0.1" "https://$BG_HEALTH_HOST/api/v1/readyz"
  else
    bg_instance -H "Host: $BG_HEALTH_HOST" "http://127.0.0.1:8080/api/v1/readyz"
  fi
}

# Waits until COLOUR passes both checks three times in a row.
bg_wait_ready() {
  local colour=$1 deadline passes=0
  deadline=$(($(date +%s) + BG_READY_TIMEOUT))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if bg_ready_http "$colour" && bg_ready_tls "$colour"; then
      passes=$((passes + 1))
      [ "$passes" -ge 3 ] && return 0
    else
      passes=0
    fi
    sleep "$BG_POLL_SECONDS"
  done
  return 1
}

bg_render_nginx() { # bg_render_nginx COLOUR DIR
  local colour=$1 dir=$2 port
  {
    echo "# active: $colour"
    echo "# Written by the Helix blue/green deploy (scripts/lib/bluegreen.sh). Do not edit."
    echo "map \$server_port \$helix_backend {"
    for port in $BG_PORTS; do
      echo "    $port 127.0.0.1:$(bg_port "$colour" "$port");"
    done
    echo "}"
  } > "$dir/stream.conf"
  {
    echo "# active: $colour"
    echo "# Written by the Helix blue/green deploy (scripts/lib/bluegreen.sh). Do not edit."
    echo "upstream helix_http { server 127.0.0.1:$(bg_port "$colour" 8080); }"
  } > "$dir/http.conf"
}

bg_nginx_colour() {
  $BG_SUDO sed -n 's/^# active: //p' "$BG_NGINX_DIR/stream.conf" 2> /dev/null | head -1
}

# Points nginx at COLOUR. An invalid config is never loaded: the previous
# files are put back.
bg_switch() {
  local colour=$1 staged f
  staged=$(mktemp -d)
  bg_render_nginx "$colour" "$staged"
  $BG_SUDO mkdir -p "$BG_NGINX_DIR"
  for f in stream.conf http.conf; do
    $BG_SUDO cp -p "$BG_NGINX_DIR/$f" "$staged/$f.prev" 2> /dev/null || true
    $BG_SUDO cp "$staged/$f" "$BG_NGINX_DIR/$f"
  done
  if ! $BG_SUDO nginx -t > "$staged/nginx-t" 2>&1; then
    cat "$staged/nginx-t" >&2
    for f in stream.conf http.conf; do
      [ -f "$staged/$f.prev" ] && $BG_SUDO cp -p "$staged/$f.prev" "$BG_NGINX_DIR/$f"
    done
    rm -r "$staged"
    return 1
  fi
  rm -r "$staged"
  $BG_SUDO nginx -s reload
}

# The edge must reach COLOUR on every poll for $BG_WATCH_SECONDS, allowing one
# miss in a row (a reload's in-flight handover).
bg_watch() {
  local colour=$1 deadline misses=0 seen
  deadline=$(($(date +%s) + BG_WATCH_SECONDS))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    seen=$(bg_edge)
    if [ "$seen" = "$colour" ]; then
      misses=0
    else
      misses=$((misses + 1))
      if [ "$misses" -ge 2 ]; then
        echo "bluegreen: the edge reached '${seen:-nothing}' instead of $colour" >&2
        return 1
      fi
    fi
    sleep "$BG_POLL_SECONDS"
  done
}

bg_remove() { # SIGTERM, wait for the drain, remove the container
  bg_dc stop -t "$BG_DRAIN_SECONDS" "api-$1"
  bg_dc rm -f "api-$1" > /dev/null
}

# Read-only: works out the colours and refuses a host whose nginx and .env
# disagree. Sets BG_OLD and BG_NEW.
bg_preflight() {
  BG_OLD=$(bg_env_get HELIX_ACTIVE_COLOUR)
  case "$BG_OLD" in blue | green) ;; *) bg_die "HELIX_ACTIVE_COLOUR is '$BG_OLD', not blue or green" ;; esac
  BG_NEW=$(bg_other "$BG_OLD")
  [ "$(bg_nginx_colour)" = "$BG_OLD" ] ||
    bg_die "nginx ($BG_NGINX_DIR) points at '$(bg_nginx_colour)' but .env says $BG_OLD is active; fix by hand before deploying"
  if [ "$(bg_edge)" = "$BG_OLD" ]; then
    bg_log "$BG_OLD is serving; deploying to $BG_NEW"
  else
    echo "bluegreen: WARNING: the edge does not reach a ready $BG_OLD; deploying to $BG_NEW anyway" >&2
  fi
}

# bg_deploy IMAGE_TAG: bg_preflight must have run. Exits non-zero on failure,
# leaving BG_STAGE for bg_rollback.
bg_deploy() {
  local tag=$1 var
  var="HELIX_VERSION_$(bg_upper "$BG_NEW")"
  bg_env_set "$var" "$tag"
  bg_log "pulling api-$BG_NEW ($tag)"
  bg_dc pull --quiet "api-$BG_NEW"
  # A colour left over from an interrupted deploy is replaced, not reused.
  bg_dc rm -f -s "api-$BG_NEW" > /dev/null 2>&1 || true

  bg_log "migrating the database with $tag"
  timeout "$BG_MIGRATE_TIMEOUT" $BG_SUDO docker compose run --rm --no-deps -T "api-$BG_NEW" --migrate-only ||
    bg_die "migrations failed or exceeded ${BG_MIGRATE_TIMEOUT}s"

  BG_STAGE=started
  bg_dc up -d --no-deps "api-$BG_NEW"
  bg_log "waiting for api-$BG_NEW to be ready (up to ${BG_READY_TIMEOUT}s$(bg_tls && echo ', HTTP and TLS'))"
  bg_wait_ready "$BG_NEW" || {
    bg_dc logs --tail 50 "api-$BG_NEW" >&2 || true
    bg_die "api-$BG_NEW did not become ready"
  }

  bg_log "switching nginx from $BG_OLD to $BG_NEW"
  bg_switch "$BG_NEW" || bg_die "nginx rejected the $BG_NEW config; still on $BG_OLD"
  BG_STAGE=switched
  bg_env_set HELIX_ACTIVE_COLOUR "$BG_NEW"
  bg_watch "$BG_NEW" || bg_die "the public edge did not serve $BG_NEW steadily"

  bg_log "draining and removing api-$BG_OLD"
  bg_remove "$BG_OLD"
  BG_STAGE=removed
  bg_log "api-$BG_NEW is live on $tag"
}

# Puts BG_OLD back in front. Run after .env holds the old colour's version
# again (the caller restores it). Returns non-zero if it could not.
bg_rollback() {
  local ok=0
  [ -n "$BG_STAGE" ] || return 0
  bg_log "rolling back to api-$BG_OLD (stage: $BG_STAGE)"
  if [ "$BG_STAGE" = removed ]; then
    if ! { bg_dc up -d --no-deps "api-$BG_OLD" && bg_wait_ready "$BG_OLD"; }; then
      echo "bluegreen: ROLLBACK FAILED - api-$BG_OLD did not come back" >&2
      return 1
    fi
  fi
  if [ "$BG_STAGE" = switched ] || [ "$BG_STAGE" = removed ]; then
    bg_switch "$BG_OLD" || {
      echo "bluegreen: ROLLBACK FAILED - could not point nginx back at $BG_OLD" >&2
      ok=1
    }
  fi
  bg_dc stop -t "$BG_DRAIN_SECONDS" "api-$BG_NEW" || ok=1
  bg_dc rm -f "api-$BG_NEW" > /dev/null || ok=1
  [ "$ok" = 0 ] && bg_log "api-$BG_OLD is serving again"
  return "$ok"
}
