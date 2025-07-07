#!/bin/bash

# OAuth Test Artifact Upload Script
# This script uploads test artifacts to Launchpad

set -euo pipefail

# Configuration
TEST_TYPE="${1:-}"
TEST_RESULTS_DIR="${2:-/tmp/helix-oauth-test-results}"

# Help function
show_help() {
    cat << EOF
Usage: $0 <test_type> [test_results_dir]

Parameters:
  test_type        The type of OAuth test (e.g., 'github', 'gmail')
  test_results_dir Optional directory for test results (default: /tmp/helix-oauth-test-results)

Examples:
  $0 github
  $0 gmail

Environment variables required:
  - LAUNCHPAD_URL: URL to Launchpad instance
  - CI_SHARED_SECRET: Shared secret for CI authentication
  - DRONE_BUILD_NUMBER: Build number
  - DRONE_BRANCH: Branch name
  - DRONE_COMMIT: Commit hash
EOF
}

# Check parameters
if [ -z "$TEST_TYPE" ]; then
    echo "Error: test_type is required"
    show_help
    exit 1
fi

# Install required tools
apt-get update && apt-get install -y curl zip

# Debug environment variables
echo "DEBUG - LAUNCHPAD_URL='${LAUNCHPAD_URL:-}'"
echo "DEBUG - CI_SHARED_SECRET present='${CI_SHARED_SECRET:+YES}'"
echo "DEBUG - All environment variables:"
env | grep -E "(LAUNCHPAD|CI_|DRONE_)" | sort || echo "No matching environment variables found"

# Determine test status from actual test results file
if [ -f "$TEST_RESULTS_DIR/test_result.txt" ]; then
    echo "=== Reading actual test results ==="
    cat "$TEST_RESULTS_DIR/test_result.txt"
    
    # Source the test results file to get variables
    . "$TEST_RESULTS_DIR/test_result.txt"
    
    TEST_STATUS="$status"
    TEST_DURATION="$duration"
    TEST_EXIT_CODE="$exit_code"
    
    echo "Actual test status: $TEST_STATUS"
    echo "Actual test duration: $TEST_DURATION seconds"
    echo "Actual test exit code: $TEST_EXIT_CODE"
else
    echo "=== No test results file found, using fallback ==="
    if [ "${DRONE_BUILD_STATUS:-}" = "success" ]; then
        TEST_STATUS="passed"
    else
        TEST_STATUS="failed"
    fi
    TEST_DURATION="unknown"
    TEST_EXIT_CODE="unknown"
    echo "Fallback test status: $TEST_STATUS"
    echo "Fallback test duration: $TEST_DURATION"
fi

# Construct Drone build URL
DRONE_BUILD_URL="https://drone.lukemarsden.net/helixml/helix/${DRONE_BUILD_NUMBER:-unknown}"
echo "Drone build URL: $DRONE_BUILD_URL"

# Create artifacts zip
cd /tmp

echo "=== OAuth Test Artifacts Collection ($TEST_TYPE) ==="
echo "Collecting artifacts from: $TEST_RESULTS_DIR"

if [ -d "$(basename "$TEST_RESULTS_DIR")" ]; then
    echo "Test results directory exists"
    echo "=== Directory Contents ==="
    ls -la "$(basename "$TEST_RESULTS_DIR")/" || echo "Directory exists but is empty"
    
    # Count different types of artifacts
    SCREENSHOT_COUNT=$(find "$(basename "$TEST_RESULTS_DIR")" -name "*.png" | wc -l)
    LOG_COUNT=$(find "$(basename "$TEST_RESULTS_DIR")" -name "*.log" | wc -l)
    CONVERSATION_COUNT=$(find "$(basename "$TEST_RESULTS_DIR")" -name "*conversation*.txt" | wc -l)
    
    echo "=== Artifact Summary ==="
    echo "Screenshots found: $SCREENSHOT_COUNT"
    echo "Log files found: $LOG_COUNT"
    echo "Conversation files found: $CONVERSATION_COUNT"
    
    if [ "$SCREENSHOT_COUNT" -gt 0 ]; then
        echo "=== Screenshot Files ==="
        find "$(basename "$TEST_RESULTS_DIR")" -name "*.png" -exec ls -l {} \;
    fi
    
    if [ "$LOG_COUNT" -gt 0 ]; then
        echo "=== Log Files ==="
        find "$(basename "$TEST_RESULTS_DIR")" -name "*.log" -exec ls -l {} \;
    fi
    
    if [ "$CONVERSATION_COUNT" -gt 0 ]; then
        echo "=== Conversation Files ==="
        find "$(basename "$TEST_RESULTS_DIR")" -name "*conversation*.txt" -exec ls -l {} \;
    fi
    
    # Check if there are any files at all
    if [ "$(find "$(basename "$TEST_RESULTS_DIR")" -type f | wc -l)" -gt 0 ]; then
        echo "Creating artifacts zip from test results"
        zip -r "oauth-${TEST_TYPE}-artifacts.zip" "$(basename "$TEST_RESULTS_DIR")"/
        ARTIFACTS_FILE="oauth-${TEST_TYPE}-artifacts.zip"
        echo "Artifacts zip created: $(ls -lh "$ARTIFACTS_FILE")"
    else
        echo "No test result files found, creating basic artifact"
        echo "OAuth integration test ($TEST_TYPE) run at $(date)" > "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        echo "Build: ${DRONE_BUILD_NUMBER:-unknown}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        echo "Build URL: $DRONE_BUILD_URL" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        echo "Branch: ${DRONE_BRANCH:-unknown}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        echo "Commit: ${DRONE_COMMIT:-unknown}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        echo "Status: ${TEST_STATUS}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        echo "Duration: ${TEST_DURATION} seconds" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        echo "Exit Code: ${TEST_EXIT_CODE}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        echo "Note: Test failed before generating artifacts" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
        zip -r "oauth-${TEST_TYPE}-artifacts.zip" "$(basename "$TEST_RESULTS_DIR")"/
        ARTIFACTS_FILE="oauth-${TEST_TYPE}-artifacts.zip"
    fi
