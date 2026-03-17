#!/bin/bash
# helix-claude-auth-wrapper.sh — Installs Claude CLI and runs claude auth login.
# Includes npm install with retry to handle container network not being ready
# immediately after boot. Captures stdout so the poll handler can parse the
# platform OAuth URL. Sets NO_COLOR=1 to prevent ANSI escape codes.
# Sets BROWSER to the capture script so the OAuth URL is also written to a file.
export NO_COLOR=1
export BROWSER=/usr/local/bin/helix-capture-browser

# Install Claude CLI with retry (network may not be ready immediately after boot)
for i in 1 2 3 4 5; do
    npm install -g @anthropic-ai/claude-code@latest 2>/dev/null && break
    echo "npm install attempt $i failed, retrying in 3s..." >&2
    sleep 3
done

# Verify claude is installed
if ! command -v claude &>/dev/null; then
    echo "ERROR: claude CLI not installed after 5 attempts" > /tmp/claude-auth-stdout.txt
    exit 1
fi

claude auth login > /tmp/claude-auth-stdout.txt 2>&1 &
