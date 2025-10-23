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
#    Result: Offer to install Docker + NVIDIA runtime → Install both → Create runner
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
# 9. Fedora + No Docker + NVIDIA GPU + --runner
#    Result: Offer to install Docker + NVIDIA runtime → Install both using dnf
#
# 10. Ubuntu + Docker + NVIDIA runtime installed + --runner
#     Result: Skip Docker/runtime installation → Create runner script
#
# 11. Arch Linux + No Docker + --controlplane
#     Result: Exit with error, auto-install only supports Ubuntu/Debian/Fedora
#
# 12. Ubuntu + No Docker + No GPU + --code
#     Result: Exit with error, instructions to install NVIDIA/Intel/AMD drivers
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
EXTERNAL_ZED_AGENT=false
LARGE=false
HAYSTACK=""
KODIT=""
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
EXTERNAL_ZED_RUNNER_ID=""
EXTERNAL_ZED_CONCURRENCY="1"

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
  --external-zed-agent     Install the external Zed agent runner (connects to existing controlplane)
  --large                  Install the large version of the runner (includes all models, 100GB+ download, otherwise uses small one)
  --haystack               Enable the haystack and vectorchord/postgres based RAG service (downloads tens of gigabytes of python but provides better RAG quality than default typesense/tika stack), also uses GPU-accelerated embeddings in helix runners
  --kodit                  Enable the kodit code indexing service
  --code                   Enable Helix Code features (Wolf streaming, External Agents, PDEs with Zed, Moonlight Web). Requires GPU (Intel/AMD/NVIDIA) with drivers installed and --api-host parameter.
  --api-host <host>        Specify the API host for the API to serve on and/or the runner to connect to, e.g. http://localhost:8080 or https://my-controlplane.com. Will install and configure Caddy if HTTPS and running on Ubuntu.
  --runner-token <token>   Specify the runner token when connecting a runner to an existing controlplane
  --together-api-key <token> Specify the together.ai token for inference, rag and apps without a GPU
  --openai-api-key <key>   Specify the OpenAI API key for any OpenAI compatible API
  --openai-base-url <url>  Specify the base URL for the OpenAI API
  --anthropic-api-key <key> Specify the Anthropic API key for Claude models
  --hf-token <token>       Specify the Hugging Face token for the control plane (automatically distributed to runners)
  --embeddings-provider <provider> Specify the provider for embeddings (openai, togetherai, vllm, helix, default: helix)
  --external-zed-runner-id <id> Specify runner ID for external Zed agent (default: external-zed-{hostname})
  --external-zed-concurrency <n> Specify concurrency for external Zed agent (default: 1)
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

8. Install external Zed agent to connect to existing controlplane:
   ./install.sh --external-zed-agent --api-host https://helix.mycompany.com --runner-token YOUR_RUNNER_TOKEN

9. Install CLI and controlplane with OpenAI-compatible API key and base URL:
   ./install.sh --cli --controlplane --openai-api-key YOUR_OPENAI_API_KEY --openai-base-url YOUR_OPENAI_BASE_URL

10. Install CLI and controlplane with custom embeddings provider:
   ./install.sh --cli --controlplane --embeddings-provider openai

11. Install on Windows Git Bash (requires Docker Desktop):
   ./install.sh --cli --controlplane

12. Install with Helix Code (External Agents, PDEs, streaming):
   ./install.sh --cli --controlplane --code --api-host https://helix.mycompany.com

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
        --external-zed-agent)
            EXTERNAL_ZED_AGENT=true
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
        --kodit)
            KODIT=true
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
        --external-zed-runner-id=*)
            EXTERNAL_ZED_RUNNER_ID="${1#*=}"
            shift
            ;;
        --external-zed-runner-id)
            EXTERNAL_ZED_RUNNER_ID="$2"
            shift 2
            ;;
        --external-zed-concurrency=*)
            EXTERNAL_ZED_CONCURRENCY="${1#*=}"
            shift
            ;;
        --external-zed-concurrency)
            EXTERNAL_ZED_CONCURRENCY="$2"
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
                    ;;
                fedora)
                    sudo dnf -y install dnf-plugins-core
                    sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
                    sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
                    sudo systemctl start docker
                    sudo systemctl enable docker
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
    fi

    # Docker Compose plugin is included in docker-ce installation above for Ubuntu/Debian/Fedora
    # No additional installation needed - it's part of docker-compose-plugin package
}

# default docker command
DOCKER_CMD="docker"

# Only check docker sudo if we need docker (i.e., not CLI-only installation)
if [ "$CLI" = true ] && [ "$CONTROLPLANE" = false ] && [ "$RUNNER" = false ] && [ "$EXTERNAL_ZED_AGENT" = false ]; then
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

