#!/bin/bash

# OAuth Integration Test Runner Script
# This script runs a specific OAuth integration test and handles artifact collection

set -euo pipefail

# Configuration
TEST_TYPE="${1:-}"
TEST_NAME="${2:-}"
TEST_RESULTS_DIR="${3:-/tmp/helix-oauth-test-results}"

# Help function
show_help() {
    cat << EOF
Usage: $0 <test_type> <test_name> [test_results_dir]

Parameters:
  test_type        The type of OAuth test to run (e.g., 'github', 'gmail', 'google-calendar', 'outlook', 'jira', 'confluence')
  test_name        The specific test name to run (e.g., 'TestGitHubOAuthSkillsE2E')
  test_results_dir Optional directory for test results (default: /tmp/helix-oauth-test-results)

Examples:
  $0 github TestGitHubOAuthSkillsE2E
  $0 gmail TestGmailOAuthSkillsE2E
  $0 google-calendar TestGoogleCalendarOAuthSkillsE2E
  $0 outlook TestOutlookOAuthSkillsE2E
  $0 jira TestJiraOAuthSkillsE2E
  $0 confluence TestConfluenceOAuthSkillsE2E

Environment variables required:
  - OPENAI_API_KEY: OpenAI API key for LLM calls
  - Various OAuth provider credentials (depending on test type)
  - Database and service URLs
EOF
}

# Check parameters
if [ -z "$TEST_TYPE" ] || [ -z "$TEST_NAME" ]; then
    echo "Error: test_type and test_name are required"
    show_help
    exit 1
fi

# Validate test type
case "$TEST_TYPE" in
    github|gmail|google-calendar|outlook|jira|confluence)
        echo "Running OAuth integration test: $TEST_TYPE / $TEST_NAME"
        ;;
    *)
        echo "Error: Unknown test type '$TEST_TYPE'. Supported types: github, gmail, google-calendar, outlook, jira, confluence"
        exit 1
        ;;
esac

# Ensure test results directory exists and has proper permissions
mkdir -p "$TEST_RESULTS_DIR"
chmod 755 "$TEST_RESULTS_DIR"

# Change to the correct directory
cd integration-test/skills

echo "=== Starting OAuth Integration Test: $TEST_TYPE ==="
echo "Test name: $TEST_NAME"
echo "Test results will be saved to: $TEST_RESULTS_DIR"

# Record test start time
TEST_START_TIME=$(date +%s)
echo "Test started at: $(date -d @$TEST_START_TIME)"

# Run the specific test and capture exit code, but don't fail the script immediately
set +e
go test -v -run "$TEST_NAME"
TEST_EXIT_CODE=$?
set -e

# Record test end time and calculate duration
TEST_END_TIME=$(date +%s)
TEST_DURATION=$((TEST_END_TIME - TEST_START_TIME))
echo "Test ended at: $(date -d @$TEST_END_TIME)"
echo "Test duration: $TEST_DURATION seconds"

# Write test results to file for upload step
cat > "$TEST_RESULTS_DIR/test_result.txt" << EOF
exit_code=$TEST_EXIT_CODE
duration=$TEST_DURATION
start_time=$TEST_START_TIME
end_time=$TEST_END_TIME
test_type=$TEST_TYPE
test_name=$TEST_NAME
EOF

# Determine human-readable status
if [ "$TEST_EXIT_CODE" -eq 0 ]; then
    echo "status=passed" >> "$TEST_RESULTS_DIR/test_result.txt"
    echo "=== OAuth Integration Test ($TEST_TYPE) PASSED ==="
else
    echo "status=failed" >> "$TEST_RESULTS_DIR/test_result.txt"
    echo "=== OAuth Integration Test ($TEST_TYPE) FAILED (exit code: $TEST_EXIT_CODE) ==="
fi

# List any artifacts that were created during the test
echo "=== Test Results Directory Contents ==="
if [ -d "$TEST_RESULTS_DIR" ]; then
    ls -la "$TEST_RESULTS_DIR/" || echo "No files in test results directory"
    echo "=== Screenshot Files ==="
    find "$TEST_RESULTS_DIR" -name "*.png" -exec ls -l {} \; || echo "No screenshot files found"
    echo "=== Log Files ==="
    find "$TEST_RESULTS_DIR" -name "*.log" -exec ls -l {} \; || echo "No log files found"
    echo "=== Conversation Files ==="
    find "$TEST_RESULTS_DIR" -name "*conversation*.txt" -exec ls -l {} \; || echo "No conversation files found"
else
    echo "test results directory does not exist"
fi

# Exit with original test exit code
exit $TEST_EXIT_CODE 