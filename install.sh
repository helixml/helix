#!/bin/bash

# Install:
# curl -LO https://get.helixml.tech/install.sh && chmod +x install.sh

# ================================================================================
# PLATFORM AND GPU CONFIGURATION TEST MATRIX
# ================================================================================
# IMPORTANT: When modifying this script, ask a state-of-the-art LLM to verify
# that all combinations below still work correctly. The script must handle every
# combination of platform, Docker state, GPU configuration, and flags properly.
#
# PLATFORMS:
#   - Git Bash (Windows with Docker Desktop)
#   - WSL2 (Windows Subsystem for Linux with Docker Desktop)
#   - Ubuntu/Debian (auto-install Docker supported)
#   - Fedora (auto-install Docker supported)
#   - macOS (Docker Desktop required, no auto-install)
#   - Other Linux (manual Docker install required)
#
# DOCKER STATES:
#   - Not installed (offer auto-install on Ubuntu/Debian/Fedora only)
#   - Installed, user not in docker group (needs sudo)
#   - Installed, user in docker group (no sudo needed)
#
# GPU CONFIGURATIONS:
#   - No GPU
#   - Intel/AMD GPU (/dev/dri exists, no nvidia-smi)
#   - NVIDIA GPU with drivers (nvidia-smi works)
#   - NVIDIA GPU without drivers (hardware present but no nvidia-smi)
#   - NVIDIA GPU with drivers but no Docker runtime (/etc/docker/daemon.json missing nvidia)
#
# INSTALLATION FLAGS:
#   - --cli (no Docker/GPU needed)
#   - --controlplane (Docker needed, no GPU needed)
#   - --runner (Docker + NVIDIA GPU + NVIDIA runtime required)
#   - --code (Docker + any GPU required, NVIDIA runtime if NVIDIA GPU)
#   - Combinations: --controlplane --runner, --controlplane --code, etc.
#
# KEY TEST CASES (Critical Paths):
#
# 1. Ubuntu + No Docker + No GPU + --cli
#    Result: Install CLI only, no Docker installation offered
#
# 2. Ubuntu + No Docker + NVIDIA GPU with drivers + --controlplane --runner
#    Result: Install Docker + NVIDIA runtime → Install controlplane + local runner
#
# 3. Ubuntu + Docker installed + NVIDIA GPU with drivers + No NVIDIA runtime + --code
#    Result: Detect missing NVIDIA runtime → Offer to install → Install it → Enable code profile
#
# 4. Ubuntu + No Docker + NVIDIA GPU without drivers + --runner
#    Result: Exit with error, instructions to install drivers and reboot
#
# 5. Ubuntu + No Docker + Intel/AMD GPU + --code
#    Result: Detect Intel/AMD GPU → Offer to install Docker → Install Docker → Enable code profile
#
# 6. Git Bash + Docker Desktop + --controlplane
#    Result: Use existing Docker Desktop, no sudo, install controlplane
#
# 7. WSL2 + No Docker + --controlplane
#    Result: Exit with error, tell user to install Docker Desktop for Windows
#
# 8. macOS + No Docker + --controlplane
#    Result: Exit with error, tell user to install Docker Desktop manually
#
# 9. Fedora + No Docker + NVIDIA GPU + --runner --runner-token TOKEN --api-host HOST
#    Result: Install Docker + NVIDIA runtime using dnf → Create remote runner script
#
# 10. Ubuntu + Docker + NVIDIA runtime installed + --runner --runner-token TOKEN --api-host HOST
#     Result: Skip Docker/runtime installation → Create remote runner script
#
# 11. Arch Linux + No Docker + --controlplane
#     Result: Exit with error, auto-install only supports Ubuntu/Debian/Fedora
#
# 12. Ubuntu + No Docker + No GPU + --code
#     Result: Exit with error, instructions to install NVIDIA/Intel/AMD drivers
#
# 13. Ubuntu + NVIDIA GPU + --code (without --controlplane)
#     Result: Auto-enable --cli and --controlplane (--code is a controlplane feature)
#     Note: Simplest way to install controlplane with Code features
#
# 14. Ubuntu + NVIDIA GPU + --runner --code --haystack (without --runner-token)
#     Result: Auto-enable --cli and --controlplane, install controlplane+runner+code+haystack
#     Note: --code/--haystack auto-enable controlplane, --runner (no token) adds local runner
#
# 15. Ubuntu + NVIDIA GPU + --runner (without --runner-token or --code/--haystack/--controlplane)
#     Result: ERROR - must specify --runner-token OR --controlplane OR controlplane features
#     Note: Prevents ambiguity - user must explicitly choose remote or local installation
#
# 16. Ubuntu + --runner --runner-token TOKEN --api-host HOST
#     Result: Remote runner only (connects to external controlplane at HOST)
#     Note: Does NOT auto-enable controlplane (token provided = remote mode)
#
# ================================================================================

set -euo pipefail

echo -e "\033[1;91m"
echo -ne " ░█░█░█▀▀░█░░░▀█▀░█░█░░░░█▄█░█░░"
echo -ne "\033[0m"
echo -e "\033[1;93m"
echo -ne " ░█▀█░█▀▀░█░░░░█░░▄▀▄░░░░█░█░█░░"
echo -ne "\033[0m"
echo -e "\033[1;92m"
echo -ne " ░▀░▀░▀▀▀░▀▀▀░▀▀▀░▀░▀░▀░░▀░▀░▀▀▀"
echo -e "\033[0m"
echo -e "\033[1;96m              Private GenAI Stack\033[0m"
echo

set -euo pipefail

# Default values
AUTO=true
CLI=false
CONTROLPLANE=false
RUNNER=false
SANDBOX=false
LARGE=false
HAYSTACK=""
CODE=""
API_HOST=""
RUNNER_TOKEN=""
TOGETHER_API_KEY=""
OPENAI_API_KEY=""
OPENAI_BASE_URL=""
ANTHROPIC_API_KEY=""
AUTO_APPROVE=false
HF_TOKEN=""
PROXY=https://get.helixml.tech
HELIX_VERSION=""
CLI_INSTALL_PATH="/usr/local/bin/helix"
EMBEDDINGS_PROVIDER="helix"
PROVIDERS_MANAGEMENT_ENABLED="true"
SPLIT_RUNNERS="1"
EXCLUDE_GPUS=""
GPU_VENDOR=""  # Will be set to "nvidia", "amd", or "intel" during GPU detection
TURN_PASSWORD=""  # TURN server password for sandbox nodes connecting to remote control plane
# MOONLIGHT_CREDENTIALS removed - no longer used (direct WebSocket streaming)
PRIVILEGED_DOCKER=""  # Enable privileged Docker mode for Helix-in-Helix development

# Enhanced environment detection
detect_environment() {
    case "$OSTYPE" in
        msys*|cygwin*)
            # Git Bash or Cygwin on Windows
            ENVIRONMENT="gitbash"
            OS="windows"
            ;;
        linux*)
            # Check if we're in WSL by examining /proc/version
            if [[ -f /proc/version ]] && grep -qEi "(Microsoft|WSL)" /proc/version 2>/dev/null; then
                ENVIRONMENT="wsl2"
                OS="linux"
            else
                ENVIRONMENT="linux"
                OS="linux"
            fi
            ;;
        darwin*)
            ENVIRONMENT="macos"
            OS="darwin"
            ;;
        *)
            # Fallback to linux for unknown environments
            ENVIRONMENT="linux"
            OS="linux"
            ;;
    esac
}

# Call environment detection
detect_environment

# Determine OS and architecture (keeping existing logic for compatibility)
if [ "$ENVIRONMENT" != "gitbash" ]; then
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
fi
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Set binary name based on environment
if [ "$ENVIRONMENT" = "gitbash" ]; then
    BINARY_NAME="helix-windows-${ARCH}.exe"
else
    BINARY_NAME="helix-${OS}-${ARCH}"
fi

# Set installation directory based on environment
case $ENVIRONMENT in
    "gitbash")
        INSTALL_DIR="$HOME/HelixML"
        CLI_INSTALL_PATH="$HOME/bin/helix.exe"
        ;;
    "linux"|"wsl2")
        INSTALL_DIR="/opt/HelixML"
        # CLI_INSTALL_PATH keeps default: /usr/local/bin/helix
        ;;
    "macos")
        INSTALL_DIR="$HOME/HelixML"
        # CLI_INSTALL_PATH keeps default: /usr/local/bin/helix
        ;;
esac

# Function to check if docker works without sudo
check_docker_sudo() {
    # Git Bash doesn't use sudo
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        if docker ps >/dev/null 2>&1; then
            echo "false"
        else
            echo "Docker is not running or not installed. Please start Docker Desktop!" >&2
            exit 1
        fi
        return
    fi

    # Original logic for other environments
    # Try without sudo first
    if docker ps >/dev/null 2>&1; then
        echo "false"
    else
        # Try with sudo
        if sudo docker ps >/dev/null 2>&1; then
            echo "true"
        else
            echo "Docker is not running or not installed. Please start Docker!" >&2
            exit 1
        fi
    fi
}

# Function to display help message
display_help() {
    cat << EOF
Usage: ./install.sh [OPTIONS]

Options:
  --cli                    Install the CLI (binary in /usr/local/bin on Linux/macOS, ~/bin/helix.exe on Git Bash)
  --controlplane           Install the controlplane (API, Postgres etc in Docker Compose in $INSTALL_DIR)
  --runner                 Install the runner (single container with runner.sh script to start it in $INSTALL_DIR)
  --sandbox                Install sandbox node (RevDial client with direct WebSocket streaming for remote machine)
  --large                  Install the large version of the runner (includes all models, 100GB+ download, otherwise uses small one)
  --haystack               Enable the haystack and vectorchord/postgres based RAG service (downloads tens of gigabytes of python but provides better RAG quality than default typesense/tika stack), also uses GPU-accelerated embeddings in helix runners
  --code                   Enable Helix Code features (External Agents, PDEs with Zed, direct WebSocket streaming). Requires GPU (Intel/AMD/NVIDIA) with drivers installed and --api-host parameter.
  --api-host <host>        Specify the API host for the API to serve on and/or the runner/sandbox to connect to, e.g. http://localhost:8080 or https://my-controlplane.com. Will install and configure Caddy if HTTPS and running on Ubuntu.
  --runner-token <token>   Specify the runner token when connecting a runner or sandbox to an existing controlplane
  --turn-password <pass>   Specify the TURN server password for sandbox nodes (required for WebRTC NAT traversal when connecting to remote control plane)
  --privileged-docker        Enable privileged Docker mode in sandbox (allows agents to access host Docker socket for Helix-in-Helix development)
  --together-api-key <token> Specify the together.ai token for inference, rag and apps without a GPU
  --openai-api-key <key>   Specify the OpenAI API key for any OpenAI compatible API
  --openai-base-url <url>  Specify the base URL for the OpenAI API
  --anthropic-api-key <key> Specify the Anthropic API key for Claude models
  --hf-token <token>       Specify the Hugging Face token for the control plane (automatically distributed to runners)
  --embeddings-provider <provider> Specify the provider for embeddings (openai, togetherai, vllm, helix, default: helix)
  --providers-management-enabled <true|false> Enable/disable user-facing AI provider API keys management (default: true)
  --no-providers-management Disable user-facing AI provider API keys management (shorthand for --providers-management-enabled=false)
  --split-runners <n>      Split GPUs across N runner containers (default: 1, must divide evenly into total GPU count)
  --exclude-gpu <id>       Exclude specific GPU(s) from runner (can be specified multiple times, e.g., --exclude-gpu 0 --exclude-gpu 1)
  -y                       Auto approve the installation

  --helix-version <version>  Override the Helix version to install (e.g. 1.4.0-rc4, defaults to latest stable)
  --cli-install-path <path> Specify custom installation path for the CLI binary (default: /usr/local/bin/helix)

Examples:

1. Install the CLI, the controlplane and a runner if a GPU is available (auto mode):
   ./install.sh

2. Install alongside Ollama already running:
   ./install.sh --openai-api-key ollama --openai-base-url http://host.docker.internal:11434/v1

3. Install just the CLI:
   ./install.sh --cli

4. Install CLI and controlplane with external TogetherAI token:
   ./install.sh --cli --controlplane --together-api-key YOUR_TOGETHER_API_KEY

5. Install CLI and controlplane (to install runner separately), specifying a DNS name, automatically setting up TLS:
   ./install.sh --cli --controlplane --api-host https://helix.mycompany.com

6. Install CLI, controlplane, and runner on a node with a GPU:
   ./install.sh --cli --controlplane --runner

7. Install just the runner, pointing to a controlplane with a DNS name (find runner token in /opt/HelixML/.env):
   ./install.sh --runner --api-host https://helix.mycompany.com --runner-token YOUR_RUNNER_TOKEN

8. Install CLI and controlplane with OpenAI-compatible API key and base URL:
   ./install.sh --cli --controlplane --openai-api-key YOUR_OPENAI_API_KEY --openai-base-url YOUR_OPENAI_BASE_URL

9. Install CLI and controlplane with custom embeddings provider:
   ./install.sh --cli --controlplane --embeddings-provider openai

10. Install on Windows Git Bash (requires Docker Desktop):
    ./install.sh --cli --controlplane

11. Install with Helix Code (auto-enables --cli --controlplane):
    ./install.sh --code --api-host https://helix.mycompany.com

12. Install everything locally on a GPU machine (controlplane + runner + code + haystack):
    ./install.sh --runner --code --haystack --api-host https://helix.mycompany.com

13. Install runner with GPUs split across 4 containers (for 8 GPUs = 2 GPUs per container):
    ./install.sh --runner --api-host https://helix.mycompany.com --runner-token YOUR_RUNNER_TOKEN --split-runners 4

14. Install runner excluding GPU 0 (use GPUs 1-7 only):
    ./install.sh --runner --api-host https://helix.mycompany.com --runner-token YOUR_RUNNER_TOKEN --exclude-gpu 0

15. Install sandbox node (RevDial client with direct WebSocket streaming):
    ./install.sh --sandbox --api-host https://helix.mycompany.com --runner-token YOUR_RUNNER_TOKEN --turn-password YOUR_TURN_PASSWORD

EOF
}

