#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

# Load environment variables from .env file
if [[ -f .env ]]; then
    source .env
else
    echo "Error: .env file not found"
    exit 1
fi

# Check if realm.json file exists
if [[ ! -f realm.json ]]; then
    echo "Error: realm.json file not found"
    exit 1
fi

# Function to update field in realm.json file
update_realm_attribute() {
    # Update field in realm.json
    jq --arg frontendUrl ${KEYCLOAK_FRONTEND_URL:-"http://localhost/auth"} '.attributes.frontendUrl |= $frontendUrl' realm.json > tmp.json
    mv tmp.json realm.json
}

update_realm_client() {
    # Update field in realm.json
    jq --arg clientId frontend --arg baseUrl ${SERVER_URL:-"http://localhost/"} '( .clients[] | select(.clientId == $clientId) ).baseUrl |= $baseUrl' realm.json > tmp.json
    mv tmp.json realm.json
}

# Update fields in realm.json file
update_realm_attribute
update_realm_client
# Add more fields as needed

echo "Realm JSON file updated successfully"