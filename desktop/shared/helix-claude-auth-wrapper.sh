#!/bin/bash
# helix-claude-auth-wrapper.sh — Runs claude auth login with stdout capture.
# Backgrounds the process and redirects output so the exec handler returns
# immediately while claude auth login continues running.
# Sets NO_COLOR=1 to prevent ANSI escape codes in the captured output.
# Sets BROWSER to the capture script so the OAuth URL is also written to a file.
export NO_COLOR=1
export BROWSER=/usr/local/bin/helix-capture-browser
claude auth login > /tmp/claude-auth-stdout.txt 2>&1 &