# Function to check if hostname resolves to localhost
check_hostname_localhost() {
    local hostname=$1
    local resolved_ip=""

    # Try different resolution methods based on platform
    if command -v getent &> /dev/null; then
        # Linux/WSL2: Use getent (respects /etc/hosts)
        resolved_ip=$(getent hosts "$hostname" 2>/dev/null | awk '{ print $1 }' | head -1)
    elif command -v dscacheutil &> /dev/null; then
        # macOS: Use dscacheutil (respects /etc/hosts)
        resolved_ip=$(dscacheutil -q host -a name "$hostname" 2>/dev/null | grep "^ip_address:" | head -1 | awk '{ print $2 }')
    elif command -v ping &> /dev/null; then
        # Fallback: Use ping (works on most systems, respects /etc/hosts)
        # Try to extract IP from ping output
        if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]]; then
            # Windows (Git Bash): ping output format is different
            resolved_ip=$(ping -n 1 "$hostname" 2>/dev/null | grep "Pinging" | sed -E 's/.*\[([0-9.]+)\].*/\1/')
        else
            # Unix-like systems
            resolved_ip=$(ping -c 1 -W 1 "$hostname" 2>/dev/null | grep -oE '\(([0-9]{1,3}\.){3}[0-9]{1,3}\)' | head -1 | tr -d '()')
        fi
    fi

    # Check if resolved IP starts with 127. (localhost range)
    if [ -n "$resolved_ip" ] && [[ "$resolved_ip" == 127.* ]]; then
        return 0
    fi

    return 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --help)
            display_help
            exit 0
            ;;
        --cli)
            CLI=true
            AUTO=false
            shift
            ;;
        --controlplane)
            CONTROLPLANE=true
            AUTO=false
            shift
            ;;
        --runner)
            RUNNER=true
            AUTO=false
            shift
            ;;
        --sandbox)
            SANDBOX=true
            AUTO=false
            shift
            ;;
        --large)
            LARGE=true
            shift
            ;;
        --haystack)
            HAYSTACK=true
            shift
            ;;
        --code)
            CODE=true
            shift
            ;;
        --api-host=*)
            API_HOST="${1#*=}"
            shift
            ;;
        --api-host)
            API_HOST="$2"
            shift 2
            ;;
        --runner-token=*)
            RUNNER_TOKEN="${1#*=}"
            shift
            ;;
        --runner-token)
            RUNNER_TOKEN="$2"
            shift 2
            ;;
        --turn-password=*)
            TURN_PASSWORD="${1#*=}"
            shift
            ;;
        --turn-password)
            TURN_PASSWORD="$2"
            shift 2
            ;;
        --privileged-docker)
            PRIVILEGED_DOCKER="true"
            shift
            ;;
        --together-api-key=*)
            TOGETHER_API_KEY="${1#*=}"
            shift
            ;;
        --together-api-key)
            TOGETHER_API_KEY="$2"
            shift 2
            ;;
        --openai-api-key=*)
            OPENAI_API_KEY="${1#*=}"
            shift
            ;;
        --openai-api-key)
            OPENAI_API_KEY="$2"
            shift 2
            ;;
        --openai-base-url=*)
            OPENAI_BASE_URL="${1#*=}"
            shift
            ;;
        --openai-base-url)
            OPENAI_BASE_URL="$2"
            shift 2
            ;;
        --anthropic-api-key=*)
            ANTHROPIC_API_KEY="${1#*=}"
            shift
            ;;
        --anthropic-api-key)
            ANTHROPIC_API_KEY="$2"
            shift 2
            ;;
        --embeddings-provider=*)
            EMBEDDINGS_PROVIDER="${1#*=}"
            shift
            ;;
        --embeddings-provider)
            EMBEDDINGS_PROVIDER="$2"
            shift 2
            ;;
        --hf-token=*)
            HF_TOKEN="${1#*=}"
            shift
            ;;
        --hf-token)
            HF_TOKEN="$2"
            shift 2
            ;;
        --providers-management-enabled=*)
            PROVIDERS_MANAGEMENT_ENABLED="${1#*=}"
            shift
            ;;
        --providers-management-enabled)
            PROVIDERS_MANAGEMENT_ENABLED="$2"
            shift 2
            ;;
        --no-providers-management)
            PROVIDERS_MANAGEMENT_ENABLED="false"
            shift
            ;;
        -y)
            AUTO_APPROVE=true
            shift
            ;;
        --helix-version=*)
            HELIX_VERSION="${1#*=}"
            shift
            ;;
        --helix-version)
            HELIX_VERSION="$2"
            shift 2
            ;;
        --cli-install-path=*)
            CLI_INSTALL_PATH="${1#*=}"
            shift
            ;;
        --cli-install-path)
            CLI_INSTALL_PATH="$2"
            shift 2
            ;;
        --split-runners=*)
            SPLIT_RUNNERS="${1#*=}"
            shift
            ;;
        --split-runners)
            SPLIT_RUNNERS="$2"
            shift 2
            ;;
        --exclude-gpu=*)
            # Can be specified multiple times, build comma-separated list
            if [ -z "$EXCLUDE_GPUS" ]; then
                EXCLUDE_GPUS="${1#*=}"
            else
                EXCLUDE_GPUS="$EXCLUDE_GPUS,${1#*=}"
            fi
            shift
            ;;
        --exclude-gpu)
            # Can be specified multiple times, build comma-separated list
            if [ -z "$EXCLUDE_GPUS" ]; then
                EXCLUDE_GPUS="$2"
            else
                EXCLUDE_GPUS="$EXCLUDE_GPUS,$2"
            fi
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            display_help
            exit 1
            ;;
    esac
done

# Validate --api-host if provided (must not resolve to localhost)
if [ -n "$API_HOST" ]; then
    # Extract hostname from API_HOST (remove protocol and port)
    API_HOSTNAME=$(echo "$API_HOST" | sed -E 's|^https?://||' | sed 's|:[0-9]+$||')

    # Check if hostname resolves to localhost
    if check_hostname_localhost "$API_HOSTNAME"; then
        echo "┌───────────────────────────────────────────────────────────────────────────────────────────────────────"
        echo "│ ❌ ERROR: --api-host hostname resolves to a localhost address (127.x.x.x)"
        echo "│ "
        echo "│ The hostname '$API_HOSTNAME' currently resolves to a localhost IP address."
        echo "│ This breaks Docker networking, as containers cannot reach 127.0.0.1 addresses properly."
        echo "│ "
        echo "│ Please ensure the hostname resolves to the real external IP address of this server."
        echo "│ You may need to check:"
        echo "│   - /etc/hosts file for incorrect localhost mappings"
        echo "│   - DNS configuration"
        echo "│   - Use the actual external IP or correct DNS name"
        echo "│ "
        echo "│ Note: For local development, omit --api-host entirely (defaults to http://localhost:8080)"
        echo "└───────────────────────────────────────────────────────────────────────────────────────────────────────"
        exit 1
    fi
fi

# Function to check if running on WSL2 (don't auto-install docker in that case)
check_wsl2_docker() {
    if grep -qEi "(Microsoft|WSL)" /proc/version &> /dev/null; then
        echo "Detected WSL2 (Windows) environment."
        echo "Please install Docker Desktop for Windows from https://docs.docker.com/desktop/windows/install/"
        exit 1
    fi
}

# Function to check if Docker needs to be installed
check_docker_needed() {
    if ! command -v docker &> /dev/null; then
        return 0  # Docker not found, needs installation
    fi

    # Check for Docker Compose plugin (skip for Git Bash)
    if [ "$ENVIRONMENT" != "gitbash" ] && ! docker compose version &> /dev/null; then
        return 0  # Compose plugin missing, needs installation
    fi

    return 1  # Docker already installed
}

# Function to install Docker and Docker Compose plugin (called after user approval)
install_docker() {
    if ! command -v docker &> /dev/null; then
        # Git Bash: assume Docker Desktop should be installed manually
        if [ "$ENVIRONMENT" = "gitbash" ]; then
            echo "Docker not found. Please install Docker Desktop for Windows."
            echo "Download from: https://docs.docker.com/desktop/windows/install/"
            echo "Make sure to enable WSL 2 integration if you plan to use WSL 2 as well."
            exit 1
        fi

        # Skip Docker installation for WSL2 (should use Docker Desktop)
        if [ "$ENVIRONMENT" = "wsl2" ]; then
            echo "Detected WSL2 environment. Please install Docker Desktop for Windows."
            echo "Download from: https://docs.docker.com/desktop/windows/install/"
            echo "Make sure to enable WSL 2 integration in Docker Desktop settings."
            exit 1
        fi

        check_wsl2_docker
        echo "Installing Docker..."
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                ubuntu|debian)
                    sudo apt-get update
                    sudo apt-get install -y ca-certificates curl gnupg
                    sudo install -m 0755 -d /etc/apt/keyrings
                    curl -fsSL https://download.docker.com/linux/$ID/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
                    sudo chmod a+r /etc/apt/keyrings/docker.gpg
                    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$ID $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
                    sudo apt-get update
                    sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
                    sudo systemctl start docker
                    sudo systemctl enable docker
                    # Wait for Docker to be ready
                    echo "Waiting for Docker to start..."
                    for i in {1..30}; do
                        if docker ps >/dev/null 2>&1 || sudo docker ps >/dev/null 2>&1; then
                            echo "Docker is ready."
                            break
                        fi
                        if [ $i -eq 30 ]; then
                            echo "Error: Docker failed to start after installation."
                            echo "Please check: sudo systemctl status docker"
                            exit 1
                        fi
                        sleep 1
                    done
                    ;;
                fedora)
                    sudo dnf -y install dnf-plugins-core
                    sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
                    sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
                    sudo systemctl start docker
                    sudo systemctl enable docker
                    # Wait for Docker to be ready
                    echo "Waiting for Docker to start..."
                    for i in {1..30}; do
                        if docker ps >/dev/null 2>&1 || sudo docker ps >/dev/null 2>&1; then
                            echo "Docker is ready."
                            break
                        fi
                        if [ $i -eq 30 ]; then
                            echo "Error: Docker failed to start after installation."
                            echo "Please check: sudo systemctl status docker"
                            exit 1
                        fi
                        sleep 1
                    done
                    ;;
                *)
                    echo "Unsupported distribution for automatic Docker installation."
                    echo "Only Ubuntu/Debian/Fedora are supported for automatic installation."
                    echo "Please install Docker manually from https://docs.docker.com/engine/install/"
                    exit 1
                    ;;
            esac
        else
            echo "Unable to determine OS distribution. Please install Docker manually."
            exit 1
        fi
    else
        # Docker is already installed, but check if Docker Compose plugin is missing
        if ! docker compose version &> /dev/null; then
            install_docker_compose_only
        fi
    fi

    # Docker Compose plugin is included in docker-ce installation above for Ubuntu/Debian/Fedora
    # No additional installation needed - it's part of docker-compose-plugin package
}

# Function to install Docker Compose plugin if Docker is installed but Compose is missing
install_docker_compose_only() {
    if command -v docker &> /dev/null && ! docker compose version &> /dev/null; then
        echo "Docker is installed but Docker Compose plugin is missing."
        echo "Installing Docker Compose plugin..."

        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                ubuntu|debian)
                    # Always ensure Docker's official repo is configured
                    echo "Setting up Docker official repository..."
                    sudo apt-get update
                    sudo apt-get install -y ca-certificates curl gnupg
                    sudo install -m 0755 -d /etc/apt/keyrings
                    curl -fsSL https://download.docker.com/linux/$ID/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
                    sudo chmod a+r /etc/apt/keyrings/docker.gpg
                    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$ID $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
                    sudo apt-get update

                    # Install docker-compose-plugin
                    if ! sudo apt-get install -y docker-compose-plugin; then
                        echo "Failed to install docker-compose-plugin from Docker official repository."
                        echo "Please try installing manually:"
                        echo "  sudo apt-get update"
                        echo "  sudo apt-get install -y docker-compose-plugin"
                        exit 1
                    fi
                    ;;
                fedora)
                    # Ensure Docker's official repo is configured
                    if ! sudo dnf repolist | grep -q docker-ce-stable; then
                        echo "Setting up Docker official repository..."
                        sudo dnf -y install dnf-plugins-core
                        sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
                    fi
                    sudo dnf install -y docker-compose-plugin
                    ;;
                *)
                    echo "Unsupported distribution for Docker Compose plugin installation."
                    echo "Please install Docker Compose manually from https://docs.docker.com/compose/install/"
                    exit 1
                    ;;
            esac
        else
            echo "Unable to determine OS distribution."
            echo "Please install Docker Compose manually from https://docs.docker.com/compose/install/"
            exit 1
        fi
    fi
}

# default docker command
DOCKER_CMD="docker"

# Only check docker sudo if we need docker (i.e., not CLI-only installation)
if [ "$CLI" = true ] && [ "$CONTROLPLANE" = false ] && [ "$RUNNER" = false ] && [ "$SANDBOX" = false ]; then
    NEED_SUDO="false"
else
    # Docker is needed - check if it's installed
    if ! command -v docker &> /dev/null; then
        # For non-Linux platforms, exit with instructions
        if [ "$ENVIRONMENT" = "gitbash" ]; then
            echo "Docker not found. Please install Docker Desktop for Windows."
            echo "Download from: https://docs.docker.com/desktop/windows/install/"
            exit 1
        elif [ "$ENVIRONMENT" = "wsl2" ]; then
            echo "Docker not found. Please install Docker Desktop for Windows."
            echo "Download from: https://docs.docker.com/desktop/windows/install/"
            echo "Make sure to enable WSL 2 integration in Docker Desktop settings."
            exit 1
        elif [ "$OS" != "linux" ]; then
            echo "Docker not found. Please install Docker manually."
            echo "Visit https://docs.docker.com/engine/install/ for installation instructions."
            exit 1
        fi
        # For Linux, check if it's Ubuntu/Debian/Fedora before proceeding
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            if [[ "$ID" != "ubuntu" && "$ID" != "debian" && "$ID" != "fedora" ]]; then
                echo "Docker not found."
                echo "Automatic Docker installation is only supported on Ubuntu/Debian/Fedora."
                echo "Please install Docker manually from https://docs.docker.com/engine/install/"
                exit 1
            fi
        fi
    fi

    # Determine if we need sudo for docker commands (Git Bash never needs sudo)
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        NEED_SUDO="false"
        DOCKER_CMD="docker"
    else
        # If Docker is not installed, check if we can auto-install it
        if ! command -v docker &> /dev/null; then
            # For Ubuntu/Debian/Fedora, we can auto-install Docker - skip sudo check for now
            if [ "$OS" = "linux" ] && [ -f /etc/os-release ]; then
                . /etc/os-release
                if [[ "$ID" == "ubuntu" || "$ID" == "debian" || "$ID" == "fedora" ]]; then
                    echo "Docker not installed. Will offer to install Docker during setup."
                    # Will check sudo requirement after Docker is installed
                    NEED_SUDO="false"
                    DOCKER_CMD="docker"
                else
                    # Non-Ubuntu/Debian/Fedora Linux - already handled above
                    NEED_SUDO="false"
                    DOCKER_CMD="docker"
                fi
            else
                NEED_SUDO="false"
                DOCKER_CMD="docker"
            fi
        else
            # Docker is installed - check if we need sudo
            NEED_SUDO=$(check_docker_sudo)
            if [ "$NEED_SUDO" = "true" ]; then
                DOCKER_CMD="sudo docker"
            fi
        fi
    fi
