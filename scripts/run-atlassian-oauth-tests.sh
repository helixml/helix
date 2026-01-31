#!/bin/bash

# Atlassian OAuth Test Runner Script
# This script runs the Atlassian OAuth integration tests locally

set -euo pipefail

# Configuration
TEST_TYPE="${1:-all}"

# Help function
show_help() {
    cat << EOF
Usage: $0 [test_type]

Parameters:
  test_type    The type of test to run: 'jira', 'confluence', or 'all' (default: all)

Examples:
  $0           # Run all Atlassian OAuth tests
  $0 jira      # Run only Jira OAuth tests
  $0 confluence # Run only Confluence OAuth tests

Environment variables required:
  - ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_ID
  - ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_SECRET
  - ATLASSIAN_SKILL_TEST_OAUTH_USERNAME
  - ATLASSIAN_SKILL_TEST_OAUTH_PASSWORD
  - ATLASSIAN_SKILL_TEST_CLOUD_ID
  - GMAIL_CREDENTIALS_BASE64

Optional environment variables:
  - BROWSER_URL (default: http://chrome:7317)
  - WEB_SERVER_HOST (default: localhost)
EOF
}

# Check parameters
if [ "$TEST_TYPE" = "help" ] || [ "$TEST_TYPE" = "--help" ] || [ "$TEST_TYPE" = "-h" ]; then
    show_help
    exit 0
fi

# Validate environment variables
echo "=== Validating Environment Variables ==="
required_vars=(
    "ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_ID"
    "ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_SECRET"
    "ATLASSIAN_SKILL_TEST_OAUTH_USERNAME"
    "ATLASSIAN_SKILL_TEST_OAUTH_PASSWORD"
    "ATLASSIAN_SKILL_TEST_CLOUD_ID"
    "GMAIL_CREDENTIALS_BASE64"
)

missing_vars=()
for var in "${required_vars[@]}"; do
    if [ -z "${!var:-}" ]; then
        missing_vars+=("$var")
    else
        echo "✓ $var is set"
    fi
done

if [ ${#missing_vars[@]} -gt 0 ]; then
    echo "❌ Missing required environment variables:"
    for var in "${missing_vars[@]}"; do
        echo "  - $var"
    done
    echo
    show_help
    exit 1
fi

# Set default values for optional variables
export BROWSER_URL="${BROWSER_URL:-http://chrome:7317}"
export WEB_SERVER_HOST="${WEB_SERVER_HOST:-localhost}"

echo "=== Environment Configuration ==="
echo "Browser URL: $BROWSER_URL"
echo "Web Server Host: $WEB_SERVER_HOST"
echo "Test Type: $TEST_TYPE"

# Navigate to integration test directory
cd integration-test/skills/

# Run tests based on type
echo "=== Running Atlassian OAuth Tests ==="
case "$TEST_TYPE" in
    "jira")
        echo "Running Jira OAuth tests..."
        go test -v -run TestJiraOAuthSkillsE2E -timeout 10m
        ;;
    "confluence")
        echo "Running Confluence OAuth tests..."
        go test -v -run TestConfluenceOAuthSkillsE2E -timeout 10m
        ;;
    "all")
        echo "Running all Atlassian OAuth tests..."
        go test -v -run "TestJiraOAuthSkillsE2E|TestConfluenceOAuthSkillsE2E" -timeout 15m
        ;;
    *)
        echo "❌ Invalid test type: $TEST_TYPE"
        echo "Valid options: jira, confluence, all"
        exit 1
        ;;
esac

echo "=== Atlassian OAuth Tests Complete ===" 