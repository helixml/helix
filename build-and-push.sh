#!/bin/bash
set -xeuo pipefail
#IMAGE="public.ecr.aws/s8w9i7x4/helix:v0.0.10"
IMAGE="europe-docker.pkg.dev/helixml/helix/runner:v0.0.10"
docker build -f Dockerfile.runner -t $IMAGE .
docker push $IMAGE
