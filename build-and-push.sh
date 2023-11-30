#!/bin/bash
set -xeuo pipefail
IMAGE="quay.io/lukemarsden/helix-runner:v0.0.7"
docker build -f Dockerfile.runner -t $IMAGE .
docker push $IMAGE
