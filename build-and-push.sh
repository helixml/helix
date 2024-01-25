#!/bin/bash
set -xeuo pipefail
IMAGE="europe-docker.pkg.dev/helixml/helix/runner:dev"
docker build -f Dockerfile.runner -t $IMAGE .
# docker push $IMAGE
