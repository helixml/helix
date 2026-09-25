#!/usr/bin/env bash
set -euo pipefail

TARGET_SHA="${1:-${DRONE_COMMIT_SHA:-}}"
if [[ ! "$TARGET_SHA" =~ ^[0-9a-f]{40}$ ]]; then
  echo "usage: $0 COMMIT_SHA" >&2
  exit 1
fi
: "${META_DEPLOY_SSH_KEY:?META_DEPLOY_SSH_KEY is required}"

KEY_FILE=$(mktemp)
KNOWN_HOSTS_FILE=$(mktemp)
trap 'rm -f "$KEY_FILE" "$KNOWN_HOSTS_FILE"' EXIT
printf '%s' "$META_DEPLOY_SSH_KEY" | base64 -d > "$KEY_FILE"
chmod 600 "$KEY_FILE"
printf '%s\n' 'node01.lukemarsden.net ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJAfN0P9SELddDmpBwgQNnL+r2+af0U5We3ds89NjrwO' > "$KNOWN_HOSTS_FILE"

ssh \
  -i "$KEY_FILE" \
  -o BatchMode=yes \
  -o IdentitiesOnly=yes \
  -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$KNOWN_HOSTS_FILE" \
  -o ConnectTimeout=20 \
  -o ServerAliveInterval=30 \
  luke@node01.lukemarsden.net \
  bash -s -- "$TARGET_SHA" <<'REMOTE'
set -euo pipefail

TARGET_SHA=$1
exec 9>/tmp/helix-meta-deploy.lock
if ! flock -w 3600 9; then
  echo "Timed out after 1 hour waiting for another Meta deployment" >&2
  exit 1
fi

HELIX_DIR=/prod/home/luke/pm/helix
ZED_DIR=/prod/home/luke/pm/zed
STATE_FILE=/prod/home/luke/.local/state/helix-meta-deploy
PRE_DEPLOY_SHA=$(git -C "$HELIX_DIR" rev-parse HEAD)

report_failure() {
  status=$?
  if (( status != 0 )); then
    echo "Meta deployment failed. Pre-deploy Helix SHA: $PRE_DEPLOY_SHA" >&2
    echo "Inspect Meta and restore that SHA manually if recovery is required." >&2
  fi
}
trap report_failure EXIT
echo "Pre-deploy Helix SHA: $PRE_DEPLOY_SHA"

for repo in "$HELIX_DIR" "$ZED_DIR"; do
  branch=$(git -C "$repo" symbolic-ref --quiet --short HEAD) || {
    echo "$repo is not on a branch" >&2
    exit 1
  }
  if [[ "$branch" != main ]]; then
    echo "$repo is on $branch, expected main" >&2
    exit 1
  fi
  if [[ -n $(git -C "$repo" status --porcelain --untracked-files=no) ]]; then
    echo "$repo has tracked changes" >&2
    exit 1
  fi
done

git -C "$HELIX_DIR" fetch origin main
git -C "$HELIX_DIR" cat-file -e "$TARGET_SHA^{commit}"
git -C "$HELIX_DIR" merge-base --is-ancestor "$TARGET_SHA" origin/main || {
  echo "$TARGET_SHA is not on origin/main" >&2
  exit 1
}

CURRENT_SHA=$PRE_DEPLOY_SHA
if git -C "$HELIX_DIR" merge-base --is-ancestor "$TARGET_SHA" "$CURRENT_SHA"; then
  if [[ "$CURRENT_SHA" != "$TARGET_SHA" ]]; then
    echo "Meta is ahead of $TARGET_SHA at $CURRENT_SHA; refusing to report a stale deployment" >&2
    exit 1
  fi
elif git -C "$HELIX_DIR" merge-base --is-ancestor "$CURRENT_SHA" "$TARGET_SHA"; then
  git -C "$HELIX_DIR" merge --ff-only "$TARGET_SHA"
else
  echo "Meta main cannot fast-forward from $CURRENT_SHA to $TARGET_SHA" >&2
  exit 1
fi

git -C "$ZED_DIR" pull --ff-only origin main
ZED_SHA_AFTER=$(git -C "$ZED_DIR" rev-parse HEAD)

