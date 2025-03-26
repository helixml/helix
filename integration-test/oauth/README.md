# OAuth Integration Tests

This directory contains integration tests for OAuth functionality in Helix, specifically focusing on validating that OAuth tokens are properly injected into API tools.

## What These Tests Verify

The primary test (`TestOAuthAPIToolIntegration`) verifies that:

1. When a user connects their OAuth account (e.g., GitHub), their tokens are correctly included in API requests made by API tools.

2. It tests the specific mechanism by which OAuth tokens are injected into API request headers.

## How It Works

The test creates a controlled environment to verify the OAuth token flow:

1. **Mock GitHub Server**: Sets up a test server that simulates GitHub's API and captures the Authorization header from incoming requests.

2. **Test User and OAuth Connection**:
   - Creates a test user (using Keycloak if available, or directly in the database if not)
   - Sets up an OAuth provider record pointing to our mock GitHub server
   - Creates an OAuth connection for the user with a test token

3. **Direct API Call Testing**:
   - Makes a direct call to the mock GitHub server with the OAuth token
   - Verifies that the Authorization header contains the correct Bearer token

This approach isolates the key functionality we're testing (token injection) from other aspects of the system.

## Running the Tests

### Prerequisites

- A running PostgreSQL database (localhost:5432 by default)
- Keycloak running (localhost:8080/auth, optional)

### Run the Test

```bash
cd /path/to/helix
go test -v -tags=integration ./integration-test/oauth/...
```

## What This Test Doesn't Verify

This test focuses specifically on the API tool OAuth token injection mechanism. It doesn't test:

1. The actual OAuth authentication flow (obtaining tokens from providers)
2. Token refresh mechanisms
3. The LLM integration and tool selection process

## Best Practices for API Tool OAuth Integration

Based on this test design, here are key points for properly handling OAuth tokens in API tools:

1. Ensure the `OAuthProvider` field in `ToolAPIConfig` exactly matches the OAuth provider name in the database.

2. Initialize the `Headers` map in `ToolAPIConfig` before attempting to add tokens.

3. Add the token as a standard Bearer token in the Authorization header. 