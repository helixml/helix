#!/bin/bash

# Debug script for external agent runners
# This script helps monitor and debug the external agent system

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}🔍 Helix External Agent Debug Tool${NC}"
echo -e "${CYAN}====================================${NC}"

# Function to show usage
show_usage() {
    echo -e "${YELLOW}Usage:${NC}"
    echo "  $0 [command]"
    echo ""
    echo -e "${YELLOW}Commands:${NC}"
    echo "  logs        - Show all external agent related logs"
    echo "  api-logs    - Show API server logs with EXTERNAL_AGENT_DEBUG"
    echo "  runner-logs - Show runner container logs"
    echo "  status      - Show runner connection status"
    echo "  restart     - Restart external agent runners"
    echo "  build       - Rebuild and restart external agent system"
    echo "  test        - Test session creation"
    echo "  nats        - Show NATS stream information"
    echo ""
    echo -e "${YELLOW}Examples:${NC}"
    echo "  $0 logs"
    echo "  $0 status"
    echo "  $0 test"
}

# Function to check if docker compose is available
check_docker_compose() {
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}❌ docker not found${NC}"
        exit 1
    fi
    if ! docker compose version &> /dev/null; then
        echo -e "${RED}❌ docker compose not available${NC}"
        exit 1
    fi
}

# Function to show API logs with external agent debug info
show_api_logs() {
    echo -e "${BLUE}📊 API Server External Agent Debug Logs${NC}"
    echo -e "${BLUE}===========================================${NC}"
    
    docker logs helix-api-1 2>&1 | grep "EXTERNAL_AGENT_DEBUG" | tail -50 | while read line; do
        if echo "$line" | grep -q "❌\|ERROR\|error"; then
            echo -e "${RED}$line${NC}"
        elif echo "$line" | grep -q "✅\|🟢\|success"; then
            echo -e "${GREEN}$line${NC}"
        elif echo "$line" | grep -q "⚠\|🟠\|WARN\|warn"; then
            echo -e "${YELLOW}$line${NC}"
        else
            echo "$line"
        fi
    done
}

# Function to show runner logs
show_runner_logs() {
    echo -e "${PURPLE}🏃 External Agent Runner Logs${NC}"
    echo -e "${PURPLE}==============================${NC}"
    
    # Get all zed-runner containers
    containers=$(docker ps --filter "name=helix-zed-runner" --format "{{.Names}}" | sort)
    
    if [ -z "$containers" ]; then
        echo -e "${RED}❌ No external agent runner containers found${NC}"
        return 1
    fi
    
    for container in $containers; do
        echo -e "${CYAN}📦 $container logs:${NC}"
        echo -e "${CYAN}$(printf '%.0s-' {1..50})${NC}"
        
        # Show supervisor logs for helix-external-agent-runner
        echo -e "${BLUE}📄 Helix External Agent Runner stdout:${NC}"
        docker exec "$container" cat /var/log/helix-external-agent-runner.log 2>/dev/null | tail -20 | while read line; do
            if echo "$line" | grep -q "EXTERNAL_AGENT_DEBUG"; then
                if echo "$line" | grep -q "❌\|ERROR\|error"; then
                    echo -e "${RED}$line${NC}"
                elif echo "$line" | grep -q "✅\|🟢\|success"; then
                    echo -e "${GREEN}$line${NC}"
                elif echo "$line" | grep -q "⚠\|🟠\|WARN\|warn"; then
                    echo -e "${YELLOW}$line${NC}"
                else
                    echo -e "${BLUE}$line${NC}"
                fi
            else
                echo "$line"
            fi
        done
        
        echo -e "${BLUE}📄 Helix External Agent Runner stderr:${NC}"
        docker exec "$container" cat /var/log/helix-external-agent-runner.err.log 2>/dev/null | tail -10 | while read line; do
            if echo "$line" | grep -q "ERROR\|error\|FATAL\|fatal"; then
                echo -e "${RED}$line${NC}"
            elif echo "$line" | grep -q "WARN\|warn"; then
                echo -e "${YELLOW}$line${NC}"
            else
                echo "$line"
            fi
        done
        
        echo -e "${BLUE}📄 Supervisor logs:${NC}"
        docker exec "$container" tail -10 /var/log/supervisor/supervisord.log 2>/dev/null | while read line; do
            if echo "$line" | grep -q "helix-external-agent-runner"; then
                if echo "$line" | grep -q "FATAL\|failed\|error"; then
                    echo -e "${RED}$line${NC}"
                elif echo "$line" | grep -q "RUNNING\|success"; then
                    echo -e "${GREEN}$line${NC}"
                else
                    echo -e "${YELLOW}$line${NC}"
                fi
            else
                echo -e "\033[2m$line\033[0m"
            fi
        done
        echo ""
    done
}

# Function to show all external agent logs
show_all_logs() {
    echo -e "${GREEN}📋 All External Agent Related Logs${NC}"
    echo -e "${GREEN}===================================${NC}"
    echo ""
    
    show_api_logs
    echo ""
    show_runner_logs
}

