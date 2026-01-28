#!/bin/bash
# Test Zed SSE MCP transport by asking an agent to retrieve a secret
set -euo pipefail

SECRET="HELIX-SSE-MCP-SECRET-7f3a9b2c"
HELIX="${HELIX_CLI:-/tmp/helix}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load credentials
set -a && source "$DIR/../.env.usercreds" && set +a

# Verify SSE server has the secret
curl -sf http://localhost:3333/secret | grep -q "$SECRET" || {
    echo "Start SSE server: docker compose -f docker-compose.dev.yaml up sse-mcp-secret"
    exit 1
}

# Create agent and capture ID
AGENT=$($HELIX agent apply -f "$DIR/test-zed-sse-mcp-agent.yaml" | tail -1)
echo "Agent: $AGENT"

# Start session
SESSION=$($HELIX spectask start --agent "$AGENT" --project "$HELIX_PROJECT" -n "SSE MCP Test" 2>&1 | grep -oP 'ses_\w+' | head -1)
echo "Session: $SESSION"
trap "$HELIX spectask stop $SESSION 2>/dev/null || true" EXIT

# Wait for Zed
echo "Waiting for Zed..."
sleep 60

# Ask for the secret and get response
RESPONSE=$($HELIX spectask send "$SESSION" "Use the get_secret tool and tell me exactly what it returns." --wait --max-wait 120 2>/dev/null || echo "")

# Check result
if echo "$RESPONSE" | grep -q "$SECRET"; then
    echo "✓ PASSED: Agent retrieved '$SECRET' via SSE MCP"
else
    echo "✗ FAILED: Secret not found"
    echo "$RESPONSE"
    exit 1
fi