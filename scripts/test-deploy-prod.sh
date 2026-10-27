#!/bin/bash
set -euo pipefail

repo=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
real_sed=$(command -v sed)
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
printf '%s\n' "$host" >> "$SSH_HOSTS"
printf '%s\n---\n' "$remote" >> "$SSH_SCRIPTS"
if [ "${EXEC_REMOTE:-false}" = "true" ]; then
  bash -c "$remote"
fi
EOF

cat > "$work/bin/docker" <<'EOF'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >> "$DOCKER_CALLS"
if [ "${1:-}" = "pull" ] && [ "${FAIL_PULL:-false}" = "true" ]; then
  exit 1
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
chmod +x "$work/bin/ssh" "$work/bin/docker" "$work/bin/sed"

run_deploy() {
  env -u HELIX_CD_SSH_KEY -u SLACK_WEBHOOK_URL -u LONDON_SSH \
    -u RUNNER_SSH -u RUNNER_UPGRADE_SCRIPT -u RUNNER_START_SCRIPT \
    -u RUNNER_CONTAINER -u DEPLOY_CONTROLPLANE -u DEPLOY_RUNNER \
    PATH="$work/bin:$PATH" HOME="$work/home" REAL_SED="$real_sed" \
    SSH_HOSTS="$work/ssh-hosts" SSH_SCRIPTS="$work/ssh-scripts" \
    DOCKER_CALLS="$work/docker-calls" "$@" "$repo/scripts/deploy-prod.sh" 9.8.7
}

: > "$work/ssh-hosts"
: > "$work/ssh-scripts"
: > "$work/docker-calls"
run_deploy DEPLOY_RUNNER=false >/dev/null
[ "$(cat "$work/ssh-hosts")" = "luke_helix_ml@34.39.116.64" ] || fail "control-plane step contacted another host"
grep -q 'HELIX_VERSION=.*9.8.7' "$work/ssh-scripts" || fail "control-plane step did not deploy the requested version"
if grep -q 'helix-sandbox' "$work/ssh-scripts"; then
  fail "control-plane step included a runner deployment"
fi

legacy="$work/upgrade.sh"
cat > "$legacy" <<'EOF'
#!/bin/bash
SANDBOX_TAG="old"
printf '%s\n' "$SANDBOX_TAG" >> "$RUN_LOG"
EOF
chmod +x "$legacy"
: > "$work/ssh-hosts"
: > "$work/ssh-scripts"
: > "$work/docker-calls"
: > "$work/run-log"
run_deploy DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true RUN_LOG="$work/run-log" \
  RUNNER_SSH=root@code.helix.ml RUNNER_UPGRADE_SCRIPT="$legacy" >/dev/null
[ "$(cat "$work/ssh-hosts")" = "root@code.helix.ml" ] || fail "code runner step contacted the wrong host"
grep -q '^pull ghcr.io/helixml/helix-sandbox:9.8.7$' "$work/docker-calls" || fail "code runner did not pull the release image"
grep -q '^SANDBOX_TAG="9.8.7"$' "$legacy" || fail "code runner did not update its launcher"
[ "$(cat "$work/run-log")" = "9.8.7" ] || fail "code runner did not run its launcher"

start="$work/run.sh"
cat > "$start" <<'EOF'
#!/bin/bash
image=ghcr.io/helixml/helix-sandbox:old
printf '%s\n' "$image" >> "$RUN_LOG"
EOF
chmod +x "$start"
: > "$work/ssh-hosts"
: > "$work/ssh-scripts"
: > "$work/docker-calls"
: > "$work/run-log"
run_deploy DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true RUN_LOG="$work/run-log" \
  RUNNER_SSH=root@dev.code.helix.ml RUNNER_START_SCRIPT="$start" >/dev/null
[ "$(cat "$work/ssh-hosts")" = "root@dev.code.helix.ml" ] || fail "dev runner step contacted the wrong host"
grep -q '^image=ghcr.io/helixml/helix-sandbox:9.8.7$' "$start" || fail "dev runner did not update its launcher"
grep -q '^stop helix-sandbox-app$' "$work/docker-calls" || fail "dev runner did not stop its container"
grep -q '^rm helix-sandbox-app$' "$work/docker-calls" || fail "dev runner did not remove its container"
[ "$(cat "$work/run-log")" = "ghcr.io/helixml/helix-sandbox:9.8.7" ] || fail "dev runner did not run its launcher"

: > "$work/run-log"
: > "$work/docker-calls"
run_deploy DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true CONTAINER_EXISTS=false \
  RUN_LOG="$work/run-log" RUNNER_SSH=root@dev.code.helix.ml \
  RUNNER_START_SCRIPT="$start" >/dev/null
if grep -qE '^(stop|rm) ' "$work/docker-calls"; then
  fail "dev runner tried to remove a missing container"
fi
[ -s "$work/run-log" ] || fail "dev runner retry did not run its launcher"

sed -i.bak 's/^image=.*/image=ghcr.io\/helixml\/helix-sandbox:old/' "$start"
: > "$work/run-log"
if run_deploy DEPLOY_CONTROLPLANE=false EXEC_REMOTE=true FAIL_PULL=true \
  RUN_LOG="$work/run-log" RUNNER_SSH=root@dev.code.helix.ml \
  RUNNER_START_SCRIPT="$start" >/dev/null 2>&1; then
  fail "runner pull failure did not fail the deployment"
fi
grep -q '^image=ghcr.io/helixml/helix-sandbox:old$' "$start" || fail "runner launcher changed before a successful pull"
[ ! -s "$work/run-log" ] || fail "runner launcher ran after a failed pull"

echo "deploy-prod test passed"