fi

# Determine version to install
if [ -n "$HELIX_VERSION" ]; then
    LATEST_RELEASE="$HELIX_VERSION"
    echo "Using specified Helix version: $LATEST_RELEASE"
    echo
else
    LATEST_RELEASE=$(curl -s ${PROXY}/latest.txt)
    echo "Using latest Helix version: $LATEST_RELEASE"
    echo
fi

# Function to check for NVIDIA GPU
check_nvidia_gpu() {
    # On windows, WSL2 doesn't support nvidia-smi but docker info can give us a clue
    if command -v nvidia-smi &> /dev/null || $DOCKER_CMD info 2>/dev/null | grep -i nvidia &> /dev/null; then
        return 0
    else
        return 1
    fi
}

# Function to check for AMD GPU specifically (for Helix Code with ROCm)
check_amd_gpu() {
    # Check for AMD GPU via lspci
    if command -v lspci &> /dev/null; then
        if lspci | grep -iE "(VGA|3D|Display).*AMD" &> /dev/null; then
            # Verify /dev/kfd (ROCm Kernel Fusion Driver) and /dev/dri exist
            if [ -e "/dev/kfd" ] && [ -d "/dev/dri" ] && [ -n "$(ls -A /dev/dri 2>/dev/null)" ]; then
                return 0
            fi
        fi
    fi
    return 1
}

# Function to check for Intel/AMD GPU (for Helix Code)
# Note: This checks for /dev/dri but doesn't distinguish between Intel and AMD
# Use check_amd_gpu() for specific AMD detection with ROCm support
check_intel_amd_gpu() {
    # Check for /dev/dri devices, but only if NVIDIA is NOT present
    # (NVIDIA also creates /dev/dri, so we check NVIDIA first in the calling code)
    if [ -d "/dev/dri" ] && [ -n "$(ls -A /dev/dri 2>/dev/null)" ]; then
        return 0
    else
        return 1
    fi
}

# Function to check if NVIDIA Docker runtime needs installation
check_nvidia_runtime_needed() {
    # Only relevant if we have an NVIDIA GPU
    if ! check_nvidia_gpu; then
        return 1  # No NVIDIA GPU, so no NVIDIA runtime needed
    fi

    # Check if NVIDIA runtime is already configured in Docker
    # This is the definitive check - docker info will show "Runtimes: ... nvidia ..." if configured
    if timeout 10 $DOCKER_CMD info 2>/dev/null | grep -i "runtimes.*nvidia" &> /dev/null; then
        return 1  # Already configured
    fi

    # NVIDIA GPU is present but runtime is not configured in Docker
    # (The toolkit package may or may not be installed, but it needs to be configured)
    return 0
}

# Function to install NVIDIA Docker runtime
install_nvidia_docker() {
    if ! check_nvidia_gpu; then
        echo "NVIDIA GPU not detected. Skipping NVIDIA Docker runtime installation."
        return
    fi

    # Git Bash: assume Docker Desktop handles NVIDIA support
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        if ! timeout 10 $DOCKER_CMD info 2>/dev/null | grep -i nvidia &> /dev/null; then
            echo "NVIDIA Docker runtime not detected in Docker Desktop."
            echo "Please ensure:"
            echo "1. NVIDIA drivers are installed on Windows"
            echo "2. Docker Desktop is configured with WSL 2 backend"
            echo "3. GPU support is enabled in Docker Desktop settings"
            echo ""
            echo "For more information, see: https://docs.docker.com/desktop/gpu/"
            exit 1
        fi
        return
    fi

    # Check if NVIDIA runtime needs installation
    NVIDIA_IN_DOCKER=$(timeout 10 $DOCKER_CMD info 2>/dev/null | grep -i "runtimes.*nvidia" &> /dev/null && echo "true" || echo "false")
    NVIDIA_CTK_EXISTS=$(command -v nvidia-container-toolkit &> /dev/null && echo "true" || echo "false")

    echo "Checking NVIDIA Docker runtime status..."
    echo "  - NVIDIA in docker info: $NVIDIA_IN_DOCKER"
    echo "  - nvidia-container-toolkit installed: $NVIDIA_CTK_EXISTS"

    if [ "$NVIDIA_IN_DOCKER" = "false" ] || [ "$NVIDIA_CTK_EXISTS" = "false" ]; then
        # Skip NVIDIA Docker installation for WSL2 (should use Docker Desktop)
        if [ "$ENVIRONMENT" = "wsl2" ]; then
            echo "WSL2 detected. Please ensure NVIDIA Docker support is enabled in Docker Desktop."
            echo "See: https://docs.docker.com/desktop/gpu/"
            return
        fi

        check_wsl2_docker

        # If toolkit is already installed, just configure Docker (don't reinstall package)
        if [ "$NVIDIA_CTK_EXISTS" = "true" ]; then
            echo "NVIDIA Container Toolkit already installed. Configuring Docker to use it..."
        else
            echo "Installing NVIDIA Docker runtime..."
            if [ -f /etc/os-release ]; then
                . /etc/os-release
                case $ID in
                    ubuntu|debian)
                        # Remove any existing NVIDIA repository configurations to avoid conflicts
                        sudo rm -f /etc/apt/sources.list.d/nvidia-container-toolkit.list
                        sudo rm -f /etc/apt/sources.list.d/nvidia-docker.list

                        # Use nvidia-container-toolkit
                        curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
                        curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
                            sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
                            sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
                        sudo apt-get update
                        sudo apt-get install -y nvidia-container-toolkit nvidia-container-runtime
                        ;;
                    fedora)
                        # Use nvidia-container-toolkit for Fedora
                        curl -s -L https://nvidia.github.io/libnvidia-container/stable/rpm/nvidia-container-toolkit.repo | \
                            sudo tee /etc/yum.repos.d/nvidia-container-toolkit.repo
                        sudo dnf install -y nvidia-container-toolkit nvidia-container-runtime
                        ;;
                    *)
                        echo "Unsupported distribution for automatic NVIDIA Docker runtime installation. Please install NVIDIA Docker runtime manually."
                        exit 1
                        ;;
                esac
            else
                echo "Unable to determine OS distribution. Please install NVIDIA Docker runtime manually."
                exit 1
            fi
        fi

        # Configure Docker to use NVIDIA runtime (runs whether we just installed or toolkit was pre-installed)
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                ubuntu|debian|fedora)
                    if sudo nvidia-ctk runtime configure --runtime=docker 2>/dev/null; then
                        echo "Configured NVIDIA runtime using nvidia-ctk"
                    else
                        # Fallback: manually configure daemon.json if nvidia-ctk doesn't support runtime configure
                        echo "Configuring NVIDIA runtime via /etc/docker/daemon.json..."
                        sudo mkdir -p /etc/docker
                        sudo tee /etc/docker/daemon.json > /dev/null <<'EOF'
{
  "runtimes": {
    "nvidia": {
      "path": "nvidia-container-runtime",
      "runtimeArgs": []
    }
  }
}
EOF
                    fi

                    echo "Restarting Docker to load NVIDIA runtime..."
                    sudo systemctl restart docker
                    # Wait for Docker to fully restart and verify NVIDIA runtime is available
                    echo "Waiting for Docker to restart..."
                    sleep 5
                    for i in {1..12}; do
                        if timeout 5 $DOCKER_CMD info 2>/dev/null | grep -i nvidia &> /dev/null; then
                            echo "NVIDIA runtime successfully configured in Docker."
                            break
                        fi
                        if [ $i -eq 12 ]; then
                            echo "Warning: NVIDIA runtime not detected after Docker restart. Please verify manually with: docker info | grep -i nvidia"
                        fi
                        sleep 5
                    done
                    ;;
                *)
                    echo "Unsupported distribution for automatic NVIDIA Docker runtime installation. Please install NVIDIA Docker runtime manually."
                    exit 1
                    ;;
            esac
        else
            echo "Unable to determine OS distribution. Please install NVIDIA Docker runtime manually."
            exit 1
        fi
    fi
}

# Function to check if Ollama is running on localhost:11434 or Docker bridge IP
check_ollama() {
    # Check localhost with a short read timeout using curl
    if curl -s --connect-timeout 2 -o /dev/null -w "%{http_code}" http://localhost:11434/v1/models >/dev/null; then
        return 0
    fi

    # Check Docker bridge IP
    DOCKER_BRIDGE_IP=$($DOCKER_CMD network inspect bridge --format='{{range .IPAM.Config}}{{.Gateway}}{{end}}' 2>/dev/null)
    if [ -n "$DOCKER_BRIDGE_IP" ]; then
        if curl -s --connect-timeout 2 -o /dev/null -w "%{http_code}" "http://${DOCKER_BRIDGE_IP}:11434/v1/models" >/dev/null; then
            return 0
        fi
    fi

    return 1
}

# Adjust default values based on provided arguments and AUTO mode
if [ "$AUTO" = true ]; then
    CLI=true
    CONTROLPLANE=true

    # If user specified an LLM provider, don't auto-detect
    if [ -n "$OPENAI_API_KEY" ] || [ -n "$TOGETHER_API_KEY" ]; then
        echo -e "Auto-install mode detected. Installing CLI and Control Plane.\n"
        if [ -n "$OPENAI_API_KEY" ]; then
            echo "Using OpenAI-compatible API for inference."
        else
            echo "Using Together.ai for inference."
        fi
        echo
    else
        # Only auto-detect if no LLM provider was specified
        if check_nvidia_gpu; then
            RUNNER=true
        fi
        echo -e "Auto-install mode detected. Installing CLI and Control Plane.\n"
        if check_nvidia_gpu; then
            echo "🚀 NVIDIA GPU detected. Runner will be installed locally."
            echo
        elif check_ollama; then
            echo "🦙 Ollama detected. Using local Ollama for inference provider."
            echo
        else
            echo "No NVIDIA GPU or Ollama detected. Ensure Ollama is running if you want to "
            echo "use it for inference. Otherwise, you need to point a DNS name at this server "
            echo "and set --api-host (e.g. --api-host https://helix.mycompany.com) and then "
            echo "connect a separate GPU node to this controlplane."
            echo
            echo "Command will be printed at the end to install runner separately on a GPU node, "
            echo "or pass --together-api-key to connect to together.ai for LLM inference."
            echo "See --help for more options."
            echo
        fi
    fi
fi

# Auto-enable controlplane if --code or --haystack specified (they're controlplane features)
if [ "$CONTROLPLANE" = false ] && [[ -n "$CODE" || -n "$HAYSTACK" ]]; then
    echo "Note: --code or --haystack specified (controlplane features)."
    echo "      Auto-enabling: --cli --controlplane"
    CONTROLPLANE=true
    CLI=true
fi

# Validate GPU requirements for --runner flag (MUST happen before token validation)
if [ "$RUNNER" = true ]; then
    # Check for NVIDIA GPU first
    if check_nvidia_gpu; then
        echo "NVIDIA GPU detected. Runner requirements satisfied."
        GPU_VENDOR="nvidia"

        if check_nvidia_runtime_needed; then
            # Check if toolkit already installed or needs fresh install
            if command -v nvidia-container-toolkit &> /dev/null; then
                echo "Note: NVIDIA Docker runtime will be configured automatically."
            else
                echo "Note: NVIDIA Docker runtime will be installed and configured automatically."
            fi
        fi
    elif check_amd_gpu; then
        echo "AMD GPU detected with ROCm support. Runner requirements satisfied."
        echo "Note: Ollama will use AMD GPU. vLLM will use CPU until ROCm-enabled runner image is available."
        GPU_VENDOR="amd"
    else
        echo "┌───────────────────────────────────────────────────────────────────────────"
        echo "│ ❌ ERROR: --runner requires GPU (NVIDIA or AMD)"
        echo "│"
        echo "│ No compatible GPU detected. Helix Runner requires a GPU with drivers."
        echo "│"
        echo "│ If you have an NVIDIA GPU:"
        echo "│   1. Install NVIDIA drivers: sudo ubuntu-drivers install"
        echo "│   2. Reboot: sudo reboot"
        echo "│   3. Verify: nvidia-smi"
        echo "│"
        echo "│ If you have an AMD GPU:"
        echo "│   1. Install AMD drivers and ROCm"
        echo "│   2. Verify: ls /dev/kfd /dev/dri"
        echo "└───────────────────────────────────────────────────────────────────────────"
        exit 1
    fi
fi

if [ "$RUNNER" = true ] && [ "$CONTROLPLANE" = false ]; then
    # Three cases:
    # 1. --runner WITH --runner-token = remote runner (needs API_HOST)
    # 2. --runner WITHOUT token but controlplane already enabled by --code/--haystack = local installation
    # 3. --runner WITHOUT token and no controlplane features = ERROR (missing token)

    if [ -n "$RUNNER_TOKEN" ]; then
        # Case 1: Remote runner - require API_HOST
        if [ -z "$API_HOST" ]; then
            echo "Error: When installing a remote runner, you must specify both --api-host and --runner-token"
            echo "to connect to an external controlplane, for example:"
            echo
            echo "./install.sh --runner --api-host https://your-controlplane-domain.com --runner-token YOUR_RUNNER_TOKEN"
            echo
            echo "You can find the runner token in <HELIX_INSTALL_DIR>/.env on the controlplane node."
            exit 1
        fi
    else
        # Case 2: --runner without token = ERROR (need either token or controlplane)
        echo "Error: --runner requires either:"
        echo "  1. --runner-token (for remote runner connecting to external controlplane)"
        echo "  2. --controlplane or controlplane features like --code/--haystack (for local installation)"
        echo
        echo "Examples:"
        echo "  Remote runner:  ./install.sh --runner --api-host HOST --runner-token TOKEN"
        echo "  Local install:  ./install.sh --runner --code --api-host HOST"
        echo "  Local install:  ./install.sh --controlplane --runner"
        exit 1
    fi
