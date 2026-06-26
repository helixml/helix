# Requirements: External MCP Server Connectivity and Long-Lived API Tokens for Agent Automation

## Background

A project drives Helix programmatically: it uses an API key to call the Helix
API and spawn sessions against agents whose configuration (including MCP tools)
is passed in dynamically. Two problems block this workflow:

1. **External MCP servers appear unreachable.** An agent configured with an MCP
   server (`tool_type: mcp`) pointing at a public/external URL (e.g. a
   Cloudflare tunnel) never receives any requests during a session. Only
   `localhost`/internal MCP URLs appear to work. The team concluded Helix's
   inference runtime cannot egress to external MCP URLs.

2. **The automation API token expires mid-session.** A token generated from the
   account UI (`<install>/account`) started returning `401` partway through a
   run. The account UI offers no way to set an expiry, and there is no obvious
   way to mint a long-lived service token for automation.

## Research Findings (confirmed against the codebase)

These findings drive the requirements below and should be treated as established
facts, not assumptions.

### MCP connectivity — the "external is blocked" claim is FALSE in code

- The single MCP client (`api/pkg/agent/skill/mcp/mcp_client.go:37-116`) uses
  `http.DefaultClient` and connects to **whatever URL is configured**. There is
  **no** localhost restriction, SSRF guard, allowlist, or egress filter on the
  MCP path. (The only SSRF guard in the repo,
  `app_handlers.go:1920-1932`, applies solely to knowledge seed-zip downloads,
  not MCP.)
- MCP tool **discovery runs at app-save / validation time**
  (`api/pkg/tools/validation.go:170-187`). If the MCP server cannot be reached
  then, the failure is logged only as a `log.Warn(...)` ("might not work during
  runtime") and **the app still saves with an empty `Tools[]`**.
- At runtime the agent can only call MCP functions that were discovered and
  stored. With an empty `Tools[]`, the agent never opens a connection — which
  exactly matches the symptom "external MCP server receives zero requests."
- Transport is chosen by URL/flag: `Transport == "sse"` **or a URL ending in
  `sse`** forces SSE; everything else uses Streamable HTTP
  (`mcp_client.go:62-87`). A tunnel URL whose path mismatches the server's
  actual transport will fail to initialize.
- The connection originates from the **Helix API server process**
  (`controller/inference_agent.go:164-166`); the GPU runner has zero MCP code.
  So real egress restrictions, if any, live in the API server's network
  namespace (a deployment concern, not Helix logic).

### API tokens — account `hl-` keys do NOT expire

- Account-UI keys are persistent `hl-` API keys
  (`system/apikey.go`, `frontend/src/components/account/ApiKeysSettings.tsx`).
- The `ApiKey` type (`api/pkg/types/types.go:907`) has **no `ExpiresAt`/TTL
  field** — only `Created`. The auth middleware's `hl-` branch
  (`auth_middleware.go:184-243`) does a DB lookup with **no expiry check**.
  These keys cannot expire on their own.
- The tokens that DO expire are JWT/OIDC tokens: Helix regular-auth JWT
  (`REGULAR_AUTH_TOKEN_VALIDITY`, default 7 days) and OIDC/Keycloak access
  tokens (lifespan set in Keycloak, often minutes/hours). These are accepted on
  the same `Authorization: Bearer` header, so an automation that copied a
  browser JWT instead of the `hl-` key would 401 mid-run.
- There are also **ephemeral scoped keys** (`SessionID`/`SpecTaskID`/`ProjectID`
  on `ApiKey`) created internally for sessions/spec-tasks; these are short-lived
  by design and not meant as personal automation credentials.

## User Stories

### Story 1: Configure an agent with an external MCP server
**As** an automation developer,
**I want** to point an agent's MCP tool at a public/external URL and have the
agent actually connect to it during sessions,
**so that** I am not constrained to localhost-only MCP servers.

Acceptance criteria:
- [ ] It is documented and demonstrably true that external/public MCP URLs are
      supported by Helix (no code-level localhost restriction).
- [ ] When MCP tool discovery fails at app-save time, the failure is **surfaced
      to the user** (visible error/warning in the API response and app editor
      UI), not only written to a `Warn` log.
- [ ] A configured MCP server that is reachable produces a non-empty `Tools[]`
      and the agent connects to it during a session (the external server
      receives `initialize` / `tools/list` / `tools/call` requests).
- [ ] MCP connection attempts and failures during a session are logged with
      enough detail (URL, transport, error) to diagnose egress vs. transport vs.
      auth problems.
- [ ] Documentation explains the deployment requirement: the **API server
      container** must have network egress to the external MCP URL, and how to
      verify it.

### Story 2: Diagnose why an external MCP server gets zero requests
**As** an operator of a Helix install,
**I want** a clear, fast way to tell whether an external MCP failure is a Helix
config issue, a transport mismatch, or a deployment egress problem,
**so that** I can fix the right layer instead of guessing.

Acceptance criteria:
- [ ] A "test connection" / re-discovery action (or equivalent) verifies an MCP
      URL from the API server and reports success or the exact error.
- [ ] Transport selection behavior (SSE when URL ends in `sse`, else Streamable
      HTTP) is documented so URL/transport mismatches are avoidable.
- [ ] Guidance documents how to read API-server logs to confirm an outbound MCP
      attempt and its error (timeout / no route / refused / TLS / 4xx).

### Story 3: Issue a long-lived token for automation
**As** an automation developer,
**I want** to mint a long-lived, non-expiring token for application/service
interactions,
**so that** my automation does not 401 mid-session.

Acceptance criteria:
- [ ] Documentation clearly states that account `hl-` API keys are the
      long-lived, non-expiring credential for automation, and how to use them
      (`Authorization: Bearer hl-...` / `HELIX_API_KEY`).
- [ ] The account UI clearly distinguishes a persistent `hl-` API key from a
      short-lived session/browser token, and indicates that `hl-` keys do not
      expire (no misleading "expiry" expectation).
- [ ] Documentation warns that "Regenerate" immediately invalidates the old key
      (breaking any running automation), and that browser/OIDC JWTs must **not**
      be used as automation tokens.
- [ ] If `meta.helix.ml` runs OIDC, it is confirmed whether the account page
      actually surfaces an `hl-` key or an OIDC token, and any mismatch is fixed
      or documented.

## Out of Scope
- Changing Keycloak/OIDC token lifespans (configured in Keycloak, not Helix).
- Adding an SSRF allowlist or per-tenant egress policy for MCP (current code has
  no restriction; introducing one is a separate security decision).
- stdio-transport MCP for the inference runtime (the agent path is HTTP/SSE-only
  by design; store conversion drops stdio fields).
