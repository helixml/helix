#!/bin/bash
#
# Test that the debug shell offered by helix-workspace-setup.sh is a real
# interactive shell.
#
# The setup script tees all of its output into ~/.helix-setup.log, which
# replaces stdout AND stderr with a pipe. Bash only considers itself
# interactive when stdin and stderr are both ttys, so if that pipe leaks into
# the "Start an interactive shell for debugging" option the user gets a shell
# with no prompt, no readline and no job control — but commands still run,
# because stdin is still the terminal.
#
# This drives the real script under a pty: an unset GIT_USER_EMAIL makes it
# fail fast, which fires the EXIT trap and shows the menu. We answer 2, ask the
# resulting shell for its option flags, and check for the "i" flag.
#
# Usage: ./test-setup-terminal-shell.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SETUP_SCRIPT="${1:-$SCRIPT_DIR/helix-workspace-setup.sh}"

if [ ! -f "$SETUP_SCRIPT" ]; then
    echo "ERROR: setup script not found: $SETUP_SCRIPT"
    exit 1
fi

if ! command -v script >/dev/null 2>&1; then
    echo "ERROR: script(1) is required to allocate a pty"
    exit 1
fi

echo "=============================================="
echo "Testing interactive debug shell"
echo "=============================================="
echo "Testing: $SETUP_SCRIPT"
echo ""

FAKE_HOME=$(mktemp -d /tmp/helix-setup-test-XXXXXX)
TRANSCRIPT=$(mktemp /tmp/helix-setup-transcript-XXXXXX)
trap 'rm -rf "$FAKE_HOME" "$TRANSCRIPT"' EXIT

# Sleeps let the setup script reach its `read` and the debug shell reach its
# prompt before the next line arrives; the trailing one keeps the pty open long
# enough for the final output to be flushed.
(
    sleep 3
    printf '2\n'
    sleep 2
    printf 'echo HELIX_TEST_FLAGS=$-\n'
    sleep 1
    printf 'echo HELIX_TEST_MONITOR=$(set -o | grep ^monitor | awk "{print \\$2}")\n'
    sleep 1
    printf 'exit\n'
    sleep 1
) | env -i \
        HOME="$FAKE_HOME" \
        PATH="$PATH" \
        TERM=xterm \
        script -q -e -c "bash $SETUP_SCRIPT" "$TRANSCRIPT" >/dev/null 2>&1 || true

TESTS_PASSED=0
TESTS_FAILED=0

check() {
    local NAME="$1"
    shift
    if "$@"; then
        echo "  ✅ $NAME"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo "  ❌ $NAME"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

echo "Menu:"
check "menu prompt was shown" grep -q "Enter choice \[1-2\]" "$TRANSCRIPT"

echo ""
echo "Debug shell:"
FLAGS=$(grep -o 'HELIX_TEST_FLAGS=[a-zA-Z]*' "$TRANSCRIPT" | tail -n 1 | cut -d= -f2)
check "shell ran the command (flags reported: ${FLAGS:-none})" test -n "$FLAGS"
check "shell is interactive (\$- contains i)" test "${FLAGS#*i}" != "$FLAGS"
# Job control is only enabled for an interactive shell with a controlling tty;
# without it Ctrl-C kills the shell instead of the foreground command.
check "job control is on (Ctrl-C works)" \
    grep -q "HELIX_TEST_MONITOR=on" "$TRANSCRIPT"

echo ""
echo "Logging (must keep working):"
check "setup log was written" test -s "$FAKE_HOME/.helix-setup.log"
check "setup log has the setup output" \
    grep -q "GIT_USER_EMAIL not set" "$FAKE_HOME/.helix-setup.log"
check "failure sentinel was written" test -f "$FAKE_HOME/.helix-setup-failed"
check "failure sentinel records the exit code" \
    grep -q '"exit_code": 1' "$FAKE_HOME/.helix-setup-failed"
check "failure sentinel carries the log tail" \
    grep -q 'GIT_USER_EMAIL not set' "$FAKE_HOME/.helix-setup-failed"

echo ""
echo "=============================================="
echo "Test Results: $TESTS_PASSED passed, $TESTS_FAILED failed"
echo "=============================================="

if [ "$TESTS_FAILED" -gt 0 ]; then
    echo ""
    echo "Transcript:"
    cat "$TRANSCRIPT"
    exit 1
fi

exit 0