fi

if [ "$SANDBOX" = true ]; then
    # When installing sandbox node without controlplane, API_HOST, RUNNER_TOKEN are required
    if [ "$CONTROLPLANE" != true ] && ([ -z "$API_HOST" ] || [ -z "$RUNNER_TOKEN" ]); then
        echo "Error: When installing sandbox node, you must specify --api-host, --runner-token"
        echo "to connect to an external controlplane, for example:"
        echo
        echo "./install.sh --sandbox --api-host https://your-controlplane-domain.com --runner-token YOUR_RUNNER_TOKEN --turn-password YOUR_TURN_PASSWORD"
        echo
        echo "You can find these values in <HELIX_INSTALL_DIR>/.env on the controlplane node:"
        echo "  - RUNNER_TOKEN=..."
        echo "  - TURN_PASSWORD=... (optional)"
        exit 1
    fi
fi

# Validate GPU requirements for --code flag
if [ "$CODE" = true ]; then
    # Check NVIDIA first (most specific detection via nvidia-smi)
    if check_nvidia_gpu; then
        echo "NVIDIA GPU detected. Helix Code desktop streaming requirements satisfied."
        GPU_VENDOR="nvidia"

        if check_nvidia_runtime_needed; then
            # Check if toolkit already installed or needs fresh install
            if command -v nvidia-container-toolkit &> /dev/null; then
                echo "Note: NVIDIA Docker runtime will be configured automatically."
            else
                echo "Note: NVIDIA Docker runtime will be installed and configured automatically."
            fi
        fi
    elif check_amd_gpu; then
        # AMD GPU with ROCm support
        echo "AMD GPU detected with ROCm support (/dev/kfd + /dev/dri). Helix Code desktop streaming requirements satisfied."
        GPU_VENDOR="amd"
    elif check_intel_amd_gpu; then
        # No NVIDIA/AMD, but /dev/dri exists - assume Intel GPU
        echo "Intel GPU detected (/dev/dri). Helix Code desktop streaming requirements satisfied."
        GPU_VENDOR="intel"
    else
        # No GPU detected
        echo "┌───────────────────────────────────────────────────────────────────────────"
        echo "│ ❌ ERROR: --code requires GPU support for desktop streaming"
        echo "│"
        echo "│ No compatible GPU detected. Helix Code requires a GPU with drivers installed."
        echo "│"
        echo "│ If you have an NVIDIA GPU:"
        echo "│   1. Install NVIDIA drivers (Ubuntu/Debian):"
        echo "│      sudo ubuntu-drivers install"
        echo "│      # OR manually: sudo apt install nvidia-driver-<version>"
        echo "│"
        echo "│   2. Reboot your system:"
        echo "│      sudo reboot"
        echo "│"
        echo "│   3. Verify drivers are loaded:"
        echo "│      nvidia-smi"
        echo "│"
        echo "│   4. Re-run this installer - it will automatically install Docker and"
        echo "│      the NVIDIA Docker runtime for you."
        echo "│"
        echo "│ For Intel/AMD GPUs, ensure /dev/dri devices exist (drivers usually included"
        echo "│ in the kernel)."
        echo "└───────────────────────────────────────────────────────────────────────────"
        exit 1
    fi
fi

# Function to gather planned modifications
gather_modifications() {
    local modifications=""

    if [ "$CLI" = true ]; then
        modifications+="  - Install Helix CLI version ${LATEST_RELEASE}\n"
    fi

    # Check if Docker needs to be installed
    if [ "$CONTROLPLANE" = true ] || [ "$RUNNER" = true ] || [ "$SANDBOX" = true ]; then
        if check_docker_needed; then
            # Only add Docker installation for Ubuntu/Debian/Fedora on Linux
            if [ "$OS" = "linux" ] && [ -f /etc/os-release ]; then
                . /etc/os-release
                if [[ "$ID" == "ubuntu" || "$ID" == "debian" || "$ID" == "fedora" ]]; then
                    modifications+="  - Install Docker and Docker Compose plugin (Ubuntu/Debian/Fedora)\n"
                fi
            fi
        fi
    fi

    if [ "$CONTROLPLANE" = true ]; then
        modifications+="  - Set up Docker Compose stack for Helix Control Plane ${LATEST_RELEASE}\n"
        modifications+="  - Start/upgrade controlplane services and delete old Helix Docker images\n"
    fi

    if [ "$RUNNER" = true ]; then
        if check_nvidia_runtime_needed; then
            # Check if toolkit already installed or needs fresh install
            if command -v nvidia-container-toolkit &> /dev/null; then
                modifications+="  - Configure NVIDIA Docker runtime\n"
            else
                modifications+="  - Install and configure NVIDIA Docker runtime\n"
            fi
        fi
        modifications+="  - Set up start script for Helix Runner ${LATEST_RELEASE}\n"
        modifications+="  - Start/upgrade runner containers and delete old Helix runner images\n"
    fi

    # Install NVIDIA Docker runtime for --code with NVIDIA GPU (even without --runner)
    if [ "$CODE" = true ] && [ "$RUNNER" = false ]; then
        if check_nvidia_runtime_needed; then
            # Check if toolkit already installed or needs fresh install
            if command -v nvidia-container-toolkit &> /dev/null; then
                modifications+="  - Configure NVIDIA Docker runtime for desktop streaming\n"
            else
                modifications+="  - Install and configure NVIDIA Docker runtime for desktop streaming\n"
            fi
        fi
    fi


    if [ "$SANDBOX" = true ]; then
        modifications+="  - Set up Docker Compose for Sandbox Node (RevDial client with direct WebSocket streaming)\n"
        modifications+="  - Start/upgrade sandbox container and delete old Helix sandbox images\n"
    fi

    echo -e "$modifications"
}

# Function to ask for user approval
ask_for_approval() {
    if [ "$AUTO_APPROVE" = true ]; then
        return 0
    fi

    echo "┌───────────────────────────────────────────────────────────────────────────┐"
    echo "│ The following modifications will be made to your system:                  │"
    echo "└───────────────────────────────────────────────────────────────────────────┘"
    echo
    gather_modifications
    echo "┌───────────────────────────────────────────────────────────────────────────┐"
    echo "│ If this is not what you want, re-run the script with --help at the end to │"
    echo "│ see other options.                                                        │"
    echo "└───────────────────────────────────────────────────────────────────────────┘"
    echo
    read -p "Do you want to proceed? (y/N) " response
    case "$response" in
        [yY][eE][sS]|[yY])
            return 0
            ;;
        *)
            echo "Installation aborted."
            exit 1
            ;;
    esac
}

# Ask for user approval before proceeding
ask_for_approval

# Install Docker if needed and approved (only after user approval)
if [ "$CONTROLPLANE" = true ] || [ "$RUNNER" = true ] || [ "$SANDBOX" = true ]; then
    if check_docker_needed; then
        install_docker

        # After installing Docker, check if we need sudo to run it
        if [ "$ENVIRONMENT" != "gitbash" ]; then
            NEED_SUDO=$(check_docker_sudo)
            if [ "$NEED_SUDO" = "true" ]; then
                DOCKER_CMD="sudo docker"
            else
                DOCKER_CMD="docker"
            fi
        fi
    fi
fi

# Install NVIDIA Docker runtime for --code with NVIDIA GPU (even without --runner)
if [ "$CODE" = true ] && [ "$RUNNER" = false ]; then
    if check_nvidia_runtime_needed; then
        install_nvidia_docker
    fi
fi

# Load uhid kernel module for Helix Code (required for virtual HID devices in sandbox)
if [ "$CODE" = true ]; then
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        echo "Skipping uhid module check on Windows Git Bash"
    else
        # Check if uhid module is already loaded
        if lsmod | grep -q "^uhid "; then
            echo "✓ uhid module already loaded"
        else
            echo "uhid module not loaded - loading now for virtual HID device support..."
            if sudo modprobe uhid 2>/dev/null; then
                echo "✓ uhid module loaded"

                # Only configure auto-load if we had to load it manually
                if [ ! -f /etc/modules-load.d/helix.conf ] || ! grep -q "^uhid" /etc/modules-load.d/helix.conf; then
                    echo "uhid" | sudo tee -a /etc/modules-load.d/helix.conf > /dev/null
                    echo "✓ uhid module configured to auto-load on boot (/etc/modules-load.d/helix.conf)"
                fi
            else
                echo "Warning: Failed to load uhid module - virtual HID devices may not work correctly"
            fi
        fi
    fi
fi

# Increase inotify limits for sandbox nodes (Zed IDE watches many files per instance)
# Each Zed instance can use thousands of inotify watches; with multiple sandboxes running,
# the default limits (65536 watches, 128 instances) are quickly exhausted
if [ "$SANDBOX" = true ]; then
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        echo "Skipping inotify configuration on Windows Git Bash"
    else
        # Check current limits
        CURRENT_WATCHES=$(cat /proc/sys/fs/inotify/max_user_watches 2>/dev/null || echo "0")
        CURRENT_INSTANCES=$(cat /proc/sys/fs/inotify/max_user_instances 2>/dev/null || echo "0")

        # Target values: 1M watches, 1024 instances (enough for many Zed instances)
        TARGET_WATCHES=1048576
        TARGET_INSTANCES=1024

        NEEDS_UPDATE=false
        if [ "$CURRENT_WATCHES" -lt "$TARGET_WATCHES" ] || [ "$CURRENT_INSTANCES" -lt "$TARGET_INSTANCES" ]; then
            NEEDS_UPDATE=true
        fi

        if [ "$NEEDS_UPDATE" = true ]; then
            echo "Increasing inotify limits for Zed IDE file watching..."
            echo "  Current: max_user_watches=$CURRENT_WATCHES, max_user_instances=$CURRENT_INSTANCES"
            echo "  Target:  max_user_watches=$TARGET_WATCHES, max_user_instances=$TARGET_INSTANCES"

            # Apply immediately
            sudo sysctl -w fs.inotify.max_user_watches=$TARGET_WATCHES >/dev/null 2>&1 || true
            sudo sysctl -w fs.inotify.max_user_instances=$TARGET_INSTANCES >/dev/null 2>&1 || true

            # Make permanent via sysctl.d
            SYSCTL_CONF="/etc/sysctl.d/99-helix-inotify.conf"
            if [ ! -f "$SYSCTL_CONF" ] || ! grep -q "fs.inotify.max_user_watches" "$SYSCTL_CONF"; then
                cat << EOF | sudo tee "$SYSCTL_CONF" > /dev/null
# Helix Code: Increase inotify limits for Zed IDE file watching
# Each Zed instance can use thousands of watches; multiple sandboxes exhaust defaults
fs.inotify.max_user_watches = $TARGET_WATCHES
fs.inotify.max_user_instances = $TARGET_INSTANCES
EOF
                echo "✓ inotify limits increased and persisted to $SYSCTL_CONF"
            else
                echo "✓ inotify limits already configured in $SYSCTL_CONF"
            fi
        else
            echo "✓ inotify limits already sufficient (watches=$CURRENT_WATCHES, instances=$CURRENT_INSTANCES)"
        fi
    fi

    # Configure networking for Docker-in-Docker localhost forwarding
    # route_localnet allows 127.x.x.x addresses to be routed to non-loopback interfaces
    # This is required for DNAT rules that forward localhost:PORT to container networks
    echo "Configuring networking for Docker-in-Docker support..."

    # Apply immediately
    sudo sysctl -w net.ipv4.conf.all.route_localnet=1 >/dev/null 2>&1 || true
    sudo sysctl -w net.ipv4.conf.default.route_localnet=1 >/dev/null 2>&1 || true
    sudo sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 || true

    # Make permanent via sysctl.d
    SYSCTL_NET_CONF="/etc/sysctl.d/99-helix-networking.conf"
    if [ ! -f "$SYSCTL_NET_CONF" ] || ! grep -q "route_localnet" "$SYSCTL_NET_CONF"; then
        cat << EOF | sudo tee "$SYSCTL_NET_CONF" > /dev/null
# Helix Code: Networking configuration for Docker-in-Docker
# route_localnet: Allow 127.x.x.x addresses on non-loopback interfaces
# Required for localhost:PORT forwarding to container networks via DNAT
net.ipv4.conf.all.route_localnet = 1
net.ipv4.conf.default.route_localnet = 1
net.ipv4.ip_forward = 1
EOF
        echo "✓ Docker-in-Docker networking configured and persisted to $SYSCTL_NET_CONF"
    else
        echo "✓ Docker-in-Docker networking already configured in $SYSCTL_NET_CONF"
    fi
fi

# Create installation directories (platform-specific)
if [ "$ENVIRONMENT" = "gitbash" ]; then
    mkdir -p "$INSTALL_DIR"
    mkdir -p "$INSTALL_DIR/data/helix-"{postgres,filestore,pgvector}
    mkdir -p "$INSTALL_DIR/scripts/postgres/"
else
    sudo mkdir -p $INSTALL_DIR
    # Change the owner of the installation directory to the current user
    sudo chown -R $(id -un):$(id -gn) $INSTALL_DIR
    mkdir -p $INSTALL_DIR/data/helix-{postgres,filestore,pgvector}
    mkdir -p $INSTALL_DIR/scripts/postgres/
fi

# Install CLI if requested or in AUTO mode
if [ "$CLI" = true ]; then
    echo -e "\nDownloading Helix CLI..."
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        mkdir -p "$(dirname "$CLI_INSTALL_PATH")"
        curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/${BINARY_NAME}" -o "$CLI_INSTALL_PATH"
        chmod +x "$CLI_INSTALL_PATH"
    else
        sudo mkdir -p "$(dirname "$CLI_INSTALL_PATH")"
        sudo curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/${BINARY_NAME}" -o "$CLI_INSTALL_PATH"
        sudo chmod +x "$CLI_INSTALL_PATH"
    fi
    echo "Helix CLI has been installed to $CLI_INSTALL_PATH"