# Function to check for Intel/AMD GPU (for Helix Code)
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
    if timeout 10 $DOCKER_CMD info 2>/dev/null | grep -i nvidia &> /dev/null; then
        return 1  # Already configured
    fi

    # Check if nvidia-container-toolkit command exists
    if command -v nvidia-container-toolkit &> /dev/null; then
        return 1  # Already installed
    fi

    return 0  # NVIDIA GPU present but runtime not installed
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
    NVIDIA_IN_DOCKER=$(timeout 10 $DOCKER_CMD info 2>/dev/null | grep -i nvidia &> /dev/null && echo "true" || echo "false")
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
        echo "NVIDIA Docker runtime not found or incomplete. Installing NVIDIA Docker runtime..."
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                ubuntu|debian)
                    # Use nvidia-container-toolkit (modern method)
                    curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
                    curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
                        sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
                        sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
                    sudo apt-get update
                    sudo apt-get install -y nvidia-container-toolkit
                    sudo nvidia-ctk runtime configure --runtime=docker
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
                fedora)
                    # Use nvidia-container-toolkit for Fedora
                    curl -s -L https://nvidia.github.io/libnvidia-container/stable/rpm/nvidia-container-toolkit.repo | \
                        sudo tee /etc/yum.repos.d/nvidia-container-toolkit.repo
                    sudo dnf install -y nvidia-container-toolkit
                    sudo nvidia-ctk runtime configure --runtime=docker
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

if [ "$RUNNER" = true ] && [ "$CONTROLPLANE" = false ] && [ -z "$API_HOST" ]; then
    echo "Error: When installing only the runner, you must specify --api-host and --runner-token"
    echo "to connect to an external controlplane, for example:"
    echo
    echo "./install.sh --runner --api-host https://your-controlplane-domain.com --runner-token YOUR_RUNNER_TOKEN"
    echo
    echo "You can find the runner token in <HELIX_INSTALL_DIR>/.env on the controlplane node."
    exit 1
fi

if [ "$EXTERNAL_ZED_AGENT" = true ] && [ -z "$API_HOST" ]; then
    echo "Error: When installing the external Zed agent, you must specify --api-host and --runner-token"
    echo "to connect to an external controlplane, for example:"
    echo
    echo "./install.sh --external-zed-agent --api-host https://your-controlplane-domain.com --runner-token YOUR_RUNNER_TOKEN"
    echo
    echo "You can find the runner token in <HELIX_INSTALL_DIR>/.env on the controlplane node."
    exit 1
fi

# Validate GPU requirements for --runner flag
if [ "$RUNNER" = true ]; then
    if ! check_nvidia_gpu; then
        echo "┌───────────────────────────────────────────────────────────────────────────"
        echo "│ ❌ ERROR: --runner requires NVIDIA GPU"
        echo "│"
        echo "│ No NVIDIA GPU detected. Helix Runner requires an NVIDIA GPU."
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
        echo "└───────────────────────────────────────────────────────────────────────────"
        exit 1
    fi
    echo "NVIDIA GPU detected. Runner requirements satisfied."
    if check_nvidia_runtime_needed; then
        echo "Note: NVIDIA Docker runtime will be installed automatically."
    fi
fi

# Validate GPU requirements for --code flag
if [ "$CODE" = true ]; then
    # Check NVIDIA first (most specific detection via nvidia-smi)
    if check_nvidia_gpu; then
        echo "NVIDIA GPU detected. Helix Code desktop streaming requirements satisfied."

        if check_nvidia_runtime_needed; then
            echo "Note: NVIDIA Docker runtime will be installed automatically."
        fi
    elif check_intel_amd_gpu; then
        # No NVIDIA, but /dev/dri exists - assume Intel/AMD GPU
        echo "Intel/AMD GPU detected (/dev/dri). Helix Code desktop streaming requirements satisfied."
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

# Function to check if jq needs to be installed
check_jq_needed() {
    # jq is only needed for --code flag (Wolf certificate sync)
    if [ "$CODE" != true ]; then
        return 1  # Not needed
    fi

    if command -v jq &> /dev/null; then
        return 1  # Already installed
    fi

    return 0  # Needed but not installed
}

# Function to install jq
install_jq() {
    if ! command -v jq &> /dev/null; then
        echo "Installing jq..."
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                ubuntu|debian)
                    sudo apt-get update
                    sudo apt-get install -y jq
                    ;;
                fedora)
                    sudo dnf install -y jq
                    ;;
                *)
                    echo "Unsupported distribution for automatic jq installation."
                    echo "Please install jq manually:"
                    echo "  Ubuntu/Debian: sudo apt-get install jq"
                    echo "  Fedora: sudo dnf install jq"
                    exit 1
                    ;;
            esac
        else
            echo "Unable to determine OS distribution. Please install jq manually."
            exit 1
        fi
    fi
}

