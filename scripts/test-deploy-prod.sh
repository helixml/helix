#!/bin/bash
set -euo pipefail
umask 022

# Drives scripts/deploy-prod.sh against PATH-shimmed ssh/sudo/docker/zfs/curl.
# `docker compose config` runs for real, so image resolution is compose's own.

repo=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
real_sed=$(command -v sed)
real_docker=$(command -v docker)
trap 'rm -r "$work"' EXIT
mkdir -p "$work/bin" "$work/home"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cat > "$work/bin/ssh" <<'EOF'
#!/bin/bash
set -euo pipefail
host="${@: -2:1}"
remote="${@: -1}"
if [ "$remote" = "bash -s" ]; then
  remote=$(cat)
fi
printf '%s\n' "$host" >> "$SSH_HOSTS"
printf '%s\n---\n' "$remote" >> "$SSH_SCRIPTS"
if [ "${EXEC_REMOTE:-false}" = "true" ]; then
  bash -c "$remote"
fi
EOF

# Running containers come from $RUNNING ("service image PG_VERSION|-"); each
# runs as container ctr-<service>. These reads are not logged.
cat > "$work/bin/docker" <<'EOF'
#!/bin/bash
set -euo pipefail
if [ "${1:-}" = "compose" ]; then
  for arg in "$@"; do
    [ "$arg" = "config" ] && exec "$REAL_DOCKER" "$@"
    if [ "$arg" = "ps" ]; then
      awk -v s="${*: -1}" '$1 == s { print "ctr-" s }' "$RUNNING"
      exit 0
    fi
  done
fi
if [ "${1:-} ${2:-} ${3:-}" = "inspect --format {{.Config.Image}}" ]; then
  awk -v s="${4#ctr-}" '$1 == s { print $2 }' "$RUNNING"
  exit 0
fi
if [ "${1:-} ${2:-}" = "volume inspect" ]; then
  case " ${EXISTING_VOLUMES:-} " in *" $3 "*) exit 0 ;; esac
  exit 1
fi
if [ "${1:-}" = "exec" ] && [ "${3:-}" = "sh" ]; then
  version=$(awk -v s="${2#ctr-}" '$1 == s { print $3 }' "$RUNNING")
  [ -n "$version" ] && [ "$version" != "-" ] || exit 1
  echo "$version"
  exit 0
fi
printf '%s\n' "$*" >> "$DOCKER_CALLS"
if [ "${1:-} ${2:-}" = "manifest inspect" ]; then
  case " ${UNPULLABLE:-} " in *" $3 "*) exit 1 ;; esac
fi
if [ "${1:-} ${2:-}" = "compose pull" ] && [ "${3:-}" = "${FAIL_PULL_SERVICE:-}" ]; then
  exit 1
fi
if [ "${1:-} ${2:-}" = "compose pull" ] && [ "${3:-}" = "${TERM_ON_PULL_SERVICE:-}" ]; then
  kill -TERM "$PPID"
  exit 0
fi
if [ "${1:-} ${2:-}" = "compose up" ] && [ -n "${FAIL_UP_ONCE:-}" ] && [ ! -e "$FAIL_UP_ONCE" ]; then
  : > "$FAIL_UP_ONCE"
  [ "${BREAK_ENV_ON_UP_FAILURE:-false}" = "true" ] && : > "$FAIL_UP_ONCE.restore"
  exit 1
fi
if [ "${1:-}" = "pull" ] && [ "${FAIL_PULL:-false}" = "true" ]; then
  exit 1
fi
if [ "$*" = "ps -aq" ] && [ "${FAIL_CLEANUP:-false}" = "true" ]; then
  exit 1
fi
if [ "$*" = "image ls --no-trunc --format {{.Repository}}\\t{{.Tag}}\\t{{.ID}} ghcr.io/helixml/helix-sandbox" ]; then
  printf 'ghcr.io/helixml/helix-sandbox\t9.8.7\tsha256:current\n'
  printf 'ghcr.io/helixml/helix-sandbox\t9.8.6\tsha256:rollback\n'
  printf 'ghcr.io/helixml/helix-sandbox\t9.8.5\tsha256:obsolete\n'
fi
if [ "$*" = "container inspect helix-sandbox-app" ] && [ "${CONTAINER_EXISTS:-true}" = "false" ]; then
  exit 1
fi
EOF

cat > "$work/bin/sed" <<'EOF'
#!/bin/bash
set -euo pipefail
if [ "$(uname -s)" = "Darwin" ] && [ "${1:-}" = "-i" ]; then
  shift
  exec "$REAL_SED" -i '' "$@"
fi
exec "$REAL_SED" "$@"
EOF

