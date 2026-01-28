#!/bin/bash
#
# Integration test for Zed SSE MCP transport
#
# This script verifies that Zed can communicate with an MCP server using the
# legacy SSE transport (MCP 2024-11-05 spec).
#
# Test flow:
# 1. Verify SSE MCP secret server is running
# 2. Create/apply test agent with MCP config pointing to SSE server
# 3. Create a spec task using that agent
# 4. Ask the agent to get the secret using the MCP tool
# 5. Verify the response contains the expected secret value
# 6. Cleanup
#
# Prerequisites:
# - Helix stack running: ./stack start
# - SSE MCP secret server running: docker compose -f docker-compose.dev.yaml up sse-mcp-secret
# - Helix CLI built: cd api && go build -o /tmp/helix .
# - Environment configured: .env.usercreds with HELIX_API_KEY, HELIX_URL, HELIX_PROJECT
#
# Usage:
#   ./scripts/test-zed-sse-mcp.sh
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELIX_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HELIX_CLI="${HELIX_CLI:-/tmp/helix}"

# The secret value that the SSE MCP server returns
# This must match SECRET_VALUE in zed/script/test_sse_mcp_server.py
EXPECTED_SECRET="HELIX-SSE-MCP-SECRET-7f3a9b2c"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

cleanup() {
    log_info "Cleaning up..."
    if [ -n "${SESSION_ID:-}" ]; then
        log_info "Stopping session $SESSION_ID"
        "$HELIX_CLI" spectask stop "$SESSION_ID" 2>/dev/null || true
    fi
}

trap cleanup EXIT

# Load environment
load_env() {
    if [ -f "$HELIX_ROOT/.env.usercreds" ]; then
        log_info "Loading credentials from .env.usercreds"
        export HELIX_API_KEY=$(grep HELIX_API_KEY "$HELIX_ROOT/.env.usercreds" | cut -d= -f2-)
        export HELIX_URL=$(grep HELIX_URL "$HELIX_ROOT/.env.usercreds" | cut -d= -f2-)
        export HELIX_PROJECT=$(grep HELIX_PROJECT "$HELIX_ROOT/.env.usercreds" | cut -d= -f2-)
    else
        log_error ".env.usercreds not found"
        log_info "Create it with:"
        echo "  HELIX_API_KEY=hl-xxx"
        echo "  HELIX_URL=http://localhost:8080"
        echo "  HELIX_PROJECT=prj_xxx"
        exit 1
    fi

    if [ -z "${HELIX_API_KEY:-}" ] || [ -z "${HELIX_URL:-}" ]; then
        log_error "HELIX_API_KEY and HELIX_URL must be set"
        exit 1
    fi
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if [ ! -f "$HELIX_CLI" ]; then
        log_error "Helix CLI not found at $HELIX_CLI"
        log_info "Build it with: cd api && go build -o /tmp/helix ."
        exit 1
    fi
    log_success "Helix CLI found"

    # Check SSE MCP server is running
    if ! curl -sf --connect-timeout 5 "http://localhost:3333/health" >/dev/null 2>&1; then
        log_error "SSE MCP secret server not running on localhost:3333"
        log_info "Start it with: docker compose -f docker-compose.dev.yaml up sse-mcp-secret"
        exit 1
    fi
    log_success "SSE MCP secret server is running"

    # Verify it returns the expected secret
    local direct_secret
    direct_secret=$(curl -sf "http://localhost:3333/secret" 2>/dev/null || echo "")
    if [ "$direct_secret" != "$EXPECTED_SECRET" ]; then
        log_error "SSE server returned unexpected secret: '$direct_secret'"
        log_error "Expected: '$EXPECTED_SECRET'"
        exit 1
    fi
    log_success "SSE server secret verified"

    # Check Helix API is reachable
    if ! curl -sf --connect-timeout 5 "${HELIX_URL}/api/v1/status" >/dev/null 2>&1; then
        log_error "Helix API not reachable at $HELIX_URL"
        log_info "Start the stack with: ./stack start"
        exit 1
    fi
    log_success "Helix API is reachable"
}

# Create or update the test agent
setup_agent() {
    log_info "Setting up SSE MCP test agent..."
    
    local agent_config="$SCRIPT_DIR/test-zed-sse-mcp-agent.yaml"
    if [ ! -f "$agent_config" ]; then
        log_error "Agent config not found: $agent_config"
        exit 1
    fi

    # Apply the agent configuration
    if ! "$HELIX_CLI" agent apply -f "$agent_config" 2>&1; then
        log_error "Failed to apply agent configuration"
        exit 1
    fi
    log_success "Agent configuration applied"

    # Get the agent ID
    AGENT_ID=$("$HELIX_CLI" agent list --json 2>/dev/null | jq -r '.[] | select(.name == "sse-mcp-test-agent") | .id' | head -1)
    if [ -z "$AGENT_ID" ] || [ "$AGENT_ID" = "null" ]; then
        log_error "Failed to get agent ID for sse-mcp-test-agent"
        exit 1
    fi
    log_success "Agent ID: $AGENT_ID"
}

# Start a spec task session
start_session() {
    log_info "Starting spec task session..."

    if [ -z "${HELIX_PROJECT:-}" ]; then
        log_error "HELIX_PROJECT not set in .env.usercreds"
        exit 1
    fi

    # Start the session
    local output
    output=$("$HELIX_CLI" spectask start \
        --agent "$AGENT_ID" \
        --project "$HELIX_PROJECT" \
        --name "SSE MCP Integration Test" 2>&1)
    
    SESSION_ID=$(echo "$output" | grep -oP 'ses_[a-zA-Z0-9]+' | head -1)
    if [ -z "$SESSION_ID" ]; then
        log_error "Failed to start session. Output: $output"
        exit 1
    fi
    log_success "Session started: $SESSION_ID"

    # Wait for session to be ready
    log_info "Waiting for session to initialize (60s)..."
    sleep 60
}

# Send prompt and verify response
test_secret_retrieval() {
    log_info "Sending prompt to retrieve secret..."

    local prompt="What is the secret? Use the get_secret tool to find out and tell me the exact value."
    
    # Send the message and wait for completion
    "$HELIX_CLI" spectask send "$SESSION_ID" "$prompt" --wait --max-wait 120 2>&1 || true

    # Give it a moment for the response to be fully processed
    sleep 5

    # Get the conversation history
    log_info "Retrieving conversation history..."
    local history
    history=$("$HELIX_CLI" spectask interact "$SESSION_ID" --history --json 2>/dev/null || echo "{}")

    # Check if the secret appears in the response
    if echo "$history" | grep -q "$EXPECTED_SECRET"; then
        log_success "Secret found in response!"
        log_success "The agent successfully retrieved '$EXPECTED_SECRET' via SSE MCP"
        return 0
    else
        log_error "Secret NOT found in response"
        log_info "Expected to find: $EXPECTED_SECRET"
        log_info "Response history:"
        echo "$history" | head -100
        return 1
    fi
}

# Main
main() {
    echo ""
    echo "=========================================="
    echo "  Zed SSE MCP Transport Integration Test"
    echo "=========================================="
    echo ""

    load_env
    check_prerequisites
    setup_agent
    start_session

    if test_secret_retrieval; then
        echo ""
        log_success "============================================"
        log_success "  TEST PASSED: SSE MCP transport works!"
        log_success "============================================"
        echo ""
        exit 0
    else
        echo ""
        log_error "============================================"
        log_error "  TEST FAILED: SSE MCP transport broken"
        log_error "============================================"
        echo ""
        exit 1
    fi
}

main "$@"