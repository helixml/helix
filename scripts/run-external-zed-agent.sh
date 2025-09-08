#!/bin/bash
set -e

# External Zed Agent Runner - Simple Docker Run Script
# This script allows you to easily attach an external machine as a Zed agent runner

# Configuration - Override these with environment variables or command line arguments
API_HOST="${API_HOST:-http://localhost:80}"
API_TOKEN="${API_TOKEN:-oh-hallo-insecure-token}"
RUNNER_ID="${RUNNER_ID:-external-zed-$(hostname)}"
CONCURRENCY="${CONCURRENCY:-1}"
MAX_TASKS="${MAX_TASKS:-0}"
SESSION_TIMEOUT="${SESSION_TIMEOUT:-3600}"
WORKSPACE_DIR="${WORKSPACE_DIR:-/tmp/zed-workspaces}"
DISPLAY_NUM="${DISPLAY_NUM:-1}"
LOG_LEVEL="${LOG_LEVEL:-debug}"

# Docker image configuration
DOCKER_IMAGE="${DOCKER_IMAGE:-helix-zed-agent:latest}"
CONTAINER_NAME="${CONTAINER_NAME:-helix-external-zed-agent}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --api-host)
            API_HOST="$2"
            shift 2
            ;;
        --api-token)
            API_TOKEN="$2"
            shift 2
            ;;
        --runner-id)
            RUNNER_ID="$2"
            shift 2
            ;;
        --concurrency)
            CONCURRENCY="$2"
            shift 2
            ;;
        --max-tasks)
            MAX_TASKS="$2"
            shift 2
            ;;
        --workspace-dir)
            WORKSPACE_DIR="$2"
            shift 2
            ;;
        --docker-image)
            DOCKER_IMAGE="$2"
            shift 2
            ;;
        --container-name)
            CONTAINER_NAME="$2"
            shift 2
            ;;
        --help|-h)
            echo "External Zed Agent Runner"
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --api-host HOST          Helix API host (default: http://localhost:80)"
            echo "  --api-token TOKEN        API authentication token (required)"
            echo "  --runner-id ID           Unique runner identifier (default: external-zed-hostname)"
            echo "  --concurrency N          Number of concurrent sessions (default: 1)"
            echo "  --max-tasks N            Maximum tasks before restart, 0=unlimited (default: 0)"
            echo "  --workspace-dir PATH     Workspace directory path (default: /tmp/zed-workspaces)"
            echo ""
            echo "  --docker-image IMAGE     Docker image to use (default: helix-zed-agent:latest)"
            echo "  --container-name NAME    Container name (default: helix-external-zed-agent)"
            echo "  --help, -h               Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  All options can also be set via environment variables (e.g., API_HOST, API_TOKEN)"
            echo ""
            echo "Examples:"
            echo "  # Basic usage with custom API host and token"
            echo "  $0 --api-host http://helix.company.com --api-token your-runner-token"
            echo ""
            echo "  # Run with custom runner ID and higher concurrency"
            echo "  $0 --runner-id dev-machine-1 --concurrency 3 --api-token your-runner-token"
            echo ""
            echo "  # Use environment variables"
            echo "  export API_HOST=http://helix.company.com"
            echo "  export API_TOKEN=your-runner-token"
            echo "  $0"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Validate required parameters
if [ "$API_TOKEN" = "oh-hallo-insecure-token" ]; then
    echo "⚠️  WARNING: Using default insecure token. For production, set --api-token or API_TOKEN environment variable."
fi

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo "❌ Error: Docker is not installed or not in PATH"
    echo "Please install Docker first: https://docs.docker.com/get-docker/"
    exit 1
fi

# Stop and remove existing container if it exists
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "🛑 Stopping existing container: $CONTAINER_NAME"
    docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
    docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true
fi

echo "🚀 Starting External Zed Agent Runner"
echo "   API Host: $API_HOST"
echo "   Runner ID: $RUNNER_ID"
echo "   Concurrency: $CONCURRENCY"
echo "   Max Tasks: $MAX_TASKS"
echo "   Workspace: $WORKSPACE_DIR"
echo "   Container: $CONTAINER_NAME"
echo "   Image: $DOCKER_IMAGE"

# Build the docker run command
DOCKER_CMD="docker run -d"
DOCKER_CMD="$DOCKER_CMD --name $CONTAINER_NAME"
DOCKER_CMD="$DOCKER_CMD --restart unless-stopped"

# Environment variables
DOCKER_CMD="$DOCKER_CMD -e API_HOST=$API_HOST"
DOCKER_CMD="$DOCKER_CMD -e API_TOKEN=$API_TOKEN"
DOCKER_CMD="$DOCKER_CMD -e RUNNER_ID=$RUNNER_ID"
DOCKER_CMD="$DOCKER_CMD -e LOG_LEVEL=$LOG_LEVEL"
DOCKER_CMD="$DOCKER_CMD -e CONCURRENCY=$CONCURRENCY"
DOCKER_CMD="$DOCKER_CMD -e MAX_TASKS=$MAX_TASKS"
DOCKER_CMD="$DOCKER_CMD -e SESSION_TIMEOUT=$SESSION_TIMEOUT"
DOCKER_CMD="$DOCKER_CMD -e WORKSPACE_DIR=$WORKSPACE_DIR"
DOCKER_CMD="$DOCKER_CMD -e DISPLAY=:$DISPLAY_NUM"


# Add the Docker image
DOCKER_CMD="$DOCKER_CMD $DOCKER_IMAGE"

# Execute the command
echo "🐳 Running: $DOCKER_CMD"
if eval $DOCKER_CMD; then
    echo "✅ External Zed Agent Runner started successfully!"
    echo ""
    echo "📊 Container Status:"
    docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
    echo ""
    echo "📋 Useful Commands:"
    echo "   View logs:     docker logs -f $CONTAINER_NAME"
    echo "   Stop runner:   docker stop $CONTAINER_NAME"
    echo "   Remove runner: docker rm $CONTAINER_NAME"
    echo "   Container shell: docker exec -it $CONTAINER_NAME /bin/bash"
    echo ""
    echo "🖥️  RDP Access:"
    echo "   RDP credentials are managed by the Helix control plane"
    echo ""
    echo "🔍 The runner should appear in your Helix control plane as: $RUNNER_ID"
else
    echo "❌ Failed to start External Zed Agent Runner"
    echo "Check Docker logs for details: docker logs $CONTAINER_NAME"
    exit 1
fi
