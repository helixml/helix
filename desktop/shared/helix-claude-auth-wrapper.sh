#!/bin/bash
# helix-claude-auth-wrapper.sh — Installs Claude CLI and runs claude auth login.
# The browser opens INSIDE the container's desktop — the user interacts with
# the auth flow via the desktop stream viewer. localhost:PORT/callback is
# reachable within the container, so the OAuth callback works natively.

# Set display environment so the browser can open in the desktop session.
export DISPLAY=:0
# Find the DBUS session address from the gnome-shell process.
GNOME_PID=$(pgrep -f gnome-shell | head -1)
if [ -n "$GNOME_PID" ]; then
    export DBUS_SESSION_BUS_ADDRESS=$(cat /proc/$GNOME_PID/environ 2>/dev/null | tr '\0' '\n' | grep DBUS_SESSION_BUS_ADDRESS | cut -d= -f2-)
fi

# Install Claude CLI to user prefix (no root required).
mkdir -p ~/.local
for i in 1 2 3 4 5; do
    # Pin to 2.1.75: versions 2.1.76+ trigger a server-side Anthropic bug where
    # the redirect_uri is mangled (http://localhost → http:/localhost) during the
    # login→authorize redirect. See: https://github.com/anthropics/claude-code/issues/36015
    npm install -g --prefix ~/.local @anthropic-ai/claude-code@2.1.75 2>>/tmp/npm-install.log && break
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
