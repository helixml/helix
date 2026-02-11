#!/bin/bash
# Integration test for docker-in-desktop mode.
# Run against a running stack with sandbox-nvidia healthy.
#
# Usage: ./scripts/test-docker-in-desktop.sh
#
# Requires: .env.usercreds with HELIX_API_KEY, HELIX_URL, HELIX_PROJECT
#           /tmp/helix CLI binary (cd api && CGO_ENABLED=0 go build -o /tmp/helix .)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(dirname "$SCRIPT_DIR")"
cd "$REPO_DIR"

# Load credentials
set -a && source .env.usercreds && set +a
export HELIX_API_KEY HELIX_URL

SVC="${SANDBOX_SERVICE:-sandbox-nvidia}"
DC="docker compose -f docker-compose.dev.yaml"
SESSION_ID=""
CTR=""
PASS=0
FAIL=0
ERRORS=""

cleanup() {
    if [ -n "$SESSION_ID" ]; then
        echo ""
        echo "=== Cleaning up session $SESSION_ID ==="
        /tmp/helix spectask stop "$SESSION_ID" 2>/dev/null || true
    fi
    echo ""
    echo "================================"
    echo "Results: $PASS passed, $FAIL failed"
    if [ -n "$ERRORS" ]; then
        echo ""
        echo "Failures:"
        printf "%s" "$ERRORS"
    fi
    echo "================================"
    [ "$FAIL" -eq 0 ]
}
trap cleanup EXIT

check() {
    local name="$1"
    shift
    echo -n "  $name... "
    if output=$("$@" 2>&1); then
        echo "PASS"
        PASS=$((PASS + 1))
    else
        echo "FAIL"
        FAIL=$((FAIL + 1))
        ERRORS="${ERRORS}  - ${name}: $(echo "$output" | head -3)\n"
    fi
}

# Helper: run command inside sandbox
se() { $DC exec -T "$SVC" "$@"; }

# Helper: run command inside desktop container (inside sandbox)
de() { se docker exec "$CTR" "$@"; }

echo "=== Docker-in-Desktop Integration Test ==="
echo ""

# Pre-flight checks
echo "--- Pre-flight ---"
check "CLI binary exists" test -x /tmp/helix
check "Sandbox healthy" test "$(docker inspect --format='{{.State.Health.Status}}' "helix-${SVC}-1" 2>/dev/null)" = "healthy"

# Check cgroup controllers at sandbox level
SANDBOX_CTRLS=$(se cat /sys/fs/cgroup/cgroup.subtree_control 2>&1)
check "Cgroup controllers delegated" echo "$SANDBOX_CTRLS" | grep -q memory

# Start session
echo ""
echo "--- Starting session ---"
echo -n "  Creating session... "
CLI_OUT=$(/tmp/helix spectask start --project "$HELIX_PROJECT" -n "integration-test" --prompt "test" -q 2>&1 || true)
if [[ "$CLI_OUT" == ses_* ]]; then
    SESSION_ID="$CLI_OUT"
    echo "PASS ($SESSION_ID)"
    PASS=$((PASS + 1))
else
    # CLI may timeout but session still starts - wait for container
    echo "CLI timed out, waiting for container..."
    sleep 30
    CTR=$(se docker ps --format "{{.Names}}" 2>&1 | grep ubuntu-external | head -1 || true)
    if [ -n "$CTR" ]; then
        SESSION_ID=$(echo "$CTR" | sed 's/ubuntu-external-/ses_/')
        echo "  Found container: $CTR"
        PASS=$((PASS + 1))
    else
        echo "  FAIL - no desktop container found"
        FAIL=$((FAIL + 1))
        exit 1
    fi
fi

# Wait for container
if [ -z "$CTR" ]; then
    echo -n "  Waiting for desktop container... "
    for i in $(seq 1 60); do
        CTR=$(se docker ps --format "{{.Names}}" 2>&1 | grep ubuntu-external | head -1 || true)
        if [ -n "$CTR" ]; then break; fi
        sleep 2
    done
    if [ -n "$CTR" ]; then
        echo "PASS ($CTR)"
        PASS=$((PASS + 1))
    else
        echo "FAIL - timeout"
        FAIL=$((FAIL + 1))
        exit 1
    fi
