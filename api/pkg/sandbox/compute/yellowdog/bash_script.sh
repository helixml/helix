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
# Hardware-specific bits are parameterised via task Environment so a
# pool with non-NVIDIA GPUs (or no GPU at all) can override them at
# task-submit time without forking this script. The defaults match the
# NVIDIA POC pool so existing deployments keep working unchanged.
#
# Overridable via task Environment (defaults in parens):
#   GPU_VENDOR     - container env var passed to helix-sandbox (nvidia)
#   GPU_FLAGS      - docker run hardware flags (--gpus all)
#   DEVICE_FLAGS   - docker run --device flags (NVIDIA DRI nodes)
#   MAX_SANDBOXES  - per-runner sandbox cap (2)
#
# When Helix grows multi-profile support these become first-class
# Provider.Spec fields and the Provider populates the task Environment
# from the profile. See titan/helix/design/2026-06-09-yd-bash-script-alternatives.md.
set -euo pipefail

HELIX_URL="${1:?missing arg 1: control plane URL}"
RUNNER_TOK="${2:?missing arg 2: runner token}"
IMG="${3:?missing arg 3: helix-sandbox image}"

WORKER_TAG="${YD_WORKER_SLOT:-default}"
CONTAINER_NAME="yd-helix-runner-${WORKER_TAG}"
SANDBOX_ID="${SANDBOX_INSTANCE_ID:-yd-inline-$(date -u +%Y%m%d-%H%M%S)-w${WORKER_TAG}}"

GPU_VENDOR="${GPU_VENDOR:-nvidia}"
MAX_SANDBOXES="${MAX_SANDBOXES:-2}"
# GPU_FLAGS / DEVICE_FLAGS are unquoted on use - operator-provided
# strings are tokenised by the shell to become individual docker run args.
GPU_FLAGS="${GPU_FLAGS:---gpus all}"
DEVICE_FLAGS="${DEVICE_FLAGS:---device /dev/dri/renderD128 --device /dev/dri/card1}"

echo "=== Helix runner $(date -u +%Y-%m-%dT%H:%M:%SZ) ==="
echo "helix_url=$HELIX_URL"
echo "image=$IMG"
echo "container_name=$CONTAINER_NAME"
echo "sandbox_id=$SANDBOX_ID"
echo "gpu_vendor=$GPU_VENDOR gpu_flags=$GPU_FLAGS device_flags=$DEVICE_FLAGS max_sandboxes=$MAX_SANDBOXES"

exec sudo docker run --rm --name "$CONTAINER_NAME" \
  --privileged $GPU_FLAGS $DEVICE_FLAGS \
  -v /var/lib/docker \
  -e HELIX_API_URL="$HELIX_URL" \
  -e RUNNER_TOKEN="$RUNNER_TOK" \
  -e SANDBOX_INSTANCE_ID="$SANDBOX_ID" \
  -e GPU_VENDOR="$GPU_VENDOR" \
  -e MAX_SANDBOXES="$MAX_SANDBOXES" \
  "$IMG"