# Function to show runner connection status
show_status() {
    echo -e "${CYAN}📊 External Agent Runner Status${NC}"
    echo -e "${CYAN}================================${NC}"
    
    # Check running containers
    echo -e "${YELLOW}🐳 Running Containers:${NC}"
    docker ps --filter "name=helix-zed-runner" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
    echo ""
    
    # Check API connection logs
    echo -e "${YELLOW}🔗 Recent Connection Events:${NC}"
    docker logs helix-api-1 2>&1 | grep -E "(runner_connected|runner_disconnect|Connected external agent)" | tail -10
    echo ""
    
    # Check NATS stream info if available
    echo -e "${YELLOW}📡 NATS Stream Status:${NC}"
    docker logs helix-api-1 2>&1 | grep -E "(ZED_AGENTS|stream|queue)" | grep -v "DEBUG" | tail -5 || echo "No NATS stream info found"
}

# Function to restart runners
restart_runners() {
    echo -e "${YELLOW}🔄 Restarting External Agent Runners...${NC}"
    
    # Restart all zed-runner services
    docker compose -f docker-compose.dev.yaml restart zed-runner
    
    echo -e "${GREEN}✅ External agent runners restarted${NC}"
    
    # Wait a moment for them to start
    echo -e "${BLUE}⏳ Waiting for runners to reconnect...${NC}"
    sleep 5
    
    show_status
}

# Function to rebuild and restart
rebuild_system() {
    echo -e "${YELLOW}🔨 Rebuilding External Agent System...${NC}"
    
    # Stop current runners
    docker compose -f docker-compose.dev.yaml stop zed-runner
    
    # Rebuild the container
    docker compose -f docker-compose.dev.yaml build zed-runner
    
    # Start with scaled runners
    docker compose -f docker-compose.dev.yaml up -d --scale zed-runner=3 zed-runner
    
    echo -e "${GREEN}✅ External agent system rebuilt and restarted${NC}"
    
    # Wait for reconnection
    echo -e "${BLUE}⏳ Waiting for runners to reconnect...${NC}"
    sleep 10
    
    show_status
}

# Function to test session creation
test_session() {
    echo -e "${CYAN}🧪 Testing External Agent Session Creation${NC}"
    echo -e "${CYAN}===========================================${NC}"
    
    # Check if API is responding
    echo -e "${BLUE}🔍 Checking API health...${NC}"
    if curl -s http://localhost:8080/api/v1/health > /dev/null; then
        echo -e "${GREEN}✅ API is responding${NC}"
    else
        echo -e "${RED}❌ API is not responding${NC}"
        return 1
    fi
    
    # Check runner connections
    echo -e "${BLUE}🔍 Checking runner connections...${NC}"
    connection_count=$(docker logs helix-api-1 2>&1 | grep "Connected external agent runner websocket" | wc -l)
    echo -e "${GREEN}📊 Found $connection_count runner connections in logs${NC}"
    
    echo ""
    echo -e "${YELLOW}💡 To test session creation, use the Helix frontend at:${NC}"
    echo -e "${CYAN}   http://localhost:8080${NC}"
    echo ""
    echo -e "${YELLOW}💡 Create a session with agent_type: 'zed_external'${NC}"
    echo -e "${YELLOW}   and monitor logs with: $0 logs${NC}"
}

# Function to show NATS information
show_nats_info() {
    echo -e "${PURPLE}📡 NATS Stream Information${NC}"
    echo -e "${PURPLE}==========================${NC}"
    
    echo -e "${BLUE}🔍 NATS Stream Creation:${NC}"
    docker logs helix-api-1 2>&1 | grep -E "ZED_AGENTS.*stream" | tail -10
    
    echo ""
    echo -e "${BLUE}🔍 NATS Stream Requests:${NC}"
    docker logs helix-api-1 2>&1 | grep -E "StreamRequest.*ZED_AGENTS" | tail -10
    
    echo ""
    echo -e "${BLUE}🔍 NATS Stream Subscriptions:${NC}"
    docker logs helix-api-1 2>&1 | grep -E "subscribe.*ZED_AGENTS\|ZED_AGENTS.*subscribe" | tail -10
}

# Main script logic
main() {
    check_docker_compose
    
    case "${1:-logs}" in
        "logs")
            show_all_logs
            ;;
        "api-logs")
            show_api_logs
            ;;
        "runner-logs")
            show_runner_logs
            ;;
        "status")
            show_status
            ;;
        "restart")
            restart_runners
            ;;
        "build")
            rebuild_system
            ;;
        "test")
            test_session
            ;;
        "nats")
            show_nats_info
            ;;
        "help"|"-h"|"--help")
            show_usage
            ;;
        *)
            echo -e "${RED}❌ Unknown command: $1${NC}"
            echo ""
            show_usage
            exit 1
            ;;
    esac
}

# Run the main function
main "$@"