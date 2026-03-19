#!/bin/bash
# helix-claude-auth-wrapper.sh — Installs Claude CLI and runs claude auth login.
# Captures stdout so the poll handler can parse the platform OAuth URL.
# Sets BROWSER to the capture script so the localhost OAuth URL is also written
# to a file (used by the API proxy to rewrite the redirect_uri).
export NO_COLOR=1
export BROWSER=/usr/local/bin/helix-capture-browser

# Install Claude CLI to user prefix (no root required).
mkdir -p ~/.local
for i in 1 2 3 4 5; do
    npm install -g --prefix ~/.local @anthropic-ai/claude-code@latest 2>>/tmp/npm-install.log && break
    [ "$i" -lt 5 ] && sleep 3
done
export PATH="$HOME/.local/bin:$PATH"

# Verify claude is installed
if ! command -v claude &>/dev/null; then
    echo "ERROR: claude CLI not installed after 5 attempts" > /tmp/claude-auth-stdout.txt
    exit 1
fi

# Use `script` to create a pseudo-TTY so Node.js (claude) uses line buffering.
# claude auth login starts a local HTTP server for the OAuth callback.
# The API rewrites the authorize URL to proxy through its own callback endpoint,
# which then forwards the code to claude's local server via helix-claude-auth-submit.
script -qefc "claude auth login" /tmp/claude-auth-stdout.txt
