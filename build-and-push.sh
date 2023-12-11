#!/bin/bash
set -xeuo pipefail
IMAGE="quay.io/lukemarsden/helix-runner:v0.1.0"
docker build -f Dockerfile.runner -t $IMAGE .
docker push $IMAGE