fi

# Function to generate random alphanumeric password
generate_password() {
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        # Try PowerShell if available, fallback to date-based
        if command -v powershell.exe &> /dev/null; then
            powershell.exe -Command "[System.Web.Security.Membership]::GeneratePassword(16, 0)" 2>/dev/null | tr -d '\r\n' || echo "helix$(date +%s)" | head -c 16
        else
            echo "helix$(date +%s)" | head -c 16
        fi
    else
        openssl rand -base64 12 | tr -dc 'a-zA-Z0-9' | head -c 16
    fi
}

# Function to generate 64-character hex encryption key (32 bytes for AES-256)
generate_encryption_key() {
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        # Try PowerShell for proper crypto random on Windows
        if command -v powershell.exe &> /dev/null; then
            powershell.exe -Command "[System.BitConverter]::ToString((1..32 | ForEach-Object { Get-Random -Maximum 256 })).Replace('-','')" 2>/dev/null | tr -d '\r\n' | head -c 64
        else
            # Fallback: Use current time + hostname hash (less secure)
            echo -n "$(hostname)$(date +%s%N)" | sha256sum | head -c 64
        fi
    else
        # Use openssl for crypto-secure random bytes
        openssl rand -hex 32
    fi
}

# Function to clean up old Helix Docker images that are no longer needed
# This helps reclaim disk space after upgrades by removing old image versions
# Parameters:
#   $1 - image_prefix: Prefix pattern for images to clean (e.g., "registry.helixml.tech/helix/")
#   $2 - keep_version: Version tag to keep (e.g., "1.5.0") - all other versions are removed
#   $3 - image_filter: Optional filter pattern (e.g., "controlplane" or "runner")
cleanup_old_helix_images() {
    local image_prefix="${1:-registry.helixml.tech/helix/}"
    local keep_version="${2:-}"
    local image_filter="${3:-}"

    echo ""
    echo "🧹 Cleaning up old Docker images..."

    # Safety check: don't clean up if we don't know what version to keep
    if [ -z "$keep_version" ]; then
        echo "   Skipping cleanup: no version specified to keep"
        return 0
    fi

    # Get all Helix images matching the prefix (and optional filter)
    local all_images
    if [ -n "$image_filter" ]; then
        all_images=$($DOCKER_CMD images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | grep "^${image_prefix}" | grep "$image_filter" | sort -u)
    else
        all_images=$($DOCKER_CMD images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | grep "^${image_prefix}" | sort -u)
    fi

    # Skip if no images found
    if [ -z "$all_images" ]; then
        echo "   No old images to clean up"
        return 0
    fi

    local removed_count=0
    local kept_count=0

    # Remove images that don't match the version we're installing
    while IFS= read -r image; do
        # Skip images with <none> tags
        if [[ "$image" == *":<none>"* ]]; then
            continue
        fi

        # Extract the tag from the image
        local image_tag="${image##*:}"

        # Keep images matching the installed version (or containing it, e.g., "1.5.0-small")
        if [ -n "$keep_version" ] && [[ "$image_tag" == *"$keep_version"* ]]; then
            kept_count=$((kept_count + 1))
        else
            echo "   Removing old image: $image"
            if $DOCKER_CMD rmi "$image" 2>/dev/null; then
                removed_count=$((removed_count + 1))
            fi
        fi
    done <<< "$all_images"

    # Final summary
    if [ "$removed_count" -gt 0 ]; then
        echo "✅ Cleaned up $removed_count old image(s), kept $kept_count current image(s)"
    else
        if [ "$kept_count" -gt 0 ]; then
            echo "   No old images to clean up (all $kept_count images are current version)"
        else
            echo "   No images matched cleanup criteria"
        fi
    fi
}

# Install controlplane if requested or in AUTO mode
if [ "$CONTROLPLANE" = true ]; then
    echo -e "\nDownloading docker-compose.yaml..."
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/docker-compose.yaml" -o $INSTALL_DIR/docker-compose.yaml
    else
        sudo curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/docker-compose.yaml" -o $INSTALL_DIR/docker-compose.yaml
    fi
    echo "docker-compose.yaml has been downloaded to $INSTALL_DIR/docker-compose.yaml"

    # Create database creation script
    cat << EOF > "$INSTALL_DIR/scripts/postgres/postgres-db.sh"
#!/bin/bash

set -e
set -u

function create_user_and_database() {
	local database=\$1
	echo "  Creating database '\$database'"
	psql -v ON_ERROR_STOP=1 --username "\$POSTGRES_USER" <<-EOSQL
	    CREATE DATABASE \$database;
EOSQL
}

if [ -n "\$POSTGRES_DATABASES" ]; then
	echo "Database creation requested: \$POSTGRES_DATABASES"
	for db in $(echo \$POSTGRES_DATABASES | tr ',' ' '); do
		create_user_and_database \$db
	done
	echo "databases created"
fi
EOF
    chmod +x $INSTALL_DIR/scripts/postgres/postgres-db.sh
    # Fix permissions for Docker container to read initialization scripts
    chmod 755 $INSTALL_DIR/scripts/postgres
    chmod 644 $INSTALL_DIR/scripts/postgres/postgres-db.sh

    # Create searxng settings.yml and limiter.toml files
    mkdir -p "$INSTALL_DIR/searxng"
    cat << EOF > "$INSTALL_DIR/searxng/settings.yml"
use_default_settings: true
general:
    instance_name: 'helix-instance'
search:
    autocomplete: 'google'
server:
    secret_key: 'replace_me' # Is overwritten by \${SEARXNG_SECRET}
engines:
    - name: wolframalpha
      disabled: false
EOF

    cat << EOF > "$INSTALL_DIR/searxng/limiter.toml"
[botdetection.ip_limit]
# activate link_token method in the ip_limit method
link_token = false

[botdetection.ip_lists]
block_ip = []
pass_ip = []
EOF

    # Create .env file
    ENV_FILE="$INSTALL_DIR/.env"
    echo -e "\nCreating/updating .env file..."
    echo

    # Default to localhost if it wasn't passed
    if [ -z "$API_HOST" ]; then
        API_HOST="http://localhost:8080"
    fi

    if [ -f "$ENV_FILE" ]; then
        echo ".env file already exists. Reusing existing secrets."

        # Make a backup copy of the .env file
        DATE=$(date +%Y%m%d%H%M%S)
        cp "$ENV_FILE" "$ENV_FILE-$DATE"
        echo "Backup of .env file created: $ENV_FILE-$DATE"
        echo
        echo "To see what changed, run:"
        echo "diff $ENV_FILE $ENV_FILE-$DATE"
        echo

        KEYCLOAK_ADMIN_PASSWORD=$(grep '^KEYCLOAK_ADMIN_PASSWORD=' "$ENV_FILE" | sed 's/^KEYCLOAK_ADMIN_PASSWORD=//' || generate_password)
        POSTGRES_ADMIN_PASSWORD=$(grep '^POSTGRES_ADMIN_PASSWORD=' "$ENV_FILE" | sed 's/^POSTGRES_ADMIN_PASSWORD=//' || generate_password)
        RUNNER_TOKEN=$(grep '^RUNNER_TOKEN=' "$ENV_FILE" | sed 's/^RUNNER_TOKEN=//' || generate_password)
        PGVECTOR_PASSWORD=$(grep '^PGVECTOR_PASSWORD=' "$ENV_FILE" | sed 's/^PGVECTOR_PASSWORD=//' || generate_password)
        HELIX_ENCRYPTION_KEY=$(grep '^HELIX_ENCRYPTION_KEY=' "$ENV_FILE" | sed 's/^HELIX_ENCRYPTION_KEY=//' || generate_encryption_key)

        # Preserve API keys if not provided as command line arguments
        if [ -z "$ANTHROPIC_API_KEY" ]; then
            ANTHROPIC_API_KEY=$(grep '^ANTHROPIC_API_KEY=' "$ENV_FILE" | sed 's/^ANTHROPIC_API_KEY=//' || echo "")
        fi

        # Preserve Code credentials if --code flag is set
        if [[ -n "$CODE" ]]; then
            TURN_PASSWORD=$(grep '^TURN_PASSWORD=' "$ENV_FILE" | sed 's/^TURN_PASSWORD=//' || generate_password)
        fi

    else
        echo ".env file does not exist. Generating new passwords."
        KEYCLOAK_ADMIN_PASSWORD=$(generate_password)
        POSTGRES_ADMIN_PASSWORD=$(generate_password)
        RUNNER_TOKEN=${RUNNER_TOKEN:-$(generate_password)}
        PGVECTOR_PASSWORD=$(generate_password)
        HELIX_ENCRYPTION_KEY=$(generate_encryption_key)

        # Generate Code credentials if --code flag is set
        if [[ -n "$CODE" ]]; then
            TURN_PASSWORD=$(generate_password)
        fi
    fi

    # Build comma-separated list of Docker Compose profiles
    # Note: Sandbox profiles (code-nvidia, code-amd-intel) are NOT set here because
    # production sandboxes are managed by sandbox.sh, not docker-compose
    COMPOSE_PROFILES=""
    if [[ -n "$HAYSTACK" ]]; then
        COMPOSE_PROFILES="haystack"
    fi

    # Set RAG provider
    RAG_DEFAULT_PROVIDER=""
    if [[ -n "$HAYSTACK" ]]; then
        RAG_DEFAULT_PROVIDER="haystack"
    fi

    # Generate .env content
    cat << EOF > "$ENV_FILE"
# Set passwords
KEYCLOAK_ADMIN_PASSWORD=$KEYCLOAK_ADMIN_PASSWORD
POSTGRES_ADMIN_PASSWORD=$POSTGRES_ADMIN_PASSWORD
RUNNER_TOKEN=${RUNNER_TOKEN:-$(generate_password)}
PGVECTOR_PASSWORD=$PGVECTOR_PASSWORD

# Encryption key for secrets at rest (SSH keys, PATs, etc.)
# 64-character hex string (32 bytes for AES-256-GCM)
HELIX_ENCRYPTION_KEY=$HELIX_ENCRYPTION_KEY

# URLs
KEYCLOAK_FRONTEND_URL=${API_HOST}/auth/
SERVER_URL=${API_HOST}

# Docker Compose profiles
COMPOSE_PROFILES=$COMPOSE_PROFILES

# GPU vendor (nvidia, amd, intel, or empty)
GPU_VENDOR=${GPU_VENDOR:-}

# Haystack features
RAG_HAYSTACK_ENABLED=${HAYSTACK:-false}
RAG_DEFAULT_PROVIDER=$RAG_DEFAULT_PROVIDER

# Storage
# Uncomment the lines below and create the directories if you want to persist
# direct to disk rather than a docker volume. You may need to set up the
# directory user and group on the filesystem and in the docker-compose.yaml
# file.
#POSTGRES_DATA=$INSTALL_DIR/data/helix-postgres
#FILESTORE_DATA=$INSTALL_DIR/data/helix-filestore
#PGVECTOR_DATA=$INSTALL_DIR/data/helix-pgvector

# Optional integrations:

## LLM provider
EOF

    AUTODETECTED_LLM=false
    # If user hasn't specified LLM provider, check if Ollama is running on localhost:11434
    if [ -z "$OPENAI_API_KEY" ] && [ -z "$OPENAI_BASE_URL" ] && [ -z "$TOGETHER_API_KEY" ]; then
        echo "No LLM provider specified. Checking if Ollama is running on localhost:11434..."
        if check_ollama; then
            echo "Ollama (or another OpenAI compatible API) detected on localhost:11434. Configuring Helix to use it."
            echo
            echo "OPENAI_API_KEY=ollama" >> "$ENV_FILE"
            echo "OPENAI_BASE_URL=http://host.docker.internal:11434/v1" >> "$ENV_FILE"
            echo "INFERENCE_PROVIDER=openai" >> "$ENV_FILE"
            echo "FINETUNING_PROVIDER=openai" >> "$ENV_FILE"
            AUTODETECTED_LLM=true
        else
            # Only warn the user if there's also no GPU
            if ! check_nvidia_gpu; then
                echo
                echo "┌────────────────────────────────────────────────────────────────────────────────────────────────────────"
                echo "│ ⚠️ Ollama not detected on localhost."
                echo "│ "
                echo "│ Note that Helix will be non-functional without an LLM provider or GPU runner attached."
                echo "│ "
                echo "│ You have 4 options:"
                echo "│ "
                echo "│ 1. USE OLLAMA LOCALLY"
                echo "│    If you want to use Ollama, start it and re-run the installer so that it can be detected"
                echo "│ "
                echo "│ 2. ATTACH YOUR OWN NVIDIA GPU(S)"
                echo "│    You can attach a separate node(s) with an NVIDIA GPU as helix runners (instructions printed below)"
                echo "│ "
                echo "│ 3. USE TOGETHER.AI"
                echo "│    You can re-run the installer with --together-api-key (see --help for details)"
                echo "│ "
                echo "│ 4. USE ANTHROPIC"
                echo "│    You can re-run the installer with --anthropic-api-key (see --help for details)"
                echo "│ "
                echo "│ 5. USE EXTERNAL OPENAI COMPATIBLE LLM"
                echo "│    You can re-run the installer with --openai-api-key and --openai-base-url (see --help for details)"
                echo "└────────────────────────────────────────────────────────────────────────────────────────────────────────"
                echo
            fi
        fi
    fi

    # Add TogetherAI configuration if token is provided
    if [ -n "$TOGETHER_API_KEY" ]; then
        cat << EOF >> "$ENV_FILE"
INFERENCE_PROVIDER=togetherai
FINETUNING_PROVIDER=togetherai
TOGETHER_API_KEY=$TOGETHER_API_KEY
EOF
    fi

    # Add OpenAI configuration if key and base URL are provided
    if [ -n "$OPENAI_API_KEY" ]; then
        cat << EOF >> "$ENV_FILE"
INFERENCE_PROVIDER=openai
FINETUNING_PROVIDER=openai
OPENAI_API_KEY=$OPENAI_API_KEY
EOF
    fi

    if [ -n "$OPENAI_BASE_URL" ]; then
        cat << EOF >> "$ENV_FILE"