cat > "$work/bin/sudo" <<'EOF'
#!/bin/bash
# BREAK_ENV_ON_UP_FAILURE: once compose up has failed, restoring .env fails.
if [ "${1:-} ${2:-} ${4:-}" = "cp -p .env" ] && [ -e "${FAIL_UP_ONCE:-/nonexistent}.restore" ]; then
  exit 1
fi
exec "$@"
EOF

cat > "$work/bin/zfs" <<'EOF'
#!/bin/bash
printf '%s\n' "$*" >> "$ZFS_CALLS"
EOF

# Health-check requests (--resolve to 127.0.0.1) answer HEALTH_CODE; the public
# pprof edge check gets a 404.
cat > "$work/bin/curl" <<'EOF'
#!/bin/bash
for arg in "$@"; do
  if [ "$arg" = "--resolve" ]; then
    printf '%s' "${HEALTH_CODE:-200}"
    exit 0
  fi
done
printf 'not found\n404'
EOF

cat > "$work/bin/sleep" <<'EOF'
#!/bin/bash
EOF
chmod +x "$work/bin/"*

# A checkout of release 9.8.7: the script deploys the compose file it ships with.
src="$work/src"
mkdir -p "$src/scripts/lib"
cp "$repo/scripts/deploy-prod.sh" "$repo/scripts/cleanup-runner-images.sh" "$src/scripts/"
cp "$repo/scripts/lib/deploy-controlplane-remote.sh" "$src/scripts/lib/"
cp "$repo/docker-compose.yaml" "$src/"
git -C "$src" init -q
git -C "$src" add -A
git -C "$src" -c user.name=t -c user.email=t@example.com commit -qm release
git -C "$src" tag 9.8.7

run_deploy() {
  local version=$1
  shift
  [ "${running_pinned:-false}" = "true" ] || render_running
  env -u HELIX_CD_SSH_KEY -u SLACK_WEBHOOK_URL -u LONDON_SSH \
    -u RUNNER_SSH -u RUNNER_UPGRADE_SCRIPT -u RUNNER_START_SCRIPT \
    -u RUNNER_CONTAINER -u DEPLOY_CONTROLPLANE -u DEPLOY_RUNNER \
    -u COMPOSE_SYNC -u RECREATE_POSTGRES \
    PATH="$work/bin:$PATH" HOME="$work/home" TMPDIR="$work" REAL_SED="$real_sed" \
    REAL_DOCKER="$real_docker" SSH_HOSTS="$work/ssh-hosts" SSH_SCRIPTS="$work/ssh-scripts" \
    DOCKER_CALLS="$work/docker-calls" ZFS_CALLS="$work/zfs-calls" RUNNING="$work/running" \
    LONDON_STACK_DIR="$stack" "$@" "$src/scripts/deploy-prod.sh" "$version"
}

# Unless a test pins $work/running, the host runs exactly what its compose
# file and .env render, on data of each PostgreSQL image's major.
render_running() {
  local override=() svc img tag
  : > "$work/running"
  [ -f "$stack/docker-compose.yaml" ] || return 0
  [ -f "$stack/docker-compose.override.yaml" ] && override=(-f "$stack/docker-compose.override.yaml")
  "$real_docker" compose --project-directory "$stack" -f "$stack/docker-compose.yaml" "${override[@]}" config |
    awk '
      /^[^ ]/ { in_services = ($0 == "services:"); next }
      in_services && /^  [^ ]/ { svc = $1; sub(/:$/, "", svc); next }
      in_services && /^    image: / { img = $0; sub(/^    image: /, "", img); print svc, img }
    ' | while read -r svc img; do
      tag=${img##*:}
      tag=${tag#pg}
      case "$img" in
        *postgres* | *pgvector* | *vchord*) echo "$svc $img ${tag%%[!0-9]*}" ;;
        *) echo "$svc $img -" ;;
      esac
    done > "$work/running"
}

reset_calls() {
  running_pinned=false
  : > "$work/ssh-hosts"
  : > "$work/ssh-scripts"
  : > "$work/docker-calls"
  : > "$work/zfs-calls"
}

# --- control plane --------------------------------------------------------------
stack="$work/stack"

# An older compose file of the shape still running on London: rod's chrome
# image, an older searxng, the release's vectorchord-kodit (kept last).
# old_compose <postgres image>
old_compose() {
  cat <<EOF
services:
  api:
    image: ghcr.io/helixml/controlplane:\${HELIX_VERSION:-latest}
    env_file:
      - .env
  postgres:
    image: $1
  searxng:
    image: searxng/searxng:2025.7.24-1b4c5ab
  chrome:
    image: ghcr.io/go-rod/rod:v0.115.0
  vectorchord-kodit:
    image: tensorchord/vchord-suite:pg17-20250601
EOF
}

