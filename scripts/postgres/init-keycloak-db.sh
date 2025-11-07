#!/bin/sh

# Wrapper script that explicitly invokes postgres-db.sh with sh
# This avoids reliance on bash being available in Alpine Linux
sh "$(dirname "$0")/postgres-db.sh"
