#!/bin/bash
# Build a WSL2-compatible rootfs tarball for Helix Desktop (Windows)
#
# This creates an Ubuntu 24.04 rootfs with:
# - Docker CE pre-installed
# - systemd enabled (for Docker service management)
# - User 'ubuntu' configured as default WSL user
# - Helix compose files pre-deployed
#
# Requirements: Docker (to run the build), ~2GB disk space
#
# Usage: ./build-wsl-rootfs.sh [output-dir]
# Output: helix-wsl-rootfs.tar.gz in the output directory

set -euo pipefail

OUTPUT_DIR="${1:-$(pwd)}"
CONTAINER_NAME="helix-wsl-builder-$$"
IMAGE_TAG="helix-wsl-rootfs:build"

echo "Building Helix WSL2 rootfs..."

# Create a Dockerfile for the rootfs
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

cat > "$TMPDIR/Dockerfile" <<'DOCKERFILE'
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# Install base packages
RUN apt-get update && apt-get install -y \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    sudo \
    systemd \
    systemd-sysv \
    dbus \
    iproute2 \
    iputils-ping \
    openssh-server \
    wget \
    git \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Install Docker CE
RUN install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
    $(. /etc/os-release && echo "$VERSION_CODENAME") stable" > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/*

# Create ubuntu user with sudo access
RUN useradd -m -s /bin/bash -G sudo,docker ubuntu \
    && echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers.d/ubuntu

# Configure WSL
RUN cat > /etc/wsl.conf <<'WSLCONF'
[boot]
systemd=true

[user]
default=ubuntu

[network]
generateResolvConf=true
WSLCONF

# Enable Docker to start on boot via systemd
RUN systemctl enable docker

# Create helix directory structure
RUN mkdir -p /home/ubuntu/helix && chown ubuntu:ubuntu /home/ubuntu/helix

# Set up SSH
RUN mkdir -p /home/ubuntu/.ssh && \
    chmod 700 /home/ubuntu/.ssh && \
    chown ubuntu:ubuntu /home/ubuntu/.ssh

# Clean up
RUN apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
DOCKERFILE

# Build the image
echo "Building Docker image..."
docker build -t "$IMAGE_TAG" "$TMPDIR"

# Export as rootfs tarball
echo "Exporting rootfs..."
docker create --name "$CONTAINER_NAME" "$IMAGE_TAG" /bin/true
docker export "$CONTAINER_NAME" | gzip > "$OUTPUT_DIR/rootfs.tar.gz"
docker rm "$CONTAINER_NAME"
docker rmi "$IMAGE_TAG" 2>/dev/null || true

SIZE=$(du -sh "$OUTPUT_DIR/rootfs.tar.gz" | awk '{print $1}')
SHA256=$(sha256sum "$OUTPUT_DIR/rootfs.tar.gz" | awk '{print $1}')

echo ""
echo "=== WSL2 Rootfs Build Complete ==="
echo "File: $OUTPUT_DIR/rootfs.tar.gz"
echo "Size: $SIZE"
echo "SHA256: $SHA256"
echo ""
echo "To import manually:"
echo "  wsl --import Helix C:\\Helix\\WSL $OUTPUT_DIR/rootfs.tar.gz --version 2"
echo ""
echo "Update vm-manifest-windows.json with:"
echo "  {\"name\": \"rootfs.tar.gz\", \"size\": $(stat -c%s "$OUTPUT_DIR/rootfs.tar.gz" 2>/dev/null || stat -f%z "$OUTPUT_DIR/rootfs.tar.gz"), \"sha256\": \"$SHA256\"}"
