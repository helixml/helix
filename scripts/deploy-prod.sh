#!/bin/bash
set -euo pipefail

# =============================================================================
# Production self-deploy (continuous delivery)
# =============================================================================
# Rolls a released version onto the live SaaS, automating
# design/2026-06-25-prod-version-bump-runbook.md:
#   1. controlplane (helix-cloud-london), scripts/lib/deploy-controlplane-remote.sh:
#      diff the host's docker-compose.yaml and resolved image pins against this
#      checkout's, check every new image is pullable, then ZFS-snapshot the DB,
#      bump HELIX_VERSION, converge every non-database service (api and chrome
#      both follow HELIX_VERSION), health-check, and roll back in place on
#      failure. Compose drift fails the deploy unless COMPOSE_SYNC=true, which
#      backs up the host's file and installs this checkout's (the host's
#      docker-compose.override.yaml is kept). Database services are recreated
#      only with RECREATE_POSTGRES=true and never across a PostgreSQL major:
#      the major is read from each running database's PG_VERSION, not from
#      .env (docs/postgres-upgrade.md).
#   2. runners: pull the released sandbox image; then, if the release ships the
#      same sandbox artifacts as the running one (helix.sandbox.fingerprint),
#      roll only the desktop image into the running sandbox and leave every
#      desktop on it running (scripts/lib/deploy-runner-remote.sh). Otherwise
#      the host's launcher is re-run, which recreates the sandbox.
#
# The hosts are reached over plain SSH with one dedicated CD key (the key's
# public half is authorised on each deployment host).
#
# Invoked by the `deploy-prod` Drone pipeline on a GREEN release tag (so the
# controlplane/sandbox images for $DRONE_TAG already exist on ghcr). Can also be
# run manually from a checkout of VERSION:  scripts/deploy-prod.sh VERSION
# VERSION is a release tag or the 7-character SHA of a green main build (main
# builds publish images tagged with it).
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
#   RUNNER_START_SCRIPT=  launcher with an image= assignment (alternative to upgrade script)
#   RUNNER_CONTAINER=helix-sandbox-app
#   RUNNER_FORCE_RECREATE=false  (true: recreate the runner's sandbox even when
#                                only the desktop image changed)
#   DEPLOY_CONTROLPLANE=true   (set false to skip the control plane)
#   DEPLOY_RUNNER=true   (set false to skip the runner upgrade)
#   COMPOSE_SYNC=false   (true: install this checkout's docker-compose.yaml on drift)
#   RECREATE_POSTGRES=false   (true: allow a same-major database image change)
#   SLACK_WEBHOOK_URL    optional notifications
# =============================================================================

VERSION="${1:-${DRONE_TAG:-}}"
if ! [[ "$VERSION" =~ ^([0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?|[0-9a-f]{7})$ ]]; then
  echo "usage: $0 VERSION  (or set DRONE_TAG); VERSION is a release tag or a 7-character main SHA" >&2
  exit 1
fi
REPO_DIR=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)

LONDON_SSH="${LONDON_SSH:-luke_helix_ml@34.39.116.64}"
LONDON_STACK_DIR="${LONDON_STACK_DIR:-/data/helix-app/helix}"
LONDON_DB_DATASET="${LONDON_DB_DATASET:-data/helix-postgres}"
LONDON_HEALTH_HOST="${LONDON_HEALTH_HOST:-app.helix.ml}"
RUNNER_SSH="${RUNNER_SSH:-root@code.helix.ml}"
RUNNER_UPGRADE_SCRIPT="${RUNNER_UPGRADE_SCRIPT:-/opt/HelixML/upgrade-sandbox-app.helix.ml.sh}"
RUNNER_START_SCRIPT="${RUNNER_START_SCRIPT:-}"
RUNNER_CONTAINER="${RUNNER_CONTAINER:-helix-sandbox-app}"
RUNNER_FORCE_RECREATE="${RUNNER_FORCE_RECREATE:-false}"
DEPLOY_CONTROLPLANE="${DEPLOY_CONTROLPLANE:-true}"
DEPLOY_RUNNER="${DEPLOY_RUNNER:-true}"
COMPOSE_SYNC="${COMPOSE_SYNC:-false}"
RECREATE_POSTGRES="${RECREATE_POSTGRES:-false}"

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

DEPLOYED=""