OPENAI_BASE_URL=$OPENAI_BASE_URL
EOF
    fi

    # Add Anthropic configuration if API key is provided
    if [ -n "$ANTHROPIC_API_KEY" ]; then
        cat << EOF >> "$ENV_FILE"
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY
EOF
    fi

    # Add Hugging Face token configuration if provided
    if [ -n "$HF_TOKEN" ]; then
        cat << EOF >> "$ENV_FILE"
HF_TOKEN=$HF_TOKEN
EOF
    fi
    # Add embeddings provider configuration
    cat << EOF >> "$ENV_FILE"
RAG_PGVECTOR_PROVIDER=$EMBEDDINGS_PROVIDER
EOF

    # Add providers management configuration
    cat << EOF >> "$ENV_FILE"
PROVIDERS_MANAGEMENT_ENABLED=$PROVIDERS_MANAGEMENT_ENABLED
EOF

    # Set default FINETUNING_PROVIDER to helix if neither OpenAI nor TogetherAI are specified
    if [ -z "$OPENAI_API_KEY" ] && [ -z "$TOGETHER_API_KEY" ] && [ "$AUTODETECTED_LLM" = false ]; then
        cat << EOF >> "$ENV_FILE"
FINETUNING_PROVIDER=helix
INFERENCE_PROVIDER=helix
EOF
    fi

    # Add Helix Code configuration if --code flag is set
    if [[ -n "$CODE" ]]; then
        # Extract hostname from API_HOST for TURN server
        TURN_HOST=$(echo "$API_HOST" | sed -E 's|^https?://||' | sed 's|:[0-9]+$||')

        # Auto-detect GPU render node for sandbox based on GPU_VENDOR
        # On some systems (Lambda Labs), renderD128 is virtio-gpu (virtual), actual GPU starts at renderD129
        WOLF_RENDER_NODE="/dev/dri/renderD128"  # Default

        # Handle "none" case for software rendering (no GPU available)
        if [ "$GPU_VENDOR" = "none" ]; then
            WOLF_RENDER_NODE="software"
            echo "No GPU detected - using software rendering (llvmpipe)"
        elif [ -d "/sys/class/drm" ]; then
            # Determine which driver to look for based on GPU_VENDOR
            case "$GPU_VENDOR" in
                nvidia)
                    target_driver="nvidia"
                    ;;
                amd)
                    target_driver="amdgpu"
                    ;;
                *)
                    target_driver=""
                    ;;
            esac

            if [ -n "$target_driver" ]; then
                for render_node in /dev/dri/renderD*; do
                    if [ -e "$render_node" ]; then
                        # Check if this render node matches our target driver
                        node_name=$(basename "$render_node")
                        driver_link="/sys/class/drm/$node_name/device/driver"
                        if [ -L "$driver_link" ]; then
                            driver=$(readlink "$driver_link" | grep -o '[^/]*$')
                            if [[ "$driver" == "$target_driver" ]]; then
                                WOLF_RENDER_NODE="$render_node"
                                echo "Auto-detected $GPU_VENDOR render node: $WOLF_RENDER_NODE (driver: $driver)"
                                break
                            fi
                        fi
                    fi
                done
            fi
        fi

        cat << EOF >> "$ENV_FILE"

## Helix Code Configuration (External Agents / PDEs)
# Sandbox streaming configuration
WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock
WOLF_RENDER_NODE=${WOLF_RENDER_NODE}
# GPU vendor for video pipeline configuration: nvidia, amd, intel, none (software rendering)
GPU_VENDOR=${GPU_VENDOR}
ZED_IMAGE=registry.helixml.tech/helix/zed-agent:${LATEST_RELEASE}
HELIX_HOST_HOME=${INSTALL_DIR}

# Helix hostname (displayed in browser to distinguish between servers)
HELIX_HOSTNAME=${TURN_HOST}

# TURN server for WebRTC NAT traversal
TURN_ENABLED=true
TURN_PUBLIC_IP=${TURN_HOST}
TURN_PORT=3478
TURN_REALM=${TURN_HOST}
TURN_USERNAME=helix
TURN_PASSWORD=${TURN_PASSWORD}

# GOP size (keyframe interval in frames)
# 15 = keyframe every 0.25s at 60fps (good quality, higher bandwidth)
# 60 = keyframe every 1s (balanced)
# 120 = keyframe every 2s (lower bandwidth, recommended for Helix Code)
GOP_SIZE=120
EOF

        # Create wolf directory for desktop configuration
        mkdir -p "$INSTALL_DIR/wolf"
        echo "Desktop config directory created (configs will be generated by containers)"

        # Generate self-signed certificates for sandbox HTTPS only if they don't exist
        if [ ! -f "$INSTALL_DIR/wolf/cert.pem" ] || [ ! -f "$INSTALL_DIR/wolf/key.pem" ]; then
            echo "Generating sandbox SSL certificates..."
            # Create temp config for SAN (Subject Alternative Names)
            cat > /tmp/wolf-cert-san.conf <<EOF
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
C = IT
O = GamesOnWhales
CN = localhost

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = wolf
EOF
            openssl req -x509 -newkey rsa:2048 -keyout "$INSTALL_DIR/wolf/key.pem" -out "$INSTALL_DIR/wolf/cert.pem" \
                -days 365 -nodes -config /tmp/wolf-cert-san.conf -extensions v3_req 2>/dev/null
            rm -f /tmp/wolf-cert-san.conf
            echo "Sandbox SSL certificates created at $INSTALL_DIR/wolf/"
        else
            echo "Sandbox SSL certificates already exist at $INSTALL_DIR/wolf/ (preserving existing)"
        fi
    fi

    # Continue with the rest of the .env file
    cat << EOF >> "$ENV_FILE"

## Analytics
# GOOGLE_ANALYTICS_FRONTEND=
# SENTRY_DSN_FRONTEND=
# SENTRY_DSN_API=

## Notifications
# EMAIL_SMTP_HOST=smtp.example.com
# EMAIL_SMTP_PORT=25
# EMAIL_SMTP_USERNAME=REPLACE_ME
# EMAIL_SMTP_PASSWORD=REPLACE_ME

# EMAIL_MAILGUN_DOMAIN=REPLACE_ME
# EMAIL_MAILGUN_API_KEY=REPLACE_ME
EOF

    CADDY=false
    # Install Caddy if API_HOST is an HTTPS URL and system is Ubuntu
    if [[ "$API_HOST" == https* ]]; then
        if [[ "$ENVIRONMENT" = "gitbash" ]]; then
            echo "Caddy installation is not supported in Git Bash. Please install and configure Caddy manually on Windows."
            echo "For Windows installation, see: https://caddyserver.com/docs/install#windows"
        elif [[ "$OS" != "linux" ]]; then
            echo "Caddy installation is only supported on Ubuntu. Please install and configure Caddy manually (check the install.sh script for details)."
        else
            CADDY=true
            . /etc/os-release
            if [[ "$ID" != "ubuntu" && "$ID" != "debian" ]]; then
                echo "Caddy installation is only supported on Ubuntu. Please install and configure Caddy manually (check the install.sh script for details)."
            else
                echo "Installing Caddy..."
                sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl

                # Always refresh the GPG key (keys can expire)
                curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
                # Fix file permissions so _apt user can read the keyring
                sudo chmod 644 /usr/share/keyrings/caddy-stable-archive-keyring.gpg

                # Check if the source list file already exists
                if [ ! -f /etc/apt/sources.list.d/caddy-stable.list ]; then
                    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
                fi

                sudo apt update
                sudo apt install caddy

                # Create Caddyfile
                CADDYFILE="/etc/caddy/Caddyfile"
                echo "Creating Caddyfile..."
                # Strip https:// and port from API_HOST
                CADDY_HOST=$(echo "$API_HOST" | sed -e 's/^https:\/\///' -e 's/:.*//')
                sudo bash -c "cat << 'CADDYEOF' > \"$CADDYFILE\"
$CADDY_HOST {
    # RevDial endpoint needs special handling for long-lived hijacked connections
    @revdial path /api/v1/revdial*
    reverse_proxy @revdial localhost:8080 {
        flush_interval -1
        transport http {
            response_header_timeout 0
            read_timeout 0
            write_timeout 0
        }
    }
    reverse_proxy localhost:8080
}
CADDYEOF"
                # Substitute CADDY_HOST variable (since we used 'CADDYEOF' to prevent other expansion)
                sudo sed -i "s/\\\$CADDY_HOST/$CADDY_HOST/g" "$CADDYFILE"
                # Add OLLAMA_HOST environment variable to ollama.service on Linux
                if [ "$OS" = "linux" ]; then
                    OLLAMA_SERVICE_FILE="/etc/systemd/system/ollama.service"
                    if [ -f "$OLLAMA_SERVICE_FILE" ]; then
                        echo "Detecting Docker bridge IP..."
                        DOCKER_BRIDGE_IP=$(docker network inspect bridge --format='{{range .IPAM.Config}}{{.Gateway}}{{end}}')
                        if [ -n "$DOCKER_BRIDGE_IP" ]; then
                            echo "Adding OLLAMA_HOST environment variable to ollama.service..."
                            sudo sed -i "/^\[Service\]/a Environment=\"OLLAMA_HOST=$DOCKER_BRIDGE_IP\"" "$OLLAMA_SERVICE_FILE"
                            sudo systemctl daemon-reload
                            echo "Restarting Ollama service..."
                            sudo systemctl restart ollama
                            echo "ollama.service has been updated with OLLAMA_HOST=$DOCKER_BRIDGE_IP and restarted."
                        else
                            echo "Warning: Failed to detect Docker bridge IP. Please add 'Environment=\"OLLAMA_HOST=<your-docker-bridge-ip>\"' to the [Service] section of $OLLAMA_SERVICE_FILE manually and restart the service."
                        fi
                    else
                        echo "Warning: $OLLAMA_SERVICE_FILE not found. Please add 'Environment=\"OLLAMA_HOST=<your-docker-bridge-ip>\"' to the [Service] section manually and restart the service."
                    fi
                fi
                echo "Caddyfile has been created at $CADDYFILE"
                echo "Please start Caddy manually after starting the Docker Compose stack:"
            fi
        fi
    fi

    echo ".env file has been created at $ENV_FILE"
    echo
    echo "┌───────────────────────────────────────────────────────────────────────────"
    echo "│ ❗ To complete installation, you MUST now:"
    if [ "$API_HOST" != "http://localhost:8080" ]; then
        echo "│"
        echo "│ If you haven't already, set up DNS for your domain:"
        echo "│   - Create an A record for $(echo "$API_HOST" | sed -E 's|^https?://||' | sed 's|:[0-9]+$||') pointing to your server's IP address"
    fi
    echo "│"
    echo "│ ⚠️  Ensure the following firewall ports are open:"
    if [[ "$API_HOST" == https* ]]; then
        echo "│   - TCP 443: HTTPS (Caddy reverse proxy)"
    else
        echo "│   - TCP 8080: Main API"
    fi
    if [[ -n "$CODE" ]]; then
        echo "│"
        echo "│ ⚠️  Additional ports for desktop streaming (Helix Code):"
        echo "│   - UDP 3478: TURN server for WebRTC NAT traversal"
        echo "│   - UDP 40000-40100: WebRTC media ports"
    fi
    # When sandbox is being installed with controlplane, we start services later (after sandbox setup)
    if [ "$SANDBOX" = true ]; then
        echo "│"
        echo "│ Services will be started automatically after sandbox setup."
        echo "│ Helix will be available at $API_HOST"
        echo "└───────────────────────────────────────────────────────────────────────────"
    else
        # Always start/upgrade controlplane services
        echo "│"
        echo "│ Starting Helix services..."
        echo "└───────────────────────────────────────────────────────────────────────────"
        echo
        cd "$INSTALL_DIR"
        if [ "$NEED_SUDO" = "true" ]; then
            sudo docker compose up -d --remove-orphans
        else
            docker compose up -d --remove-orphans
        fi
        # Clean up old controlplane Docker images to free disk space
        cleanup_old_helix_images "registry.helixml.tech/helix/" "$LATEST_RELEASE"
        if [ "$CADDY" = true ]; then
            echo "Restarting Caddy reverse proxy..."
            sudo systemctl restart caddy
        fi
        echo
        echo "✅ Helix $LATEST_RELEASE started"
        echo "   Helix is available at $API_HOST"
    fi
fi

# Install runner if requested or in AUTO mode with GPU
if [ "$RUNNER" = true ]; then
    # Only install NVIDIA Docker runtime if GPU is NVIDIA (not AMD)
    if [ "$GPU_VENDOR" = "nvidia" ]; then
        install_nvidia_docker
    fi

    # Determine runner tag
    if [ "$LARGE" = true ]; then
        RUNNER_TAG="${LATEST_RELEASE}-large"
    else
        RUNNER_TAG="${LATEST_RELEASE}-small"
    fi

    # Determine runner token
    if [ -z "$RUNNER_TOKEN" ]; then
        if [ -f "$INSTALL_DIR/.env" ]; then
            RUNNER_TOKEN=$(grep RUNNER_TOKEN $INSTALL_DIR/.env | cut -d '=' -f2)
        else
            echo "Error: RUNNER_TOKEN not found in .env file and --runner-token not provided."
            echo "Please provide the runner token using the --runner-token argument."
            exit 1
        fi
    fi

    # Create runner.sh
    cat << EOF > $INSTALL_DIR/runner.sh
#!/bin/bash

# Configuration variables
RUNNER_TAG="${RUNNER_TAG}"
API_HOST="${API_HOST}"
RUNNER_TOKEN="${RUNNER_TOKEN}"
SPLIT_RUNNERS="${SPLIT_RUNNERS}"
EXCLUDE_GPUS="${EXCLUDE_GPUS}"
GPU_VENDOR="${GPU_VENDOR}"  # Set by install.sh: "nvidia" or "amd"

# HF_TOKEN is now managed by the control plane and distributed to runners automatically
# No longer setting HF_TOKEN on runners to avoid confusion
HF_TOKEN_PARAM=""

# Check if api-1 container is running
if docker ps --format '{{.Image}}' | grep 'registry.helixml.tech/helix/controlplane'; then
    API_HOST="http://api:8080"
    echo "Detected controlplane container running. Setting API_HOST to \${API_HOST}"