fi

# Wait for dockerd
echo -n "  Waiting for inner dockerd... "
for i in $(seq 1 30); do
    if de docker.real info &>/dev/null; then break; fi
    sleep 1
done
check "Inner dockerd ready" de docker.real info

echo ""
echo "--- Docker functionality ---"
check "docker info" de docker info
check "docker run hello-world" de docker run --rm hello-world
check "docker build" de bash -c 'echo "FROM alpine" | docker build -t test-integ -'
check "docker compose version" de docker compose version
check "DNS resolution" de docker run --rm alpine ping -c 1 google.com

echo ""
echo "--- Shared BuildKit cache ---"
check "/buildkit-cache mounted" de ls /buildkit-cache
check "BUILDKIT_HOST set" de printenv BUILDKIT_HOST

BUILD_OUT=$(de bash -c 'echo "FROM alpine
RUN echo bk-test" | docker build -t bk-test -' 2>&1 || true)
echo -n "  BuildKit remote builder... "
if echo "$BUILD_OUT" | grep -q "helix-shared"; then
    echo "PASS"
    PASS=$((PASS + 1))
else
    echo "FAIL"
    FAIL=$((FAIL + 1))
    ERRORS="${ERRORS}  - BuildKit remote builder: builder not helix-shared\n"
fi

echo ""
echo "--- cgroup v2 delegation ---"
DESKTOP_CTRLS=$(de cat /sys/fs/cgroup/cgroup.controllers 2>&1)
check "memory controller available" echo "$DESKTOP_CTRLS" | grep -q memory
DESKTOP_SUBTREE=$(de cat /sys/fs/cgroup/cgroup.subtree_control 2>&1)
check "memory controller delegated" echo "$DESKTOP_SUBTREE" | grep -q memory

echo ""
echo "--- Kind (Kubernetes) ---"
check "kind binary" de kind version
check "kubectl binary" de kubectl version --client

echo -n "  kind create cluster... "
KIND_OUT=$(de kind create cluster --name integ-test 2>&1 || true)
if echo "$KIND_OUT" | grep -q "Have a nice day"; then
    echo "PASS"
    PASS=$((PASS + 1))

    # Wait for node readiness
    sleep 15
    check "kubectl get nodes (Ready)" de kubectl get nodes

    de kind delete cluster --name integ-test >/dev/null 2>&1 || true
else
    echo "FAIL"
    FAIL=$((FAIL + 1))
    ERRORS="${ERRORS}  - kind create cluster: $(echo "$KIND_OUT" | tail -3)\n"
fi

echo ""
echo "--- No old infrastructure ---"
MOUNTS=$(se docker inspect "$CTR" --format '{{json .Mounts}}' 2>&1)
echo -n "  No docker.sock mount... "
if echo "$MOUNTS" | grep -q "docker.sock"; then
    echo "FAIL"; FAIL=$((FAIL + 1)); ERRORS="${ERRORS}  - docker.sock found in mounts\n"
else
    echo "PASS"; PASS=$((PASS + 1))
fi

echo -n "  No host-docker.sock mount... "
if echo "$MOUNTS" | grep -q "host-docker"; then
    echo "FAIL"; FAIL=$((FAIL + 1)); ERRORS="${ERRORS}  - host-docker.sock found in mounts\n"
else
    echo "PASS"; PASS=$((PASS + 1))
fi

PRIVILEGED=$(se docker inspect "$CTR" --format '{{.HostConfig.Privileged}}' 2>&1)
check "Container is privileged" test "$PRIVILEGED" = "true"

echo -n "  /var/lib/docker is volume... "
if echo "$MOUNTS" | grep -q "docker-data"; then
    echo "PASS"; PASS=$((PASS + 1))
else
    echo "FAIL"; FAIL=$((FAIL + 1)); ERRORS="${ERRORS}  - docker-data volume not found in mounts\n"
fi