# setup_stack <compose source file> [postgres default] — .env pins postgres 12.
setup_stack() {
  rm -rf "$stack"
  mkdir -p "$stack"
  cp "$1" "$stack/docker-compose.yaml"
  chmod 664 "$stack/docker-compose.yaml"
  printf 'services:\n  api:\n    ports:\n      - "9090:9090"\n' > "$stack/docker-compose.override.yaml"
  printf 'HELIX_VERSION="9.8.6"\nHELIX_POSTGRES_IMAGE=postgres:12.13-alpine\n' > "$stack/.env"
  chmod 600 "$stack/.env"
}

mode_of() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }
compose_calls() { grep '^compose ' "$work/docker-calls" || true; }

old_compose "\${HELIX_POSTGRES_IMAGE:-postgres:12.13-alpine}" > "$work/old-compose.yaml"
converge_calls="compose pull api
compose pull chrome
compose pull searxng
compose up -d --no-deps api chrome searxng"

# assert_rolled_back <label> — the host holds exactly what it held before.
assert_rolled_back() {
  grep -q 'rolling back to 9.8.6' <<<"$out" || fail "$1: rollback not reported"
  cmp -s "$stack/.env" "$work/env.before" || fail "$1: .env not restored byte-for-byte"
  [ "$(mode_of "$stack/.env")" = 600 ] || fail "$1: .env lost mode 600"
  cmp -s "$stack/docker-compose.yaml" "$work/old-compose.yaml" || fail "$1: compose file not restored"
  [ "$(compose_calls | tail -1)" = "compose up -d --no-deps api chrome searxng" ] ||
    fail "$1: previous services not reconverged: $(compose_calls)"
  grep -q 'DB snapshot for manual restore if a migration was incompatible: data/helix-postgres@pre-9.8.7-' <<<"$out" ||
    fail "$1: rollback omitted the snapshot"
}

# No drift: version-following images move, databases stay, override is kept.
setup_stack "$repo/docker-compose.yaml"
printf 'CHROME_LAUNCHER_TOKEN=existing\n' >> "$stack/.env"
reset_calls
out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true 2>&1) || fail "no-drift deploy failed: $out"
[ "$(cat "$work/ssh-hosts")" = "luke_helix_ml@34.39.116.64" ] || fail "control-plane step contacted another host"
grep -q '^HELIX_VERSION="9.8.7"$' "$stack/.env" || fail "no-drift deploy did not bump HELIX_VERSION"
grep -q 'chrome: ghcr.io/helixml/chrome:9.8.6 -> ghcr.io/helixml/chrome:9.8.7' <<<"$out" || fail "chrome pin change not printed"
grep -q 'postgres: postgres:12.13-alpine (unchanged)' <<<"$out" || fail "postgres pin not printed"
[ "$(grep -c '^manifest inspect ' "$work/docker-calls")" = 3 ] || fail "no-drift deploy did not check every image it pulls"
[ "$(compose_calls)" = "$converge_calls" ] || fail "no-drift deploy converged the wrong services: $(compose_calls)"
grep -q '^snapshot data/helix-postgres@pre-9.8.7-' "$work/zfs-calls" || fail "no-drift deploy took no snapshot"
cmp -s "$stack/docker-compose.yaml" "$repo/docker-compose.yaml" || fail "no-drift deploy changed the compose file"
[ -f "$stack/docker-compose.override.yaml" ] || fail "override file removed"
[ "$(grep -c '^CHROME_LAUNCHER_TOKEN=' "$stack/.env")" = 1 ] || fail "existing launcher token was replaced"
if compgen -G "$stack/docker-compose.yaml.bak*" > /dev/null; then fail "no-drift deploy made a compose backup"; fi
grep -q "^exec ctr-postgres psql -U postgres -c CHECKPOINT;$" "$work/docker-calls" ||
  fail "CHECKPOINT did not go to the compose-resolved postgres container"
if grep -q 'helix-sandbox' "$work/ssh-scripts"; then fail "control-plane step included a runner deployment"; fi

# The plan reads what runs, not what .env now renders: postgres 12 runs on PG12
# data while someone has pinned HELIX_POSTGRES_IMAGE=postgres:17-alpine.
pin_pg17_over_pg12() {
  setup_stack "$repo/docker-compose.yaml"
  sed -i 's/^HELIX_POSTGRES_IMAGE=.*/HELIX_POSTGRES_IMAGE=postgres:17-alpine/' "$stack/.env"
  reset_calls
  render_running
  sed -i 's/^postgres .*/postgres postgres:12.13-alpine 12/' "$work/running"
  running_pinned=true
}
for recreate in true false; do
  pin_pg17_over_pg12
  if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true RECREATE_POSTGRES=$recreate 2>&1); then
    fail "RECREATE_POSTGRES=$recreate: postgres 17 over PG12 data was not refused"
  fi
  grep -q 'postgres: postgres:12.13-alpine -> postgres:17-alpine \[host compose now renders postgres:17-alpine\]' <<<"$out" ||
    fail "RECREATE_POSTGRES=$recreate: pin diff hid the running postgres image"
  grep -q 'postgres data is PostgreSQL 12 but would run postgres:17-alpine' <<<"$out" ||
    fail "RECREATE_POSTGRES=$recreate: data major mismatch not explained"
  if [ -s "$work/zfs-calls" ] || [ -s "$work/docker-calls" ]; then
    fail "RECREATE_POSTGRES=$recreate: data major mismatch touched the host"
  fi
