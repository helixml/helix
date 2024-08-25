#!/bin/bash

# To install, run:
# curl -fsSL https://raw.githubusercontent.com/helixml/helix/main/install.sh | sudo bash

set -e

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

# Create installation directory for docker-compose.yaml
sudo mkdir -p /opt/HelixML

# Download binary
echo "Downloading Helix binary..."
sudo curl -L "https://github.com/helixml/helix/releases/download/${LATEST_RELEASE}/${BINARY_NAME}" -o /usr/local/bin/helix
sudo chmod +x /usr/local/bin/helix

# Download docker-compose.yaml
echo "Downloading docker-compose.yaml..."
sudo curl -L "https://github.com/helixml/helix/releases/download/${LATEST_RELEASE}/docker-compose.yaml" -o /opt/HelixML/docker-compose.yaml

echo "Helix CLI has been installed to /usr/local/bin/helix"
echo "docker-compose.yaml has been downloaded to /opt/HelixML/docker-compose.yaml"
echo "You can now cd /opt/HelixML and run 'docker compose up -d' to start Helix"
