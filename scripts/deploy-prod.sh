#!/bin/bash
set -euo pipefail

# =============================================================================
# Production self-deploy (continuous delivery)
# =============================================================================
# Rolls a released version onto the live SaaS, automating
# design/2026-06-25-prod-version-bump-runbook.md:
#   1. controlplane (helix-cloud-london): CHECKPOINT + ZFS-snapshot the DB, bump
#      HELIX_VERSION, `docker compose pull api && up -d api`, health-check, and
#      roll the image version back in place if the health-check fails.
#   2. runner (code.helix.ml): bump SANDBOX_TAG in the upgrade script, re-run it.
#
# Both hosts are reached over plain SSH with one dedicated CD key (the key's
# public half is authorised on luke_helix_ml@london, which has passwordless
# sudo, and root@code.helix.ml).
#
# Invoked by the `deploy-prod` Drone pipeline on a GREEN release tag (so the
# controlplane/sandbox images for $DRONE_TAG already exist on ghcr). Can also be
# run manually:  scripts/deploy-prod.sh VERSION
#
# Required env (CI provides this from a Drone secret):
#   HELIX_CD_SSH_KEY   base64-encoded OpenSSH private key authorised on both
#                      hosts. When unset, falls back to the ambient SSH
#                      agent/keys (local runs).
# Optional env / config (defaults shown):
#   LONDON_SSH=luke_helix_ml@34.39.116.64   LONDON_STACK_DIR=/data/helix-app/helix
#   LONDON_DB_DATASET=data/helix-postgres   LONDON_HEALTH_HOST=app.helix.ml
#   RUNNER_SSH=root@code.helix.ml
#   RUNNER_UPGRADE_SCRIPT=/opt/HelixML/upgrade-sandbox-app.helix.ml.sh
#   DEPLOY_RUNNER=true   (set false to skip the runner upgrade)
#   SLACK_WEBHOOK_URL    optional notifications
# =============================================================================

VERSION="${1:-${DRONE_TAG:-}}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 VERSION  (or set DRONE_TAG)" >&2
  exit 1
fi

LONDON_SSH="${LONDON_SSH:-luke_helix_ml@34.39.116.64}"
LONDON_STACK_DIR="${LONDON_STACK_DIR:-/data/helix-app/helix}"
LONDON_DB_DATASET="${LONDON_DB_DATASET:-data/helix-postgres}"
LONDON_HEALTH_HOST="${LONDON_HEALTH_HOST:-app.helix.ml}"
RUNNER_SSH="${RUNNER_SSH:-root@code.helix.ml}"
RUNNER_UPGRADE_SCRIPT="${RUNNER_UPGRADE_SCRIPT:-/opt/HelixML/upgrade-sandbox-app.helix.ml.sh}"
DEPLOY_RUNNER="${DEPLOY_RUNNER:-true}"

log()  { echo "[deploy-prod] $*"; }
warn() { echo "[deploy-prod] WARNING: $*" >&2; }

notify() {
  [ -n "${SLACK_WEBHOOK_URL:-}" ] || return 0
  curl -sf -X POST -H 'Content-type: application/json' \
    --data "{\"text\":\"$1\"}" "$SLACK_WEBHOOK_URL" >/dev/null 2>&1 || warn "slack notify failed"
}

fail() {
  warn "$1"
  notify ":rotating_light: *prod deploy of ${VERSION} FAILED*: $1"
  exit 1
}

# --- auth -------------------------------------------------------------------
SSH_OPTS="-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null -o ConnectTimeout=20 -o ServerAliveInterval=30"
if [ -n "${HELIX_CD_SSH_KEY:-}" ]; then
  mkdir -p "$HOME/.ssh"
  echo "$HELIX_CD_SSH_KEY" | base64 -d > "$HOME/.ssh/helix_cd"
  chmod 600 "$HOME/.ssh/helix_cd"
  SSH_OPTS="$SSH_OPTS -i $HOME/.ssh/helix_cd -o IdentitiesOnly=yes"
fi

rsh() { # rsh <user@host> <script>
  # shellcheck disable=SC2086
  ssh $SSH_OPTS "$1" "$2"
}

# --- 1. controlplane (london) ----------------------------------------------
SNAP="${LONDON_DB_DATASET}@pre-${VERSION}-$(date +%Y%m%d-%H%M%S)"
log "Deploying controlplane ${VERSION} to ${LONDON_SSH} (snapshot ${SNAP}) ..."

# Remote script. Unquoted heredoc: local values ($VERSION, $SNAP, $LONDON_*) are
# substituted here; values evaluated ON the remote are escaped (\$).
CP_DEPLOY=$(cat <<REMOTE
set -e
cd "$LONDON_STACK_DIR"
OLD_VERSION=\$(grep -E '^HELIX_VERSION=' .env | head -1 | cut -d= -f2- | tr -d '"')
echo "controlplane: \${OLD_VERSION:-<none>} -> $VERSION"
# Crash-consistent DB snapshot for rollback (CHECKPOINT first to minimise WAL replay).
sudo docker exec helix-postgres-1 psql -U postgres -c 'CHECKPOINT;' >/dev/null 2>&1 || true
sudo zfs snapshot "$SNAP"
sudo sed -i 's/^HELIX_VERSION=.*/HELIX_VERSION="$VERSION"/' .env
sudo docker compose pull api
sudo docker compose up -d api
ok=0
for i in \$(seq 1 36); do
  code=\$(curl -s -o /dev/null -w '%{http_code}' "https://$LONDON_HEALTH_HOST/" --resolve "$LONDON_HEALTH_HOST:443:127.0.0.1" --max-time 10 || echo 000)
  if [ "\$code" = "200" ]; then ok=1; break; fi
  sleep 5
done
if [ "\$ok" != "1" ]; then
  echo "HEALTHCHECK FAILED — rolling image back to \${OLD_VERSION}"
  if [ -n "\${OLD_VERSION:-}" ]; then
    sudo sed -i "s/^HELIX_VERSION=.*/HELIX_VERSION=\\"\${OLD_VERSION}\\"/" .env
    sudo docker compose up -d api || true
  fi
  echo "DB snapshot for manual restore if a migration was incompatible: $SNAP"
  exit 42
fi
echo "controlplane healthy on $VERSION"
REMOTE
)

if ! rsh "$LONDON_SSH" "$CP_DEPLOY"; then
  fail "controlplane deploy/health-check failed (image rolled back in place). DB snapshot: ${SNAP}"
fi
log "controlplane on ${VERSION} ✓"

# --- 2. runner (code.helix.ml) ---------------------------------------------
if [ "$DEPLOY_RUNNER" = "true" ]; then
  log "Deploying runner ${VERSION} to ${RUNNER_SSH} ..."
  RUNNER_DEPLOY=$(cat <<REMOTE
set -e
sed -i 's/^SANDBOX_TAG=.*/SANDBOX_TAG="$VERSION"/' "$RUNNER_UPGRADE_SCRIPT"
"$RUNNER_UPGRADE_SCRIPT"
REMOTE
)
  if ! rsh "$RUNNER_SSH" "$RUNNER_DEPLOY"; then
    fail "runner deploy failed — controlplane is on ${VERSION} but the runner may still be on the old image (version skew). Re-run: ssh ${RUNNER_SSH} ${RUNNER_UPGRADE_SCRIPT}"
  fi
  log "runner on ${VERSION} ✓"
else
  log "DEPLOY_RUNNER=false — skipping runner upgrade"
fi

notify ":rocket: *prod self-deployed ${VERSION}* (controlplane + runner)"
log "prod deploy of ${VERSION} complete"
