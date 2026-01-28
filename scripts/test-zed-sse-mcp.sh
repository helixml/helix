#!/bin/bash
#
# Test Zed SSE MCP transport by asking an agent to retrieve a secret
#
# Prerequisites:
#   docker compose -f docker-compose.dev.yaml up sse-mcp-secret
#   cd api && go build -o /tmp/helix .
#   Create .env.usercreds with HELIX_API_KEY, HELIX_URL, HELIX_PROJECT
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SECRET="HELIX-SSE-MCP-SECRET-7f3a9b2c"

# Load credentials
set -a && source "$SCRIPT_DIR/../.env.usercreds" && set +a

# Verify SSE server is running and has the secret
curl -sf http://localhost:3333/secret | grep -q "$SECRET" || {
    echo "ERROR: SSE server not running. Start with: docker compose -f docker-compose.dev.yaml up sse-mcp-secret"
    exit 1
}

# Create/update the test agent
/tmp/helix agent apply -f "$SCRIPT_DIR/test-zed-sse-mcp-agent.yaml"

# Get agent ID
AGENT_ID=$(/tmp/helix agent list --json | jq -r '.[] | select(.name == "sse-mcp-test-agent") | .id' | head -1)

# Start session
echo "Starting session..."
SESSION_ID=$(/tmp/helix spectask start --agent "$AGENT_ID" --project "$HELIX_PROJECT" -n "SSE MCP Test" 2>&1 | grep -oP 'ses_[a-zA-Z0-9]+' | head -1)
trap "/tmp/helix spectask stop $SESSION_ID 2>/dev/null || true" EXIT

echo "Session: $SESSION_ID"
echo "Waiting for Zed to initialize..."
sleep 60

# Ask for the secret
echo "Asking agent for the secret..."
/tmp/helix spectask send "$SESSION_ID" "Use the get_secret tool and tell me exactly what it returns." --wait --max-wait 120 || true

# Check if secret is in response
sleep 5
HISTORY=$(/tmp/helix spectask interact "$SESSION_ID" --history --json 2>/dev/null || echo "{}")

if echo "$HISTORY" | grep -q "$SECRET"; then
    echo "✓ PASSED: Agent retrieved '$SECRET' via SSE MCP"
    exit 0
else
    echo "✗ FAILED: Secret not found in response"
    echo "$HISTORY" | tail -20
    exit 1
fi