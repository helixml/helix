#!/bin/bash
set -xeuo pipefail
IMAGE="quay.io/lukemarsden/helix-runner:v0.0.6"
docker build -f Dockerfile.runner -t $IMAGE .
docker push $IMAGE
