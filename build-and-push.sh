#!/bin/bash
set -xeuo pipefail
IMAGE="registry.helix.ml/helix/runner:dev"
docker build -f Dockerfile.runner -t $IMAGE .
# docker push $IMAGE
