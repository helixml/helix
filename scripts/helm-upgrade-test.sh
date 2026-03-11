#!/bin/bash
set -euo pipefail

# Helm Upgrade Compatibility Test
#
# Tests that upgrading between published Helm chart versions works without
# breaking. Installs the second-latest published version (what existing
# customers have) then upgrades to the latest published version (what
# customers would get on `helm repo update && helm upgrade`).
#
# Usage:
#   ./scripts/helm-upgrade-test.sh           # full test (creates Kind cluster)
#   SKIP_CLUSTER_CREATE=1 ./scripts/helm-upgrade-test.sh  # reuse existing cluster
#
# Requirements: docker, kind, kubectl, helm, jq

CLUSTER_NAME="helm-upgrade-test"
HELM_REPO_URL="https://charts.helixml.tech"
RELEASE_NAME="upgrade-test"
TIMEOUT="5m"

log() { echo "==> $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

cleanup() {
  if [ "${SKIP_CLEANUP:-}" != "1" ]; then
    log "Cleaning up Kind cluster..."
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
  fi
}

# Cleanup on exit unless told not to
if [ "${SKIP_CLEANUP:-}" != "1" ]; then
  trap cleanup EXIT
fi

# --- Create Kind cluster ---
if [ "${SKIP_CLUSTER_CREATE:-}" != "1" ]; then
  log "Deleting any existing cluster..."
  kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true

  log "Creating Kind cluster..."
  kind create cluster --name "$CLUSTER_NAME" --wait 120s
fi

# When running inside a container with the host Docker socket mounted (e.g. Drone CI),
# Kind's kubeconfig points to 127.0.0.1 which is the container's own localhost, not
# the host where the Kind API server container actually listens. Fix by:
# 1. Connecting this container to Kind's Docker network
# 2. Patching kubeconfig to use the control plane container's IP
if [ -S /var/run/docker.sock ] && ! kubectl cluster-info --context "kind-${CLUSTER_NAME}" >/dev/null 2>&1; then
  log "Cannot reach cluster via localhost, patching kubeconfig for container environment..."

  # Find our own container ID using multiple methods (Drone overrides hostname)
  SELF_CONTAINER_ID=""
  # Method 1: hostname (Docker default, but orchestrators may override)
  if [ -z "$SELF_CONTAINER_ID" ] && docker inspect "$(hostname)" >/dev/null 2>&1; then
    SELF_CONTAINER_ID="$(hostname)"
  fi
  # Method 2: cgroup v1 cpuset path contains the 64-char container ID
  if [ -z "$SELF_CONTAINER_ID" ]; then
    SELF_CONTAINER_ID=$(cat /proc/1/cpuset 2>/dev/null | grep -oE '[a-f0-9]{64}' | head -1 || true)
  fi
  # Method 3: mountinfo contains docker/containers/<id> paths
  if [ -z "$SELF_CONTAINER_ID" ]; then
    SELF_CONTAINER_ID=$(cat /proc/self/mountinfo 2>/dev/null | grep -oE 'docker/containers/[a-f0-9]{64}' | head -1 | sed 's:docker/containers/::' || true)
  fi

  if [ -n "$SELF_CONTAINER_ID" ]; then
    log "Detected container ID: ${SELF_CONTAINER_ID:0:12}, connecting to Kind network..."
    docker network connect kind "$SELF_CONTAINER_ID" 2>/dev/null || true
  else
    log "WARNING: Could not detect container ID, kubectl may not reach the cluster"
  fi

  CONTROL_PLANE_IP=$(docker inspect "${CLUSTER_NAME}-control-plane" \
    --format '{{ .NetworkSettings.Networks.kind.IPAddress }}' 2>/dev/null || true)
  if [ -n "$CONTROL_PLANE_IP" ]; then
    log "Patching kubeconfig to use control plane IP: ${CONTROL_PLANE_IP}"
    # The Kind API server TLS cert is issued for 127.0.0.1/localhost, not the
    # container IP, so we must skip TLS verification when connecting via IP.
    kubectl config set-cluster "kind-${CLUSTER_NAME}" \
      --server="https://${CONTROL_PLANE_IP}:6443" \
      --insecure-skip-tls-verify=true
  fi
fi

log "Cluster info:"
kubectl cluster-info --context "kind-${CLUSTER_NAME}"

# --- Fetch published chart versions ---
log "Adding Helix Helm repo..."
helm repo add helix --force-update "$HELM_REPO_URL"
helm repo update

log "Fetching published chart versions..."
# Get all versions including pre-release (--devel), sorted newest first
ALL_VERSIONS=$(helm search repo helix/helix-controlplane --versions --devel --output json \
  | jq -r '.[].version')

LATEST_VERSION=$(echo "$ALL_VERSIONS" | head -1)
PREVIOUS_VERSION=$(echo "$ALL_VERSIONS" | head -2 | tail -1)

if [ -z "$LATEST_VERSION" ] || [ -z "$PREVIOUS_VERSION" ]; then
  fail "Could not find at least two published chart versions (got latest=$LATEST_VERSION, previous=$PREVIOUS_VERSION)"
fi

if [ "$LATEST_VERSION" = "$PREVIOUS_VERSION" ]; then
  fail "Only one published version found ($LATEST_VERSION) — need at least two to test upgrade"
fi

log "Upgrade path: v${PREVIOUS_VERSION} -> v${LATEST_VERSION}"

# --- Step 1: Install the previous published chart (simulates existing customer) ---
# Minimal values to get the chart running without real API keys
COMMON_VALUES=()
COMMON_VALUES+=("--set" "controlplane.runnerToken=test-token-for-helm-validation")
COMMON_VALUES+=("--set" "controlplane.haystack.enabled=false")

# Helper: dump pod diagnostics on failure
dump_diagnostics() {
  log "--- Pod status ---"
  kubectl get pods -l "app.kubernetes.io/instance=${RELEASE_NAME}" -o wide 2>/dev/null || true
  log "--- Controlplane container logs ---"
  kubectl logs -l "app.kubernetes.io/component=controlplane" -c controlplane --tail=30 2>/dev/null || true
  log "--- Init container logs ---"
  kubectl logs -l "app.kubernetes.io/component=controlplane" -c wait-for-postgres --tail=30 2>/dev/null || true
  log "--- Pod describe ---"
  kubectl describe pod -l "app.kubernetes.io/component=controlplane" 2>/dev/null | tail -40 || true
  log "--- Events ---"
  kubectl get events --sort-by=.lastTimestamp --field-selector involvedObject.kind=Pod 2>/dev/null | tail -20 || true
}

# Helper: wait for controlplane deployment rollout with diagnostics on failure
wait_for_controlplane() {
  local phase="$1"
  log "Waiting for controlplane rollout ($phase)..."
  if ! kubectl rollout status deployment/"${RELEASE_NAME}-helix-controlplane" \
    --timeout=300s 2>&1; then
    log "Controlplane rollout failed after $phase"
    dump_diagnostics
    fail "Controlplane deployment never rolled out after $phase"
  fi
}

log "Installing previous chart (v${PREVIOUS_VERSION})..."
helm install "$RELEASE_NAME" helix/helix-controlplane \
  --version "$PREVIOUS_VERSION" \
  "${COMMON_VALUES[@]}" \
  --timeout "$TIMEOUT" || fail "Failed to install published chart v${PREVIOUS_VERSION}"

log "Previous chart installed. Checking pods..."
kubectl get pods -l "app.kubernetes.io/instance=${RELEASE_NAME}"
wait_for_controlplane "install v${PREVIOUS_VERSION}"

# --- Step 2: Upgrade to the latest published chart ---
log "Upgrading to latest chart (v${LATEST_VERSION})..."
helm upgrade "$RELEASE_NAME" helix/helix-controlplane \
  --version "$LATEST_VERSION" \
  "${COMMON_VALUES[@]}" \
  --timeout "$TIMEOUT" || fail "Helm upgrade from v${PREVIOUS_VERSION} to v${LATEST_VERSION} failed"

log "Upgrade complete. Checking pods..."
kubectl get pods -l "app.kubernetes.io/instance=${RELEASE_NAME}"
wait_for_controlplane "upgrade to v${LATEST_VERSION}"

# Check no pods are in CrashLoopBackOff
CRASH_PODS=$(kubectl get pods -l "app.kubernetes.io/instance=${RELEASE_NAME}" \
  -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.containerStatuses[*].state.waiting.reason}{"\n"}{end}' \
  | grep -c "CrashLoopBackOff" || true)
if [ "$CRASH_PODS" -gt 0 ]; then
  dump_diagnostics
  fail "$CRASH_PODS pod(s) in CrashLoopBackOff after upgrade"
fi

# Run helm test (validates service connectivity)
log "Running helm test..."
helm test "$RELEASE_NAME" --timeout 60s || fail "helm test failed after upgrade"

# Check controlplane service exists
log "Checking controlplane service..."
kubectl get svc "${RELEASE_NAME}-helix-controlplane" > /dev/null \
  || fail "Service ${RELEASE_NAME}-helix-controlplane not found"

# --- Done ---
log ""
log "========================================="
log "  Helm upgrade test PASSED"
log "  Upgrade: v${PREVIOUS_VERSION} -> v${LATEST_VERSION}"
log "========================================="
