#!/bin/bash
# helix-claude-auth-submit.sh — Forwards the OAuth auth code to claude's
# local callback server running inside the container.
#
# Usage: helix-claude-auth-submit <code-or-url>
#
# Accepts either:
#   - A full callback URL: http://localhost:PORT/callback?code=CODE&state=STATE
#   - A raw auth code string
#
# claude auth login starts a local HTTP server for the OAuth callback.
# The BROWSER-captured URL in /tmp/claude-auth-url.txt contains the localhost
# redirect_uri with the port and state.
if [ -z "$1" ]; then
    echo "Usage: helix-claude-auth-submit <code-or-url>" >&2
    exit 1
fi

INPUT="$1"
URL_FILE="/tmp/claude-auth-url.txt"

if [ ! -f "$URL_FILE" ]; then
    echo "ERROR: $URL_FILE not found" >&2
    exit 1
fi

CAPTURED_URL=$(cat "$URL_FILE")

# Extract port from the captured URL (e.g. http://localhost:38393/callback)
PORT=$(echo "$CAPTURED_URL" | grep -oP 'localhost:\K[0-9]+')

# Extract the state parameter from the captured URL
STATE=$(echo "$CAPTURED_URL" | grep -oP 'state=\K[^&]+')

if [ -z "$PORT" ] || [ -z "$STATE" ]; then
    echo "ERROR: could not extract port or state from $URL_FILE" >&2
    exit 1
fi

# If the input looks like a URL, extract the code parameter from it
if echo "$INPUT" | grep -qP '^https?://'; then
    CODE=$(echo "$INPUT" | grep -oP '[?&]code=\K[^&]+')
    if [ -z "$CODE" ]; then
        echo "ERROR: could not extract code from URL" >&2
        exit 1
    fi
else
    CODE="$INPUT"
fi

# Hit claude's local callback server with the auth code
exec curl -sf "http://localhost:${PORT}/callback?code=${CODE}&state=${STATE}"
