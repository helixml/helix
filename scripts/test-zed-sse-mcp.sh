#!/bin/bash
# Test Zed SSE MCP transport by asking an agent to retrieve a secret
set -euo pipefail

SECRET="HELIX-SSE-MCP-SECRET-7f3a9b2c"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

alias helix="${HELIX_CLI:-/tmp/helix}"
shopt -s expand_aliases

# Load credentials
set -a && source "$DIR/../.env.usercreds" && set +a

# Start SSE server
docker rm -f sse-mcp-test 2>/dev/null || true
docker run -d --name sse-mcp-test --network helix_default -p 3333:3333 \
    -v "$DIR/../../zed/script/test_sse_mcp_server.py:/app/server.py:ro" \
    python:3.11-slim python /app/server.py 3333
trap "docker rm -f sse-mcp-test 2>/dev/null || true" EXIT

# Wait for SSE server
for i in {1..10}; do curl -sf http://localhost:3333/secret | grep -q "$SECRET" && break; sleep 1; done

# Create agent
AGENT=$(helix agent apply -f "$DIR/test-zed-sse-mcp-agent.yaml" | tail -1)
echo "Agent: $AGENT"

# Start session
SESSION=$(helix spectask start -q --agent "$AGENT" --project "$HELIX_PROJECT" -n "SSE MCP Test")
echo "Session: $SESSION"
trap "docker rm -f sse-mcp-test 2>/dev/null || true; helix spectask stop $SESSION 2>/dev/null || true" EXIT

# Ask for the secret
RESPONSE=$(helix spectask send "$SESSION" "Use the get_secret tool and tell me exactly what it returns." --wait --max-wait 180)

# Check result
if echo "$RESPONSE" | grep -q "$SECRET"; then
    echo "✓ PASSED: Agent retrieved '$SECRET' via SSE MCP"
else
    echo "✗ FAILED: Secret not found"
    echo "$RESPONSE"
    exit 1
fi