#!/bin/bash
# helix-claude-auth-submit.sh — Writes the auth code to the named pipe
# that helix-claude-auth-wrapper.sh feeds to claude auth login's stdin.
# Usage: helix-claude-auth-submit <code>
if [ -z "$1" ]; then
    echo "Usage: helix-claude-auth-submit <code>" >&2
    exit 1
fi
printf '%s\n' "$1" > /tmp/claude-auth-input