done

# A database the host defines but does not run has an unknown data major:
# RECREATE_POSTGRES fails closed; without it the database is simply held.
setup_stack "$repo/docker-compose.yaml"
reset_calls
render_running
sed -i '/^postgres /d' "$work/running"
running_pinned=true
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true RECREATE_POSTGRES=true 2>&1); then
  fail "RECREATE_POSTGRES with postgres not running did not fail"
fi
grep -q 'postgres is defined on this host but not running' <<<"$out" || fail "stopped postgres refusal not explained"
[ ! -s "$work/zfs-calls" ] || fail "stopped postgres refusal took a snapshot"
setup_stack "$repo/docker-compose.yaml"
printf 'CHROME_LAUNCHER_TOKEN=existing\n' >> "$stack/.env"
reset_calls
render_running
sed -i '/^postgres /d' "$work/running"
running_pinned=true
out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true 2>&1) || fail "deploy with postgres held and stopped failed: $out"
[ "$(compose_calls)" = "$converge_calls" ] || fail "stopped postgres was touched: $(compose_calls)"
if grep -q 'CHECKPOINT' "$work/docker-calls"; then fail "CHECKPOINT sent with no postgres container"; fi

# A green main build deploys by its short SHA.
setup_stack "$repo/docker-compose.yaml"
reset_calls
sha=$(git -C "$src" rev-parse --short=7 HEAD)
out=$(run_deploy "$sha" DEPLOY_RUNNER=false EXEC_REMOTE=true 2>&1) || fail "main SHA deploy failed: $out"
grep -q "^HELIX_VERSION=\"$sha\"$" "$stack/.env" || fail "main SHA deploy did not set HELIX_VERSION"
grep -q "^manifest inspect ghcr.io/helixml/chrome:$sha$" "$work/docker-calls" || fail "main SHA chrome image not checked"

# Drift without COMPOSE_SYNC fails before any change.
setup_stack "$work/old-compose.yaml"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true 2>&1); then
  fail "compose drift without COMPOSE_SYNC did not fail"
fi
grep -q 'chrome: ghcr.io/go-rod/rod:v0.115.0 -> ghcr.io/helixml/chrome:9.8.7' <<<"$out" || fail "drift image diff not printed"
grep -q 'searxng: searxng/searxng:2025.7.24-1b4c5ab -> searxng/searxng:2025.12.7' <<<"$out" || fail "searxng drift not printed"
grep -q 'COMPOSE_SYNC=true' <<<"$out" || fail "drift failure does not name COMPOSE_SYNC"
if [ -s "$work/zfs-calls" ] || [ -s "$work/docker-calls" ]; then fail "drift failure touched the host"; fi
cmp -s "$stack/docker-compose.yaml" "$work/old-compose.yaml" || fail "drift failure changed the compose file"
grep -q '^HELIX_VERSION="9.8.6"$' "$stack/.env" || fail "drift failure changed .env"

# Drift with COMPOSE_SYNC installs the release compose file and a launcher token.
setup_stack "$work/old-compose.yaml"
printf 'CHROME_LAUNCHER_TOKEN=\n' >> "$stack/.env"
reset_calls
out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true 2>&1) || fail "COMPOSE_SYNC deploy failed: $out"
cmp -s "$stack/docker-compose.yaml" "$repo/docker-compose.yaml" || fail "COMPOSE_SYNC did not install the release compose file"
[ "$(mode_of "$stack/docker-compose.yaml")" = 664 ] || fail "installed compose file lost its mode"
backup=$(compgen -G "$stack/docker-compose.yaml.bak-*") || fail "COMPOSE_SYNC made no backup"
cmp -s "$backup" "$work/old-compose.yaml" || fail "backup does not hold the previous compose file"
[ "$(mode_of "$backup")" = 664 ] || fail "backup lost its mode"
[ "$(mode_of "$stack/.env")" = 600 ] || fail ".env lost mode 600"
[ "$(grep -c '^CHROME_LAUNCHER_TOKEN=' "$stack/.env")" = 1 ] || fail "launcher token not written exactly once"
token=$(sed -n 's/^CHROME_LAUNCHER_TOKEN=//p' "$stack/.env")
[[ "$token" =~ ^[0-9a-f]{64}$ ]] || fail "launcher token is not 32 random bytes"
if grep -qF "$token" <<<"$out" || grep -qF "$token" "$work/docker-calls" "$work/ssh-scripts"; then
  fail "launcher token leaked into output or argv"
