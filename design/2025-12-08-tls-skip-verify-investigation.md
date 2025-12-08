# TLS Skip Verify Investigation for Enterprise Deployments

**Date**: 2025-12-08
**Status**: Investigation Complete, Fix Applied

## Problem Statement

Customer on Helix 2.5.25 reported TLS certificate errors when using a database-configured inference provider (Qwen), even though `TOOLS_TLS_SKIP_VERIFY=true` was set:

```
ERR app/api/pkg/openai/openai_client.go:256 > failed to list models from OpenAI compatible API
error="failed to send request to provider's models endpoint: Get \"https://internal-server.customer.example:8010/v1/models\":
tls: failed to verify certificate: x509: certificate signed by unknown authority"
```

Notably, MCP tool calls with the same TLS skip verify setting DID work, indicating the environment variable was being loaded correctly.

## Code Flow Analysis

### For Database-Configured Providers (Web UI)

1. `provider_handlers.go:getProviderModels()` calls `providerManager.GetClient()`
2. `GetClient()` checks `globalClients` first (not found for user-configured "Qwen")
3. Falls through to `initializeClient()` for database-configured provider
4. `initializeClient()` creates client with:
   ```go
   openaiClient := openai.NewWithOptions(apiKey, endpoint.BaseURL, endpoint.BillingEnabled, openai.ClientOptions{
       TLSSkipVerify: m.cfg.Tools.TLSSkipVerify,
   }, endpoint.Models...)
   ```
5. `NewWithOptions()` creates http.Client with Transport if TLSSkipVerify is true
6. `listOpenAIModels()` uses `c.httpClient.Do(req)` to make the request

### The 2.5.25 Code

```go
// In NewWithOptions (2.5.25):
httpClient := &http.Client{
    Timeout: 5 * time.Minute,
}

if opts.TLSSkipVerify {
    httpClient.Transport = &http.Transport{
        TLSClientConfig: &tls.Config{
            InsecureSkipVerify: true,
        },
    }
}

config.HTTPClient = &openAIClientInterceptor{
    Client:      *httpClient,  // COPY by value
    rateLimiter: rateLimiter,
    baseURL:     baseURL,
}

return &RetryableClient{
    apiClient:      client,
    httpClient:     httpClient,  // Original pointer
    ...
}
```

## Key Observations

1. **TLS fix IS in 2.5.25**: Verified via `git merge-base --is-ancestor 23b70a061 2.5.25` - the commit that added TOOLS_TLS_SKIP_VERIFY support is an ancestor of 2.5.25.

2. **Code path is correct**: The `initializeClient()` function correctly passes `m.cfg.Tools.TLSSkipVerify` to `NewWithOptions()`.

3. **No race condition**: `cfg.Tools.TLSSkipVerify` is only read, never written after initialization.

4. **MCP uses different pattern**: MCP modifies `http.DefaultClient` globally:
   ```go
   httpClient := http.DefaultClient
   if d.TLSSkipVerify {
       httpClient.Transport = &http.Transport{...}
   }
   ```
   This is a side effect that affects all users of `http.DefaultClient`, but the OpenAI client creates its own `http.Client` instance.

5. **No diagnostic logging in 2.5.25**: The 2.5.25 code had NO logging when initializing clients for database-configured providers. We cannot verify what value `TLSSkipVerify` actually had at runtime.

## Potential Root Causes (Unconfirmed)

1. **Environment variable not correctly loaded**: Though customer believes it was set, we have no logs to confirm the actual value at runtime.

2. **Transport handling edge case**: The 2.5.25 code creates a minimal `&http.Transport{}` which might have subtle issues compared to cloning `http.DefaultTransport`.

3. **Struct embedding by value**: The `openAIClientInterceptor` embeds `http.Client` by value, and we copy `*httpClient` into it. While the Transport interface value should be preserved, there could be edge cases.

## Fixes Applied

### 1. Use Clone() for Transport (Already in HEAD, not in 2.5.25)

```go
// Current HEAD code:
if opts.TLSSkipVerify {
    transport := http.DefaultTransport.(*http.Transport).Clone()
    transport.TLSClientConfig = &tls.Config{
        InsecureSkipVerify: true,
    }
    httpClient.Transport = transport
}
```

This preserves default settings (proxy, timeouts, connection pooling) while adding InsecureSkipVerify.

### 2. Added Diagnostic Logging (This Commit)

Added logging at multiple points to diagnose future TLS issues:

- **initializeClient()**: Logs TLS config when creating database-configured provider clients
- **openAIClientInterceptor.Do()**: Logs Transport state before making requests
- **listOpenAIModels()**: Logs Transport state and detailed TLS error messages

Example log output on TLS error:
```
LISTMODELS TLS CERTIFICATE ERROR - If tls_skip_verify_configured=false,
TOOLS_TLS_SKIP_VERIFY env var was not set or not applied to this client
```

### 3. Clear Error Messages

TLS errors now include:
- Transport type (nil vs *http.Transport)
- Whether InsecureSkipVerify was configured
- Clear remediation instructions

## Testing Recommendations

1. Deploy updated version with logging
2. If TLS errors occur, check logs for:
   - `tls_skip_verify=true/false` at startup
   - `tls_skip_verify_configured=true/false` at request time
   - Transport type (`*http.Transport` vs `nil`)

3. If `tls_skip_verify_configured=false` at request time but `true` at startup, there's a bug in config propagation.

## Files Modified

- `api/pkg/openai/openai_client.go`: Added TLS diagnostic logging
- `api/pkg/openai/manager/provider_manager.go`: Added logging for database-configured providers

## Related Commits

- `23b70a061`: feat: add TOOLS_TLS_SKIP_VERIFY support for LLM clients
- `f050ab4fa`: fix: improve TLS skip verify support for enterprise deployments
- (This commit): Added diagnostic logging for TLS issues
