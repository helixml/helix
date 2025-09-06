#!/bin/bash

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Configuration
ZED_BUILD_DIR="${PROJECT_ROOT}/zed-build"
DOCKER_TAG_PREFIX="helix"
DOCKER_TAG_VERSION="${DOCKER_TAG_VERSION:-latest}"

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

log_success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] ✓ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] ⚠ $1${NC}"
}

log_error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ✗ $1${NC}"
}

usage() {
    cat << EOF
Build script for Helix External Agents

Usage: $0 [COMMAND] [OPTIONS]

Commands:
    build-zed       Build Zed with external agent support
    build-agents    Build external agent Docker containers
    build-all       Build everything (Zed + containers)
    test-agents     Test external agents
    start-agents    Start external agent pool
    stop-agents     Stop external agent pool
    status          Show external agent status
    logs            Show external agent logs
    clean           Clean up build artifacts

Options:
    --tag TAG       Docker tag version (default: latest)
    --agents N      Number of agent instances (default: 2)
    --force         Force rebuild even if up to date
    --verbose       Enable verbose output
    --help          Show this help message

Examples:
    $0 build-all                    # Build everything
    $0 build-agents --tag v1.0.0    # Build with custom tag
    $0 start-agents --agents 5      # Start 5 agent instances
    $0 test-agents                  # Run integration tests

EOF
}

check_dependencies() {
    local missing_deps=()
    
    # Check required tools
    for tool in docker docker-compose git curl jq; do
        if ! command -v "$tool" &> /dev/null; then
            missing_deps+=("$tool")
        fi
    done
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log "Please install the missing tools and try again."
        exit 1
    fi
    
    # Check Docker is running
    if ! docker info &> /dev/null; then
        log_error "Docker is not running or not accessible"
        exit 1
    fi
    
    log_success "All dependencies satisfied"
}

build_zed() {
    log "Building Zed with external agent support..."
    
    local zed_repo_url="https://github.com/helixml/zed"
    local zed_branch="external-websocket-sync"
    
    # Create build directory
    mkdir -p "$ZED_BUILD_DIR"
    
    # Clone or update Zed repository
    if [ ! -d "$ZED_BUILD_DIR/.git" ]; then
        log "Cloning Zed repository..."
        git clone --depth 1 --branch "$zed_branch" "$zed_repo_url" "$ZED_BUILD_DIR"
    else
        log "Updating existing Zed repository..."
        cd "$ZED_BUILD_DIR"
        git fetch origin "$zed_branch"
        git reset --hard "origin/$zed_branch"
        cd "$PROJECT_ROOT"
    fi
    
    # Check if Rust is installed
    if ! command -v cargo &> /dev/null; then
        log_error "Rust/Cargo not found. Please install Rust first:"
        log "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
        exit 1
    fi
    
    # Build Zed
    log "Compiling Zed binary (this may take several minutes)..."
    cd "$ZED_BUILD_DIR"
    
    # Install required system dependencies for building
    if command -v apt-get &> /dev/null; then
        log "Installing build dependencies (you may be prompted for sudo password)..."
        sudo apt-get update -qq
        sudo apt-get install -qqy \
            build-essential \
            libssl-dev \
            libxcb-composite0-dev \
            libfontconfig1-dev \
            libfreetype6-dev \
            libxcb-shape0-dev \
            libxcb-xfixes0-dev \
            libxkbcommon-dev \
            libgtk-3-dev \
            libasound2-dev \
            pkg-config
    fi
    
    # Build in release mode with external sync feature
    RUST_LOG=info cargo build --release --features external_websocket_sync
    
    # Copy binary to expected location
    mkdir -p "$PROJECT_ROOT/zed-build"
    cp target/release/zed "$PROJECT_ROOT/zed-build/zed"
    
    # Verify the binary works
    if "$PROJECT_ROOT/zed-build/zed" --help &> /dev/null; then
        log_success "Zed binary built successfully"
        
        # Check for external sync feature
        if strings "$PROJECT_ROOT/zed-build/zed" | grep -q "external_websocket_sync"; then
            log_success "External WebSocket Thread Sync feature detected in binary"
        else
            log_warning "External WebSocket Thread Sync feature not clearly detectable"
        fi
    else
        log_error "Zed binary build failed or is not executable"
        exit 1
    fi
    
    cd "$PROJECT_ROOT"
}

build_agents() {
    log "Building external agent Docker containers..."
    
    # Ensure Zed binary exists
    if [ ! -f "$PROJECT_ROOT/zed-build/zed" ]; then
        log_error "Zed binary not found. Run 'build-zed' first or 'build-all'."
        exit 1
    fi
    
    # Build main agent container
    log "Building Zed agent container..."
    docker build \
        -f "$PROJECT_ROOT/Dockerfile.zed-agent" \
        -t "${DOCKER_TAG_PREFIX}/zed-agent:${DOCKER_TAG_VERSION}" \
        "$PROJECT_ROOT"
    
    log_success "Zed agent container built: ${DOCKER_TAG_PREFIX}/zed-agent:${DOCKER_TAG_VERSION}"
    
    # Build agent pool manager (if Dockerfile exists)
    if [ -f "$PROJECT_ROOT/Dockerfile.agent-pool-manager" ]; then
        log "Building agent pool manager..."
        docker build \
            -f "$PROJECT_ROOT/Dockerfile.agent-pool-manager" \
            -t "${DOCKER_TAG_PREFIX}/agent-pool-manager:${DOCKER_TAG_VERSION}" \
            "$PROJECT_ROOT"
        log_success "Agent pool manager built"
    else
        log_warning "Agent pool manager Dockerfile not found, skipping"
    fi
    
    # Show built images
    log "Built Docker images:"
    docker images | grep "${DOCKER_TAG_PREFIX}.*agent"
}

