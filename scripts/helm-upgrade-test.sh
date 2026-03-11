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
COMMON_VALUES+=("--set" "controlplane.haystack.enabled=false")

log "Installing previous chart (v${PREVIOUS_VERSION})..."
helm install "$RELEASE_NAME" helix/helix-controlplane \
  --version "$PREVIOUS_VERSION" \
  "${COMMON_VALUES[@]}" \
  --wait --timeout "$TIMEOUT" || fail "Failed to install published chart v${PREVIOUS_VERSION}"

log "Previous chart installed. Checking pods..."
kubectl get pods -l "app.kubernetes.io/instance=${RELEASE_NAME}"

# Wait for controlplane to be ready
log "Waiting for controlplane pod to be ready..."
kubectl wait --for=condition=ready pod \
  -l "app.kubernetes.io/name=helix-controlplane" \
  --timeout=300s || fail "Controlplane pod never became ready after install"

# --- Step 2: Upgrade to the latest published chart ---
log "Upgrading to latest chart (v${LATEST_VERSION})..."
helm upgrade "$RELEASE_NAME" helix/helix-controlplane \
  --version "$LATEST_VERSION" \
  "${COMMON_VALUES[@]}" \
  --wait --timeout "$TIMEOUT" || fail "Helm upgrade from v${PREVIOUS_VERSION} to v${LATEST_VERSION} failed"

log "Upgrade complete. Checking pods..."
kubectl get pods -l "app.kubernetes.io/instance=${RELEASE_NAME}"

# --- Step 3: Validate the upgrade ---
log "Waiting for controlplane pod to be ready after upgrade..."
kubectl wait --for=condition=ready pod \
  -l "app.kubernetes.io/name=helix-controlplane" \
  --timeout=300s || fail "Controlplane pod never became ready after upgrade"

# Check no pods are in CrashLoopBackOff
CRASH_PODS=$(kubectl get pods -l "app.kubernetes.io/instance=${RELEASE_NAME}" \
  -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.containerStatuses[*].state.waiting.reason}{"\n"}{end}' \
  | grep -c "CrashLoopBackOff" || true)
if [ "$CRASH_PODS" -gt 0 ]; then
  kubectl get pods -l "app.kubernetes.io/instance=${RELEASE_NAME}"
  kubectl logs -l "app.kubernetes.io/name=helix-controlplane" --tail=50
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
