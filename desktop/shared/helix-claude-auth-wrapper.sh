#!/bin/bash
# helix-claude-auth-wrapper.sh — Installs Claude CLI and runs claude auth login.
#
# Workaround for Anthropic's server-side redirect_uri mangling bug:
# Instead of opening the browser inside the container (which hits the bug),
# we suppress the browser and show the auth URL in the Helix UI. The user
# opens it in their host browser (where they're already logged in to Claude,
# bypassing the login redirect that causes the mangling), gets a code, and
# enters it in the terminal visible in the desktop stream.
#
# See: https://github.com/anthropics/claude-code/issues/36015

# Set display environment so the terminal can open in the desktop session.
export DISPLAY=:0
# Find the DBUS session address from the gnome-shell process.
GNOME_PID=$(pgrep -f gnome-shell | head -1)
if [ -n "$GNOME_PID" ]; then
    export DBUS_SESSION_BUS_ADDRESS=$(cat /proc/$GNOME_PID/environ 2>/dev/null | tr '\0' '\n' | grep DBUS_SESSION_BUS_ADDRESS | cut -d= -f2-)
fi

# Suppress the container browser — the auth URL will be shown in the Helix UI
# instead, so the user can open it in their host browser.
export BROWSER=false

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

# Open claude auth login in a visible terminal window so the user can
# see the prompts and enter the auth code via the desktop stream.
# Use script(1) to capture stdout for the polling endpoint while keeping
# the terminal interactive for code entry.
kitty --title "Claude Login" -e bash -c 'script -qefc "claude auth login" /tmp/claude-auth-stdout.txt; echo "Press Enter to close..."; read'
