#!/bin/bash
set -xeuo pipefail
IMAGE="europe-docker.pkg.dev/helixml/helix/runner:v0.2.5"
docker build -f Dockerfile.runner -t $IMAGE .
docker push $IMAGE
