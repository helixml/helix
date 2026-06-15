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

# Cleanup on task abort or normal exit. Without this, a YD task cancel
# (CANCELLING transition - YD agent sends SIGTERM to this script) leaves
# the helix-sandbox container running, which holds the GPU slot and
# prevents the next task on this worker from launching. Patterned after
# YD's own docker-run.sh userdata script. Docker --rm alone isn't enough:
# it covers clean exits but not abort-by-signal, and not SIGKILL at all.
cleanup() {
  echo "=== Cleanup: stopping container $CONTAINER_NAME ==="
  sudo docker stop -t 10 "$CONTAINER_NAME" 2>/dev/null || true
  sudo docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
}
trap cleanup EXIT
# Re-raise on signal so the EXIT trap fires AND the right exit code is
# reported back to the YD agent (143 = 128+SIGTERM, 130 = 128+SIGINT).
trap 'exit 143' TERM
trap 'exit 130' INT

# Pre-flight: remove any stale container with the same name from a
# previous task on this worker that died without firing its EXIT trap
# (e.g. killed by SIGKILL, OOM, or worker host reboot). Without this,
# `docker run --name` will fail with "container name already in use"
# and the new task will never start.
sudo docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

# Run docker in the foreground (NOT exec) so this bash process stays
# alive and owns signal handling. With `exec`, bash is replaced by
# docker and the trap above never fires.
sudo docker run --rm --name "$CONTAINER_NAME" \
  --privileged $GPU_FLAGS $DEVICE_FLAGS \
  -v /var/lib/docker \
  -e HELIX_API_URL="$HELIX_URL" \
  -e RUNNER_TOKEN="$RUNNER_TOK" \
  -e SANDBOX_INSTANCE_ID="$SANDBOX_ID" \
  -e GPU_VENDOR="$GPU_VENDOR" \
  -e MAX_SANDBOXES="$MAX_SANDBOXES" \
  "$IMG"
