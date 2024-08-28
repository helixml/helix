#!/bin/bash

# Install:
# curl -O install-helix.sh https://get.helix.ml
# chmod +x install-helix.sh
# ./install-helix.sh --help
#
# Examples:
#
# 1. Install the CLI, the controlplane and a runner if a GPU is available (auto mode):
#    ./install-helix.sh
#
# 2. Install just the CLI:
#    ./install-helix.sh --cli
#
# 3. Install CLI and controlplane with external TogetherAI token:
#    ./install-helix.sh --cli --controlplane --togetherai-token YOUR_TOGETHERAI_TOKEN
#
# 4. Install CLI and controlplane (to install runner separately):
#    ./install-helix.sh --cli --controlplane
#
# 5. Install CLI, controlplane, and runner on a node with a GPU:
#    ./install-helix.sh --cli --controlplane --runner
#
# 6. Install just the runner, pointing to a controlplane with a DNS name:
#    ./install-helix.sh --runner --api-host your-controlplane-domain.com --runner-token YOUR_RUNNER_TOKEN

set -euo pipefail

# Default values
AUTO=true
CLI=false
CONTROLPLANE=false
RUNNER=false
LARGE=false
API_HOST=""
RUNNER_TOKEN=""
TOGETHERAI_TOKEN=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
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
        --large)
            LARGE=true
            shift
            ;;
        --api-host)
            API_HOST=$2
            shift 2
            ;;
        --runner-token)
            RUNNER_TOKEN="$2"
            shift 2
            ;;
        --togetherai-token)
            TOGETHERAI_TOKEN="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Adjust default values based on provided arguments and AUTO mode
if [ "$AUTO" = true ]; then
    CLI=true
    CONTROLPLANE=true
    if command -v nvidia-smi &> /dev/null; then
        RUNNER=true
    fi
    echo "Auto-install mode detected. Installing CLI and Control Plane."
    if command -v nvidia-smi &> /dev/null; then
        echo "NVIDIA GPU detected. Runner will be installed."
    else
        echo "No NVIDIA GPU detected. Runner will not be installed. See command "
        echo "at the end to install runner separately on a GPU node."
    fi
fi

if [ "$RUNNER" = true ] && [ "$CONTROLPLANE" = false ] && [ -z "$API_HOST" ]; then
    echo "Error: When installing only the runner, you must specify --api-host"
    exit 1
fi

# Determine OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Determine latest release
LATEST_RELEASE=$(curl -s https://api.github.com/repos/helixml/helix/releases/latest | grep -oP '"tag_name": "\K(.*)(?=")')

# Set binary name
BINARY_NAME="helix-${OS}-${ARCH}"

# Create installation directory
sudo mkdir -p /opt/HelixML/data/helix-{postgres,filestore}

# Install CLI if requested or in AUTO mode
if [ "$CLI" = true ]; then
    echo "Downloading Helix CLI..."
    sudo curl -L "https://github.com/helixml/helix/releases/download/${LATEST_RELEASE}/${BINARY_NAME}" -o /usr/local/bin/helix
    sudo chmod +x /usr/local/bin/helix
    echo "Helix CLI has been installed to /usr/local/bin/helix"
fi

# Function to generate random alphanumeric password
generate_password() {
    openssl rand -base64 12 | tr -dc 'a-zA-Z0-9' | head -c 16
}

# Function to install Docker and Docker Compose plugin
install_docker() {
    if ! command -v docker &> /dev/null; then
        echo "Docker not found. Installing Docker..."
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
                    ;;
                fedora)
                    sudo dnf -y install dnf-plugins-core
                    sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
                    sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
                    sudo systemctl start docker
                    sudo systemctl enable docker
                    ;;
                *)
                    echo "Unsupported distribution for automatic Docker installation. Please install Docker manually."
                    exit 1
                    ;;
            esac
        else
            echo "Unable to determine OS distribution. Please install Docker manually."
            exit 1
        fi
    fi

    if ! docker compose version &> /dev/null; then
        echo "Docker Compose plugin not found. Installing Docker Compose plugin..."
        sudo apt-get update
        sudo apt-get install -y docker-compose-plugin
    fi
}

# Function to install NVIDIA Docker runtime
install_nvidia_docker() {
    if ! command -v nvidia-smi &> /dev/null; then
        echo "NVIDIA GPU not detected. Skipping NVIDIA Docker runtime installation."
        return
    fi

    if ! docker info | grep -i nvidia &> /dev/null; then
        echo "NVIDIA Docker runtime not found. Installing NVIDIA Docker runtime..."
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                ubuntu|debian)
                    distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
                    curl -s -L https://nvidia.github.io/nvidia-docker/gpgkey | sudo apt-key add -
                    curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.list | sudo tee /etc/apt/sources.list.d/nvidia-docker.list
                    sudo apt-get update
                    sudo apt-get install -y nvidia-docker2
                    sudo systemctl restart docker
                    ;;
                fedora)
                    distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
                    curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.repo | sudo tee /etc/yum.repos.d/nvidia-docker.repo
                    sudo dnf install -y nvidia-docker2
                    sudo systemctl restart docker
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

