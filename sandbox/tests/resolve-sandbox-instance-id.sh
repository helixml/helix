#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
source "$repo_root/sandbox/overlay/opt/helix/resolve-sandbox-instance-id.sh"

work=$(mktemp -d)
trap 'rm -r -- "$work"' EXIT
marker="$work/.helix-runner-id"
shim_dir="$work/bin"
order_log="$work/order.log"
mkdir "$shim_dir"

REAL_LN=$(command -v ln)
REAL_RM=$(command -v rm)
export REAL_LN REAL_RM
export ORDER_LOG="$order_log"

cat > "$shim_dir/sync" <<'EOF'
#!/usr/bin/env bash
set -eu
if [[ -f "$1" ]]; then
    printf 'sync-file:%s\n' "$(cat "$1")" >> "$ORDER_LOG"
    [[ "${SYNC_FAILURE:-}" != "file" ]]
else
    printf 'sync-directory\n' >> "$ORDER_LOG"
    [[ "${SYNC_FAILURE:-}" != "directory" ]]
fi
EOF
cat > "$shim_dir/ln" <<'EOF'
#!/usr/bin/env bash
set -eu
printf 'link-marker\n' >> "$ORDER_LOG"
exec "$REAL_LN" "$@"
EOF
cat > "$shim_dir/rm" <<'EOF'
#!/usr/bin/env bash
set -eu
if [[ "${1:-}" == "-f" ]]; then
    printf 'remove-temp\n' >> "$ORDER_LOG"
fi
exec "$REAL_RM" "$@"
EOF
chmod +x "$shim_dir/sync" "$shim_dir/ln" "$shim_dir/rm"
export PATH="$shim_dir:$PATH"

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
printf 'export:%s\n' "$SANDBOX_INSTANCE_ID" >> "$order_log"
expected_order=$(cat <<'EOF'
sync-file:sandbox-boot-a-abc123
link-marker
remove-temp
sync-directory
export:sandbox-boot-a-abc123
EOF
)
[[ "$(cat "$order_log")" == "$expected_order" ]] || fail "identity persistence order is incorrect"

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

file_sync_marker="$work/file-sync-failure"
SANDBOX_INSTANCE_ID_FILE="$file_sync_marker"
unset SANDBOX_INSTANCE_ID SANDBOX_INSTANCE_ID_FROM_POD
HOSTNAME="file-sync-runner"
export SYNC_FAILURE=file
if helix_resolve_sandbox_instance_id >/dev/null 2>&1; then
    fail "file synchronization failure did not fail startup"
fi
unset SYNC_FAILURE
[[ ! -e "$file_sync_marker" ]] || fail "file synchronization failure published a marker"
[[ -z "${SANDBOX_INSTANCE_ID+x}" ]] || fail "file synchronization failure exported an identity"
if compgen -G "$file_sync_marker.tmp.*" >/dev/null; then
    fail "file synchronization failure left its temporary file"
fi

existing_sync_marker="$work/existing-sync-failure"
printf 'existing-runner\n' > "$existing_sync_marker"
SANDBOX_INSTANCE_ID_FILE="$existing_sync_marker"
unset SANDBOX_INSTANCE_ID
export SYNC_FAILURE=file
if helix_resolve_sandbox_instance_id >/dev/null 2>&1; then
    fail "existing marker file synchronization failure did not fail startup"
fi
unset SYNC_FAILURE
[[ -z "${SANDBOX_INSTANCE_ID+x}" ]] || fail "existing marker synchronization failure exported an identity"
[[ "$(cat "$existing_sync_marker")" == "existing-runner" ]] || fail "existing marker synchronization failure changed the marker"

directory_sync_marker="$work/directory-sync-failure"
SANDBOX_INSTANCE_ID_FILE="$directory_sync_marker"
unset SANDBOX_INSTANCE_ID
HOSTNAME="directory-sync-runner"
: > "$order_log"
export SYNC_FAILURE=directory
if helix_resolve_sandbox_instance_id >/dev/null 2>&1; then
    fail "directory synchronization failure did not fail startup"
fi
[[ "$(cat "$directory_sync_marker")" == "directory-sync-runner" ]] || fail "directory synchronization failure did not preserve marker"
[[ -z "${SANDBOX_INSTANCE_ID+x}" ]] || fail "directory synchronization failure exported an identity"
HOSTNAME="replacement-after-sync-failure"
SANDBOX_INSTANCE_ID_FROM_POD=true
if helix_resolve_sandbox_instance_id >/dev/null 2>&1; then
    fail "existing marker directory synchronization failure did not fail startup"
fi
[[ -z "${SANDBOX_INSTANCE_ID+x}" ]] || fail "failed existing marker retry exported an identity"
unset SYNC_FAILURE
helix_resolve_sandbox_instance_id
[[ "$SANDBOX_INSTANCE_ID" == "directory-sync-runner" ]] || fail "retry after directory synchronization failure replaced the marker"
printf 'export:%s\n' "$SANDBOX_INSTANCE_ID" >> "$order_log"
expected_retry_order=$(cat <<'EOF'
sync-file:directory-sync-runner
link-marker
remove-temp
sync-directory
sync-file:directory-sync-runner
sync-directory
sync-file:directory-sync-runner
sync-directory
export:directory-sync-runner
EOF
)
[[ "$(cat "$order_log")" == "$expected_retry_order" ]] || fail "directory synchronization retry order is incorrect"

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
