#!/bin/bash
# Startup script for Zed editor connected to Helix controlplane (Sway version)
set -e

# Check if Zed binary exists
if [ ! -f "/usr/local/bin/zed" ]; then
    echo "Zed binary not found at /usr/local/bin/zed - cannot start Zed agent"
    exit 1
fi

# Environment variables are passed from Wolf executor via container env
# HELIX_API_URL, HELIX_API_TOKEN, ANTHROPIC_API_KEY should be available

# Set workspace to mounted work directory
cd /home/retro/work || cd /home/user/work || cd /tmp

# Launch Zed with the current directory as workspace
# In Sway, we don't need --log debug unless debugging
exec /usr/local/bin/zed .