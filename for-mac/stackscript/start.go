package stackscript

// Start is the shell script VMManager.startHelixStack runs in the VM: it
// links .env, picks the compose file, pins PostgreSQL for existing data and
// starts the stack.
func Start() string {
	return postgresPinShell + `
cd ~/helix 2>/dev/null || exit 0
if [ ! -f .env ] && [ ! -f .env.vm ]; then
    echo 'NO_ENV_FILE'
    exit 0
fi
# Docker Compose always reads .env by default; symlink so it picks up our config
if [ ! -e .env ]; then
    ln -s .env.vm .env
fi
# Detect compose file: dev (build-from-source) vs prod (install.sh)
COMPOSE_FILE=""
if [ -f docker-compose.dev.yaml ]; then
    COMPOSE_FILE="docker-compose.dev.yaml"
elif [ -f docker-compose.yaml ]; then
    COMPOSE_FILE="docker-compose.yaml"
else
    echo 'NO_COMPOSE_FILE'
    exit 0
fi
echo "COMPOSE_FILE=$COMPOSE_FILE"
# Keep an existing PostgreSQL data volume on its major (stackscript/postgres_pin.go).
INSTALL_DIR="$HOME/helix" DOCKER_CMD=docker ENVIRONMENT=linux
PG_PIN=$(pin_postgres_for_existing_data)
if [ -n "$PG_PIN" ]; then
    echo "$PG_PIN"
fi
# Check if stack is already running. Containers Docker restarted on boot
# predate a new pin, so recreate them when one was just written.
if [ -z "$PG_PIN" ] && docker compose -f "$COMPOSE_FILE" ps --format '{{.Service}}' 2>/dev/null | grep -q api; then
    echo 'ALREADY_RUNNING'
else
    echo 'Starting Helix stack...'
    docker compose -f "$COMPOSE_FILE" up -d 2>&1
    echo 'STARTED'
fi
# In prod mode (install.sh), always restart the sandbox container.
# sandbox.sh creates the container with --restart=always, so it may have
# auto-started from a previous boot with a stale RUNNER_TOKEN. Stop/rm it
# and re-run sandbox.sh so it picks up the current token from .env.
if [ "$COMPOSE_FILE" = "docker-compose.yaml" ] && [ -f sandbox.sh ]; then
    docker stop helix-sandbox 2>/dev/null || true
    docker rm helix-sandbox 2>/dev/null || true
    nohup bash sandbox.sh > /tmp/sandbox.log 2>&1 &
    echo 'SANDBOX_RESTARTED'
fi
`
}
