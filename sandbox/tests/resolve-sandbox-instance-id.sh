#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
source "$repo_root/sandbox/overlay/opt/helix/resolve-sandbox-instance-id.sh"

work=$(mktemp -d)
trap 'rm -r -- "$work"' EXIT
marker="$work/.helix-runner-id"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_fails() {
    if ( "$@" ) >/dev/null 2>&1; then
        fail "expected command to fail: $*"
    fi
}

SANDBOX_INSTANCE_ID_FILE="$marker"
HOSTNAME="sandbox-boot-a-abc123"
SANDBOX_INSTANCE_ID="$HOSTNAME"
SANDBOX_INSTANCE_ID_FROM_POD=true
helix_resolve_sandbox_instance_id
[[ "$SANDBOX_INSTANCE_ID" == "sandbox-boot-a-abc123" ]] || fail "first boot did not use hostname"
[[ "$(cat "$marker")" == "sandbox-boot-a-abc123" ]] || fail "first boot did not persist identity"

HOSTNAME="sandbox-boot-b-def456"
SANDBOX_INSTANCE_ID="$HOSTNAME"
helix_resolve_sandbox_instance_id
[[ "$SANDBOX_INSTANCE_ID" == "sandbox-boot-a-abc123" ]] || fail "replacement boot did not reuse identity"

for process in heartbeat hydra compose-manager; do
    actual=$(PROCESS="$process" bash -c 'printf "%s:%s" "$PROCESS" "$SANDBOX_INSTANCE_ID"')
    [[ "$actual" == "$process:sandbox-boot-a-abc123" ]] || fail "$process did not inherit identity"
done

explicit_marker="$work/explicit-id"
SANDBOX_INSTANCE_ID_FILE="$explicit_marker"
SANDBOX_INSTANCE_ID="runner-production"
unset SANDBOX_INSTANCE_ID_FROM_POD
helix_resolve_sandbox_instance_id
[[ "$(cat "$explicit_marker")" == "runner-production" ]] || fail "explicit identity was not persisted"
unset SANDBOX_INSTANCE_ID
HOSTNAME="replacement-pod"
SANDBOX_INSTANCE_ID_FROM_POD=true
helix_resolve_sandbox_instance_id
[[ "$SANDBOX_INSTANCE_ID" == "runner-production" ]] || fail "persisted explicit identity was not reused"

SANDBOX_INSTANCE_ID="different-runner"
unset SANDBOX_INSTANCE_ID_FROM_POD
assert_fails helix_resolve_sandbox_instance_id

printf '\n' > "$work/empty"
SANDBOX_INSTANCE_ID_FILE="$work/empty"
unset SANDBOX_INSTANCE_ID
assert_fails helix_resolve_sandbox_instance_id

printf 'runner-one\nrunner-two\n' > "$work/multiline"
SANDBOX_INSTANCE_ID_FILE="$work/multiline"
assert_fails helix_resolve_sandbox_instance_id

mkdir "$work/not-a-file"
SANDBOX_INSTANCE_ID_FILE="$work/not-a-file"
assert_fails helix_resolve_sandbox_instance_id

SANDBOX_INSTANCE_ID_FILE="$work/missing/identity"
assert_fails helix_resolve_sandbox_instance_id

unset SANDBOX_INSTANCE_ID_FILE
SANDBOX_INSTANCE_ID="legacy-runner"
helix_resolve_sandbox_instance_id
[[ "$SANDBOX_INSTANCE_ID" == "legacy-runner" ]] || fail "no-file mode changed existing identity"

echo "sandbox instance identity tests passed"
