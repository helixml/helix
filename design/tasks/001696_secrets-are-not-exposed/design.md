# Design: Secrets Not Exposed to Human Desktop

## Current State

Secrets are injected into desktop containers via `DesktopAgentAPIEnvVars()` in `api/pkg/external-agent/hydra_executor.go`:

```go
func DesktopAgentAPIEnvVars(apiKey string) []string {
    return []string{
        fmt.Sprintf("USER_API_TOKEN=%s", apiKey),
        fmt.Sprintf("ANTHROPIC_API_KEY=%s", apiKey),
        fmt.Sprintf("OPENAI_API_KEY=%s", apiKey),
        fmt.Sprintf("ZED_HELIX_TOKEN=%s", apiKey),
    }
}
```

These environment variables are visible to all processes in the container.

## Proposed Solution: API Proxy Authentication

**Principle:** Never pass secrets to the container. Instead, route all authenticated requests through the Helix API proxy which injects credentials server-side.

### Architecture

```
Desktop Container                    Helix API Server
┌─────────────────┐                 ┌─────────────────┐
│ AI Agent/MCP    │───────────────► │ API Proxy       │
│                 │  No auth header │ (injects token) │
│ env: (no keys)  │                 └────────┬────────┘
└─────────────────┘                          │
                                             ▼
                                    ┌─────────────────┐
                                    │ Anthropic/OpenAI│
                                    │ External APIs   │
                                    └─────────────────┘
```

### Key Changes

1. **Remove secret env vars from container**
   - File: `api/pkg/external-agent/hydra_executor.go`
   - Remove `USER_API_TOKEN`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` from env
   - Keep `ZED_HELIX_TOKEN` for WebSocket auth (already short-lived, session-scoped)

2. **Configure AI tools to use proxy endpoints**
   - Set `ANTHROPIC_BASE_URL=http://helix-api:8080/v1/proxy/anthropic`
   - Set `OPENAI_BASE_URL=http://helix-api:8080/v1/proxy/openai`
   - No API key needed in container; proxy adds it server-side

3. **Proxy injects auth based on session**
   - Proxy looks up session from `X-Helix-Session-ID` header
   - Retrieves user's API token from session store
   - Forwards request with `Authorization: Bearer {token}`

4. **License key: use mounted file instead of env**
   - Write license to `/run/secrets/helix_license` (tmpfs mount)
   - Nested Helix reads from file, not environment
   - File has restricted permissions (root:root, 0400)

### Decision: Keep ZED_HELIX_TOKEN

`ZED_HELIX_TOKEN` is needed for WebSocket connections to Zed IDE. This token:
- Is session-scoped and time-limited
- Cannot be used for API calls (different auth pathway)
- Exposure risk is lower than API keys

**Alternative considered:** WebSocket proxy with session-based auth. Rejected due to complexity and Zed client modifications required.

## Files to Modify

| File | Change |
|------|--------|
| `api/pkg/external-agent/hydra_executor.go` | Remove secret env vars, add proxy URLs |
| `api/pkg/server/proxy_handlers.go` | New proxy endpoints for Anthropic/OpenAI |
| `api/pkg/config/config.go` | Add proxy configuration |
| `for-mac/settings.go` | Update env var injection |

## Codebase Patterns Observed

- Existing proxy pattern: `api/pkg/server/openai_api.go` already proxies OpenAI-compatible requests
- Session lookup: `getSessionFromRequest()` in handlers retrieves session by ID
- Secret masking: `ServiceConnectionResponse.ToResponse()` shows how to mask sensitive fields

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking existing desktop agents | Feature flag for gradual rollout |
| Proxy latency | Already proxied; no additional hop |
| MCP servers needing direct API access | MCP servers use Helix API, not direct external calls |
