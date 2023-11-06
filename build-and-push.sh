#!/bin/bash
IMAGE="quay.io/lukemarsden/helix-runner:v0.0.1"
docker build -t $IMAGE .
docker push $IMAGE
