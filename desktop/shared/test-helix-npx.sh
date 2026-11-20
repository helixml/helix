#!/bin/bash

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
TEST_DIR=$(mktemp -d -t helix-npx-test-XXXXXX)
MOCK_NPX="$TEST_DIR/mock-npx"
TEST_LAUNCHER="$TEST_DIR/helix-npx"

cleanup() {
    rm -f "$MOCK_NPX" "$TEST_LAUNCHER"
    rmdir "$TEST_DIR"/helix-npx-* 2>/dev/null || true
    rmdir "$TEST_DIR"
}
trap cleanup EXIT

printf '%s\n' \
    '#!/bin/bash' \
    'IFS= read -r line || exit 90' \
    'printf '\''read:%s\n'\'' "$line"' \
    'exit 23' > "$MOCK_NPX"
chmod +x "$MOCK_NPX"

sed "s|/usr/bin/npx|$MOCK_NPX|g" "$SCRIPT_DIR/helix-npx.sh" > "$TEST_LAUNCHER"
chmod +x "$TEST_LAUNCHER"

set +e
output=$(printf '%s\n' '{"jsonrpc":"2.0","method":"initialize"}' | TMPDIR="$TEST_DIR" "$TEST_LAUNCHER" test-package)
exit_code=$?
set -e

if [ "$exit_code" -ne 23 ]; then
    printf 'expected exit code 23, got %s\n' "$exit_code" >&2
    exit 1
fi

expected='read:{"jsonrpc":"2.0","method":"initialize"}'
if [ "$output" != "$expected" ]; then
    printf 'expected %s, got %s\n' "$expected" "$output" >&2
    exit 1
fi