fi
[ "$(compose_calls)" = "$converge_calls" ] || fail "COMPOSE_SYNC converged the wrong services: $(compose_calls)"
[ -f "$stack/docker-compose.override.yaml" ] || fail "COMPOSE_SYNC removed the override file"

# An unpullable image fails before the host changes, naming the package.
setup_stack "$repo/docker-compose.yaml"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true UNPULLABLE=ghcr.io/helixml/chrome:9.8.7 2>&1); then
  fail "unpullable chrome image did not fail the deploy"
fi
grep -q 'GitHub package helixml/chrome' <<<"$out" || fail "unpullable failure does not name the package"
[ ! -s "$work/zfs-calls" ] || fail "unpullable image: snapshot taken"
[ -z "$(compose_calls)" ] || fail "unpullable image: compose ran"
grep -q '^HELIX_VERSION="9.8.6"$' "$stack/.env" || fail "unpullable image: .env changed"

# Images that keep their tag are pulled too, so they are checked too.
setup_stack "$repo/docker-compose.yaml"
reset_calls
searxng_image=$(sed -n 's/^    image: \(searxng\/.*\)$/\1/p' "$repo/docker-compose.yaml")
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true UNPULLABLE="$searxng_image" 2>&1); then
  fail "unpullable unchanged image did not fail the deploy"
fi
[ ! -s "$work/zfs-calls" ] || fail "unpullable unchanged image: snapshot taken"

# Every failure after the snapshot restores .env and the compose file exactly
# (including the generated launcher token) and reconverges the old services.
setup_stack "$work/old-compose.yaml"
cp -p "$stack/.env" "$work/env.before"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true HEALTH_CODE=502 2>&1); then
  fail "failed health-check did not fail the deploy"
fi
grep -q 'HEALTHCHECK FAILED' <<<"$out" || fail "health-check failure not reported"
assert_rolled_back "health-check failure"

setup_stack "$work/old-compose.yaml"
cp -p "$stack/.env" "$work/env.before"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true FAIL_PULL_SERVICE=chrome 2>&1); then
  fail "failed chrome pull did not fail the deploy"
fi
[ "$(compose_calls | head -2)" = "compose pull api
compose pull chrome" ] || fail "pull failure: unexpected compose calls: $(compose_calls)"
[ "$(compose_calls | grep -c '^compose up')" = 1 ] || fail "pull failure: new services were started"
assert_rolled_back "pull failure"

setup_stack "$work/old-compose.yaml"
cp -p "$stack/.env" "$work/env.before"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true FAIL_UP_ONCE="$work/up-failed" 2>&1); then
  fail "failed compose up did not fail the deploy"
fi
rm "$work/up-failed"
assert_rolled_back "compose up failure"

# A dropped SSH session (TERM/HUP) mid-deploy still rolls back.
setup_stack "$work/old-compose.yaml"
cp -p "$stack/.env" "$work/env.before"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true TERM_ON_PULL_SERVICE=chrome 2>&1); then
  fail "TERM mid-deploy did not fail the deploy"
fi
[ "$(compose_calls | grep -c '^compose up')" = 1 ] || fail "TERM: new services were started"
assert_rolled_back "TERM mid-deploy"

# A rollback that cannot restore the files says so, starts nothing, and keeps the backups.
setup_stack "$work/old-compose.yaml"
cp -p "$stack/.env" "$work/env.before"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true FAIL_UP_ONCE="$work/up-failed" BREAK_ENV_ON_UP_FAILURE=true 2>&1); then
  fail "failed rollback did not fail the deploy"
fi
rm "$work/up-failed" "$work/up-failed.restore"
grep -q 'ROLLBACK FAILED' <<<"$out" || fail "failed rollback not reported"
[ "$(compose_calls | grep -c '^compose up')" = 1 ] || fail "failed rollback reconverged against half-restored config"
kept=$(sed -n 's/^controlplane: previous docker-compose.yaml and .env kept in //p' <<<"$out")
[ -n "$kept" ] && cmp -s "$kept/env.prev" "$work/env.before" || fail "failed rollback did not keep the .env backup"
rm -r "$kept"

# Database services are never created without RECREATE_POSTGRES, by name ...
sed '/^  vectorchord-kodit:$/,$d' "$work/old-compose.yaml" > "$work/no-kodit-compose.yaml"
setup_stack "$work/no-kodit-compose.yaml"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true 2>&1); then
  fail "new vectorchord-kodit service created without RECREATE_POSTGRES"
