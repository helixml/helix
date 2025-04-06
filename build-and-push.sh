#!/bin/bash
set -xeuo pipefail
IMAGE="registry.helixml.tech/helix/runner:dev"
docker build -f Dockerfile.runner -t $IMAGE .
# docker push $IMAGE
