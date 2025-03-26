# OAuth Integration Tests

This directory contains integration tests for OAuth functionality in Helix, specifically focusing on validating that OAuth tokens are properly injected into API tools.

## What These Tests Verify

1. **OAuth Token Integration**: Verifies that when a user has connected their OAuth account (e.g., GitHub), their tokens are correctly included in API requests made by API tools.

2. **Token Flow**: Tests the complete flow from:
   - OAuth account connection
   - User creates an app with an API tool requiring OAuth
   - API tool makes a request that properly includes the OAuth Bearer token

## Prerequisites

- A running Postgres database
- Keycloak instance for authentication
- Local Helix API server

## Running the Tests

### 1. Set up your environment

Make sure you have a `.env` file with necessary configuration for local development.

### 2. Set the environment variables

```bash
export $(cat .env | xargs)
```

### 3. Run the tests

```bash
cd /path/to/helix
./stack test-integration
```

Or to run just the OAuth tests:

```bash
cd integration-test/oauth
go test -v -tags=integration ./...
```

## Test Structure

The integration test creates:

1. A mock GitHub API server to capture and validate OAuth headers
2. A test user in Keycloak and the database
3. An OAuth provider configuration for GitHub
4. An OAuth connection between the test user and GitHub
5. An app with a GitHub API tool
6. A session with the app
7. Sends a message that triggers the GitHub API tool
8. Verifies the OAuth token is included in the Authorization header

## Troubleshooting

If tests fail, check:

1. Database connection - ensure your database is running and accessible
2. Keycloak - verify Keycloak is running and configured correctly
3. API server - make sure the Helix API server is accessible on the configured port
4. Log output - examine log output for any errors in the OAuth token flow 