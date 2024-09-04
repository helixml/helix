#!/bin/bash
set -euo pipefail

# Check if HELIX_API_KEY and HELIX_URL are set
if [ -z "${HELIX_API_KEY:-}" ]; then
    echo "Error: HELIX_API_KEY is not set. Please set it before running this script."
    exit 1
fi

if [ -z "${HELIX_URL:-}" ]; then
    echo "Error: HELIX_URL is not set. Please set it before running this script."
    exit 1
fi

# Check if Helix CLI is installed
if ! command -v helix &> /dev/null
then
    echo "Error: Helix CLI is not installed. Please install it before running this script."
    exit 1
fi

echo "Helix CLI is installed."
echo "HELIX_API_KEY and HELIX_URL are properly set."

# Create a temporary directory
TMP_DIR=$(mktemp -d)

echo "Temporary directory created successfully into $TMP_DIR"

# Clone the repository
git clone https://github.com/helixml/example-helix-app "$TMP_DIR"

echo "Repository cloned successfully into $TMP_DIR"

# # Clean up function
# cleanup() {
#     echo "Cleaning up..."
#     rm -rf "$TMP_DIR"
# }

# # Set up trap to call cleanup function on script exit
# trap cleanup EXIT

# Navigate to the cloned repository
cd "$TMP_DIR"

# Run the Helix CLI command
APP_ID=$(helix apply -f helix.yaml 2>/dev/null)
echo "Got app id: $APP_ID"

curl --request POST \
  --url ${HELIX_URL}/api/v1/sessions/chat \
  --header "Authorization: Bearer ${HELIX_API_KEY}" \
  --header 'Content-Type: application/json' \
  --data "{
    \"app_id\": \"${APP_ID}\",
    \"messages\": [
      {
        \"role\": \"user\",
        \"content\": { \"content_type\": \"text\", \"parts\": [\"what job is Marcus applying for?\"] }
      }
    ]
  }"
