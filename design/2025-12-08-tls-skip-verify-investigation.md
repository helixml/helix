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

## Root Cause Confirmed

**Helm values file misconfiguration - `extraEnv` was at root level instead of under `controlplane:`**

The customer's Helm values file had:
```yaml
# WRONG - at root level (ignored by Helm template)
extraEnv:
- name: "TOOLS_TLS_SKIP_VERIFY"
  value: "true"
- name: "MOONLIGHT_CREDENTIALS"
  value: "helix"
```

But the Helm chart expects:
```yaml
# CORRECT - under controlplane section
controlplane:
  extraEnv:
  - name: "TOOLS_TLS_SKIP_VERIFY"
    value: "true"
```

The template at `charts/helix-controlplane/templates/deployment.yaml:322` uses `.Values.controlplane.extraEnv`:
```yaml
{{- with .Values.controlplane.extraEnv }}
{{- toYaml . | nindent 12 }}
{{- end }}
```

Root-level `extraEnv` is completely ignored, so the environment variable was never set on the pod.

### Why MCP Appeared to Work

The `MOONLIGHT_CREDENTIALS` setting appeared to work because there's a fallback default in the code:
```go
// api/pkg/server/moonlight_proxy.go:426-435
func (apiServer *HelixAPIServer) getMoonlightCredentials() string {
    creds := os.Getenv("MOONLIGHT_CREDENTIALS")
    if creds == "" {
        creds = "helix"  // Default fallback
    }
    return creds
}
```

The customer was setting it to `"helix"`, which is already the default - so it worked by accident, not because the config was being applied.

### Earlier Theory (Disproven)

Initially suspected the Clone() fix was missing from 2.5.25, but testing with self-signed certificates proved the 2.5.25 code would have worked correctly IF the environment variable had been set. The code was never the issue - it was the Helm configuration.

## Customer Fix

**Correct the Helm values file indentation:**
```yaml
controlplane:
  extraEnv:
  - name: "TOOLS_TLS_SKIP_VERIFY"
    value: "true"
```

## Defensive Code Improvements (Not the Fix, But Good Practice)

### 1. ALWAYS Clone Transport

Even when TLSSkipVerify is false, we now ALWAYS clone the default transport:

```go
// Always clone first to preserve all default settings
transport := http.DefaultTransport.(*http.Transport).Clone()

// Only modify TLS config if skip verify is enabled
if opts.TLSSkipVerify {
    transport.TLSClientConfig = &tls.Config{
        InsecureSkipVerify: true,
    }
}

// Create client with the pre-configured transport (never nil)
httpClient := &http.Client{
    Timeout:   5 * time.Minute,
    Transport: transport,
}
```

This ensures:
- Transport is NEVER nil (no reliance on implicit DefaultTransport behavior)
- Proxy settings from HTTP_PROXY/HTTPS_PROXY are always respected
- Connection pooling and keep-alive settings are always inherited
- Enterprise network configurations work correctly

### 3. Added Diagnostic Logging (This Commit)

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
