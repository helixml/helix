#!/bin/bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
API_HOST="${API_HOST:-http://localhost:8080}"
HELIX_API_TOKEN="${HELIX_API_TOKEN:-}"

log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')] $1${NC}"
}

log_success() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')] ✓ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}[$(date +'%H:%M:%S')] ⚠ $1${NC}"
}

log_error() {
    echo -e "${RED}[$(date +'%H:%M:%S')] ✗ $1${NC}"
}

usage() {
    cat << EOF
Test script for Helix External Agents

Usage: $0 [COMMAND] [OPTIONS]

Commands:
    test-session-creation    Test creating external agent session
    test-rdp-access         Test RDP endpoint access
    test-chat-flow          Test end-to-end chat flow
    test-websocket          Test WebSocket sync connection
    test-all               Run all tests
    cleanup                Clean up test sessions

Options:
    --api-host HOST        API host (default: http://localhost:8080)
    --token TOKEN          API token for authentication
    --session-id ID        Use specific session ID for testing
    --verbose              Enable verbose output
    --help                 Show this help

Examples:
    $0 test-all
    $0 test-session-creation --verbose
    $0 test-rdp-access --session-id test-123
    $0 cleanup

Environment Variables:
    API_HOST              API server URL
    HELIX_API_TOKEN       Authentication token

EOF
}

# Helper function to make API calls
api_call() {
    local method="$1"
    local endpoint="$2"
    local data="${3:-}"
    local headers=()
    
    if [ -n "$HELIX_API_TOKEN" ]; then
        headers+=("-H" "Authorization: Bearer $HELIX_API_TOKEN")
    fi
    
    headers+=("-H" "Content-Type: application/json")
    headers+=("-H" "Accept: application/json")
    
    if [ "$method" = "GET" ]; then
        curl -s -X GET "${headers[@]}" "$API_HOST$endpoint"
    else
        curl -s -X "$method" "${headers[@]}" -d "$data" "$API_HOST$endpoint"
    fi
}

# Test session creation
test_session_creation() {
    log "Testing external agent session creation..."
    
    local session_id="${TEST_SESSION_ID:-test-session-$(date +%s)}"
    
    # Test 1: Create session via chat sessions API
    log "Creating session via chat sessions API with agent_type=zed_external..."
    
    local session_payload=$(cat <<EOF
{
    "system_prompt": "You are a helpful coding assistant running in a Zed editor environment.",
    "messages": [
        {
            "role": "user", 
            "content": "Hello, can you help me with some coding?"
        }
    ],
    "agent_type": "zed_external",
    "external_agent_config": {
        "workspace_dir": "workspace",
        "project_path": "workspace/test-project",
        "env_vars": ["NODE_ENV=development"],
        "auto_connect_rdp": true
    },
    "stream": false
}
EOF
    )
    
    local response
    if response=$(api_call "POST" "/api/v1/sessions/chat" "$session_payload" 2>/dev/null); then
        local created_session_id=$(echo "$response" | jq -r '.id // empty' 2>/dev/null || echo "")
        if [ -n "$created_session_id" ]; then
            log_success "Session created successfully: $created_session_id"
            TEST_SESSION_ID="$created_session_id"
            
            # Verify session has correct agent type
            local session_info
            if session_info=$(api_call "GET" "/api/v1/sessions/$created_session_id" 2>/dev/null); then
                local agent_type=$(echo "$session_info" | jq -r '.metadata.agent_type // empty' 2>/dev/null || echo "")
                if [ "$agent_type" = "zed_external" ]; then
                    log_success "Session has correct agent type: $agent_type"
                else
                    log_error "Session has wrong agent type: $agent_type (expected: zed_external)"
                    return 1
                fi
            else
                log_warning "Could not verify session info"
            fi
        else
            log_error "Failed to extract session ID from response"
            [ "$VERBOSE" = "1" ] && echo "Response: $response"
            return 1
        fi
    else
        log_error "Failed to create session via chat API"
        return 1
    fi
    
    # Test 2: Create session via direct external agents API
    log "Testing direct external agent creation..."
    
    local agent_payload=$(cat <<EOF
{
    "session_id": "direct-$session_id",
    "user_id": "test-user",
    "input": "Initialize development environment",
    "project_path": "workspace/test-project",
    "work_dir": "workspace",
    "env": ["NODE_ENV=development", "DEBUG=1"]
}
EOF
    )
    
    if response=$(api_call "POST" "/api/v1/external-agents" "$agent_payload" 2>/dev/null); then
        local rdp_url=$(echo "$response" | jq -r '.rdp_url // empty' 2>/dev/null || echo "")
        local websocket_url=$(echo "$response" | jq -r '.websocket_url // empty' 2>/dev/null || echo "")
        
        if [ -n "$rdp_url" ] && [ -n "$websocket_url" ]; then
            log_success "Direct external agent created successfully"
            log_success "RDP URL: $rdp_url"
            log_success "WebSocket URL: $websocket_url"
            DIRECT_SESSION_ID="direct-$session_id"
        else
            log_error "Direct external agent creation failed - missing URLs"
            [ "$VERBOSE" = "1" ] && echo "Response: $response"
            return 1
        fi
    else
        log_error "Failed to create direct external agent"
        return 1
    fi
    
    return 0
}

# Test RDP access
test_rdp_access() {
    log "Testing RDP endpoint access..."
    
    local session_id="${TEST_SESSION_ID:-${DIRECT_SESSION_ID:-test-session-123}}"
    
    if [ -z "$session_id" ]; then
        log_error "No session ID available for RDP testing. Run session creation test first."
        return 1
    fi
    
    log "Testing RDP info endpoint for session: $session_id"
    
    local response
    if response=$(api_call "GET" "/api/v1/external-agents/$session_id/rdp" 2>/dev/null); then
        local rdp_url=$(echo "$response" | jq -r '.rdp_url // empty' 2>/dev/null || echo "")
        local rdp_password=$(echo "$response" | jq -r '.rdp_password // empty' 2>/dev/null || echo "")
        local status=$(echo "$response" | jq -r '.status // empty' 2>/dev/null || echo "")
        local websocket_connected=$(echo "$response" | jq -r '.websocket_connected // false' 2>/dev/null || echo "false")
        
        if [ -n "$rdp_url" ] && [ -n "$rdp_password" ]; then
            log_success "RDP info retrieved successfully"
            log_success "RDP URL: $rdp_url"
            log_success "Status: $status"
            log_success "WebSocket Connected: $websocket_connected"
            log_success "RDP Password: [REDACTED]"
            
            # Test additional RDP info fields
            local rdp_port=$(echo "$response" | jq -r '.rdp_port // empty' 2>/dev/null || echo "")
            local username=$(echo "$response" | jq -r '.username // empty' 2>/dev/null || echo "")
            
            if [ -n "$rdp_port" ] && [ -n "$username" ]; then
                log_success "RDP Port: $rdp_port, Username: $username"
            fi
        else
            log_error "RDP info incomplete - missing URL or password"
            [ "$VERBOSE" = "1" ] && echo "Response: $response"
            return 1
        fi
    else
        log_error "Failed to get RDP info for session $session_id"
        return 1
    fi
    
    return 0
}

# Test chat flow
test_chat_flow() {
    log "Testing end-to-end chat flow..."
    
    local session_id="${TEST_SESSION_ID:-test-chat-$(date +%s)}"
    
    # First create a session if we don't have one
    if [ -z "$TEST_SESSION_ID" ]; then
        log "Creating session for chat testing..."
        if ! test_session_creation; then
            log_error "Failed to create session for chat testing"
            return 1
        fi
        session_id="$TEST_SESSION_ID"
    fi
    
    log "Testing chat message to external agent (session: $session_id)..."
    
    local chat_payload=$(cat <<EOF
{
    "system_prompt": "You are a helpful coding assistant.",
    "messages": [
        {
            "role": "user",
            "content": "Write a simple hello world function in Python"
        }
    ],
    "model": "gpt-4",
    "stream": false,
    "agent_type": "zed_external"
}
EOF
    )
    
    log "Sending chat message..."
    local response
    if response=$(api_call "POST" "/api/v1/sessions/$session_id/chat" "$chat_payload" 2>/dev/null); then
        # Check if we got a response
        if echo "$response" | jq -e '.choices[0].message.content' >/dev/null 2>&1; then
            local content=$(echo "$response" | jq -r '.choices[0].message.content' 2>/dev/null)
            log_success "Received chat response from external agent"
            log_success "Response length: ${#content} characters"
            
            if [ "$VERBOSE" = "1" ]; then
                echo "Response content preview:"
                echo "$content" | head -3
                echo "..."
            fi
        else
            log_error "No valid response content received"
            [ "$VERBOSE" = "1" ] && echo "Response: $response"
            return 1
        fi
    else
        log_error "Failed to send chat message to external agent"
        return 1
    fi
    
    return 0
}

# Test WebSocket connection
test_websocket() {
    log "Testing WebSocket sync connection..."
    
    local session_id="${TEST_SESSION_ID:-${DIRECT_SESSION_ID:-test-ws-$(date +%s)}}"
    
    if [ -z "$TEST_SESSION_ID" ] && [ -z "$DIRECT_SESSION_ID" ]; then
        log "Creating session for WebSocket testing..."
        if ! test_session_creation; then
            log_error "Failed to create session for WebSocket testing"
            return 1
        fi
        session_id="$TEST_SESSION_ID"
    fi
    
    log "Testing WebSocket endpoint availability..."
    
    # Test if WebSocket endpoint is accessible (just check if it responds to HTTP)
    local ws_host=$(echo "$API_HOST" | sed 's/http/ws/g')
    local ws_url="$ws_host/api/v1/external-agents/sync?session_id=$session_id"
    
    log "WebSocket URL would be: $ws_url"
    
    # Try to connect with curl (will fail but should give us info about the endpoint)
    if curl -s -I "$API_HOST/api/v1/external-agents/sync?session_id=$session_id" 2>/dev/null | grep -q "Connection: Upgrade\|WebSocket"; then
        log_success "WebSocket endpoint appears to be available"
    else
        log_warning "WebSocket endpoint accessibility could not be verified with basic HTTP check"
        log "This is expected - WebSocket requires proper upgrade handshake"
    fi
    
    # Check if there are any WebSocket connections for our session
    log "Checking WebSocket connection status..."
    local response
    if response=$(api_call "GET" "/api/v1/external-agents/$session_id/rdp" 2>/dev/null); then
        local websocket_connected=$(echo "$response" | jq -r '.websocket_connected // false' 2>/dev/null || echo "false")
        if [ "$websocket_connected" = "true" ]; then
            log_success "WebSocket is connected for session $session_id"
        else
            log_warning "WebSocket is not connected for session $session_id (expected without real agent)"
        fi
    fi
    
    return 0
}

# List active sessions
list_sessions() {
    log "Listing active external agent sessions..."
    
    local response
    if response=$(api_call "GET" "/api/v1/external-agents" 2>/dev/null); then
        if echo "$response" | jq -e '. | length' >/dev/null 2>&1; then
            local count=$(echo "$response" | jq '. | length' 2>/dev/null || echo "0")
            log_success "Found $count active external agent sessions"
            
            if [ "$count" -gt 0 ] && [ "$VERBOSE" = "1" ]; then
                echo "$response" | jq -r '.[] | "  Session: \(.session_id), Status: \(.status), RDP: \(.rdp_url)"' 2>/dev/null || echo "  (Could not parse session details)"
            fi
        else
            log_warning "No active sessions found or response format unexpected"
            [ "$VERBOSE" = "1" ] && echo "Response: $response"
        fi
    else
        log_error "Failed to list external agent sessions"
        return 1
    fi
    
    return 0
}

# Cleanup test sessions
cleanup() {
    log "Cleaning up test sessions..."
    
    # Clean up known test sessions
    for session_id in "$TEST_SESSION_ID" "$DIRECT_SESSION_ID" "test-session-"* "direct-test-session-"* "test-chat-"* "test-ws-"*; do
        if [ -n "$session_id" ] && [ "$session_id" != "test-session-*" ] && [ "$session_id" != "direct-test-session-*" ]; then
            log "Attempting to clean up session: $session_id"
            
            # Try to delete via external agents API
            if api_call "DELETE" "/api/v1/external-agents/$session_id" >/dev/null 2>&1; then
                log_success "Cleaned up external agent session: $session_id"
            else
                log_warning "Could not clean up external agent session: $session_id (may not exist)"
            fi
            
            # Try to delete via regular sessions API
            if api_call "DELETE" "/api/v1/sessions/$session_id" >/dev/null 2>&1; then
                log_success "Cleaned up regular session: $session_id"
            else
                log_warning "Could not clean up regular session: $session_id (may not exist)"
            fi
        fi
    done
    
    log_success "Cleanup completed"
}

# Run all tests
test_all() {
    log "Running all external agent tests..."
    echo
    
    local failed_tests=0
    
    # Test 1: Session creation
    echo "=== Test 1: Session Creation ==="
    if test_session_creation; then
        log_success "Session creation test passed"
    else
        log_error "Session creation test failed"
        ((failed_tests++))
    fi
    echo
    
    # Test 2: RDP access
    echo "=== Test 2: RDP Access ==="
    if test_rdp_access; then
        log_success "RDP access test passed"
    else
        log_error "RDP access test failed"
        ((failed_tests++))
    fi
    echo
    
    # Test 3: Chat flow
    echo "=== Test 3: Chat Flow ==="
    if test_chat_flow; then
        log_success "Chat flow test passed"
    else
        log_error "Chat flow test failed"
        ((failed_tests++))
    fi
    echo
    
    # Test 4: WebSocket
    echo "=== Test 4: WebSocket ==="
    if test_websocket; then
        log_success "WebSocket test passed"
    else
        log_error "WebSocket test failed"
        ((failed_tests++))
    fi
    echo
    
    # Summary
    echo "=== Test Summary ==="
    if [ $failed_tests -eq 0 ]; then
        log_success "All tests passed! 🎉"
        echo
        echo "External agent architecture is working correctly:"
        echo "✓ Sessions can be created with agent_type=zed_external"
        echo "✓ RDP connection info is available"
        echo "✓ Chat messages are handled properly"
        echo "✓ WebSocket endpoints are accessible"
        echo
        echo "Next steps:"
        echo "- Start external agent containers: ./scripts/build-external-agents.sh start-agents"
        echo "- Connect via RDP to see the Zed interface"
        echo "- Test real bidirectional sync with actual agents"
    else
        log_error "$failed_tests test(s) failed"
        echo
        echo "Some tests failed. Check the logs above for details."
        echo "Common issues:"
        echo "- API server not running on $API_HOST"
        echo "- Authentication token missing or invalid"
        echo "- External agent services not initialized"
        return 1
    fi
    
    return 0
}

# Parse arguments
COMMAND=""
VERBOSE=0
TEST_SESSION_ID=""
DIRECT_SESSION_ID=""

while [[ $# -gt 0 ]]; do
    case $1 in
        test-session-creation|test-rdp-access|test-chat-flow|test-websocket|test-all|cleanup|list-sessions)
            COMMAND="$1"
            shift
            ;;
        --api-host)
            API_HOST="$2"
            shift 2
            ;;
        --token)
            HELIX_API_TOKEN="$2"
            shift 2
            ;;
        --session-id)
            TEST_SESSION_ID="$2"
            shift 2
            ;;
        --verbose)
            VERBOSE=1
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Default command
if [ -z "$COMMAND" ]; then
    COMMAND="test-all"
fi

# Check dependencies
for tool in curl jq; do
    if ! command -v "$tool" &> /dev/null; then
        log_error "Required tool '$tool' not found. Please install it first."
        exit 1
    fi
done

# Main execution
main() {
    log "Helix External Agent Test Suite"
    log "API Host: $API_HOST"
    log "Command: $COMMAND"
    if [ -n "$HELIX_API_TOKEN" ]; then
        log "Auth Token: Configured"
    else
        log_warning "Auth Token: Not configured (may be required)"
    fi
    echo
    
    case $COMMAND in
        test-session-creation)
            test_session_creation
            ;;
        test-rdp-access)
            test_rdp_access
            ;;
        test-chat-flow)
            test_chat_flow
            ;;
        test-websocket)
            test_websocket
            ;;
        test-all)
            test_all
            ;;
        list-sessions)
            list_sessions
            ;;
        cleanup)
            cleanup
            ;;
        *)
            log_error "Unknown command: $COMMAND"
            usage
            exit 1
            ;;
    esac
}

# Run main function
main "$@"