else
    echo "Test results directory does not exist, creating basic artifact"
    mkdir -p "$(basename "$TEST_RESULTS_DIR")"
    echo "OAuth integration test ($TEST_TYPE) run at $(date)" > "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    echo "Build: ${DRONE_BUILD_NUMBER:-unknown}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    echo "Build URL: $DRONE_BUILD_URL" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    echo "Branch: ${DRONE_BRANCH:-unknown}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    echo "Commit: ${DRONE_COMMIT:-unknown}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    echo "Status: ${TEST_STATUS}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    echo "Duration: ${TEST_DURATION} seconds" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    echo "Exit Code: ${TEST_EXIT_CODE}" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    echo "Note: Test failed before creating test_results directory" >> "$(basename "$TEST_RESULTS_DIR")/test_summary.txt"
    zip -r "oauth-${TEST_TYPE}-artifacts.zip" "$(basename "$TEST_RESULTS_DIR")"/
    ARTIFACTS_FILE="oauth-${TEST_TYPE}-artifacts.zip"
fi

# Upload to Launchpad
if [ -n "${LAUNCHPAD_URL:-}" ] && [ -n "${CI_SHARED_SECRET:-}" ]; then
    echo "Uploading test results to Launchpad at $LAUNCHPAD_URL"
    UPLOAD_URL="$LAUNCHPAD_URL/api/ci/test-results"
    echo "Upload URL: $UPLOAD_URL"
    echo "Artifact file: $ARTIFACTS_FILE"
    
    # Show curl command for debugging
    echo "curl -X POST -H 'X-CI-Secret: ***' -F 'branch=${DRONE_BRANCH:-unknown}' -F 'commit=${DRONE_COMMIT:-unknown}' -F 'build_number=${DRONE_BUILD_NUMBER:-unknown}' -F 'build_url=$DRONE_BUILD_URL' -F 'test_type=oauth_${TEST_TYPE}' -F 'status=$TEST_STATUS' -F 'duration=$TEST_DURATION' -F 'exit_code=$TEST_EXIT_CODE' -F 'artifacts=@$ARTIFACTS_FILE' '$UPLOAD_URL'"
    
    # Execute the upload with verbose output
    curl -v -X POST \
      -H "X-CI-Secret: $CI_SHARED_SECRET" \
      -F "branch=${DRONE_BRANCH:-unknown}" \
      -F "commit=${DRONE_COMMIT:-unknown}" \
      -F "build_number=${DRONE_BUILD_NUMBER:-unknown}" \
      -F "build_url=$DRONE_BUILD_URL" \
      -F "test_type=oauth_${TEST_TYPE}" \
      -F "status=$TEST_STATUS" \
      -F "duration=$TEST_DURATION" \
      -F "exit_code=$TEST_EXIT_CODE" \
      -F "artifacts=@$ARTIFACTS_FILE" \
      "$UPLOAD_URL"
else
    echo "LAUNCHPAD_URL or CI_SHARED_SECRET not set, skipping test result upload"
    echo "LAUNCHPAD_URL: '${LAUNCHPAD_URL:-'(not set)'}'"
    if [ -n "${CI_SHARED_SECRET:-}" ]; then
        echo "CI_SHARED_SECRET: '(set)'"
    else
        echo "CI_SHARED_SECRET: '(not set)'"
    fi
fi

echo "=== OAuth Test Artifact Upload Complete ===" 