#!/bin/bash
# helix-claude-auth-wrapper.sh — Installs Claude CLI and runs claude auth login.
# Includes npm install with retry to handle container network not being ready
# immediately after boot. Captures stdout so the poll handler can parse the
# platform OAuth URL. Sets NO_COLOR=1 to prevent ANSI escape codes.
# Sets BROWSER to the capture script so the OAuth URL is also written to a file.
export NO_COLOR=1
export BROWSER=/usr/local/bin/helix-capture-browser

# Install Claude CLI to user prefix (no root required).
# The exec handler runs as user retro who cannot write to /usr/lib/node_modules/.
# Retry loop handles container network not being ready immediately after boot.
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

# Create a named pipe so the Helix frontend can send the auth code.
# The OAuth "code" flow shows a code on screen that must be pasted back into
# claude auth login's stdin. Since we're headless, the frontend captures the
# code from the user and writes it here via the exec endpoint.
rm -f /tmp/claude-auth-input
mkfifo /tmp/claude-auth-input

# Use `script` to create a pseudo-TTY so Node.js (claude) uses line buffering
# instead of full buffering. Without this, stdout is never flushed to the file
# and the poll handler can't find the platform OAuth URL.
#
# script reads from its stdin (the fifo). Start it in the background so we can
# open the write end on fd 3 to prevent premature EOF when no writer is connected.
script -qefc "claude auth login" /tmp/claude-auth-stdout.txt < /tmp/claude-auth-input &
SCRIPT_PID=$!

# Open write end to unblock script's read-open and keep the fifo alive.
exec 3>/tmp/claude-auth-input

# Wait for claude auth login to complete (either success or timeout).
wait $SCRIPT_PID
exec 3>&-