test_agents() {
    log "Testing external agents..."
    
    # Check if agents are running
    if ! docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" ps | grep -q "Up"; then
        log "Starting test agent for testing..."
        docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" up -d zed-agent-1
        sleep 10
    fi
    
    # Run integration tests inside the container
    log "Running integration tests..."
    docker exec zed-agent-1 /test-integration.sh || {
        log_error "Integration tests failed"
        return 1
    }
    
    # Test NATS connectivity
    if docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" ps nats | grep -q "Up"; then
        log "Testing NATS connectivity..."
        docker exec helix-nats-box nats server info || {
            log_error "NATS server not accessible"
            return 1
        }
        log_success "NATS connectivity test passed"
    fi
    
    # Test WebSocket endpoint
    log "Testing WebSocket sync endpoint..."
    local api_host="${API_HOST:-localhost:8080}"
    if curl -f "http://${api_host}/api/v1/external-agents" &> /dev/null; then
        log_success "API endpoint accessible"
    else
        log_warning "API endpoint not accessible at http://${api_host}"
    fi
    
    log_success "External agent tests completed"
}

start_agents() {
    local num_agents="${NUM_AGENTS:-2}"
    
    log "Starting external agent pool with $num_agents agents..."
    
    # Ensure environment file exists
    if [ ! -f "$PROJECT_ROOT/.env" ]; then
        log "Creating default .env file..."
        cat > "$PROJECT_ROOT/.env" << EOF
# Helix External Agent Configuration
HELIX_API_HOST=http://api:8080
HELIX_API_TOKEN=${HELIX_API_TOKEN:-}
VNC_PASSWORD=zed123
DOCKER_TAG_VERSION=${DOCKER_TAG_VERSION}
EOF
        log_warning "Please update .env file with your API token"
    fi
    
    # Start core services
    docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" up -d nats
    sleep 5
    
    # Start agents
    for i in $(seq 1 "$num_agents"); do
        service_name="zed-agent-$i"
        if [ "$i" -le 2 ]; then
            # Use predefined services for first 2 agents
            docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" up -d "$service_name"
        else
            # Scale existing service for additional agents
            docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" up -d --scale zed-agent-1="$num_agents"
            break
        fi
    done
    
    # Start pool manager if available
    docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" up -d agent-pool-manager 2>/dev/null || \
        log_warning "Agent pool manager not available"
    
    log_success "External agent pool started with $num_agents agents"
    
    # Show status
    show_status
}

stop_agents() {
    log "Stopping external agent pool..."
    
    docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" down
    
    log_success "External agent pool stopped"
}

show_status() {
    log "External Agent Status:"
    echo
    
    # Docker compose status
    if docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" ps 2>/dev/null; then
        echo
    else
        log "No external agents running"
        return
    fi
    
    # NATS status
    if docker exec helix-nats-box nats server info 2>/dev/null | head -10; then
        echo
    fi
    
    # Agent registration status (if API is available)
    local api_host="${API_HOST:-localhost:8080}"
    if curl -s "http://${api_host}/api/v1/external-agents" 2>/dev/null | jq . 2>/dev/null; then
        log_success "API connectivity confirmed"
    else
        log_warning "Cannot connect to API at http://${api_host}"
    fi
}

show_logs() {
    local service="${1:-}"
    
    if [ -n "$service" ]; then
        docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" logs -f "$service"
    else
        docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" logs -f
    fi
}

clean() {
    log "Cleaning up external agent artifacts..."
    
    # Stop and remove containers
    docker-compose -f "$PROJECT_ROOT/docker-compose.zed-agents.yaml" down --volumes 2>/dev/null || true
    
    # Remove Docker images
    docker rmi "${DOCKER_TAG_PREFIX}/zed-agent:${DOCKER_TAG_VERSION}" 2>/dev/null || true
    docker rmi "${DOCKER_TAG_PREFIX}/agent-pool-manager:${DOCKER_TAG_VERSION}" 2>/dev/null || true
    
    # Clean up build directory
    if [ -d "$ZED_BUILD_DIR" ]; then
        read -p "Remove Zed build directory? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            rm -rf "$ZED_BUILD_DIR"
            log_success "Zed build directory removed"
        fi
    fi
    
    log_success "Cleanup completed"
}

# Parse arguments
COMMAND=""
while [[ $# -gt 0 ]]; do
    case $1 in
        build-zed|build-agents|build-all|test-agents|start-agents|stop-agents|status|logs|clean)
            COMMAND="$1"
            shift
            ;;
        --tag)
            DOCKER_TAG_VERSION="$2"
            shift 2
            ;;
        --agents)
            NUM_AGENTS="$2"
            shift 2
            ;;
        --force)
            FORCE_REBUILD=1
            shift
            ;;
        --verbose)
            set -x
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
    COMMAND="build-all"
fi

# Main execution
main() {
    log "Helix External Agent Build Script"
    log "Command: $COMMAND"
    log "Project Root: $PROJECT_ROOT"
    echo
    
    check_dependencies
    
    case $COMMAND in
        build-zed)
            build_zed
            ;;
        build-agents)
            build_agents
            ;;
        build-all)
            build_zed
            build_agents
            log_success "All components built successfully"
            ;;
        test-agents)
            test_agents
            ;;
        start-agents)
            start_agents
            ;;
        stop-agents)
            stop_agents
            ;;
        status)
            show_status
            ;;
        logs)
            show_logs "${2:-}"
            ;;
        clean)
            clean
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