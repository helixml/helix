#!/bin/bash
set -euo pipefail

export HELIX_HOST_HOME=/home/retro
export HYDRA_ENABLED=false
export KODIT_ENABLED=false
export COMPOSE_PROFILES=""

cd /home/retro/work/helix

# Compose requires the file even when no provider keys are needed.
touch .env

# Builds and starts API, frontend, Postgres, postgres-mcp, and Chrome.
# It does not build Zed, desktop images, or sandbox images.
docker compose -f docker-compose.dev.yaml up -d --build api frontend