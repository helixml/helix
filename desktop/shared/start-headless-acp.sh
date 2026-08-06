#!/bin/bash
set -e

export HELIX_HEADLESS=1
/usr/local/bin/helix-workspace-setup.sh
exec /usr/local/bin/headless-acp-runner
