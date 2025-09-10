#!/bin/bash
set -e

# Main startup script - runs as root, then switches to Ubuntu user

echo "Setting up environment..."

# Ensure proper ownership of config directories
mkdir -p /home/ubuntu/.config
chown -R ubuntu:ubuntu /home/ubuntu/.config

# Switch to Ubuntu user and run desktop environment
echo "Switching to Ubuntu user and starting desktop..."
exec su ubuntu -c "/ubuntu-desktop.sh"