fi
grep -q 'creation for vectorchord-kodit; re-run with RECREATE_POSTGRES=true' <<<"$out" || fail "new database refusal not explained"
[ ! -s "$work/zfs-calls" ] || fail "new database refusal took a snapshot"

# ... or by PostgreSQL-family image.
printf '  ragdb:\n    image: pgvector/pgvector:pg16\n' > "$work/ragdb.yaml"
sed -i '/^services:$/r '"$work/ragdb.yaml" "$src/docker-compose.yaml"
git -C "$src" -c user.name=t -c user.email=t@example.com commit -qam ragdb
git -C "$src" tag 9.8.8
setup_stack "$repo/docker-compose.yaml"
reset_calls
if out=$(run_deploy 9.8.8 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true 2>&1); then
  fail "new pgvector service created without RECREATE_POSTGRES"
fi
grep -q 'creation for ragdb;' <<<"$out" || fail "pgvector service not recognised as a database"
setup_stack "$repo/docker-compose.yaml"
reset_calls
out=$(run_deploy 9.8.8 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true RECREATE_POSTGRES=true 2>&1) ||
  fail "RECREATE_POSTGRES did not allow the new pgvector service: $out"
grep -q '^compose up -d --no-deps .*ragdb' "$work/docker-calls" || fail "RECREATE_POSTGRES did not create ragdb"
git -C "$src" checkout -q 9.8.7

# release_with <tag> <service yaml> [top-level volume yaml]: a release that adds
# a database service to the 9.8.7 compose file.
release_with() {
  git -C "$src" checkout -q 9.8.7
  printf '%s' "$2" > "$work/svc.yaml"
  sed -i '/^services:$/r '"$work/svc.yaml" "$src/docker-compose.yaml"
  if [ -n "${3:-}" ]; then
    printf '%s' "$3" > "$work/vol.yaml"
    sed -i '/^volumes:$/r '"$work/vol.yaml" "$src/docker-compose.yaml"
  fi
  git -C "$src" -c user.name=t -c user.email=t@example.com commit -qam "$1"
  git -C "$src" tag "$1"
}

# A new database service may start only on storage that holds nothing yet: a
# renamed service keeping postgres's volume is refused ...
release_with 9.8.9 $'  pg-renamed:\n    image: postgres:17-alpine\n    volumes:\n      - helix-postgres-db:/var/lib/postgresql/data\n'
setup_stack "$repo/docker-compose.yaml"
reset_calls
if out=$(run_deploy 9.8.9 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true RECREATE_POSTGRES=true \
  EXISTING_VOLUMES=stack_helix-postgres-db 2>&1); then
  fail "renamed database service reusing an existing volume was not refused"
fi
grep -q 'new database service pg-renamed would mount existing volume stack_helix-postgres-db' <<<"$out" ||
  fail "existing-volume refusal does not name the volume: $out"
[ ! -s "$work/zfs-calls" ] || fail "existing-volume refusal took a snapshot"

# ... as is one whose bind-mounted data directory is not empty ...
release_with 9.8.10 $'  pg-bind:\n    image: postgres:17-alpine\n    volumes:\n      - ./pgdata:/var/lib/postgresql/data\n'
setup_stack "$repo/docker-compose.yaml"
mkdir "$stack/pgdata"
echo 12 > "$stack/pgdata/PG_VERSION"
reset_calls
if out=$(run_deploy 9.8.10 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true RECREATE_POSTGRES=true 2>&1); then
  fail "new database service on a non-empty bind mount was not refused"
fi
grep -q "new database service pg-bind would mount non-empty $stack/pgdata" <<<"$out" ||
  fail "bind refusal does not name the directory: $out"
setup_stack "$repo/docker-compose.yaml"
mkdir "$stack/pgdata"
reset_calls
out=$(run_deploy 9.8.10 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true RECREATE_POSTGRES=true 2>&1) ||
  fail "new database service on an empty bind mount was refused: $out"

# ... while a genuinely new one on a fresh volume starts with RECREATE_POSTGRES.
release_with 9.8.11 $'  pg-fresh:\n    image: postgres:17-alpine\n    volumes:\n      - pg-fresh-data:/var/lib/postgresql/data\n' $'  pg-fresh-data:\n'
setup_stack "$repo/docker-compose.yaml"
reset_calls
out=$(run_deploy 9.8.11 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true RECREATE_POSTGRES=true \
  EXISTING_VOLUMES=stack_helix-postgres-db 2>&1) || fail "new database service on a fresh volume was refused: $out"
grep -q '^compose up -d --no-deps .*pg-fresh' "$work/docker-calls" || fail "fresh database service was not created"
git -C "$src" checkout -q 9.8.7