fi

# Check if helix_default network exists
# If docker-compose.yaml exists (controlplane on same machine), require network from docker compose
# If standalone runner (no docker-compose.yaml), create network if needed
SCRIPT_DIR="\$(cd "\$(dirname "\$0")" && pwd)"
if [ -f "\$SCRIPT_DIR/docker-compose.yaml" ]; then
    # Controlplane is on same machine - network should be created by docker compose
    if ! docker network inspect helix_default >/dev/null 2>&1; then
        echo "Error: helix_default network does not exist."
        echo "Please run 'docker compose up -d' first to start the controlplane."
        echo "The controlplane creates the network with correct Docker Compose labels."
        exit 1
    fi
    # Check if network has correct Docker Compose labels
    NETWORK_LABEL=\$(docker network inspect helix_default --format '{{index .Labels "com.docker.compose.network"}}' 2>/dev/null || echo "")
    if [ "\$NETWORK_LABEL" != "default" ]; then
        echo "Error: helix_default network exists but has incorrect labels."
        echo "This happens when the network was created manually instead of by Docker Compose."
        echo ""
        echo "To fix, run:"
        echo "  docker network rm helix_default"
        echo "  docker compose up -d"
        echo "  ./runner.sh"
        exit 1
    fi
    echo "helix_default network exists with correct labels."
else
    # Standalone runner - create network if needed
    if ! docker network inspect helix_default >/dev/null 2>&1; then
        echo "Creating helix_default network..."
        docker network create helix_default
    else
        echo "helix_default network already exists."
    fi
fi

# Detect total number of GPUs based on vendor
if [ "\$GPU_VENDOR" = "nvidia" ]; then
    ALL_GPUS=\$(nvidia-smi --list-gpus 2>/dev/null | wc -l)
    if [ "\$ALL_GPUS" -eq 0 ]; then
        echo "Error: No NVIDIA GPUs detected. Cannot start runner."
        exit 1
    fi
elif [ "\$GPU_VENDOR" = "amd" ]; then
    # Count AMD GPUs via rocm-smi or /dev/dri/card* devices
    if command -v rocm-smi &> /dev/null; then
        ALL_GPUS=\$(rocm-smi --showid 2>/dev/null | grep -c "^GPU")
    else
        ALL_GPUS=\$(ls -1 /dev/dri/card* 2>/dev/null | wc -l)
    fi
    if [ "\$ALL_GPUS" -eq 0 ]; then
        echo "Error: No AMD GPUs detected. Cannot start runner."
        exit 1
    fi
else
    echo "Error: Unknown GPU_VENDOR: \$GPU_VENDOR"
    exit 1
fi

echo "Detected \$ALL_GPUS total GPUs on this system"

# Build list of available GPUs (excluding any specified in EXCLUDE_GPUS)
AVAILABLE_GPUS=""
for gpu_id in \$(seq 0 \$((\$ALL_GPUS - 1))); do
    # Check if this GPU is in the exclude list
    EXCLUDED=false
    if [ -n "\$EXCLUDE_GPUS" ]; then
        IFS=',' read -ra EXCLUDE_ARRAY <<< "\$EXCLUDE_GPUS"
        for exclude_id in "\${EXCLUDE_ARRAY[@]}"; do
            if [ "\$gpu_id" -eq "\$exclude_id" ]; then
                EXCLUDED=true
                echo "Excluding GPU \$gpu_id (--exclude-gpu)"
                break
            fi
        done
    fi

    if [ "\$EXCLUDED" = false ]; then
        if [ -z "\$AVAILABLE_GPUS" ]; then
            AVAILABLE_GPUS="\$gpu_id"
        else
            AVAILABLE_GPUS="\$AVAILABLE_GPUS \$gpu_id"
        fi
    fi
done

