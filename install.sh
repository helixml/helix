#!/bin/bash

# To install, run:
# curl -fsSL https://raw.githubusercontent.com/helixml/helix/main/install.sh | sudo bash

set -e

# Default values
CLI=true
CONTROLPLANE=true
RUNNER=false
LARGE=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --cli)
            CLI=$2
            shift 2
            ;;
        --controlplane)
            CONTROLPLANE=$2
            shift 2
            ;;
        --runner)
            RUNNER=$2
            shift 2
            ;;
        --large)
            LARGE=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

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
sudo mkdir -p /opt/HelixML

# Install CLI if requested
if [ "$CLI" = true ]; then
    echo "Downloading Helix CLI..."
    sudo curl -L "https://github.com/helixml/helix/releases/download/${LATEST_RELEASE}/${BINARY_NAME}" -o /usr/local/bin/helix
    sudo chmod +x /usr/local/bin/helix
    echo "Helix CLI has been installed to /usr/local/bin/helix"
fi

# Install controlplane if requested
if [ "$CONTROLPLANE" = true ]; then
    echo "Downloading docker-compose.yaml..."
    sudo curl -L "https://github.com/helixml/helix/releases/download/${LATEST_RELEASE}/docker-compose.yaml" -o /opt/HelixML/docker-compose.yaml
    echo "docker-compose.yaml has been downloaded to /opt/HelixML/docker-compose.yaml"
    echo "You can now cd /opt/HelixML and run 'docker compose up -d' to start Helix"
fi

# Install runner if requested
if [ "$RUNNER" = true ]; then
    # Check for NVIDIA GPU
    if ! command -v nvidia-smi &> /dev/null; then
        echo "NVIDIA GPU not detected. Skipping runner installation."
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
        WARMUP_MODELS="-e RUNTIME_OLLAMA_WARMUP_MODELS=llama3:instruct,phi3:instruct"
    fi

    # Create runner.sh
    cat << EOF > /opt/HelixML/runner.sh
#!/bin/bash
sudo docker run --privileged --gpus all --shm-size=10g \\
    --restart=always -d \\
    --name helix-runner --ipc=host --ulimit memlock=-1 \\
    --ulimit stack=67108864 \\
    -v \${HOME}/.cache/huggingface:/root/.cache/huggingface \\
    ${WARMUP_MODELS} \\
    registry.helix.ml/helix/runner:${RUNNER_TAG} \\
    --api-host <http(s)://YOUR_CONTROLPLANE_HOSTNAME> --api-token <RUNNER_TOKEN_FROM_ENV> \\
    --runner-id \$(hostname) \\
    --memory ${GPU_MEMORY}GB \\
    --allow-multiple-copies
EOF

    sudo chmod +x /opt/HelixML/runner.sh
    echo "Runner script has been created at /opt/HelixML/runner.sh"
    echo "Please edit the script to set your control plane hostname and API token before running."
fi

echo "Installation complete."