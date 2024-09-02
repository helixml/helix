#!/bin/bash
set -euo pipefail

# Check if PROJECT_ID is set
if [ -z "${PROJECT_ID:-}" ]; then
    echo "Error: PROJECT_ID is not set. Please set it to your GCP project ID before running this script."
    exit 1
fi

# Set variables
ZONE="${ZONE:-us-central1-a}"
CONTROLPLANE_MACHINE_TYPE="${CONTROLPLANE_MACHINE_TYPE:-n2-standard-2}"
CONTROLPLANE_BOOT_DISK_SIZE="${CONTROLPLANE_BOOT_DISK_SIZE:-500GB}"
RUNNER_MACHINE_TYPE="${RUNNER_MACHINE_TYPE:-g2-standard-4}"
RUNNER_BOOT_DISK_SIZE="${RUNNER_BOOT_DISK_SIZE:-500GB}"

IMAGE_FAMILY="ubuntu-2204-lts"
IMAGE_PROJECT="ubuntu-os-cloud"

# Instance names
CONTROLPLANE_INSTANCE_NAME="controlplane"
RUNNER_INSTANCE_NAME="runner"
GPU_TYPE="nvidia-l4"
GPU_COUNT="1"

# Create the controlplane instance (without GPU)
gcloud compute instances create $CONTROLPLANE_INSTANCE_NAME \
    --project=$PROJECT_ID \
    --zone=$ZONE \
    --machine-type=$CONTROLPLANE_MACHINE_TYPE \
    --image-family=$IMAGE_FAMILY \
    --image-project=$IMAGE_PROJECT \
    --boot-disk-size=$CONTROLPLANE_BOOT_DISK_SIZE \
    --maintenance-policy=TERMINATE \
    --restart-on-failure \
    --scopes="https://www.googleapis.com/auth/cloud-platform"

echo "Instance $CONTROLPLANE_INSTANCE_NAME has been created in zone $ZONE without a GPU."

# Create the runner instance (with L4 GPU)
gcloud compute instances create $RUNNER_INSTANCE_NAME \
    --project=$PROJECT_ID \
    --zone=$ZONE \
    --machine-type=$RUNNER_MACHINE_TYPE \
    --accelerator=type=$GPU_TYPE,count=$GPU_COUNT \
    --image-family=$IMAGE_FAMILY \
    --image-project=$IMAGE_PROJECT \
    --boot-disk-size=$RUNNER_BOOT_DISK_SIZE \
    --maintenance-policy=TERMINATE \
    --restart-on-failure \
    --metadata="install-nvidia-driver=True" \
    --scopes="https://www.googleapis.com/auth/cloud-platform"

# TODO: nvidia driver isn't running

echo "Instance $RUNNER_INSTANCE_NAME has been created in zone $ZONE with an $GPU_TYPE GPU."