# Convert space-separated list to array for easier processing
IFS=' ' read -ra AVAILABLE_GPU_ARRAY <<< "\$AVAILABLE_GPUS"
TOTAL_GPUS=\${#AVAILABLE_GPU_ARRAY[@]}

if [ "\$TOTAL_GPUS" -eq 0 ]; then
    echo "Error: All GPUs excluded. No GPUs available for runner."
    exit 1
fi

echo "Using \$TOTAL_GPUS available GPU(s): \$AVAILABLE_GPUS"

# Validate SPLIT_RUNNERS
if [ "\$SPLIT_RUNNERS" -lt 1 ]; then
    echo "Error: --split-runners must be at least 1"
    exit 1
fi

if [ "\$SPLIT_RUNNERS" -gt "\$TOTAL_GPUS" ]; then
    echo "Error: --split-runners (\$SPLIT_RUNNERS) cannot be greater than total GPUs (\$TOTAL_GPUS)"
    exit 1
fi

# Check if TOTAL_GPUS is evenly divisible by SPLIT_RUNNERS
if [ \$((\$TOTAL_GPUS % \$SPLIT_RUNNERS)) -ne 0 ]; then
    echo "Error: Total GPUs (\$TOTAL_GPUS) must be evenly divisible by --split-runners (\$SPLIT_RUNNERS)"
    echo "Please choose a value that divides evenly into \$TOTAL_GPUS"
    exit 1
fi

GPUS_PER_RUNNER=\$((\$TOTAL_GPUS / \$SPLIT_RUNNERS))
echo "Creating \$SPLIT_RUNNERS runner container(s) with \$GPUS_PER_RUNNER GPU(s) each"

# Stop and remove any existing runner containers
# First, always clean up the old non-numbered container (upgrade path)
if docker ps -a --format '{{.Names}}' | grep -q "^helix-runner\$"; then
    echo "Stopping and removing existing container: helix-runner"
    docker stop helix-runner >/dev/null 2>&1 || true
    docker rm helix-runner >/dev/null 2>&1 || true
fi

# Then clean up numbered containers if SPLIT_RUNNERS > 1
if [ "\$SPLIT_RUNNERS" -gt 1 ]; then
    for i in \$(seq 1 \$SPLIT_RUNNERS); do
        CONTAINER_NAME="helix-runner-\$i"
        if docker ps -a --format '{{.Names}}' | grep -q "^\${CONTAINER_NAME}\$"; then
            echo "Stopping and removing existing container: \$CONTAINER_NAME"
            docker stop \$CONTAINER_NAME >/dev/null 2>&1 || true
            docker rm \$CONTAINER_NAME >/dev/null 2>&1 || true
        fi
    done
fi

# Create runner containers
for i in \$(seq 1 \$SPLIT_RUNNERS); do
    # Calculate indices into the available GPU array
    START_IDX=\$(( (\$i - 1) * \$GPUS_PER_RUNNER ))
    END_IDX=\$(( \$START_IDX + \$GPUS_PER_RUNNER - 1 ))

    # Build device list from available GPUs (e.g., "1,2" if GPU 0 excluded)
    GPU_DEVICES=""
    for array_idx in \$(seq \$START_IDX \$END_IDX); do
        gpu_id=\${AVAILABLE_GPU_ARRAY[\$array_idx]}
        if [ -z "\$GPU_DEVICES" ]; then
            GPU_DEVICES="\$gpu_id"
        else
            GPU_DEVICES="\$GPU_DEVICES,\$gpu_id"
        fi
    done

    # Set container name
    if [ "\$SPLIT_RUNNERS" -eq 1 ]; then
        CONTAINER_NAME="helix-runner"
        RUNNER_ID="\$(hostname)"
    else
        CONTAINER_NAME="helix-runner-\$i"
        RUNNER_ID="\$(hostname)-\$i"
    fi

    echo "Starting \$CONTAINER_NAME with GPU(s): \$GPU_DEVICES (runner ID: \$RUNNER_ID)"

    # Build vendor-specific GPU flags
    if [ "\$GPU_VENDOR" = "nvidia" ]; then
        # NVIDIA: Use --gpus device=X flag
        # Docker automatically renumbers GPUs inside container (e.g., host GPUs 2,3 become container GPUs 0,1)
        GPU_FLAGS="--gpus '\"'device=\$GPU_DEVICES'\"'"
        ENV_FLAGS=""
    elif [ "\$GPU_VENDOR" = "amd" ]; then
        # AMD: Use device pass-through + ROCR_VISIBLE_DEVICES env var
        # Note: ROCR_VISIBLE_DEVICES uses GPU IDs (0,1,2) same as CUDA
        # Note: --group-add not needed since container runs with --ipc=host and device access
        GPU_FLAGS="--device /dev/kfd --device /dev/dri"
        ENV_FLAGS="-e ROCR_VISIBLE_DEVICES=\$GPU_DEVICES"
    else
        echo "Error: Unknown GPU_VENDOR: \$GPU_VENDOR"
        exit 1
    fi

    # Run the docker container with vendor-specific GPU configuration
    eval docker run \$GPU_FLAGS \$ENV_FLAGS \\
        --shm-size=10g --restart=always -d \\
        --name \$CONTAINER_NAME --ipc=host --ulimit memlock=-1 \\
        --ulimit stack=67108864 \\
        --network="helix_default" \\
        registry.helixml.tech/helix/runner:\${RUNNER_TAG} \\
        --api-host \${API_HOST} --api-token \${RUNNER_TOKEN} \\
        --runner-id \$RUNNER_ID
done

echo "Successfully started \$SPLIT_RUNNERS runner container(s)"

# Clean up old runner images to free disk space
echo ""
echo "🧹 Cleaning up old runner Docker images..."

# Safety check: don't clean up if RUNNER_TAG is empty
if [ -z "\$RUNNER_TAG" ]; then
    echo "   Skipping cleanup: no version specified to keep"
else
    # Get all runner images
    ALL_RUNNER_IMAGES=\$(docker images --format '{{.Repository}}:{{.Tag}}' | grep "registry.helixml.tech/helix/runner" | sort -u)

    REMOVED_COUNT=0
    KEPT_COUNT=0
    for image in \$ALL_RUNNER_IMAGES; do
        # Extract the tag from the image
        IMAGE_TAG="\${image##*:}"

        # Keep images matching the installed version (RUNNER_TAG contains version like "1.5.0-small")
        # RUNNER_TAG is set at the top of this script
        if [[ "\$IMAGE_TAG" == "\$RUNNER_TAG" ]]; then
            KEPT_COUNT=\$((KEPT_COUNT + 1))
        else
            echo "   Removing old image: \$image"
            if docker rmi "\$image" 2>/dev/null; then
                REMOVED_COUNT=\$((REMOVED_COUNT + 1))
            fi
        fi
    done

    if [ "\$REMOVED_COUNT" -gt 0 ]; then
        echo "✅ Cleaned up \$REMOVED_COUNT old runner image(s), kept \$KEPT_COUNT current"
    else
        if [ "\$KEPT_COUNT" -gt 0 ]; then
            echo "   No old runner images to clean up (all \$KEPT_COUNT images are current version)"
        else
            echo "   No runner images found to clean up"
        fi
    fi
fi
EOF

    if [ "$ENVIRONMENT" = "gitbash" ]; then
        chmod +x $INSTALL_DIR/runner.sh
    else
        sudo chmod +x $INSTALL_DIR/runner.sh
    fi
    echo "Runner script has been created at $INSTALL_DIR/runner.sh"

    # Always start/upgrade runner
    echo
    echo "Starting runner..."
    if [ "$NEED_SUDO" = "true" ]; then
        sudo $INSTALL_DIR/runner.sh
    else
        $INSTALL_DIR/runner.sh
    fi
    echo
    echo "✅ Runner started with version $RUNNER_TAG"
fi

# Install sandbox node if requested
if [ "$SANDBOX" = true ]; then
    echo -e "\nInstalling Helix Sandbox Node (RevDial client with direct WebSocket streaming)..."
    echo "=================================================="
    echo
    echo "API Host: $API_HOST"
    echo "Runner Token: ${RUNNER_TOKEN:0:20}..."
    echo

    # Detect GPU type for sandbox
    if check_nvidia_gpu; then
        GPU_TYPE="nvidia"
        GPU_VENDOR="nvidia"
        echo "NVIDIA GPU detected"

        if check_nvidia_runtime_needed; then
            if command -v nvidia-container-toolkit &> /dev/null; then
                echo "Note: NVIDIA Docker runtime will be configured automatically."
            else
                echo "Note: NVIDIA Docker runtime will be installed and configured automatically."
            fi
        fi
    elif check_amd_gpu; then
        GPU_TYPE="amd"
        GPU_VENDOR="amd"
        echo "AMD GPU detected with ROCm support"
    elif check_intel_amd_gpu; then
        GPU_TYPE="intel"
        GPU_VENDOR="intel"
        echo "Intel GPU detected"
    else
        echo "Warning: No GPU detected. Sandbox may not work correctly."
        GPU_TYPE=""
        GPU_VENDOR="none"
    fi

    # Generate unique sandbox instance ID (hostname)
    WOLF_ID=$(hostname)
    echo "Sandbox Instance ID: $WOLF_ID"
    echo

    # Configure NVIDIA runtime if needed
    if [ "$GPU_VENDOR" = "nvidia" ] && check_nvidia_runtime_needed; then
        echo "Configuring NVIDIA Docker runtime..."
        install_nvidia_docker
    fi

    # Create sandbox.sh script (embedded, like runner.sh)
    cat << 'EOF' > $INSTALL_DIR/sandbox.sh
#!/bin/bash

# Configuration variables (set by install.sh)
SANDBOX_TAG="${SANDBOX_TAG}"
HELIX_API_URL="${HELIX_API_URL}"
WOLF_INSTANCE_ID="${WOLF_INSTANCE_ID}"
RUNNER_TOKEN="${RUNNER_TOKEN}"
GPU_VENDOR="${GPU_VENDOR}"
MAX_SANDBOXES="${MAX_SANDBOXES}"
TURN_PUBLIC_IP="${TURN_PUBLIC_IP}"
TURN_PASSWORD="${TURN_PASSWORD}"
HELIX_HOSTNAME="${HELIX_HOSTNAME}"
PRIVILEGED_DOCKER="${PRIVILEGED_DOCKER}"

# Check if helix_default network exists
# If docker-compose.yaml exists (controlplane on same machine), require network from docker compose
# If standalone sandbox (no docker-compose.yaml), create network if needed
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/docker-compose.yaml" ]; then
    # Controlplane is on same machine - network should be created by docker compose
    if ! docker network inspect helix_default >/dev/null 2>&1; then
        echo "Error: helix_default network does not exist."
        echo "Please run 'docker compose up -d' first to start the controlplane."
        echo "The controlplane creates the network with correct Docker Compose labels."
        exit 1
    fi
    # Check if network has correct Docker Compose labels
    NETWORK_LABEL=$(docker network inspect helix_default --format '{{index .Labels "com.docker.compose.network"}}' 2>/dev/null || echo "")
    if [ "$NETWORK_LABEL" != "default" ]; then
        echo "Error: helix_default network exists but has incorrect labels."
        echo "This happens when the network was created manually instead of by Docker Compose."
        echo ""
        echo "To fix, run:"
        echo "  docker stop helix-sandbox 2>/dev/null; docker rm helix-sandbox 2>/dev/null"
        echo "  docker network rm helix_default"
        echo "  docker compose up -d"
        echo "  ./sandbox.sh"
        exit 1
    fi
    echo "helix_default network exists with correct labels."
else
    # Standalone sandbox - create network if needed
    if ! docker network inspect helix_default >/dev/null 2>&1; then
        echo "Creating helix_default network..."
        docker network create helix_default
    else
        echo "helix_default network already exists."
    fi
fi

# Stop and remove existing sandbox container if it exists
if docker ps -a --format '{{.Names}}' | grep -q "^helix-sandbox$"; then
    echo "Stopping and removing existing container: helix-sandbox"
    docker stop helix-sandbox >/dev/null 2>&1 || true
    docker rm helix-sandbox >/dev/null 2>&1 || true
fi

# Build GPU-specific flags
if [ "$GPU_VENDOR" = "nvidia" ]; then
    GPU_FLAGS="--gpus all --runtime nvidia --device /dev/dri"
    GPU_ENV_FLAGS="-e NVIDIA_DRIVER_CAPABILITIES=all -e NVIDIA_VISIBLE_DEVICES=all -e GPU_VENDOR=nvidia"
elif [ "$GPU_VENDOR" = "amd" ]; then
    # Note: --group-add not needed since container runs privileged with device access
    GPU_FLAGS="--device /dev/kfd --device /dev/dri"
    GPU_ENV_FLAGS="-e GPU_VENDOR=amd"
elif [ "$GPU_VENDOR" = "intel" ]; then
    GPU_FLAGS="--device /dev/dri"
    GPU_ENV_FLAGS="-e GPU_VENDOR=intel"
elif [ "$GPU_VENDOR" = "none" ]; then
    # Software rendering - no GPU device mounts needed
    GPU_FLAGS=""
    GPU_ENV_FLAGS="-e GPU_VENDOR=none -e WOLF_RENDER_NODE=software -e LIBGL_ALWAYS_SOFTWARE=1 -e MESA_GL_VERSION_OVERRIDE=4.5 -e WOLF_USE_ZERO_COPY=FALSE"
    echo "Using software rendering (no GPU detected)"
else
    GPU_FLAGS=""
    GPU_ENV_FLAGS=""
    echo "Warning: Unknown GPU_VENDOR '$GPU_VENDOR' - no GPU support configured"
fi

echo "Starting Helix Sandbox container..."
echo "  Control Plane: $HELIX_API_URL"
echo "  Sandbox Instance ID: $WOLF_INSTANCE_ID"
echo "  GPU Vendor: $GPU_VENDOR"
echo "  Max Sandboxes: $MAX_SANDBOXES"
echo "  TURN Server: $TURN_PUBLIC_IP"
echo "  Privileged Docker Mode: ${PRIVILEGED_DOCKER:-false}"

# Build privileged Docker flags (mount host Docker socket for Helix-in-Helix development)
if [ "$PRIVILEGED_DOCKER" = "true" ]; then
    # Mount host Docker socket to a different path so it doesn't conflict with DinD's /var/run/docker.sock
    PRIVILEGED_DOCKER_FLAGS="-v /var/run/docker.sock:/var/run/host-docker.sock:rw"
    echo "  ⚠️  Privileged mode: mounting host Docker socket for Helix development"
else
    PRIVILEGED_DOCKER_FLAGS=""
fi

# Run the sandbox container
# Note: Don't use 'eval' here - it breaks quoting for --device-cgroup-rule
# GPU_FLAGS contains --device /dev/dri for GPU modes (nvidia, amd, intel)
# GPU_ENV_FLAGS contains GPU_VENDOR and software rendering env vars for none mode
# shellcheck disable=SC2086
docker run $GPU_FLAGS $GPU_ENV_FLAGS $PRIVILEGED_DOCKER_FLAGS \
    --privileged \
    --restart=always -d \
    --name helix-sandbox \
    --network="helix_default" \
    -e HELIX_API_URL="$HELIX_API_URL" \
    -e WOLF_INSTANCE_ID="$WOLF_INSTANCE_ID" \
    -e RUNNER_TOKEN="$RUNNER_TOKEN" \
    -e MAX_SANDBOXES="$MAX_SANDBOXES" \
    -e ZED_IMAGE=helix-sway:latest \
    -e TURN_PUBLIC_IP="$TURN_PUBLIC_IP" \
    -e TURN_PASSWORD="$TURN_PASSWORD" \
    -e HELIX_HOSTNAME="$HELIX_HOSTNAME" \
    -e HYDRA_ENABLED=true \
    -e HYDRA_PRIVILEGED_MODE_ENABLED="${PRIVILEGED_DOCKER:-false}" \
    -e SANDBOX_DATA_PATH=/data \
    -e XDG_RUNTIME_DIR=/tmp/sockets \
    -e HOST_APPS_STATE_FOLDER=/etc/wolf \
    -e WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock \
    -e WOLF_PRIVATE_KEY_FILE=/etc/wolf/cfg/key.pem \
    -e WOLF_PRIVATE_CERT_FILE=/etc/wolf/cfg/cert.pem \
    -e GOP_SIZE=120 \
    -e WOLF_MAX_DUMPS=6 \
    -e WOLF_MAX_DUMPS_GB=20 \
    -v sandbox-storage:/var/lib/docker \
    -v sandbox-data:/data \
    -v sandbox-debug-dumps:/var/wolf-debug-dumps \
    -v hydra-storage:/hydra-data \
    -v /run/udev:/run/udev:rw \
    --device /dev/uinput \
    --device /dev/uhid \
    --device-cgroup-rule='c 13:* rmw' \
    -p 47984:47984 \
    -p 47989:47989 \
    -p 48010:48010 \
    -p 47415:47415/udp \
    -p 47999:47999/udp \
    -p 48100:48100/udp \
    -p 48200:48200/udp \
    -p 40000-40100:40000-40100/udp \
    registry.helixml.tech/helix/helix-sandbox:${SANDBOX_TAG}

if [ $? -eq 0 ]; then
    echo "✅ Helix Sandbox container started successfully"

    # Clean up old sandbox images to free disk space
    echo ""
    echo "🧹 Cleaning up old sandbox Docker images..."

    # Safety check: don't clean up if SANDBOX_TAG is empty
    if [ -z "$SANDBOX_TAG" ]; then
        echo "   Skipping cleanup: no version specified to keep"
    else
        # Get all sandbox images
        ALL_SANDBOX_IMAGES=$(docker images --format '{{.Repository}}:{{.Tag}}' | grep "registry.helixml.tech/helix/helix-sandbox" | sort -u)

        REMOVED_COUNT=0
        KEPT_COUNT=0
        for image in $ALL_SANDBOX_IMAGES; do
            # Extract the tag from the image
            IMAGE_TAG="${image##*:}"

            # Keep images matching the installed version (SANDBOX_TAG is set at the top of this script)
            if [ "$IMAGE_TAG" = "$SANDBOX_TAG" ]; then
                KEPT_COUNT=$((KEPT_COUNT + 1))
            else
                echo "   Removing old image: $image"
                if docker rmi "$image" 2>/dev/null; then
                    REMOVED_COUNT=$((REMOVED_COUNT + 1))
                fi
            fi
        done

        if [ "$REMOVED_COUNT" -gt 0 ]; then
            echo "✅ Cleaned up $REMOVED_COUNT old sandbox image(s), kept $KEPT_COUNT current"
        else
            if [ "$KEPT_COUNT" -gt 0 ]; then
                echo "   No old sandbox images to clean up (all $KEPT_COUNT images are current version)"
            else
                echo "   No sandbox images found to clean up"
            fi
        fi
    fi

    echo
    echo "To view logs: docker logs -f helix-sandbox"
    echo "To stop: docker stop helix-sandbox"
    echo "To restart: $0"
else
    echo "❌ Failed to start Helix Sandbox container"
    exit 1
fi
EOF

    # Extract hostname from API_HOST for TURN server and display name
    # (e.g., https://helix.mycompany.com -> helix.mycompany.com)
    TURN_PUBLIC_IP=$(echo "$API_HOST" | sed -E 's|^https?://||' | sed -E 's|:[0-9]+$||')
    HELIX_HOSTNAME="$TURN_PUBLIC_IP"

    # Substitute variables in the script
    sed -i "s|\${SANDBOX_TAG}|${LATEST_RELEASE}|g" $INSTALL_DIR/sandbox.sh
    # When controlplane is on same machine, use Docker network hostname
    # localhost inside sandbox container resolves to container itself, not the host
    if [ "$CONTROLPLANE" = true ]; then
        sed -i "s|\${HELIX_API_URL}|http://api:8080|g" $INSTALL_DIR/sandbox.sh
    else
        sed -i "s|\${HELIX_API_URL}|${API_HOST}|g" $INSTALL_DIR/sandbox.sh
    fi
    sed -i "s|\${WOLF_INSTANCE_ID}|${WOLF_ID}|g" $INSTALL_DIR/sandbox.sh
    sed -i "s|\${RUNNER_TOKEN}|${RUNNER_TOKEN}|g" $INSTALL_DIR/sandbox.sh
    sed -i "s|\${GPU_VENDOR}|${GPU_VENDOR}|g" $INSTALL_DIR/sandbox.sh
    sed -i "s|\${MAX_SANDBOXES}|10|g" $INSTALL_DIR/sandbox.sh
    sed -i "s|\${TURN_PUBLIC_IP}|${TURN_PUBLIC_IP}|g" $INSTALL_DIR/sandbox.sh
    sed -i "s|\${TURN_PASSWORD}|${TURN_PASSWORD}|g" $INSTALL_DIR/sandbox.sh
    sed -i "s|\${HELIX_HOSTNAME}|${HELIX_HOSTNAME}|g" $INSTALL_DIR/sandbox.sh
    sed -i "s|\${PRIVILEGED_DOCKER}|${PRIVILEGED_DOCKER:-false}|g" $INSTALL_DIR/sandbox.sh

    if [ "$ENVIRONMENT" = "gitbash" ]; then
        chmod +x $INSTALL_DIR/sandbox.sh
    else
        sudo chmod +x $INSTALL_DIR/sandbox.sh
    fi

    echo "Sandbox script created at $INSTALL_DIR/sandbox.sh"
    echo

    # If controlplane is also being installed, run docker compose first to create the network
    if [ "$CONTROLPLANE" = true ]; then
        echo "Starting controlplane services first (creates Docker network with correct labels)..."
        cd $INSTALL_DIR
        if [ "$NEED_SUDO" = "true" ]; then
            sudo docker compose up -d --remove-orphans
        else
            docker compose up -d --remove-orphans
        fi
        # Clean up old controlplane Docker images to free disk space
        cleanup_old_helix_images "registry.helixml.tech/helix/" "$LATEST_RELEASE"
        # Restart Caddy if it was installed (for HTTPS reverse proxy)
        if [ "$CADDY" = true ]; then
            echo "Restarting Caddy reverse proxy..."
            sudo systemctl restart caddy
        fi
        echo "Waiting for controlplane to be ready..."
        sleep 5
    fi

    # Run the sandbox script to start the container
    echo "Starting sandbox container..."
    if [ "$NEED_SUDO" = "true" ]; then
        sudo $INSTALL_DIR/sandbox.sh
    else
        $INSTALL_DIR/sandbox.sh
    fi

    echo
    echo "┌───────────────────────────────────────────────────────────────────────────"
    echo "│ ✅ Helix Sandbox Node installed successfully!"
    echo "│"
    echo "│ Connected to: $API_HOST"
    echo "│ Sandbox Instance ID: $WOLF_ID"
    echo "│ TURN Server: $TURN_PUBLIC_IP"
    echo "│"
    echo "│ ℹ️  WebRTC streaming (browser) works behind NAT via the control plane's TURN server."
    echo "│"
    echo "│ ⚠️  For better performance (direct connections), open these ports on the sandbox:"
    echo "│   - UDP 40000-40100: WebRTC media (bypasses TURN, reduces latency)"
    echo "│"
    echo "│ ⚠️  Ensure the control plane has these ports open:"
    echo "│   - UDP/TCP 3478: TURN server for WebRTC NAT traversal"
    echo "│"
    echo "│ To check logs:"
    if [ "$NEED_SUDO" = "true" ]; then
        echo "│   sudo docker logs -f helix-sandbox"
    else
        echo "│   docker logs -f helix-sandbox"
    fi
    echo "│"
    echo "│ To restart:"
    if [ "$NEED_SUDO" = "true" ]; then
        echo "│   sudo $INSTALL_DIR/sandbox.sh"
    else
        echo "│   $INSTALL_DIR/sandbox.sh"
    fi
    echo "│"
    echo "│ To stop:"
    if [ "$NEED_SUDO" = "true" ]; then
        echo "│   sudo docker stop helix-sandbox"
    else
        echo "│   docker stop helix-sandbox"
    fi
    echo "└───────────────────────────────────────────────────────────────────────────"

    exit 0
fi

if [ -n "$API_HOST" ] && [ "$CONTROLPLANE" = true ]; then
    echo
    echo "To connect an external runner to this controlplane, run on a node with a GPU:"
    echo
    echo "curl -Ls -O https://get.helixml.tech/install.sh"
    echo "chmod +x install.sh"
    echo "./install.sh --runner --api-host $API_HOST --runner-token $RUNNER_TOKEN"
    echo
    echo "To connect a sandbox node to this controlplane:"
    echo "./install.sh --sandbox --api-host $API_HOST --runner-token $RUNNER_TOKEN"
fi

echo -e "\nInstallation complete."