# The data major is read from every image form; anything unreadable fails closed.
# deploy_pg <postgres image> <PG_VERSION or - for an unreadable one>
deploy_pg() {
  setup_stack "$repo/docker-compose.yaml"
  sed -i "s|^HELIX_POSTGRES_IMAGE=.*|HELIX_POSTGRES_IMAGE=$1|" "$stack/.env"
  reset_calls
  render_running
  sed -i "s|^postgres .*|postgres $1 $2|" "$work/running"
  running_pinned=true
  out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true 2>&1)
}
digest=sha256:$(printf '%064d' 0)
for case in "postgres:17@$digest 17" "registry:5000/postgres:16 16" "pgvector/pgvector:pg16 16" \
  "tensorchord/vchord-suite:pg17-20250601 17"; do
  deploy_pg "${case% *}" "${case##* }" || fail "postgres ${case% *} on PG${case##* } data was refused: $out"
done
if deploy_pg "postgres:17@$digest" 16; then fail "digest-pinned postgres 17 over PG16 data was not refused"; fi
grep -q "postgres data is PostgreSQL 16 but would run postgres:17@$digest" <<<"$out" || fail "digest major mismatch not explained: $out"
for image in "postgres@$digest" postgres:latest registry:5000/postgres; do
  if deploy_pg "$image" 17; then fail "unparseable postgres image $image was accepted"; fi
  grep -q "cannot read a PostgreSQL major from image $image" <<<"$out" || fail "$image refusal not explained: $out"
  [ ! -s "$work/zfs-calls" ] || fail "$image refusal took a snapshot"
done
if deploy_pg postgres:12.13-alpine -; then fail "unreadable PG_VERSION (PGDATA unset or exec failure) was accepted"; fi
grep -q 'cannot read PG_VERSION from postgres (ctr-postgres)' <<<"$out" || fail "unreadable PG_VERSION refusal not explained: $out"
[ ! -s "$work/zfs-calls" ] || fail "unreadable PG_VERSION refusal took a snapshot"

# A PostgreSQL major change is refused even with every opt-in.
old_compose postgres:12.13-alpine > "$work/pg12-compose.yaml"
setup_stack "$work/pg12-compose.yaml"
sed -i '/^HELIX_POSTGRES_IMAGE=/d' "$stack/.env"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true RECREATE_POSTGRES=true 2>&1); then
  fail "PostgreSQL major change did not fail"
fi
grep -q 'postgres data is PostgreSQL 12 but would run postgres:17-alpine' <<<"$out" || fail "major change failure not explained"
if [ -s "$work/zfs-calls" ] || [ -s "$work/docker-calls" ]; then fail "major change failure touched the host"; fi

# A same-major database image change needs RECREATE_POSTGRES and still snapshots first.
old_compose postgres:17.1-alpine > "$work/pg17-compose.yaml"
setup_stack "$work/pg17-compose.yaml"
sed -i '/^HELIX_POSTGRES_IMAGE=/d' "$stack/.env"
reset_calls
if out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true 2>&1); then
  fail "database image change without RECREATE_POSTGRES did not fail"
fi
grep -q 'RECREATE_POSTGRES=true' <<<"$out" || fail "database change failure does not name RECREATE_POSTGRES"
[ ! -s "$work/zfs-calls" ] || fail "database change failure took a snapshot"
setup_stack "$work/pg17-compose.yaml"
sed -i '/^HELIX_POSTGRES_IMAGE=/d' "$stack/.env"
reset_calls
out=$(run_deploy 9.8.7 DEPLOY_RUNNER=false EXEC_REMOTE=true COMPOSE_SYNC=true RECREATE_POSTGRES=true 2>&1) ||
  fail "RECREATE_POSTGRES deploy failed: $out"
grep -q 'postgres: postgres:17.1-alpine -> postgres:17-alpine' <<<"$out" || fail "postgres pin change not printed"
grep -q '^compose pull .*postgres' "$work/docker-calls" || fail "RECREATE_POSTGRES did not pull postgres"
grep -q '^compose up -d --no-deps .*postgres' "$work/docker-calls" || fail "RECREATE_POSTGRES did not recreate postgres"
[ -s "$work/zfs-calls" ] || fail "RECREATE_POSTGRES deploy took no snapshot"

# The deployed compose file must be the release's.
setup_stack "$repo/docker-compose.yaml"
reset_calls
if out=$(run_deploy 9.9.9 DEPLOY_RUNNER=false EXEC_REMOTE=true 2>&1); then
  fail "deploy of a version absent from the checkout did not fail"
fi
[ ! -s "$work/ssh-hosts" ] || fail "checkout mismatch still contacted the host"
if run_deploy 'latest' DEPLOY_RUNNER=false >/dev/null 2>&1; then
  fail "non-release VERSION accepted"
