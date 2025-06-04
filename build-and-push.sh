#!/bin/bash
set -xeuo pipefail
IMAGE="registry.helixml.tech/helix/runner:ns-dev-0.0.1"
docker build --push --platform linux/amd64 -f Dockerfile.runner -t $IMAGE .
# docker push $IMAGE