# Function to gather planned modifications
gather_modifications() {
    local modifications=""

    if [ "$CLI" = true ]; then
        modifications+="  - Install Helix CLI version ${LATEST_RELEASE}\n"
    fi

    # Check if Docker needs to be installed
    if [ "$CONTROLPLANE" = true ] || [ "$RUNNER" = true ] || [ "$EXTERNAL_ZED_AGENT" = true ]; then
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
    fi

    if [ "$RUNNER" = true ]; then
        if check_nvidia_runtime_needed; then
            modifications+="  - Install NVIDIA Docker runtime\n"
        fi
        modifications+="  - Set up start script for Helix Runner ${LATEST_RELEASE}\n"
    fi

    # Install NVIDIA Docker runtime for --code with NVIDIA GPU (even without --runner)
    if [ "$CODE" = true ] && [ "$RUNNER" = false ]; then
        if check_nvidia_runtime_needed; then
            modifications+="  - Install NVIDIA Docker runtime for desktop streaming\n"
        fi
    fi

    if [ "$EXTERNAL_ZED_AGENT" = true ]; then
        modifications+="  - Build Zed agent Docker image\n"
        modifications+="  - Install External Zed Agent runner script\n"
    fi

    # Check if jq needs to be installed (at the end)
    if check_jq_needed; then
        if [ "$OS" = "linux" ] && [ -f /etc/os-release ]; then
            . /etc/os-release
            if [[ "$ID" == "ubuntu" || "$ID" == "debian" || "$ID" == "fedora" ]]; then
                modifications+="  - Install jq (JSON processor for Wolf certificate sync)\n"
            fi
        fi
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
if [ "$CONTROLPLANE" = true ] || [ "$RUNNER" = true ] || [ "$EXTERNAL_ZED_AGENT" = true ]; then
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

# Install jq if needed and approved (only after user approval)
if check_jq_needed; then
    install_jq
fi

# Install NVIDIA Docker runtime for --code with NVIDIA GPU (even without --runner)
if [ "$CODE" = true ] && [ "$RUNNER" = false ]; then
    if check_nvidia_runtime_needed; then
        install_nvidia_docker
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

# Function to generate random 4-digit PIN for Moonlight pairing
generate_moonlight_pin() {
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        # Generate random 4-digit number on Git Bash
        echo $((RANDOM % 9000 + 1000))
    else
        # Use /dev/urandom for better randomness on Linux/macOS
        echo $(($(od -An -N2 -i /dev/urandom) % 9000 + 1000))
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

        # Preserve API keys if not provided as command line arguments
        if [ -z "$ANTHROPIC_API_KEY" ]; then
            ANTHROPIC_API_KEY=$(grep '^ANTHROPIC_API_KEY=' "$ENV_FILE" | sed 's/^ANTHROPIC_API_KEY=//' || echo "")
        fi

    else
        echo ".env file does not exist. Generating new passwords."
        KEYCLOAK_ADMIN_PASSWORD=$(generate_password)
        POSTGRES_ADMIN_PASSWORD=$(generate_password)
        RUNNER_TOKEN=${RUNNER_TOKEN:-$(generate_password)}
        PGVECTOR_PASSWORD=$(generate_password)
    fi

    # Build comma-separated list of Docker Compose profiles
    COMPOSE_PROFILES=""
    if [[ -n "$HAYSTACK" ]]; then
        COMPOSE_PROFILES="haystack"
    fi
    if [[ -n "$KODIT" ]]; then
        COMPOSE_PROFILES="${COMPOSE_PROFILES:+$COMPOSE_PROFILES,}kodit"
    fi
    if [[ -n "$CODE" ]]; then
        COMPOSE_PROFILES="${COMPOSE_PROFILES:+$COMPOSE_PROFILES,}code"
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

# URLs
KEYCLOAK_FRONTEND_URL=${API_HOST}/auth/
SERVER_URL=${API_HOST}

# Docker Compose profiles
COMPOSE_PROFILES=$COMPOSE_PROFILES

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

    # Set default FINETUNING_PROVIDER to helix if neither OpenAI nor TogetherAI are specified
    if [ -z "$OPENAI_API_KEY" ] && [ -z "$TOGETHER_API_KEY" ] && [ "$AUTODETECTED_LLM" = false ]; then
        cat << EOF >> "$ENV_FILE"
FINETUNING_PROVIDER=helix
INFERENCE_PROVIDER=helix
EOF
    fi

    # Add Helix Code configuration if --code flag is set
    if [[ -n "$CODE" ]]; then
        # Generate TURN password, moonlight credentials, and pairing PIN
        TURN_PASSWORD=$(generate_password)
        MOONLIGHT_CREDENTIALS=$(generate_password)
        MOONLIGHT_PIN=$(generate_moonlight_pin)

        # Extract hostname from API_HOST for TURN server
        TURN_HOST=$(echo "$API_HOST" | sed -E 's|^https?://||' | sed 's|:[0-9]+$||')

        cat << EOF >> "$ENV_FILE"

## Helix Code Configuration (External Agents / PDEs)
# Wolf streaming platform
WOLF_SOCKET_PATH=/var/run/wolf/wolf.sock
ZED_IMAGE=registry.helixml.tech/helix/zed-agent:${LATEST_RELEASE}

# Moonlight Web credentials (secure random, shared between API and moonlight-web)
MOONLIGHT_CREDENTIALS=${MOONLIGHT_CREDENTIALS}

# TURN server for WebRTC NAT traversal
TURN_ENABLED=true
TURN_PUBLIC_IP=${TURN_HOST}
TURN_PORT=3478
TURN_REALM=helix.ai
TURN_USERNAME=helix
TURN_PASSWORD=${TURN_PASSWORD}

# Moonlight Web pairing (internal, secure random)
MOONLIGHT_INTERNAL_PAIRING_PIN=${MOONLIGHT_PIN}
EOF

        # Generate moonlight-web config from template
        echo "Generating Moonlight Web configuration..."
        mkdir -p "$INSTALL_DIR/moonlight-web-config"

        cat << EOF > "$INSTALL_DIR/moonlight-web-config/config.json"
{
  "bind_address": "0.0.0.0:8080",
  "credentials": "${MOONLIGHT_CREDENTIALS}",
  "webrtc_ice_servers": [
    {
      "urls": [
        "stun:l.google.com:19302",
        "stun:stun.l.google.com:19302",
        "stun:stun1.l.google.com:19302",
        "stun:stun2.l.google.com:19302",
        "stun:stun3.l.google.com:19302",
        "stun:stun4.l.google.com:19302"
      ]
    },
    {
      "urls": [
        "turn:${TURN_HOST}:3478?transport=udp"
      ],
      "username": "helix",
      "credential": "${TURN_PASSWORD}"
    }
  ],
  "webrtc_port_range": {
    "min": 40000,
    "max": 40010
  },
  "webrtc_network_types": [
    "udp4",
    "udp6"
  ]
}
EOF
        echo "Moonlight Web config created at $INSTALL_DIR/moonlight-web-config/config.json"

        # Create Wolf directory and configuration
        mkdir -p "$INSTALL_DIR/wolf"

        # Extract hostname for Wolf display name
        if [ -n "$API_HOST" ]; then
            WOLF_HOSTNAME=$(echo "$API_HOST" | sed -E 's|^https?://||' | sed 's|:[0-9]+$||')
        else
            WOLF_HOSTNAME="local"
        fi

        # Create Wolf config.toml (version 6 with GStreamer encoders and dynamic apps support)
        # Only create if it doesn't exist to preserve user modifications
        if [ ! -f "$INSTALL_DIR/wolf/config.toml" ]; then
            echo "Creating Wolf configuration..."
            WOLF_UUID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "00000000-0000-0000-0000-$(date +%s)")
            cat << WOLFCONFIG > "$INSTALL_DIR/wolf/config.toml"
apps = []
config_version = 6
hostname = 'Helix ($WOLF_HOSTNAME)'
paired_clients = []

[gstreamer.audio]
default_audio_params = 'queue max-size-buffers=3 leaky=downstream ! audiorate ! audioconvert'
default_opus_encoder = 'opusenc bitrate={bitrate} bitrate-type=cbr frame-size={packet_duration} bandwidth=fullband audio-type=restricted-lowdelay max-payload-size=1400'
default_sink = '''rtpmoonlightpay_audio name=moonlight_pay packet_duration={packet_duration} encrypt={encrypt} aes_key="{aes_key}" aes_iv="{aes_iv}" !
appsink name=wolf_udp_sink'''
default_source = 'interpipesrc name=interpipesrc_{}_audio listen-to={session_id}_audio is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=3 block=false'

[gstreamer.video]
default_sink = '''rtpmoonlightpay_video name=moonlight_pay payload_size={payload_size} fec_percentage={fec_percentage} min_required_fec_packets={min_required_fec_packets} !
appsink sync=false name=wolf_udp_sink
'''
default_source = 'interpipesrc name=interpipesrc_{}_video listen-to={session_id}_video is-live=true stream-sync=restart-ts max-bytes=0 max-buffers=1 leaky-type=downstream'

    [gstreamer.video.defaults.nvcodec]
    video_params = '''cudaupload !
cudaconvertscale add-borders=true !
video/x-raw(memory:CUDAMemory), width={width}, height={height}, chroma-site={color_range}, format=NV12, colorimetry={color_space}, pixel-aspect-ratio=1/1'''
    video_params_zero_copy = '''cudaupload !
cudaconvertscale add-borders=true !
video/x-raw(memory:CUDAMemory),format=NV12, width={width}, height={height}, pixel-aspect-ratio=1/1
'''

    [gstreamer.video.defaults.qsv]
    video_params = '''videoconvertscale !
video/x-raw, chroma-site={color_range}, width={width}, height={height}, format=NV12,
colorimetry={color_space}, pixel-aspect-ratio=1/1'''
    video_params_zero_copy = '''vapostproc add-borders=true !
video/x-raw(memory:VAMemory), format=NV12, width={width}, height={height}, pixel-aspect-ratio=1/1'''

    [gstreamer.video.defaults.va]
    video_params = '''vapostproc add-borders=true !
video/x-raw, chroma-site={color_range}, width={width}, height={height}, format=NV12,
colorimetry={color_space}, pixel-aspect-ratio=1/1'''
    video_params_zero_copy = '''vapostproc add-borders=true !
video/x-raw(memory:VAMemory), format=NV12, width={width}, height={height}, pixel-aspect-ratio=1/1'''

    [[gstreamer.video.av1_encoders]]
    check_elements = [ 'nvav1enc', 'cudaconvertscale', 'cudaupload' ]
    encoder_pipeline = '''nvav1enc gop-size=-1 bitrate={bitrate} rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !
av1parse !
video/x-av1, stream-format=obu-stream, alignment=frame, profile=main'''
    plugin_name = 'nvcodec'

    [[gstreamer.video.av1_encoders]]
    check_elements = [ 'qsvav1enc', 'vapostproc' ]
    encoder_pipeline = '''qsvav1enc gop-size=0 ref-frames=1 bitrate={bitrate} rate-control=cbr low-latency=1 target-usage=6 !
av1parse !
video/x-av1, stream-format=obu-stream, alignment=frame, profile=main'''
    plugin_name = 'qsv'

    [[gstreamer.video.av1_encoders]]
    check_elements = [ 'vaav1enc', 'vapostproc' ]
    encoder_pipeline = '''vaav1enc ref-frames=1 bitrate={bitrate} cpb-size={bitrate} key-int-max=1024 rate-control=cqp target-usage=6 !
av1parse !
video/x-av1, stream-format=obu-stream, alignment=frame, profile=main'''
    plugin_name = 'va'

    [[gstreamer.video.av1_encoders]]
    check_elements = [ 'vaav1lpenc', 'vapostproc' ]
    encoder_pipeline = '''vaav1lpenc ref-frames=1 bitrate={bitrate} cpb-size={bitrate} key-int-max=1024 rate-control=cqp target-usage=6 !
av1parse !
video/x-av1, stream-format=obu-stream, alignment=frame, profile=main'''
    plugin_name = 'va'

    [[gstreamer.video.av1_encoders]]
    check_elements = [ 'av1enc' ]
    encoder_pipeline = '''av1enc usage-profile=realtime end-usage=vbr target-bitrate={bitrate} !
av1parse !
video/x-av1, stream-format=obu-stream, alignment=frame, profile=main'''
    plugin_name = 'aom'
    video_params = '''videoconvertscale !
videorate !
video/x-raw, width={width}, height={height}, framerate={fps}/1, format=I420,
chroma-site={color_range}, colorimetry={color_space}'''
    video_params_zero_copy = '''videoconvertscale !
videorate !
video/x-raw, width={width}, height={height}, framerate={fps}/1, format=I420,
chroma-site={color_range}, colorimetry={color_space}'''

    [[gstreamer.video.h264_encoders]]
    check_elements = [ 'nvh264enc', 'cudaconvertscale', 'cudaupload' ]
    encoder_pipeline = '''nvh264enc preset=low-latency-hq zerolatency=true gop-size=0 rc-mode=cbr-ld-hq bitrate={bitrate} aud=false !
h264parse !
video/x-h264, profile=main, stream-format=byte-stream'''
    plugin_name = 'nvcodec'

    [[gstreamer.video.h264_encoders]]
    check_elements = [ 'qsvh264enc', 'vapostproc' ]
    encoder_pipeline = '''qsvh264enc b-frames=0 gop-size=0 idr-interval=1 ref-frames=1 bitrate={bitrate} rate-control=cbr target-usage=6  !
h264parse !
video/x-h264, profile=main, stream-format=byte-stream'''
    plugin_name = 'qsv'

    [[gstreamer.video.h264_encoders]]
    check_elements = [ 'vah264enc', 'vapostproc' ]
    encoder_pipeline = '''vah264enc aud=false b-frames=0 ref-frames=1 num-slices={slices_per_frame} bitrate={bitrate} cpb-size={bitrate} key-int-max=1024 rate-control=cqp target-usage=6 !
h264parse !
video/x-h264, profile=main, stream-format=byte-stream'''
    plugin_name = 'va'

    [[gstreamer.video.h264_encoders]]
    check_elements = [ 'vah264lpenc', 'vapostproc' ]
    encoder_pipeline = '''vah264lpenc aud=false b-frames=0 ref-frames=1 num-slices={slices_per_frame} bitrate={bitrate} cpb-size={bitrate} key-int-max=1024 rate-control=cqp target-usage=6 !
h264parse !
video/x-h264, profile=main, stream-format=byte-stream'''
    plugin_name = 'va'

    [[gstreamer.video.h264_encoders]]
    check_elements = [ 'x264enc' ]
    encoder_pipeline = '''x264enc pass=qual tune=zerolatency speed-preset=superfast b-adapt=false bframes=0 ref=1
sliced-threads=true threads={slices_per_frame} option-string="slices={slices_per_frame}:keyint=infinite:open-gop=0"
b-adapt=false bitrate={bitrate} aud=false !
video/x-h264, profile=high, stream-format=byte-stream'''
    plugin_name = 'x264'

    [[gstreamer.video.hevc_encoders]]
    check_elements = [ 'nvh265enc', 'cudaconvertscale', 'cudaupload' ]
    encoder_pipeline = '''nvh265enc gop-size=-1 bitrate={bitrate} aud=false rc-mode=cbr zerolatency=true preset=p1 tune=ultra-low-latency multi-pass=two-pass-quarter !
h265parse !
video/x-h265, profile=main, stream-format=byte-stream'''
    plugin_name = 'nvcodec'

    [[gstreamer.video.hevc_encoders]]
    check_elements = [ 'qsvh265enc', 'vapostproc' ]
    encoder_pipeline = '''qsvh265enc b-frames=0 gop-size=0 idr-interval=1 ref-frames=1 bitrate={bitrate} rate-control=cbr low-latency=1 target-usage=6 !
h265parse !
video/x-h265, profile=main, stream-format=byte-stream'''
    plugin_name = 'qsv'

    [[gstreamer.video.hevc_encoders]]
    check_elements = [ 'vah265enc', 'vapostproc' ]
    encoder_pipeline = '''vah265enc aud=false b-frames=0 ref-frames=1 num-slices={slices_per_frame} bitrate={bitrate} cpb-size={bitrate} key-int-max=1024 rate-control=cqp target-usage=6 !
h265parse !
video/x-h265, profile=main, stream-format=byte-stream'''
    plugin_name = 'va'

    [[gstreamer.video.hevc_encoders]]
    check_elements = [ 'vah265lpenc', 'vapostproc' ]
    encoder_pipeline = '''vah265lpenc aud=false b-frames=0 ref-frames=1 num-slices={slices_per_frame} bitrate={bitrate} cpb-size={bitrate} key-int-max=1024 rate-control=cqp target-usage=6 !
h265parse !
video/x-h265, profile=main, stream-format=byte-stream'''
    plugin_name = 'va'

    [[gstreamer.video.hevc_encoders]]
    check_elements = [ 'x265enc' ]
    encoder_pipeline = '''x265enc tune=zerolatency speed-preset=superfast bitrate={bitrate}
option-string="info=0:keyint=-1:qp=28:repeat-headers=1:slices={slices_per_frame}:aud=0:annexb=1:log-level=3:open-gop=0:bframes=0:intra-refresh=0" !
video/x-h265, profile=main, stream-format=byte-stream'''
    plugin_name = 'x265'
    video_params = '''videoconvertscale !
videorate !
video/x-raw, width={width}, height={height}, framerate={fps}/1, format=I420,
chroma-site={color_range}, colorimetry={color_space}'''
    video_params_zero_copy = '''videoconvertscale !
videorate !
video/x-raw, width={width}, height={height}, framerate={fps}/1, format=I420,
chroma-site={color_range}, colorimetry={color_space}'''
WOLFCONFIG
            echo "uuid = '$WOLF_UUID'" >> "$INSTALL_DIR/wolf/config.toml"
            echo "Wolf config created at $INSTALL_DIR/wolf/config.toml"
        else
            echo "Wolf config already exists at $INSTALL_DIR/wolf/config.toml (preserving existing)"
        fi

        # Generate self-signed certificates for Wolf HTTPS only if they don't exist
        # IMPORTANT: Must use RSA 2048-bit for Moonlight protocol compatibility
        # CRITICAL: Use CN=localhost for Docker network compatibility
        if [ ! -f "$INSTALL_DIR/wolf/cert.pem" ] || [ ! -f "$INSTALL_DIR/wolf/key.pem" ]; then
            echo "Generating Wolf SSL certificates..."
            openssl req -x509 -newkey rsa:2048 -keyout "$INSTALL_DIR/wolf/key.pem" -out "$INSTALL_DIR/wolf/cert.pem" \
                -days 365 -nodes -subj "/C=IT/O=GamesOnWhales/CN=localhost" 2>/dev/null
            echo "Wolf SSL certificates created at $INSTALL_DIR/wolf/"
        else
            echo "Wolf SSL certificates already exist at $INSTALL_DIR/wolf/ (preserving existing)"
        fi

        # Extract certificate in JSON-escaped format for moonlight-web data.json
        WOLF_CERT_ESCAPED=$(awk 'NF {sub(/\r/, ""); printf "%s\\r\\n", $0}' "$INSTALL_DIR/wolf/cert.pem")

        # Update moonlight-web data.json with the new Wolf server certificate
        # This is CRITICAL - moonlight-web must trust Wolf's certificate for HTTPS connections
        if [ -f "$INSTALL_DIR/moonlight-web-config/data.json" ]; then
            echo "Updating moonlight-web data.json with Wolf server certificate..."
            # Use jq if available, otherwise use sed
            if command -v jq &> /dev/null; then
                jq --arg cert "$WOLF_CERT_ESCAPED" \
                    '.hosts[0].paired.server_certificate = $cert' \
                    "$INSTALL_DIR/moonlight-web-config/data.json" > "$INSTALL_DIR/moonlight-web-config/data.json.tmp"
                mv "$INSTALL_DIR/moonlight-web-config/data.json.tmp" "$INSTALL_DIR/moonlight-web-config/data.json"
            else
                echo "Warning: jq not found, Wolf certificate not automatically updated in moonlight-web config"
                echo "You may need to manually update moonlight-web-config/data.json"
            fi
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

                # Check if the keyring file already exists
                if [ ! -f /usr/share/keyrings/caddy-stable-archive-keyring.gpg ]; then
                    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
                fi

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
                sudo bash -c "cat << EOF > \"$CADDYFILE\"
$CADDY_HOST {
    reverse_proxy localhost:8080
}
EOF"
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
        echo "│   - UDP 40000-40010: WebRTC media ports"
    fi
    echo "│"
    echo "│ Start the Helix services by running:"
    echo "│"
    echo "│ cd $INSTALL_DIR"
    if [ "$NEED_SUDO" = "true" ]; then
        echo "│ sudo docker compose up -d --remove-orphans"
    else
        echo "│ docker compose up -d --remove-orphans"
    fi
    if [ "$CADDY" = true ]; then
        echo "│ sudo systemctl restart caddy"
    fi
    echo "│"
    echo "│ to start/upgrade Helix.  Helix will be available at $API_HOST"
    echo "│ This will take a minute or so to boot."
    echo "└───────────────────────────────────────────────────────────────────────────"
fi

# Install runner if requested or in AUTO mode with GPU
if [ "$RUNNER" = true ]; then
    install_nvidia_docker

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

# HF_TOKEN is now managed by the control plane and distributed to runners automatically
# No longer setting HF_TOKEN on runners to avoid confusion
HF_TOKEN_PARAM=""

# Check if api-1 container is running
if docker ps --format '{{.Image}}' | grep 'registry.helixml.tech/helix/controlplane'; then
    API_HOST="http://api:8080"
    echo "Detected controlplane container running. Setting API_HOST to \${API_HOST}"
fi

# Check if helix_default network exists, create it if it doesn't
if ! docker network inspect helix_default >/dev/null 2>&1; then
    echo "Creating helix_default network..."
    docker network create helix_default
else
    echo "helix_default network already exists."
fi

# Run the docker container
docker run --privileged --gpus all --shm-size=10g \\
    --restart=always -d \\
    --name helix-runner --ipc=host --ulimit memlock=-1 \\
    --ulimit stack=67108864 \\
    --network="helix_default" \\
    registry.helixml.tech/helix/runner:\${RUNNER_TAG} \\
    --api-host \${API_HOST} --api-token \${RUNNER_TOKEN} \\
    --runner-id \$(hostname)
EOF

    if [ "$ENVIRONMENT" = "gitbash" ]; then
        chmod +x $INSTALL_DIR/runner.sh
    else
        sudo chmod +x $INSTALL_DIR/runner.sh
    fi
    echo "Runner script has been created at $INSTALL_DIR/runner.sh"
    echo "┌───────────────────────────────────────────────────────────────────────────"
    echo "│ To start the runner, run:"
    echo "│"
    if [ "$NEED_SUDO" = "true" ]; then
        echo "│   sudo $INSTALL_DIR/runner.sh"
    else
        echo "│   $INSTALL_DIR/runner.sh"
    fi
    echo "│"
    echo "└───────────────────────────────────────────────────────────────────────────"
fi

# Install external Zed agent if requested
if [ "$EXTERNAL_ZED_AGENT" = true ]; then
    # Set default runner ID if not provided
    if [ -z "$EXTERNAL_ZED_RUNNER_ID" ]; then
        EXTERNAL_ZED_RUNNER_ID="external-zed-$(hostname)"
    fi

    echo -e "\nInstalling External Zed Agent..."

    # Download the external Zed agent scripts
    echo "Downloading external Zed agent scripts..."
    if [ "$ENVIRONMENT" = "gitbash" ]; then
        curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/run-external-zed-agent.sh" -o "$INSTALL_DIR/run-external-zed-agent.sh"
        curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/external-zed-agent.env.example" -o "$INSTALL_DIR/external-zed-agent.env.example"
        curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/Dockerfile.zed-agent" -o "$INSTALL_DIR/Dockerfile.zed-agent"
        chmod +x "$INSTALL_DIR/run-external-zed-agent.sh"
    else
        sudo curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/run-external-zed-agent.sh" -o "$INSTALL_DIR/run-external-zed-agent.sh"
        sudo curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/external-zed-agent.env.example" -o "$INSTALL_DIR/external-zed-agent.env.example"
        sudo curl -L "${PROXY}/helixml/helix/releases/download/${LATEST_RELEASE}/Dockerfile.zed-agent" -o "$INSTALL_DIR/Dockerfile.zed-agent"
        sudo chmod +x "$INSTALL_DIR/run-external-zed-agent.sh"
        # Change ownership to current user
        sudo chown $(id -un):$(id -gn) "$INSTALL_DIR/run-external-zed-agent.sh"
        sudo chown $(id -un):$(id -gn) "$INSTALL_DIR/external-zed-agent.env.example"
        sudo chown $(id -un):$(id -gn) "$INSTALL_DIR/Dockerfile.zed-agent"
    fi

    # Create environment file with user settings
    ENV_FILE="$INSTALL_DIR/external-zed-agent.env"
    cat << EOF > "$ENV_FILE"
# External Zed Agent Configuration
API_HOST=$API_HOST
API_TOKEN=$RUNNER_TOKEN
RUNNER_ID=$EXTERNAL_ZED_RUNNER_ID
CONCURRENCY=$EXTERNAL_ZED_CONCURRENCY
MAX_TASKS=0
SESSION_TIMEOUT=3600
WORKSPACE_DIR=/tmp/zed-workspaces
DISPLAY_NUM=1
LOG_LEVEL=info
DOCKER_IMAGE=helix-zed-agent:latest
CONTAINER_NAME=helix-external-zed-agent
EOF

    echo "External Zed Agent has been installed to $INSTALL_DIR"
    echo "┌───────────────────────────────────────────────────────────────────────────"
    echo "│ To complete the External Zed Agent setup:"
    echo "│"
    echo "│ 1. Build the Zed agent Docker image:"
    echo "│    cd $INSTALL_DIR"
    echo "│    # First, build Zed with external sync support (requires Helix source)"
    echo "│    # ./stack build-zed"
    echo "│    # docker build -f Dockerfile.zed-agent -t helix-zed-agent:latest ."
    echo "│"
    echo "│ 2. Start the external Zed agent:"
    echo "│    source $INSTALL_DIR/external-zed-agent.env"
    echo "│    $INSTALL_DIR/run-external-zed-agent.sh"
    echo "│"
    echo "│ The agent will connect to: $API_HOST"
    echo "│ Runner ID: $EXTERNAL_ZED_RUNNER_ID"
    echo "│ Concurrency: $EXTERNAL_ZED_CONCURRENCY"
    echo "└───────────────────────────────────────────────────────────────────────────"
fi

if [ -n "$API_HOST" ] && [ "$CONTROLPLANE" = true ]; then
    echo
    echo "To connect an external runner to this controlplane, run on a node with a GPU:"
    echo
    echo "curl -Ls -O https://get.helixml.tech/install.sh"
    echo "chmod +x install.sh"
    echo "./install.sh --runner --api-host $API_HOST --runner-token $RUNNER_TOKEN"
    echo
    echo "To connect an external Zed agent to this controlplane:"
    echo "./install.sh --external-zed-agent --api-host $API_HOST --runner-token $RUNNER_TOKEN"
fi

echo -e "\nInstallation complete."