deployed_helix_sha=
deployed_zed_sha=
if [[ -f "$STATE_FILE" ]]; then
  read -r deployed_helix_sha deployed_zed_sha < "$STATE_FILE"
  if [[ ! "$deployed_helix_sha" =~ ^[0-9a-f]{40}$ || ! "$deployed_zed_sha" =~ ^[0-9a-f]{40}$ ]]; then
    echo "Invalid Meta deployment state in $STATE_FILE" >&2
    exit 1
  fi
fi

cd "$HELIX_DIR"
(cd frontend && yarn build)

if [[ -z "$deployed_helix_sha" ]] ||
  ! git diff --quiet "$deployed_helix_sha" "$TARGET_SHA" -- go.mod go.sum; then
  ./stack up --force-recreate --no-deps api
fi

zed_changed=false
if [[ -z "$deployed_zed_sha" || "$deployed_zed_sha" != "$ZED_SHA_AFTER" || ! -f zed-build/zed ]] ||
  ! git diff --quiet "$deployed_helix_sha" "$TARGET_SHA" -- Dockerfile.zed-build; then
  zed_changed=true
  ./stack build-zed release
fi

ubuntu_changed=false
if [[ -z "$deployed_helix_sha" ]] || ! git diff --quiet "$deployed_helix_sha" "$TARGET_SHA" -- \
  Dockerfile.ubuntu-helix go.mod go.sum api/ desktop/ mcp-servers/drone-ci/ \
  qwen-code-build/ sandbox-versions.txt WORKDIR_README.md; then
  ubuntu_changed=true
fi

sandbox_changed=false
if [[ -z "$deployed_helix_sha" ]] || ! git diff --quiet "$deployed_helix_sha" "$TARGET_SHA" -- \
  .dockerignore Dockerfile.sandbox stack go.mod go.sum api/ sandbox/ \
  desktop/sway-config/setup-telemetry-firewall.sh; then
  sandbox_changed=true
fi

if [[ "$zed_changed" == true || "$sandbox_changed" == true ]]; then
  ./stack build-sandbox
elif [[ "$ubuntu_changed" == true ]]; then
  ./stack build-ubuntu
else
  echo "Zed, Ubuntu, and sandbox inputs unchanged; skipping image builds"
fi

if [[ "$zed_changed" == true || "$ubuntu_changed" == true || "$sandbox_changed" == true ]]; then
  ubuntu_version=$(<sandbox-images/helix-ubuntu.version)
  if [[ ! "$ubuntu_version" =~ ^[0-9a-f]{6}$ ]]; then
    echo "Invalid Ubuntu image version: $ubuntu_version" >&2
    exit 1
  fi
  sandbox_id=$(docker ps --filter label=com.docker.compose.service \
    --format '{{.ID}} {{.Label "com.docker.compose.service"}}' \
    | awk '$2 ~ /^sandbox-(nvidia|amd-intel|software|macos)$/ && !found { print $1; found=1 }')
  if [[ -z "$sandbox_id" ]] ||
    ! docker exec "$sandbox_id" docker image inspect "helix-ubuntu:$ubuntu_version" >/dev/null; then
    echo "Ubuntu image helix-ubuntu:$ubuntu_version is not loaded in the sandbox" >&2
    exit 1
  fi
fi

if [[ $(git rev-parse HEAD) != "$TARGET_SHA" ]]; then
  echo "Meta HEAD changed during deployment" >&2
  exit 1
fi
for _ in $(seq 1 60); do
  if curl -fsS --max-time 10 -D - -o /dev/null http://localhost:8080/api/v1/config \
    | grep -Eiq '^content-type:[[:space:]]*application/json([[:space:]]*;|[[:space:]]*$)'; then
    mkdir -p "$(dirname "$STATE_FILE")"
    state_tmp=$(mktemp "$STATE_FILE.XXXXXX")
    printf '%s %s\n' "$TARGET_SHA" "$ZED_SHA_AFTER" > "$state_tmp"
    mv "$state_tmp" "$STATE_FILE"
    echo "Meta is healthy at $TARGET_SHA"
    exit 0
  fi
  sleep 5
done
echo "Meta did not become healthy at $TARGET_SHA" >&2
exit 1
REMOTE