# Install controlplane if requested or in AUTO mode
if [ "$CONTROLPLANE" = true ]; then
    install_docker
    echo "Downloading docker-compose.yaml..."
    sudo curl -L "https://github.com/helixml/helix/releases/download/${LATEST_RELEASE}/docker-compose.yaml" -o /opt/HelixML/docker-compose.yaml
    echo "docker-compose.yaml has been downloaded to /opt/HelixML/docker-compose.yaml"

    # Create .env file
    ENV_FILE="/opt/HelixML/.env"
    echo "Creating .env file..."
    
    # Set domain
    if [ -z "$API_HOST" ]; then
        DOMAIN="http://localhost:8080"
    else
        DOMAIN="https://${API_HOST}"
    fi

    # Generate .env content
    cat << EOF > "$ENV_FILE"
# Set passwords
KEYCLOAK_ADMIN_PASSWORD=$(generate_password)
POSTGRES_ADMIN_PASSWORD=$(generate_password)
RUNNER_TOKEN=${RUNNER_TOKEN:-$(generate_password)}

# URLs
KEYCLOAK_FRONTEND_URL=${DOMAIN}/auth/
SERVER_URL=${DOMAIN}

# Storage
POSTGRES_DATA=/opt/HelixML/data/helix-postgres
FILESTORE_DATA=/opt/HelixML/data/helix-filestore

# Optional integrations:

## External LLM provider
EOF

    # Add TogetherAI configuration if token is provided
    if [ -n "$TOGETHERAI_TOKEN" ]; then
        cat << EOF >> "$ENV_FILE"
INFERENCE_PROVIDER=togetherai
TOGETHER_API_KEY=$TOGETHERAI_TOKEN
EOF
    else
        cat << EOF >> "$ENV_FILE"
#INFERENCE_PROVIDER=togetherai
#TOGETHER_API_KEY=xxx
EOF
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

    echo ".env file has been created at $ENV_FILE"
    echo "You can now cd /opt/HelixML and run 'docker compose up -d' to start Helix"
fi

# Install runner if requested or in AUTO mode with GPU
if [ "$RUNNER" = true ]; then
    install_docker
    install_nvidia_docker
    # Check for NVIDIA GPU
    if ! command -v nvidia-smi &> /dev/null; then
        echo "NVIDIA GPU not detected. Skipping runner installation."
        echo "Set up a runner separately, per https://docs.helix.ml/helix/private-deployment/controlplane/#attaching-a-runner"
        exit 1
    fi

    # Determine GPU memory
    GPU_MEMORY=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | awk '{print int($1/1024)}')

    # Determine runner tag and warmup models
    if [ "$LARGE" = true ]; then
        RUNNER_TAG="${LATEST_RELEASE}-large"
        WARMUP_MODELS=""
    else
        RUNNER_TAG="${LATEST_RELEASE}-small"
        WARMUP_MODELS="llama3:instruct,phi3:instruct"
    fi

    # Determine API host if not set
    if [ -z "$API_HOST" ]; then
        if [ "$CONTROLPLANE" = true ]; then
            API_HOST="http://localhost:8080"
        elif grep -qi microsoft /proc/version; then
            # Running in WSL2
            API_HOST="http://host.docker.internal:8080"
        else
            case "$(uname -s)" in
                Linux*)     API_HOST="http://172.17.0.1:8080" ;;
                Darwin*)    API_HOST="http://host.docker.internal:8080" ;;
                *)          echo "Unsupported operating system. Please specify --api-host manually."; exit 1 ;;
            esac
        fi
    fi

    # Determine runner token
    if [ -z "$RUNNER_TOKEN" ]; then
        if [ -f "/opt/HelixML/.env" ]; then
            RUNNER_TOKEN=$(grep RUNNER_TOKEN /opt/HelixML/.env | cut -d '=' -f2)
        else
            echo "Error: RUNNER_TOKEN not found in .env file and --runner-token not provided."
            echo "Please provide the runner token using the --runner-token argument."
            exit 1
        fi
    fi

    # Create runner.sh
    cat << EOF > /opt/HelixML/runner.sh
#!/bin/bash

# Configuration variables
RUNNER_TAG="${RUNNER_TAG}"
API_HOST="${API_HOST}"
GPU_MEMORY="${GPU_MEMORY}"
WARMUP_MODELS="${WARMUP_MODELS}"
RUNNER_TOKEN="${RUNNER_TOKEN}"

# Set warmup models parameter
if [ -n "\$WARMUP_MODELS" ]; then
    WARMUP_MODELS_PARAM="-e RUNTIME_OLLAMA_WARMUP_MODELS=\$WARMUP_MODELS"
else
    WARMUP_MODELS_PARAM=""
fi

# Run the docker container
sudo docker run --privileged --gpus all --shm-size=10g \\
    --restart=always -d \\
    --name helix-runner --ipc=host --ulimit memlock=-1 \\
    --ulimit stack=67108864 \\
    -v \${HOME}/.cache/huggingface:/root/.cache/huggingface \\
    \${WARMUP_MODELS_PARAM} \\
    registry.helix.ml/helix/runner:\${RUNNER_TAG} \\
    --api-host \${API_HOST} --api-token \${RUNNER_TOKEN} \\
    --runner-id \$(hostname) \\
    --memory \${GPU_MEMORY}GB \\
    --allow-multiple-copies
EOF

    sudo chmod +x /opt/HelixML/runner.sh
    echo "Runner script has been created at /opt/HelixML/runner.sh"
    echo "To start the runner, run: sudo /opt/HelixML/runner.sh"
fi

echo "Installation complete."