# --- 1. controlplane (london) ----------------------------------------------
if [ "$DEPLOY_CONTROLPLANE" = "true" ]; then
  # The compose file deployed is this checkout's, so it must be VERSION's.
  head=$(git -C "$REPO_DIR" rev-parse HEAD)
  want=$(git -C "$REPO_DIR" rev-parse -q --verify "${VERSION}^{commit}") ||
    fail "${VERSION} is not a tag or commit in ${REPO_DIR}; run from a checkout of ${VERSION}"
  [ "$head" = "$want" ] || fail "${REPO_DIR} is at ${head}, not ${VERSION} (${want})"
  git -C "$REPO_DIR" diff --quiet HEAD -- docker-compose.yaml ||
    fail "${REPO_DIR}/docker-compose.yaml has uncommitted changes"

  SNAP="${LONDON_DB_DATASET}@pre-${VERSION}-$(date +%Y%m%d-%H%M%S)"
  log "Deploying controlplane ${VERSION} to ${LONDON_SSH} (snapshot ${SNAP}) ..."

  CP_DEPLOY="$(printf '%s=%q\n' \
    STACK_DIR "$LONDON_STACK_DIR" \
    VERSION "$VERSION" \
    SNAP "$SNAP" \
    HEALTH_HOST "$LONDON_HEALTH_HOST" \
    COMPOSE_SYNC "$COMPOSE_SYNC" \
    RECREATE_POSTGRES "$RECREATE_POSTGRES" \
    REPO_COMPOSE_B64 "$(base64 < "$REPO_DIR/docker-compose.yaml" | tr -d '\n')")
$(cat "$REPO_DIR/scripts/lib/deploy-controlplane-remote.sh")"

  # shellcheck disable=SC2086
  if ! ssh $SSH_OPTS "$LONDON_SSH" 'bash -s' <<<"$CP_DEPLOY"; then
    fail "controlplane deploy failed: see the log above (nothing changes before the snapshot; a failed health-check rolls back in place). DB snapshot, if taken: ${SNAP}"
  fi
  log "controlplane on ${VERSION}"
  DEPLOYED="control plane"

  # TST-01-050: Go pprof must never be reachable anonymously through the public edge.
  for path in '/debug/pprof/' '/debug/pprof/goroutine?debug=1'; do
    body=$(curl -s --max-time 15 -w '\n%{http_code}' "https://${LONDON_HEALTH_HOST}${path}" || true)
    code=${body##*$'\n'}
    if [ "$code" = "200" ] || grep -qE 'Types of profiles available|^goroutine profile:' <<<"$body"; then
      fail "${path} is publicly reachable on ${LONDON_HEALTH_HOST} (HTTP ${code}) - TST-01-050 regression"
    fi
    [ "$code" = "000" ] && warn "could not verify ${path} on ${LONDON_HEALTH_HOST} (request failed)"
    log "edge check ${path}: HTTP ${code}"
  done
else
  log "DEPLOY_CONTROLPLANE=false - skipping control plane"
fi

# --- 2. runner --------------------------------------------------------------
if [ "$DEPLOY_RUNNER" = "true" ]; then
  log "Deploying runner ${VERSION} to ${RUNNER_SSH} ..."
  RUNNER_LAUNCHER="$RUNNER_UPGRADE_SCRIPT"
  RUNNER_LAUNCHER_KIND=tag
  if [ -n "$RUNNER_START_SCRIPT" ]; then
    RUNNER_LAUNCHER="$RUNNER_START_SCRIPT"
    RUNNER_LAUNCHER_KIND=image
  fi

  RUNNER_DEPLOY="$(printf '%s=%q\n' \
    VERSION "$VERSION" \
    SANDBOX_IMAGE "ghcr.io/helixml/helix-sandbox:$VERSION" \
    LAUNCHER "$RUNNER_LAUNCHER" \
    LAUNCHER_KIND "$RUNNER_LAUNCHER_KIND" \
    CONTAINER "$RUNNER_CONTAINER" \
    FORCE_RECREATE "$RUNNER_FORCE_RECREATE")
$(cat "$REPO_DIR/scripts/lib/deploy-runner-remote.sh")"

  # bash -s like the control plane: the runner script uses bash constructs and
  # reads the whole script before main() can consume the ssh stdin.
  if ! rsh "$RUNNER_SSH" 'bash -s' <<<"$RUNNER_DEPLOY"; then
    fail "runner deploy failed on ${RUNNER_SSH}"
  fi
  log "runner ${RUNNER_SSH} on ${VERSION}"
  DEPLOYED="${DEPLOYED:+${DEPLOYED}, }runner ${RUNNER_SSH}"

  printf -v quoted_version '%q' "$VERSION"
  RUNNER_IMAGE_REPORT="HELIX_RUNNER_CURRENT_VERSION=$quoted_version
$(cat "$REPO_DIR/scripts/cleanup-runner-images.sh")"
  if ! rsh "$RUNNER_SSH" "$RUNNER_IMAGE_REPORT"; then
    warn "runner image retention dry run failed on ${RUNNER_SSH}"
  fi
else
  log "DEPLOY_RUNNER=false - skipping runner upgrade"
fi

notify ":rocket: *prod deployed ${VERSION}*: ${DEPLOYED}"
log "prod deploy of ${VERSION} complete: ${DEPLOYED}"
