#!/bin/sh

set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
chart="$repo_root/charts/helix-sandbox"
work=$(mktemp -d)
trap 'rm -r "$work"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

render() {
    helm template test "$chart" "$@"
}

render > "$work/default.yaml"
grep -q 'name: SANDBOX_INSTANCE_ID_FILE' "$work/default.yaml" || fail "default Deployment has no identity file"
grep -q 'value: "/hydra-data/.helix-runner-id"' "$work/default.yaml" || fail "default Deployment has wrong identity file"
grep -A4 'name: SANDBOX_INSTANCE_ID$' "$work/default.yaml" | grep -q 'fieldPath: metadata.name' || fail "default Deployment is not compatible with an older sandbox image"
grep -A1 'name: SANDBOX_INSTANCE_ID_FROM_POD$' "$work/default.yaml" | grep -q 'value: "true"' || fail "default Deployment does not identify the pod-derived seed"

render --set sandbox.instanceId=stable-runner > "$work/explicit.yaml"
grep -A1 'name: SANDBOX_INSTANCE_ID$' "$work/explicit.yaml" | grep -q 'value: "stable-runner"' || fail "explicit identity was not rendered"
grep -q 'name: SANDBOX_INSTANCE_ID_FILE' "$work/explicit.yaml" || fail "explicit identity does not seed persistent marker"
if grep -q 'name: SANDBOX_INSTANCE_ID_FROM_POD' "$work/explicit.yaml"; then
    fail "explicit identity was marked as pod-derived"
fi

render --set sandbox.instanceId=stable-runner --set persistence.hydraData.enabled=false > "$work/explicit-no-pvc.yaml"
if grep -q 'name: SANDBOX_INSTANCE_ID_FILE' "$work/explicit-no-pvc.yaml"; then
    fail "identity file was rendered without persistent Hydra data"
fi

render --set deployment.type=StatefulSet --set replicaCount=2 --set sandbox.instanceId=ignored > "$work/statefulset.yaml"
grep -A4 'name: SANDBOX_INSTANCE_ID$' "$work/statefulset.yaml" | grep -q 'fieldPath: metadata.name' || fail "StatefulSet no longer uses pod name"
if grep -q 'name: SANDBOX_INSTANCE_ID_FILE' "$work/statefulset.yaml"; then
    fail "StatefulSet rendered Deployment identity file"
fi

if render --set persistence.hydraData.enabled=false > "$work/no-persistence.yaml" 2> "$work/no-persistence.err"; then
    fail "empty Deployment identity rendered without persistent Hydra data"
fi
grep -q 'requires persistence.hydraData.enabled: true' "$work/no-persistence.err" || fail "missing persistence guard message"

if render --set replicaCount=2 > "$work/replicas.yaml" 2> "$work/replicas.err"; then
    fail "multi-replica Deployment rendered"
fi
grep -q 'requires replicaCount: 1' "$work/replicas.err" || fail "missing replica guard message"

if render --set deployment.strategy.type=RollingUpdate > "$work/rolling.yaml" 2> "$work/rolling.err"; then
    fail "RollingUpdate Deployment rendered"
fi
grep -q 'requires deployment.strategy.type: Recreate' "$work/rolling.err" || fail "missing strategy guard message"

echo "Helm runner identity tests passed"