fi

# --- runners ----------------------------------------------------------------
legacy="$work/upgrade.sh"
cat > "$legacy" <<'EOF'
#!/bin/bash
SANDBOX_TAG="old"
printf '%s\n' "$SANDBOX_TAG" >> "$RUN_LOG"
EOF
chmod +x "$legacy"
reset_calls
: > "$work/run-log"
deploy_output=$(run_deploy 9.8.7 DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true RUN_LOG="$work/run-log" \
  RUNNER_SSH=root@code.helix.ml RUNNER_UPGRADE_SCRIPT="$legacy")
[ "$(sort -u "$work/ssh-hosts")" = "root@code.helix.ml" ] || fail "code runner step contacted the wrong host"
grep -q '^pull ghcr.io/helixml/helix-sandbox:9.8.7$' "$work/docker-calls" || fail "code runner did not pull the release image"
grep -q '^SANDBOX_TAG="9.8.7"$' "$legacy" || fail "code runner did not update its launcher"
[ "$(cat "$work/run-log")" = "9.8.7" ] || fail "code runner did not run its launcher"
grep -q 'Runner image retention:' "$work/ssh-scripts" || fail "successful runner deploy omitted image retention dry run"
printf '%s\n' "$deploy_output" | grep -q 'Would remove obsolete runner image: ghcr.io/helixml/helix-sandbox:9.8.5' ||
  fail "successful runner deploy omitted the image retention candidate"
if grep -q '^image rm ' "$work/docker-calls"; then
  fail "runner deploy image retention dry run removed an image"
fi

reset_calls
cleanup_output=$(run_deploy 9.8.7 DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true FAIL_CLEANUP=true RUN_LOG="$work/run-log" \
  RUNNER_SSH=root@code.helix.ml RUNNER_UPGRADE_SCRIPT="$legacy" 2>&1)
printf '%s\n' "$cleanup_output" | grep -q 'WARNING: runner image retention dry run failed' ||
  fail "image retention failure was not reported"
printf '%s\n' "$cleanup_output" | grep -q 'prod deploy of 9.8.7 complete' ||
  fail "image retention failure hid the successful runner rollout"

start="$work/run.sh"
cat > "$start" <<'EOF'
#!/bin/bash
image=ghcr.io/helixml/helix-sandbox:old
printf '%s\n' "$image" >> "$RUN_LOG"
EOF
chmod +x "$start"
reset_calls
: > "$work/run-log"
run_deploy 9.8.7 DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true RUN_LOG="$work/run-log" \
  RUNNER_SSH=root@dev.code.helix.ml RUNNER_START_SCRIPT="$start" >/dev/null
[ "$(sort -u "$work/ssh-hosts")" = "root@dev.code.helix.ml" ] || fail "dev runner step contacted the wrong host"
grep -q '^image=ghcr.io/helixml/helix-sandbox:9.8.7$' "$start" || fail "dev runner did not update its launcher"
grep -q '^stop helix-sandbox-app$' "$work/docker-calls" || fail "dev runner did not stop its container"
grep -q '^rm helix-sandbox-app$' "$work/docker-calls" || fail "dev runner did not remove its container"
[ "$(cat "$work/run-log")" = "ghcr.io/helixml/helix-sandbox:9.8.7" ] || fail "dev runner did not run its launcher"

: > "$work/run-log"
: > "$work/docker-calls"
run_deploy 9.8.7 DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true CONTAINER_EXISTS=false \
  RUN_LOG="$work/run-log" RUNNER_SSH=root@dev.code.helix.ml \
  RUNNER_START_SCRIPT="$start" >/dev/null
if grep -qE '^(stop|rm) ' "$work/docker-calls"; then
  fail "dev runner tried to remove a missing container"
fi
[ -s "$work/run-log" ] || fail "dev runner retry did not run its launcher"

sed -i.bak 's/^image=.*/image=ghcr.io\/helixml\/helix-sandbox:old/' "$start"
: > "$work/run-log"
: > "$work/ssh-scripts"
if run_deploy 9.8.7 DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true FAIL_PULL=true \
  RUN_LOG="$work/run-log" RUNNER_SSH=root@dev.code.helix.ml \
  RUNNER_START_SCRIPT="$start" >/dev/null 2>&1; then
  fail "runner pull failure did not fail the deployment"
fi
grep -q '^image=ghcr.io/helixml/helix-sandbox:old$' "$start" || fail "runner launcher changed before a successful pull"
[ ! -s "$work/run-log" ] || fail "runner launcher ran after a failed pull"
if grep -q 'Runner image retention:' "$work/ssh-scripts"; then
  fail "failed runner deploy ran image retention"
fi

echo "deploy-prod test passed"
