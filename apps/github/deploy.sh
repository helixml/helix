#!/bin/bash
set -e

if [ ! -f .env ]; then
    echo "Error: .env file not found"
    echo "Please copy .env.example to .env and fill in your GitHub credentials"
    exit 1
fi

# Load environment variables
source .env

# Check required environment variables
if [ -z "$GITHUB_API_TOKEN" ]; then
    echo "Error: GITHUB_API_TOKEN is not set in .env"
    exit 1
fi

if [ -z "$GITHUB_OWNER" ]; then
    echo "Error: GITHUB_OWNER is not set in .env"
    exit 1
fi

# Process the helix.yaml file with environment variables
envsubst < helix.yaml > helix.processed.yaml

# Deploy to Helix (changed from 'apps' to 'app')
helix app deploy helix.processed.yaml

# Clean up
rm helix.processed.yaml

echo "Deployment complete!"
