#!/bin/bash
# helix-claude-auth-submit.sh — Forwards the OAuth callback to claude's local
# callback server. Called by the API's proxy endpoint.
# Usage: helix-claude-auth-submit <callback-url>
#   e.g. helix-claude-auth-submit "http://localhost:37093/callback?code=CODE&state=STATE"
if [ -z "$1" ]; then
    echo "Usage: helix-claude-auth-submit <callback-url>" >&2
    exit 1
fi
exec curl -sf "$1"
