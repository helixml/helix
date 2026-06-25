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
if [ "$GPU_VENDOR" = "neuron" ]; then
  # AWS Inferentia/Trainium: no --gpus / runtime. Mount every /dev/neuron*
  # device node (count varies by inf2/trn SKU) so hydra can pass them through
  # to the nested Docker. Globbed here on the host because the control plane
  # can't know the device-node count. Operator-provided DEVICE_FLAGS wins.
  GPU_FLAGS="${GPU_FLAGS:-}"
  if [ -z "${DEVICE_FLAGS:-}" ]; then
    DEVICE_FLAGS=""
    for nd in /dev/neuron*; do
      [ -e "$nd" ] && DEVICE_FLAGS="$DEVICE_FLAGS --device $nd"
    done
  fi
else
  GPU_FLAGS="${GPU_FLAGS:---gpus all}"
  DEVICE_FLAGS="${DEVICE_FLAGS:---device /dev/dri/renderD128 --device /dev/dri/card1}"
fi

echo "=== Helix runner $(date -u +%Y-%m-%dT%H:%M:%SZ) ==="
echo "helix_url=$HELIX_URL"
echo "image=$IMG"
echo "container_name=$CONTAINER_NAME"
echo "sandbox_id=$SANDBOX_ID"
echo "gpu_vendor=$GPU_VENDOR gpu_flags=$GPU_FLAGS device_flags=$DEVICE_FLAGS max_sandboxes=$MAX_SANDBOXES"

# Cleanup on task abort or normal exit. Without this, a YD task cancel
# leaves the helix-sandbox container running, holding the GPU slot
# and blocking the next task on this worker. Docker --rm covers clean
# container exits but not abort-by-signal.
cleanup() {
  echo "=== Cleanup: stopping container $CONTAINER_NAME ==="
  sudo docker stop -t 10 "$CONTAINER_NAME" 2>/dev/null || true
  sudo docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
  if [ -n "${DOCKER_PID:-}" ]; then
    wait "$DOCKER_PID" 2>/dev/null || true
  fi
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

# ECR auth. When HELIX_SANDBOX_REGISTRY points the image at a private
# ECR (<acct>.dkr.ecr.<region>.amazonaws.com), the docker pull below
# needs a login - the default ghcr.io image is public so this is a
# no-op for existing deployments. The worker authenticates via its EC2
# instance profile (no creds shipped in the task); the region is read
# from the host segment of the image ref. Mirror the image first with
# scripts/yd-mirror-sandbox-image.sh.
ECR_HOST="${IMG%%/*}"
if [[ "$ECR_HOST" == *.dkr.ecr.*.amazonaws.com ]]; then
  ECR_REGION=$(echo "$ECR_HOST" | cut -d. -f4)
  echo "=== ECR login: $ECR_HOST (region $ECR_REGION) ==="
  aws ecr get-login-password --region "$ECR_REGION" \
    | sudo docker login --username AWS --password-stdin "$ECR_HOST"
fi

# Run docker BACKGROUNDED (& + wait) rather than in the foreground.
# Bash queues signals received during a foreground synchronous command
# and only delivers them after that command exits - so a SIGTERM from
# YD on abort would sit queued behind `docker run`, which never exits
# on its own, and the EXIT/TERM traps above would never fire. The
# previous "foreground (NOT exec)" comment got that wrong: foreground
# DOES make bash the parent process, but it ALSO makes bash unable to
# act on signals until the child terminates.
#
# The `wait` builtin DOES respond to signals immediately: it returns
# with status > 128 (= 128 + signal number) and bash runs the trap,
# which stops the container, which causes the docker client to exit,
# which our subsequent wait inside cleanup() then reaps. Net effect:
# a clean SIGTERM path from YD to the helix-sandbox container.
#
# Caveat: this is correct on the bash side. Whether YD's agent
# actually delivers SIGTERM on task abort is a separate concern -
# empirically (2026-06-16) `yd-abort` returned success but tasks
# stayed EXECUTING, suggesting YD's abort is sometimes a platform
# status flip without OS-signal delivery. When YD does signal, this
# fix lets us shut down cleanly; when it doesn't, the only recourse
# is `yd-terminate` on the Compute Requirement (which kills the VM).
# DELIVERY CONTRACT: this `-e` list is the ONLY way a control-plane / operator
# env var reaches compose-manager (and everything else) inside the sandbox on a
# YD-provisioned runner. If you add a var that compose-manager or the sandbox
# reads, it MUST be forwarded here (or set by install.sh for bare runners) or it
# is silently dead on YD hosts. Values for the optional knobs below arrive via
# the YD task environment (provider.taskEnvironment) and so are in THIS script's
# env. MUST use the `-e NAME="$NAME"` value form (expanded by the shell before
# `sudo`): the docker run below is `sudo docker run`, and sudo's env_reset
# strips the var, so the bare `-e NAME` passthrough form silently delivers
# NOTHING. (GPU_VENDOR uses the value form for exactly this reason; the bare
# form was an earlier bug that dropped these knobs on every YD runner.) Only
# forward when set so an unconfigured knob leaves compose-manager on its default.
EXTRA_ENV=""
[ -n "${HELIX_NEURON_COMPILE_CACHE_URL:-}" ] && EXTRA_ENV="$EXTRA_ENV -e HELIX_NEURON_COMPILE_CACHE_URL=$HELIX_NEURON_COMPILE_CACHE_URL"
[ -n "${HELIX_RUNNER_READINESS_TIMEOUT:-}" ] && EXTRA_ENV="$EXTRA_ENV -e HELIX_RUNNER_READINESS_TIMEOUT=$HELIX_RUNNER_READINESS_TIMEOUT"

sudo docker run --rm --name "$CONTAINER_NAME" \
  --privileged $GPU_FLAGS $DEVICE_FLAGS \
  -v /var/lib/docker \
  -e HELIX_API_URL="$HELIX_URL" \
  -e RUNNER_TOKEN="$RUNNER_TOK" \
  -e SANDBOX_INSTANCE_ID="$SANDBOX_ID" \
  -e GPU_VENDOR="$GPU_VENDOR" \
  -e MAX_SANDBOXES="$MAX_SANDBOXES" \
  $EXTRA_ENV \
  "$IMG" &
DOCKER_PID=$!
wait "$DOCKER_PID"
