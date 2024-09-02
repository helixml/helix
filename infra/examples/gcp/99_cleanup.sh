#!/bin/bash
set -euo pipefail

# Check if PROJECT_ID is set
if [ -z "${PROJECT_ID:-}" ]; then
    echo "Error: PROJECT_ID is not set. Please set it to your GCP project ID before running this script."
    exit 1
fi

# Set variables
ZONE="${ZONE:-us-central1-a}"
CONTROLPLANE_INSTANCE_NAME="controlplane"
RUNNER_INSTANCE_NAME="runner"

# Delete the controlplane instance
gcloud compute instances delete $CONTROLPLANE_INSTANCE_NAME \
    --project=$PROJECT_ID \
    --zone=$ZONE \
    --quiet

echo "Instance $CONTROLPLANE_INSTANCE_NAME has been deleted from zone $ZONE."

# Delete the runner instance
gcloud compute instances delete $RUNNER_INSTANCE_NAME \
    --project=$PROJECT_ID \
    --zone=$ZONE \
    --quiet

echo "Instance $RUNNER_INSTANCE_NAME has been deleted from zone $ZONE."

echo "Cleanup complete."