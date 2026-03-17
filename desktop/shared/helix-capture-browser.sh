#!/bin/bash
# helix-capture-browser.sh — Captures a URL instead of opening a browser.
# Used by `claude auth login` with BROWSER env var override.
# The frontend polls for this file to open the URL in the user's native browser.
echo "$1" > /tmp/claude-auth-url.txt
