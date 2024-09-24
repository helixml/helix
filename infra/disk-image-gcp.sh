#!/bin/bash
set -euo pipefail

# Get the directory of the current script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Define variables
PACKER_TEMPLATE="$SCRIPT_DIR/gcp/packer.json"

# Function to build the GCP image
build_gcp_image() {
    echo "Starting GCP image build process..."
    packer build $PACKER_TEMPLATE
    echo "GCP image build process completed."
}

# Execute the build function
build_gcp_image
