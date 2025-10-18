# Atlassian OAuth Testing Guide

This guide explains how to run the Atlassian OAuth integration tests locally and debug common issues.

## Overview

The Atlassian OAuth tests (`TestJiraOAuthSkillsE2E` and `TestConfluenceOAuthSkillsE2E`) verify that the Helix OAuth integration works correctly with Jira and Confluence. These tests use browser automation to perform real OAuth flows against Atlassian's servers.

## Issue Fixed

Previously, the tests were failing in CI because they were getting stuck on Atlassian's MFA (Multi-Factor Authentication) page. The issue was:

1. **MFA not implemented**: The `AtlassianOAuthHandler` had stub implementations that didn't handle MFA
2. **CI triggers MFA**: The CI environment triggered MFA (likely due to IP address), while local development might not
3. **Test gets stuck**: The test expected authorization buttons but found MFA verification buttons instead

### Solution

The fix implements Gmail-based MFA handling in the `AtlassianOAuthHandler`:

1. **Gmail Integration**: Uses Gmail API to automatically read MFA codes from emails
2. **Multi-strategy approach**: 
   - First tries to skip MFA using various skip button selectors
   - Falls back to Gmail integration to read MFA codes from emails
   - Automatically enters codes into the browser
3. **Robust email parsing**: Handles multiple regex patterns for extracting Atlassian MFA codes
4. **Proper error handling**: Returns informative errors when MFA can't be handled

#### Technical Implementation

- **Gmail Service Setup**: Uses service account credentials with domain-wide delegation to impersonate `test@helix.ml`
- **Email Search**: Searches for emails from `no-reply@atlassian.com` with time filtering (last 10 minutes)
- **Code Extraction**: Multiple regex patterns to extract 6-character alphanumeric codes like "BRDV7G"
- **Browser Automation**: Automatically finds MFA input fields, enters codes, and submits forms

## Running Tests Locally

### Prerequisites

1. **Atlassian OAuth App**: You need an Atlassian OAuth app with proper configuration
2. **Test Account**: An Atlassian account for testing (MFA will be handled automatically)
3. **Gmail API Credentials**: Service account credentials for Gmail API access
4. **Environment Variables**: Set the required credentials

### Required Environment Variables

```bash
export ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_ID='your_client_id'
export ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_SECRET='your_client_secret'
export ATLASSIAN_SKILL_TEST_OAUTH_USERNAME='your_username'
export ATLASSIAN_SKILL_TEST_OAUTH_PASSWORD='your_password'
export ATLASSIAN_SKILL_TEST_CLOUD_ID='your_cloud_id'
export GMAIL_CREDENTIALS_BASE64='your_base64_encoded_gmail_credentials'
```

### Gmail API Setup

The tests require Gmail API credentials for reading MFA codes:

1. **Service Account**: Create a Google Cloud service account with Gmail API access
2. **Domain-wide Delegation**: Enable domain-wide delegation for the service account
3. **Impersonation**: The service account should be able to impersonate `test@helix.ml`
4. **Base64 Encoding**: Encode the service account JSON credentials to base64

### Running the Tests

Use the provided script to run the tests:

```bash
# Run both Jira and Confluence tests
./scripts/run-atlassian-oauth-tests.sh

# Run individual tests
go test -v -run TestJiraOAuthSkillsE2E ./integration-test/skills/
go test -v -run TestConfluenceOAuthSkillsE2E ./integration-test/skills/
```

## Common Issues and Solutions

### 1. MFA Code Not Found

**Error**: `could not find Atlassian MFA code in any recent emails`

**Possible Causes**:
- Gmail API credentials not configured correctly
- Email not arriving (check spam folder)
- Time filter too restrictive
- Wrong email address being monitored

**Solutions**:
- Verify Gmail API credentials and domain-wide delegation
- Check that emails are being sent to `test@helix.ml`
- Increase time filter window in the code
- Verify Atlassian account email matches the monitored address

### 2. Gmail API Authentication Errors

**Error**: `failed to setup Gmail service`

