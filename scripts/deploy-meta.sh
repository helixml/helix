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
flock 9

HELIX_DIR=/prod/home/luke/pm/helix
ZED_DIR=/prod/home/luke/pm/zed

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

CURRENT_SHA=$(git -C "$HELIX_DIR" rev-parse HEAD)
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

cd "$HELIX_DIR"
./stack build
./stack build-zed release
./stack build-sandbox
./stack start

if [[ $(git rev-parse HEAD) != "$TARGET_SHA" ]]; then
  echo "Meta HEAD changed during deployment" >&2
  exit 1
fi
for _ in $(seq 1 60); do
  if curl -fsS --max-time 10 http://localhost:8080/healthz >/dev/null; then
    echo "Meta is healthy at $TARGET_SHA"
    exit 0
  fi
  sleep 5
done
echo "Meta did not become healthy at $TARGET_SHA" >&2
exit 1
REMOTE
