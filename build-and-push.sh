#!/bin/bash
IMAGE="quay.io/lukemarsden/helix-runner:v0.0.1"
docker build -f Dockerfile.runner -t $IMAGE .
docker push $IMAGE
