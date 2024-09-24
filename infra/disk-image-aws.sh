#!/bin/bash
set -euo pipefail

# Get the directory of the current script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Define variables
PACKER_TEMPLATE="$SCRIPT_DIR/aws/packer.json"

# Function to build the AMI
build_ami() {
    echo "Starting AMI build process..."
    packer build $PACKER_TEMPLATE
    echo "AMI build process completed."
}

# Execute the build function
build_ami
