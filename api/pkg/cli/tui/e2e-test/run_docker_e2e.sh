#!/usr/bin/env bash
set -euo pipefail

# Build and run the TUI E2E test in Docker.
#
# Prerequisites:
#   - Pre-built Zed binary at ./zed-binary
#   - ANTHROPIC_API_KEY set (or in ../../../../../../.env.usercreds)
#
# Usage:
#   ./run_docker_e2e.sh              # build everything + run
#   ./run_docker_e2e.sh --no-build   # skip builds, use existing binaries

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Navigate to helix root (e2e-test → tui → cli → pkg → api → helix)
HELIX_DIR="$(cd "$SCRIPT_DIR/../../../../../.." && pwd)"

echo "=== Helix TUI E2E Test ==="
echo "  Script dir: $SCRIPT_DIR"
echo "  Helix dir:  $HELIX_DIR"
echo ""

# Source credentials
for envfile in "$HELIX_DIR/.env" "$HELIX_DIR/.env.usercreds"; do
    if [ -f "$envfile" ]; then
        [ -z "${ANTHROPIC_API_KEY:-}" ] && ANTHROPIC_API_KEY=$(grep '^ANTHROPIC_API_KEY=' "$envfile" | cut -d= -f2- || true)
        if grep -q ANTHROPIC_BASE_URL "$envfile" 2>/dev/null; then
            ANTHROPIC_BASE_URL=$(grep '^ANTHROPIC_BASE_URL=' "$envfile" | cut -d= -f2-)
        fi
    fi
done

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
    echo "ERROR: ANTHROPIC_API_KEY not set."
    echo "Either: export ANTHROPIC_API_KEY=sk-..."
    echo "Or:     add it to $HELIX_DIR/.env.usercreds"
    exit 1
fi

# Check for Zed binary
if [ ! -f "$SCRIPT_DIR/zed-binary" ]; then
    # Try to copy from helix zed-build
    if [ -f "$HELIX_DIR/zed-build/zed" ]; then
        echo "[setup] Copying Zed binary from $HELIX_DIR/zed-build/zed"
        cp "$HELIX_DIR/zed-build/zed" "$SCRIPT_DIR/zed-binary"
    else
        echo "ERROR: No zed-binary found at $SCRIPT_DIR/zed-binary"
        echo "Build it: cd $HELIX_DIR && ./stack build-zed release"
        echo "Then:     cp $HELIX_DIR/zed-build/zed $SCRIPT_DIR/zed-binary"
        exit 1
    fi
fi

# Build binaries (unless --no-build)
if [ "${1:-}" != "--no-build" ]; then
    echo "=== Building tui-test-server ==="
    cd "$SCRIPT_DIR/tui-test-server"
    CGO_ENABLED=0 go build -o tui-test-server .
    echo "Built: tui-test-server"
    echo ""

    echo "=== Building helix binary (with TUI) ==="
    cd "$HELIX_DIR"
    CGO_ENABLED=0 go build -o "$SCRIPT_DIR/helix-binary" ./api/
    echo "Built: helix-binary"
    echo ""
fi

# Print binary info
echo "=== Binary versions ==="
for bin in zed-binary tui-test-server/tui-test-server helix-binary; do
    if [ -f "$SCRIPT_DIR/$bin" ]; then
        echo "  $bin: $(stat -c '%y' "$SCRIPT_DIR/$bin" 2>/dev/null | cut -d. -f1)  $(md5sum "$SCRIPT_DIR/$bin" 2>/dev/null | cut -c1-12)"
    fi
done
echo ""

# Build Docker image
echo "=== Building Docker image ==="
cd "$SCRIPT_DIR"
docker build -t tui-e2e -f Dockerfile .
echo ""

# Run
echo "=== Running TUI E2E test ==="

ANTHROPIC_BASE_URL_ARG=""
if [ -n "${ANTHROPIC_BASE_URL:-}" ]; then
    ANTHROPIC_BASE_URL_ARG="-e ANTHROPIC_BASE_URL=$ANTHROPIC_BASE_URL"
fi

docker run --rm \
    --add-host=host.docker.internal:host-gateway \
    -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
    ${ANTHROPIC_BASE_URL_ARG} \
    tui-e2e
