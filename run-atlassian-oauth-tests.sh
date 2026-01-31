#!/bin/bash

set -e

# Script to run Atlassian OAuth integration tests locally
# This script helps set up the environment and run the tests

echo "=== Atlassian OAuth Integration Tests ==="
echo

# Check if environment variables are set
echo "Checking required environment variables..."
required_vars=(
    "ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_ID"
    "ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_SECRET"
    "ATLASSIAN_SKILL_TEST_OAUTH_USERNAME"
    "ATLASSIAN_SKILL_TEST_OAUTH_PASSWORD"
    "ATLASSIAN_SKILL_TEST_CLOUD_ID"
)

missing_vars=()
for var in "${required_vars[@]}"; do
    if [ -z "${!var}" ]; then
        missing_vars+=("$var")
    fi
done

if [ ${#missing_vars[@]} -gt 0 ]; then
    echo "ERROR: The following environment variables are required but not set:"
    for var in "${missing_vars[@]}"; do
        echo "  - $var"
    done
    echo
    echo "Please set these environment variables and try again."
    echo "Example:"
    echo "  export ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_ID='your_client_id'"
    echo "  export ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_SECRET='your_client_secret'"
    echo "  export ATLASSIAN_SKILL_TEST_OAUTH_USERNAME='your_username'"
    echo "  export ATLASSIAN_SKILL_TEST_OAUTH_PASSWORD='your_password'"
    echo "  export ATLASSIAN_SKILL_TEST_CLOUD_ID='your_cloud_id'"
    echo
    exit 1
fi

echo "✓ All required environment variables are set"
echo

# Set up other required environment variables for testing
export OPENAI_API_KEY="${OPENAI_API_KEY:-dummy_key_for_testing}"
export POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
export POSTGRES_USER="${POSTGRES_USER:-postgres}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
export POSTGRES_DATABASE="${POSTGRES_DATABASE:-postgres}"

# Test type selection
TEST_TYPE="${1:-both}"
case "$TEST_TYPE" in
    jira|confluence|both)
        echo "Running $TEST_TYPE tests..."
        ;;
    *)
        echo "Usage: $0 [jira|confluence|both]"
        echo "  jira       - Run only Jira OAuth tests"
        echo "  confluence - Run only Confluence OAuth tests"  
        echo "  both       - Run both Jira and Confluence OAuth tests (default)"
        exit 1
        ;;
esac

echo

# Function to run a specific test
run_test() {
    local test_name="$1"
    local test_display_name="$2"
    
    echo "=== Running $test_display_name OAuth Test ==="
    
    # Use the stack command to run the test
    if ./stack test -v integration-test/skills/*.go -run "${test_name}"; then
        echo "✓ $test_display_name OAuth test PASSED"
    else
        echo "✗ $test_display_name OAuth test FAILED"
        return 1
    fi
    
    echo
}

# Run the selected tests
case "$TEST_TYPE" in
    jira)
        run_test "TestJiraOAuthSkillsE2E" "Jira"
        ;;
    confluence)
        run_test "TestConfluenceOAuthSkillsE2E" "Confluence"
        ;;
    both)
        run_test "TestJiraOAuthSkillsE2E" "Jira"
        run_test "TestConfluenceOAuthSkillsE2E" "Confluence"
        ;;
esac

echo "=== All selected tests completed ===" 