**Possible Causes**:
- Invalid service account credentials
- Missing domain-wide delegation
- Incorrect scope configuration

**Solutions**:
- Verify service account JSON is valid
- Enable domain-wide delegation in Google Cloud Console
- Ensure `gmail.readonly` scope is enabled
- Check that `test@helix.ml` can be impersonated

### 3. MFA Input Field Not Found

**Error**: `could not find Atlassian MFA input field`

**Possible Causes**:
- Atlassian changed their MFA page structure
- Different MFA method being used
- Page not fully loaded

**Solutions**:
- Update MFA input field selectors in the code
- Take screenshots to see the actual page structure
- Add delays for page loading
- Check if different MFA methods are being used

### 4. Network and Timeout Issues

**Error**: `context deadline exceeded` or `connection timeout`

**Possible Causes**:
- Network connectivity issues
- Atlassian services down
- Timeouts too aggressive

**Solutions**:
- Check internet connection
- Increase timeout values in the test configuration
- Verify that Atlassian services are accessible

## Test Architecture

### Components

1. **OAuth Handler** (`AtlassianOAuthHandler`): Handles MFA and 2FA challenges with Gmail integration
2. **Provider Strategy** (`AtlassianProviderStrategy`): Implements Atlassian-specific login flow
3. **Browser Automator** (`BrowserOAuthAutomator`): Generic browser automation framework
4. **Test Template** (`OAuthProviderTestTemplate`): Reusable test structure

### Flow

1. **Setup**: Creates test infrastructure (OAuth manager, API server, etc.)
2. **Provider Config**: Sets up Atlassian OAuth provider
3. **App Creation**: Creates Helix app with Jira/Confluence skills
4. **OAuth Flow**: Performs browser automation to get authorization code
5. **MFA Handling**: Automatically handles MFA using Gmail integration
6. **Token Exchange**: Exchanges authorization code for access token
7. **API Testing**: Tests agent sessions with real Atlassian API calls
8. **Cleanup**: Removes test resources

## Debugging Tips

### 1. Check Screenshots

Screenshots are saved to `/tmp/helix-oauth-test-results/` at each step. Use them to see exactly what the browser is doing.

### 2. Enable Verbose Logging

The tests use structured logging with zerolog. Check the log files for detailed information about what's happening.

### 3. Gmail Integration Debugging

- Check Gmail API quotas and limits
- Verify emails are being received at `test@helix.ml`
- Monitor email delivery times
- Check regex patterns match actual email content

### 4. Test Specific Pages

You can modify the test to stop at specific points or add additional screenshots for debugging.

### 5. Manual Testing

Try the OAuth flow manually in a browser using the same credentials to see if MFA is triggered.

## CI vs Local Differences

The main difference between CI and local environments is MFA triggering:

- **CI**: More likely to trigger MFA due to unfamiliar IP addresses
- **Local**: Less likely to trigger MFA from known development machines

The Gmail integration ensures both environments work reliably.

## Gmail Integration Details

### Email Patterns

The system searches for emails from `no-reply@atlassian.com` and extracts codes using these patterns:

- `\b([A-Z0-9]{6})\b` - 6-character alphanumeric code
- `code:\s*([A-Z0-9]{6})` - "code: BRDV7G"
- `following code:\s*([A-Z0-9]{6})` - "following code: BRDV7G"
- `enter the following code:\s*([A-Z0-9]{6})` - "enter the following code: BRDV7G"
- `verification code:\s*([A-Z0-9]{6})` - "verification code: BRDV7G"
- `\b([A-Z0-9]{4}-[A-Z0-9]{2})\b` - Alternative format like "BRDV-7G"

### Security Considerations

- Uses read-only Gmail API access
- Service account credentials are base64 encoded
- Only accesses the specific test email address
- Time-filtered searches to avoid processing old emails

## Future Improvements

1. **Multiple MFA Methods**: Support for different MFA providers (SMS, authenticator apps)
2. **Backup Strategies**: Alternative MFA handling methods
3. **Rate Limiting**: Handle Gmail API rate limits more gracefully
4. **Configuration**: Make email patterns and timeouts configurable 