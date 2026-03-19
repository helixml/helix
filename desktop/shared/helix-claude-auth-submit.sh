#!/bin/bash
# helix-claude-auth-submit.sh — Forwards the OAuth auth code to claude's
# local callback server running inside the container.
# Usage: helix-claude-auth-submit <code>
#
# claude auth login starts a local HTTP server and waits for the OAuth callback.
# The BROWSER-captured URL in /tmp/claude-auth-url.txt contains the localhost
# redirect_uri with the port and state. We extract those and curl the callback.
if [ -z "$1" ]; then
    echo "Usage: helix-claude-auth-submit <code>" >&2
    exit 1
fi

CODE="$1"
URL_FILE="/tmp/claude-auth-url.txt"

if [ ! -f "$URL_FILE" ]; then
    echo "ERROR: $URL_FILE not found" >&2
    exit 1
fi

CAPTURED_URL=$(cat "$URL_FILE")

# Extract the localhost redirect_uri (e.g. http://localhost:37093/callback)
REDIRECT_URI=$(echo "$CAPTURED_URL" | grep -oP 'redirect_uri=\K[^&]+' | python3 -c "import sys,urllib.parse; print(urllib.parse.unquote(sys.stdin.read().strip()))")

# Extract the state parameter
STATE=$(echo "$CAPTURED_URL" | grep -oP 'state=\K[^&]+')

if [ -z "$REDIRECT_URI" ] || [ -z "$STATE" ]; then
    echo "ERROR: could not extract redirect_uri or state from $URL_FILE" >&2
    exit 1
fi

# Hit claude's local callback server with the auth code
curl -sf "${REDIRECT_URI}?code=${CODE}&state=${STATE}"
