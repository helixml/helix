#!/usr/bin/env bash
# Mirror the helix-sandbox image for a given Helix version from ghcr.io
# into a private ECR in the YD AWS account, so YD workers pull the ~10GB
# image intra-region (~15-30s) instead of cross-cloud from GHCR (~120s).
#
# Usage:
#   AWS_REGION=us-east-1 ECR_ACCOUNT_ID=123456789012 \
#     scripts/yd-mirror-sandbox-image.sh <helix-version>
#
# Example:
#   scripts/yd-mirror-sandbox-image.sh 2.11.17
#     ghcr.io/helixml/helix-sandbox:2.11.17
#       -> 123456789012.dkr.ecr.us-east-1.amazonaws.com/helixml/helix-sandbox:2.11.17
#
# After mirroring, point the control plane at the mirror:
#   HELIX_SANDBOX_REGISTRY=<acct>.dkr.ecr.<region>.amazonaws.com
# (resolved in api/pkg/sandbox/compute/bootstrap/bootstrap.go; the YD
# worker authenticates to ECR via its instance profile - see bash_script.sh).
#
# Requires: aws CLI (credentials/region for the YD account), docker with
# buildx. GITHUB_TOKEN is used to log in to ghcr.io only if the source
# image is private.
set -euo pipefail

VERSION="${1:?usage: yd-mirror-sandbox-image.sh <helix-version> (e.g. 2.11.17)}"
: "${AWS_REGION:?set AWS_REGION to the YD AWS account region (e.g. us-east-1)}"
: "${ECR_ACCOUNT_ID:?set ECR_ACCOUNT_ID to the YD AWS account id}"

IMAGE_PATH="helixml/helix-sandbox"          # must match sandboxImagePath in bootstrap.go
SRC="ghcr.io/${IMAGE_PATH}:${VERSION}"
ECR_HOST="${ECR_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"
DST="${ECR_HOST}/${IMAGE_PATH}:${VERSION}"

# Optional: ghcr login if the source is private (no-op for public images).
if [ -n "${GITHUB_TOKEN:-}" ]; then
  echo "$GITHUB_TOKEN" | docker login ghcr.io -u "${GITHUB_USER:-helixml}" --password-stdin
fi

# ECR repo must exist before push; create-repository is not idempotent.
aws ecr describe-repositories --region "$AWS_REGION" --repository-names "$IMAGE_PATH" >/dev/null 2>&1 \
  || aws ecr create-repository --region "$AWS_REGION" --repository-name "$IMAGE_PATH" >/dev/null

aws ecr get-login-password --region "$AWS_REGION" \
  | docker login --username AWS --password-stdin "$ECR_HOST"

# imagetools copies the full multi-arch manifest registry-to-registry -
# no 10GB local pull, and every arch (amd64 worker, graviton, inf2) is
# preserved. docker pull/tag/push would flatten to the host's arch only.
echo "Mirroring $SRC -> $DST"
docker buildx imagetools create -t "$DST" "$SRC"

echo "Done. Set on the control plane: HELIX_SANDBOX_REGISTRY=$ECR_HOST"
