#!/usr/bin/env bash
# Embedded helix-sandbox launcher for YellowDog workers.
#
# Shipped inline in the YD task arguments via `bash -c <this body>`
# rather than uploaded to S3 separately. The Helix Go binary embeds
# this file via go:embed; the Provider sends it to YD as the body
# of a bash task.
#
# Invocation (set by yellowdog.Provider.taskArguments):
#   /bin/bash -c <this body> "yd-inline" $helix_url $runner_token $helix_image
#
# That makes $0="yd-inline", $1=helix_url, $2=runner_token, $3=helix_image
# inside this script - matching the existing yellowdog-poc bash-script.sh
# convention so SANDBOX_INSTANCE_ID, container naming, etc. all behave
# identically.
#
# Hardware-specific bits (--gpus all, NVIDIA device paths,
# MAX_SANDBOXES=2) stay here for now. When Helix grows multi-profile
# support these become first-class Provider.Spec fields and this
# script becomes a Go template. See
# titan/helix/design/2026-06-09-yd-bash-script-alternatives.md.
set -euo pipefail

HELIX_URL="${1:?missing arg 1: control plane URL}"
RUNNER_TOK="${2:?missing arg 2: runner token}"
IMG="${3:?missing arg 3: helix-sandbox image}"

WORKER_TAG="${YD_WORKER_SLOT:-default}"
CONTAINER_NAME="yd-helix-runner-${WORKER_TAG}"
SANDBOX_ID="${SANDBOX_INSTANCE_ID:-yd-inline-$(date -u +%Y%m%d-%H%M%S)-w${WORKER_TAG}}"

echo "=== Helix runner $(date -u +%Y-%m-%dT%H:%M:%SZ) ==="
echo "helix_url=$HELIX_URL"
echo "image=$IMG"
echo "container_name=$CONTAINER_NAME"
echo "sandbox_id=$SANDBOX_ID"

exec sudo docker run --rm --name "$CONTAINER_NAME" \
  --privileged --gpus all \
  --device /dev/dri/renderD128 \
  --device /dev/dri/card1 \
  -v /var/lib/docker \
  -e HELIX_API_URL="$HELIX_URL" \
  -e RUNNER_TOKEN="$RUNNER_TOK" \
  -e SANDBOX_INSTANCE_ID="$SANDBOX_ID" \
  -e GPU_VENDOR=nvidia \
  -e MAX_SANDBOXES=2 \
  "$IMG"
