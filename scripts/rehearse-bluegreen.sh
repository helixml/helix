#!/bin/bash
# Rehearses the blue/green machinery end to end with real docker, compose,
# nginx, Postgres and NATS, inside a privileged docker-in-docker container so
# nothing touches the machine's own ports or stack:
#
#   1. a single `api` serving (the shape of a host today)
#   2. scripts/bluegreen-cutover.sh converts it to api-blue behind nginx
#   3. a deploy of v2 to green          (expects zero failed probe requests)
#   4. a deploy of a broken build       (expects rollback, zero failed requests)
#   5. a deploy of v1 back to blue      (expects zero failed requests)
#   6. a long-lived connection through nginx is cut when its colour drains
#
# The colours run this checkout's API binary in an image shaped like the
# release image. Requires docker with --privileged. Usage:
#   scripts/rehearse-bluegreen.sh            # builds the API, runs, cleans up
#   KEEP=1 scripts/rehearse-bluegreen.sh     # leave the container for poking
set -euo pipefail
REPO=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
NAME=bluegreen-rehearsal
log() { echo "rehearsal: $*"; }
cleanup() {
  [ "${KEEP:-}" = 1 ] && { log "kept container $NAME and $WORK"; return; }
  docker rm -f "$NAME" > /dev/null 2>&1 || true
  docker image rm bluegreen-rehearsal/controlplane:v1 bluegreen-rehearsal/controlplane:broken > /dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

log "building the API"
mkdir -p "$WORK/img"
(cd "$REPO/api" && CGO_ENABLED=1 go build -o "$WORK/img/helix" .)
cat > "$WORK/img/entrypoint.sh" << 'EOF'
#!/bin/sh
# The "broken" image migrates but never serves.
if [ -f /broken ] && [ "$1" != "--migrate-only" ]; then echo "broken build: not serving"; exec sleep infinity; fi
exec /helix serve "$@"
EOF
chmod +x "$WORK/img/entrypoint.sh"
glibc_base=$(. /etc/os-release && echo "$ID:$VERSION_ID") # the binary links this host's glibc
cat > "$WORK/img/Dockerfile" << EOF
FROM $glibc_base
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git tini && rm -rf /var/lib/apt/lists/*
COPY helix /helix
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/usr/bin/tini", "--", "/entrypoint.sh"]
EOF
docker build -q -t bluegreen-rehearsal/controlplane:v1 "$WORK/img" > /dev/null
printf 'FROM bluegreen-rehearsal/controlplane:v1\nRUN touch /broken\n' |
  docker build -q -t bluegreen-rehearsal/controlplane:broken - > /dev/null

# --- the host: a stack directory, nginx config, the deploy scripts -------------
mkdir -p "$WORK/stack" "$WORK/scripts/lib"
cp "$REPO/scripts/bluegreen-cutover.sh" "$WORK/scripts/"
cp "$REPO/scripts/lib/bluegreen.sh" "$REPO/scripts/lib/availability-probe.sh" "$WORK/scripts/lib/"
cp "$REPO/docker-compose.bluegreen.yaml" "$WORK/stack/"
cat > "$WORK/stack/docker-compose.yaml" << 'EOF'
# Stand-in for a host's base compose file, with the shape that matters: one
# `api` with env_file .env, public ports, the filestore and socket volumes.
services:
  api:
    image: ${HELIX_CONTROLPLANE_IMAGE:-localhost:5000/controlplane}:${HELIX_VERSION:-v1}
    ports: [8080:8080, 2224:2224]
    restart: always
    env_file: [.env]
    environment:
      - SERVER_PORT=8080
      - SERVER_URL=http://rehearsal.test
      - POSTGRES_HOST=postgres
      - POSTGRES_DATABASE=postgres
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=pw
      - FILESTORE_LOCALFS_PATH=/filestore
      - HELIX_EMBEDDINGS_SOCKET=/socket/embeddings.sock
      - ASSET_SSH_PROXY_LISTEN=:2224
      - RAG_CRAWLER_LAUNCHER_TOKEN=rehearsal-chrome-token
    volumes: [helix-filestore:/filestore, helix-socket:/socket]
    depends_on: [postgres, chrome]
  chrome:
    image: ghcr.io/helixml/chrome:2.12.63
    restart: always
    environment: [CHROME_LAUNCHER_TOKEN=rehearsal-chrome-token]
  postgres:
    image: postgres:17-alpine
    restart: always
    environment: [POSTGRES_PASSWORD=pw]
    volumes: [pgdata:/var/lib/postgresql/data]
volumes: {helix-filestore: {}, helix-socket: {}, pgdata: {}}
EOF
printf 'RUNNER_TOKEN=rehearsal-runner-token\nHELIX_VERSION=v1\n' > "$WORK/stack/.env"
# London's shape (http blocks -> helix_http) on :80, Meta's (stream
# passthrough) on :8080 and :2224.
cat > "$WORK/nginx.conf" << 'EOF'
worker_processes 2;
worker_shutdown_timeout 90s;
include /etc/nginx/modules/*.conf;
events { worker_connections 1024; }
http {
  include /etc/nginx/helix-active/http.conf;
  server { listen 80; location / { proxy_pass http://helix_http; proxy_set_header Host $host; } }
}
stream {
  include /etc/nginx/helix-active/stream.conf;
  server { listen 8080; proxy_pass $helix_backend; proxy_timeout 1h; }
  server { listen 2224; proxy_pass $helix_backend; proxy_timeout 1h; }
}
EOF

docker rm -f "$NAME" > /dev/null 2>&1 || true
docker run -d --privileged --name "$NAME" -v "$WORK:/work" docker:28-dind > /dev/null
D() { docker exec "$NAME" "$@"; }
for _ in $(seq 1 30); do D docker info > /dev/null 2>&1 && break; sleep 1; done
D apk add -q bash curl nginx nginx-mod-stream python3 coreutils procps
# London's deploy runs as a sudoer and snapshots ZFS; here it is root and
# the snapshot is only logged. The local registry is plain HTTP, which
# `docker manifest inspect` only accepts with --insecure.
D sh -c 'printf "#!/bin/sh\nexec \"\$@\"\n" > /usr/local/bin/sudo
printf "#!/bin/sh\necho \"\$*\" >> /work/zfs.log\n" > /usr/local/bin/zfs
printf "#!/bin/sh\n[ \"\$1 \$2\" = \"manifest inspect\" ] && { shift 2; exec /usr/local/bin/docker manifest inspect --insecure \"\$@\"; }\nexec /usr/local/bin/docker \"\$@\"\n" > /usr/local/sbin/docker
chmod +x /usr/local/bin/sudo /usr/local/bin/zfs /usr/local/sbin/docker'
D docker run -d --name registry --restart always -p 5000:5000 registry:2 > /dev/null
for _ in $(seq 1 30); do D curl -fsS -o /dev/null http://localhost:5000/v2/ && break; sleep 1; done
docker save bluegreen-rehearsal/controlplane:v1 bluegreen-rehearsal/controlplane:broken | docker exec -i "$NAME" docker load -q > /dev/null
for tag in v1 broken; do
  D docker tag "bluegreen-rehearsal/controlplane:$tag" "localhost:5000/controlplane:$tag"
  D docker push -q "localhost:5000/controlplane:$tag" > /dev/null
done
D docker tag localhost:5000/controlplane:v1 localhost:5000/controlplane:v2
D docker push -q localhost:5000/controlplane:v2 > /dev/null

# Run a bash snippet on the "host", in the stack directory, as the deploys do.
H() { docker exec -w /work/stack -e BG_SUDO= -e BG_HEALTH_HOST=rehearsal.test "$NAME" bash -c "$1"; }
probe_start() {
  H "PROBE_INTERVAL=0.1 nohup /work/scripts/lib/availability-probe.sh http://127.0.0.1:8080/api/v1/config > /work/probe.out 2>&1 & echo \$! > /work/probe.pid"
}
probe_stop() { H 'kill -TERM $(cat /work/probe.pid); while kill -0 $(cat /work/probe.pid) 2>/dev/null; do sleep 0.1; done; cat /work/probe.out'; }
failed_of() { sed -n 's/.*failed=\([0-9]*\).*/\1/p' <<< "$1"; }
active() { H '. /work/scripts/lib/bluegreen.sh; bg_edge'; }
fail() { echo "rehearsal: FAIL: $*" >&2; H 'docker compose ps -a; docker compose logs --tail 30' >&2 || true; exit 1; }

# Deploys TAG as the Meta remote script does (backup .env, bg_deploy,
# restore + bg_rollback on failure).
deploy() {
  H ". /work/scripts/lib/bluegreen.sh
    set -euo pipefail
    BG_WATCH_SECONDS=10
    cp -p .env /work/env.prev
    trap 'st=\$?; if [ \$st -ne 0 ]; then cp -p /work/env.prev .env; bg_rollback; fi; exit \$st' EXIT
    bg_preflight
    bg_deploy $1"
}

log "1. single api on v1"
H 'docker compose up -d --quiet-pull postgres chrome api 2>&1 | grep -v Pull || true'
for _ in $(seq 1 90); do H 'curl -fsS -o /dev/null http://127.0.0.1:8080/api/v1/readyz' 2> /dev/null && break; sleep 2; done
H 'curl -fsS -o /dev/null http://127.0.0.1:8080/api/v1/readyz' || fail "the single api never became ready"

log "2. cutover to blue/green"
probe_start
H 'BG_SUDO= /work/scripts/bluegreen-cutover.sh --image localhost:5000/controlplane:v1 \
    --nginx-conf /work/nginx.conf --health-host rehearsal.test --yes' || fail "cutover failed"
cutover=$(probe_stop)
log "cutover availability: $cutover (one-time downtime expected)"
[ "$(active)" = blue ] || fail "blue is not serving after the cutover"
H 'docker compose ps --format "{{.Service}} {{.State}}"' | sort

log "3. deploy v2 to green"
probe_start
deploy v2 || fail "deploy of v2 failed"
v2=$(probe_stop)
log "v2 availability: $v2"
[ "$(active)" = green ] || fail "green is not serving after deploying v2"
[ "$(failed_of "$v2")" = 0 ] || fail "requests failed during the v2 deploy: $v2"
[ -z "$(H 'docker compose ps -a -q api-blue')" ] || fail "api-blue was not removed"

log "4. deploy a broken build: must roll back to green"
probe_start
if deploy broken; then fail "the broken deploy succeeded"; fi
broken=$(probe_stop)
log "broken-deploy availability: $broken"
[ "$(active)" = green ] || fail "green is not serving after the rollback"
[ "$(failed_of "$broken")" = 0 ] || fail "requests failed during the broken deploy: $broken"
H 'grep -q "^HELIX_ACTIVE_COLOUR=\"green\"" .env && grep -q "^HELIX_VERSION_BLUE=\"v1\"" .env' || fail ".env was not restored"
[ -z "$(H 'docker compose ps -a -q api-blue')" ] || fail "the broken api-blue was left behind"

log "5. deploy v1 to blue, holding a connection open on green through nginx"
H 'exec 3<>/dev/tcp/127.0.0.1/2224; date +%s > /work/conn.start; cat <&3 > /dev/null; date +%s > /work/conn.end' &
holder=$!
sleep 2
probe_start
deploy v1 || fail "deploy of v1 failed"
v1=$(probe_stop)
log "v1 availability: $v1"
[ "$(active)" = blue ] || fail "blue is not serving after deploying v1"
[ "$(failed_of "$v1")" = 0 ] || fail "requests failed during the v1 deploy: $v1"
wait "$holder" || true
held=$(H 'echo $(( $(cat /work/conn.end) - $(cat /work/conn.start) ))')
log "6. the connection held through nginx to green closed after ${held}s, when green drained"

# London's real entry point: deploy-controlplane-remote.sh as deploy-prod.sh
# sends it (variables, bluegreen.sh, the script), on this stack.
london() { # london VERSION
  {
    printf '%s=%q\n' STACK_DIR /work/stack VERSION "$1" SNAP "data/helix-postgres@pre-$1" \
      HEALTH_HOST rehearsal.test COMPOSE_SYNC false RECREATE_POSTGRES false \
      REPO_COMPOSE_B64 "$(base64 -w0 < "$WORK/stack/docker-compose.yaml")" \
      REPO_BLUEGREEN_B64 "$(base64 -w0 < "$REPO/docker-compose.bluegreen.yaml")"
    cat "$REPO/scripts/lib/bluegreen.sh" "$REPO/scripts/lib/deploy-controlplane-remote.sh"
  } | docker exec -i -e BG_WATCH_SECONDS=10 "$NAME" bash -s
}

log "7. London deploy-controlplane-remote.sh: v2 to green"
probe_start
london v2 || fail "the London deploy of v2 failed"
lon_v2=$(probe_stop)
log "London v2 availability: $lon_v2"
[ "$(active)" = green ] || fail "green is not serving after the London deploy"
[ "$(failed_of "$lon_v2")" = 0 ] || fail "requests failed during the London deploy: $lon_v2"
H 'grep -q "^HELIX_VERSION=\"v2\"" .env' || fail "HELIX_VERSION was not bumped"
H 'grep -q "pre-v2" /work/zfs.log' || fail "no ZFS snapshot was taken"

log "8. London deploy of a broken build: must roll back to green"
H 'cp -p .env /work/env.before-broken'
probe_start
if london broken; then fail "the broken London deploy succeeded"; fi
lon_broken=$(probe_stop)
log "London broken-deploy availability: $lon_broken"
[ "$(active)" = green ] || fail "green is not serving after the London rollback"
[ "$(failed_of "$lon_broken")" = 0 ] || fail "requests failed during the broken London deploy: $lon_broken"
H 'cmp -s .env /work/env.before-broken' || fail "the London rollback did not restore .env"

log "9. nginx and .env disagree: the London deploy must refuse before changing anything"
H 'sed -i "s/^# active: green/# active: blue/" /etc/nginx/helix-active/stream.conf; cp -p .env /work/env.before-mismatch; : > /work/zfs.log'
if out=$(london v1 2>&1); then fail "the deploy ran although nginx and .env disagree"; fi
grep -q "nginx (/etc/nginx/helix-active) points at 'blue' but .env says green is active" <<< "$out" || fail "unexpected refusal: $out"
H 'cmp -s .env /work/env.before-mismatch && [ ! -s /work/zfs.log ]' || fail "the refused deploy changed .env or took a snapshot"
H 'sed -i "s/^# active: blue/# active: green/" /etc/nginx/helix-active/stream.conf'
[ "$(active)" = green ] || fail "green stopped serving"

log "10. a pg_dump-style ACCESS SHARE lock is held during deploys"
psql() { H "docker compose exec -T postgres psql -U postgres -tAc \"$1\""; }
H 'nohup docker compose exec -T postgres psql -U postgres -c "BEGIN; LOCK TABLE prompt_history_entries IN ACCESS SHARE MODE; SELECT pg_sleep(900);" > /dev/null 2>&1 & echo $! > /work/lock.pid'
sleep 2
[ "$(psql "SELECT count(*) FROM pg_locks l JOIN pg_class c ON c.oid = l.relation WHERE c.relname = 'prompt_history_entries' AND l.mode = 'AccessShareLock' AND l.granted")" -ge 1 ] ||
  fail "the ACCESS SHARE lock was not taken"
log "10a. no schema change: migrations only read, so the deploy goes through"
probe_start
deploy v1 || fail "a deploy without DDL failed under the lock"
locked_ok=$(probe_stop)
log "deploy under the lock: $locked_ok"
[ "$(failed_of "$locked_ok")" = 0 ] || fail "requests failed during the deploy under the lock: $locked_ok"
active_before=$(active)
log "10b. a migration needing ACCESS EXCLUSIVE fails fast and nothing changes"
# Undo a guarded fix so the next migration has a real ALTER to run.
psql "SET lock_timeout = '2s'; ALTER TABLE prompt_history_entries ALTER COLUMN interrupt SET DEFAULT true" > /dev/null 2>&1 &&
  fail "the ALTER should have waited for the lock" || true
H 'kill $(cat /work/lock.pid)'; sleep 1
psql "ALTER TABLE prompt_history_entries ALTER COLUMN interrupt SET DEFAULT true" > /dev/null
H 'nohup docker compose exec -T postgres psql -U postgres -c "BEGIN; LOCK TABLE prompt_history_entries IN ACCESS SHARE MODE; SELECT pg_sleep(900);" > /dev/null 2>&1 & echo $! > /work/lock.pid'
sleep 2
H 'cp -p .env /work/env.before-locked'
probe_start
started=$(date +%s)
if deploy v2; then fail "the migration ran its ALTER through the lock"; fi
took=$(($(date +%s) - started))
locked_fail=$(probe_stop)
log "failed deploy under the lock: ${took}s, $locked_fail"
[ "$took" -lt 120 ] || fail "the blocked migration took ${took}s to give up"
[ "$(failed_of "$locked_fail")" = 0 ] || fail "requests failed while the migration was blocked: $locked_fail"
[ "$(active)" = "$active_before" ] || fail "the active colour changed"
H 'cmp -s .env /work/env.before-locked' || fail ".env was not restored after the failed migration"
H 'kill $(cat /work/lock.pid)'; sleep 1
[ "$(psql "SELECT column_default FROM information_schema.columns WHERE table_name = 'prompt_history_entries' AND column_name = 'interrupt'")" = true ] ||
  fail "the ALTER ran despite the failure"

log "PASS: cutover ($cutover), v2 ($v2), broken+rollback ($broken), v1 ($v1), London v2 ($lon_v2), London broken+rollback ($lon_broken), under a dump lock ($locked_ok; DDL refused in ${took}s)"
