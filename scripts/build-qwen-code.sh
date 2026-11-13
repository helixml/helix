#!/bin/sh
# Install the Qwen Code release pinned in sandbox-versions.txt into
# qwen-code-build/. Run from the repository root.
set -eu
. ./sandbox-versions.txt
docker build --provenance=false \
  --build-arg QWEN_VERSION="$QWEN_VERSION" \
  --output type=local,dest=qwen-code-build \
  -f Dockerfile.qwen-build .
test "$(cat qwen-code-build/version)" = "$QWEN_VERSION"
echo "Qwen Code $QWEN_VERSION installed"
