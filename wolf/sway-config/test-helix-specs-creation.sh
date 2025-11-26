#!/bin/bash
#
# Test script for helix-specs branch creation logic
#
# This tests the create_helix_specs_branch function from helix-specs-create.sh
# which handles various edge cases like empty repos, detached HEAD, dirty working dirs, etc.
#
# Usage: ./test-helix-specs-creation.sh
#
# All tests should pass with green checkmarks. If any fail, the helix-specs
# creation logic in helix-specs-create.sh may need to be fixed.

set -e

# Source the helper script (the actual code being tested)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/helix-specs-create.sh"

echo "=============================================="
echo "Testing helix-specs branch creation scenarios"
echo "=============================================="
echo "Testing: $SCRIPT_DIR/helix-specs-create.sh"
echo ""

TESTS_PASSED=0
TESTS_FAILED=0

run_test() {
    local TEST_NAME=$1
    local SETUP_CMD=$2

    echo ""
    echo "--------------------------------------------"
    echo "TEST: $TEST_NAME"
    echo "--------------------------------------------"

    cd /tmp
    rm -rf test-helix-specs-repo
    mkdir test-helix-specs-repo
    cd test-helix-specs-repo

    # Create bare remote
    git init --bare remote.git 2>/dev/null
    git clone remote.git work 2>/dev/null
    cd work

    # Run setup command
    eval "$SETUP_CMD"

    REPO_PATH="$(pwd)"

    # Call the actual function from helix-specs-create.sh
    create_helix_specs_branch "$REPO_PATH"

    # Verification
    echo ""
    echo "=== Verification ==="

    local TEST_PASSED=true

    # Check if helix-specs exists
    if git show-ref --verify refs/remotes/origin/helix-specs >/dev/null 2>&1; then
        echo "  helix-specs branch exists on remote"
    else
        echo "  helix-specs branch NOT on remote"
        TEST_PASSED=false
    fi

    # Check current state
    FINAL_BRANCH=$(git branch --show-current 2>/dev/null)
    if [ -n "$FINAL_BRANCH" ]; then
        echo "  On branch: $FINAL_BRANCH"
    else
        FINAL_HEAD=$(git rev-parse HEAD 2>/dev/null)
        echo "  Detached at: ${FINAL_HEAD:0:8}"
    fi

    # Check for dirty files (only for non-empty repos that had dirty files)
    if git diff --quiet 2>/dev/null; then
        echo "  Working directory clean"
    else
        echo "  Uncommitted changes preserved"
    fi

    # Try creating worktree (this is what start-zed-helix.sh does after branch creation)
    if git worktree add /tmp/test-helix-specs-repo/worktree helix-specs 2>&1 >/dev/null; then
        echo "  Worktree creation successful"
        rm -rf /tmp/test-helix-specs-repo/worktree
        git worktree prune 2>/dev/null
    else
        echo "  Worktree creation FAILED"
        TEST_PASSED=false
    fi

    if [ "$TEST_PASSED" = true ]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        echo ""
        echo "PASSED: $TEST_NAME"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo ""
        echo "FAILED: $TEST_NAME"
    fi
}

# Test 1: Empty repository
run_test "Empty repository (no commits)" ""

# Test 2: Normal repo with commits
run_test "Normal repo with commits" '
    echo "test" > file.txt
    git add file.txt
    git commit -m "Initial commit"
    git push origin master
'

# Test 3: Detached HEAD
run_test "Detached HEAD" '
    echo "test" > file.txt
    git add file.txt
    git commit -m "Initial commit"
    git push origin master
    COMMIT=$(git rev-parse HEAD)
    git checkout "$COMMIT" 2>/dev/null
'

# Test 4: Dirty working directory (untracked file)
run_test "Dirty working directory (untracked)" '
    echo "test" > file.txt
    git add file.txt
    git commit -m "Initial commit"
    git push origin master
    echo "uncommitted" > dirty.txt
'

# Test 5: Dirty working directory (modified tracked file)
run_test "Dirty working directory (modified)" '
    echo "test" > file.txt
    git add file.txt
    git commit -m "Initial commit"
    git push origin master
    echo "modified" >> file.txt
'

# Test 6: Repo using "main" as default
run_test "Repo with main branch" '
    git checkout -b main 2>/dev/null
    echo "test" > file.txt
    git add file.txt
    git commit -m "Initial commit"
    git push origin main
'

# Test 7: Repo with staged changes
run_test "Repo with staged changes" '
    echo "test" > file.txt
    git add file.txt
    git commit -m "Initial commit"
    git push origin master
    echo "staged" > staged.txt
    git add staged.txt
'

echo ""
echo "=============================================="
echo "Test Results: $TESTS_PASSED passed, $TESTS_FAILED failed"
echo "=============================================="

# Cleanup
rm -rf /tmp/test-helix-specs-repo

if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
fi

exit 0
