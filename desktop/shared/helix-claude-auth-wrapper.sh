#!/bin/bash
# helix-claude-auth-wrapper.sh — Installs Claude CLI and runs claude auth login.
# The browser opens INSIDE the container's desktop — the user interacts with
# the auth flow via the desktop stream viewer. localhost:PORT/callback is
# reachable within the container, so the OAuth callback works natively.

# Set display environment so the browser can open in the desktop session.
export DISPLAY=:0

# Use our custom BROWSER script to fix Claude Code's malformed redirect_uri.
# Claude Code has a bug where it constructs http:/localhost (single slash) instead
# of http://localhost (double slash), causing Anthropic's OAuth to reject it.
# See: https://github.com/anthropics/claude-code/issues/36015
export BROWSER=/usr/local/bin/helix-fix-oauth-url
# Find the DBUS session address from the gnome-shell process.
GNOME_PID=$(pgrep -f gnome-shell | head -1)
if [ -n "$GNOME_PID" ]; then
    export DBUS_SESSION_BUS_ADDRESS=$(cat /proc/$GNOME_PID/environ 2>/dev/null | tr '\0' '\n' | grep DBUS_SESSION_BUS_ADDRESS | cut -d= -f2-)
fi

# Install Claude CLI to user prefix (no root required).
mkdir -p ~/.local
for i in 1 2 3 4 5; do
    npm install -g --prefix ~/.local @anthropic-ai/claude-code@latest 2>>/tmp/npm-install.log && break
    [ "$i" -lt 5 ] && sleep 3
done
export PATH="$HOME/.local/bin:$PATH"

# Verify claude is installed
if ! command -v claude &>/dev/null; then
    echo "ERROR: claude CLI not installed after 5 attempts" > /tmp/claude-auth-status.txt
    exit 1
fi

echo "starting" > /tmp/claude-auth-status.txt

# Use script to create a pseudo-TTY so Node.js (claude) uses line buffering.
# The browser will open inside the container's desktop environment.
script -qefc "claude auth login" /tmp/claude-auth-stdout.txt
