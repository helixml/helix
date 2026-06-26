# Design: External MCP Server Connectivity and Long-Lived API Tokens for Agent Automation

## Summary

Both reported problems are, at the code level, **already supported**: Helix can
connect to external MCP URLs, and account `hl-` API keys never expire. The work
is therefore mostly **observability + UX + documentation** to make the existing
behavior visible and trustworthy, plus small targeted fixes so failures stop
being silent.

The two issues are independent and can ship separately.

---

## Part A — External MCP connectivity

### Root cause analysis

The "Helix can't egress to external MCP" conclusion is not supported by the code.
The MCP client (`api/pkg/agent/skill/mcp/mcp_client.go:37-116`) uses
`http.DefaultClient` with no IP/host filtering. The real, code-plausible reasons
an external MCP server sees **zero** requests:

1. **Discovery failed at app-save time, silently.**
   `tools/validation.go:170-187` calls `InitializeMCPClientSkill` when the app is
   saved. On failure it logs `log.Warn("...might not work during runtime")` and
   the app saves anyway with an **empty `Tools[]`**. At runtime the agent has no
   MCP functions, so it never connects → zero requests. This is the single most
   likely explanation and is invisible unless you read API-server logs.

2. **Transport mismatch.** `mcp_client.go:62-87` forces SSE when
   `Transport == "sse"` or the URL ends in `sse`; otherwise Streamable HTTP. A
   tunnel URL pointed at a server expecting the other transport fails to
   `initialize`.

3. **Genuine deployment egress block.** The connection is made from the **API
   server container** (`controller/inference_agent.go:164-166`), not the runner.
   An air-gapped/firewalled API server can't reach the public URL. This is a
   deployment fix, not a code fix.

### Design decisions

- **Do not add a localhost allowlist or "enable external MCP" flag.** There is
  nothing to unblock — external is already permitted. Adding a gate would be a
  regression for this use case.
- **Make discovery failure loud.** Return the discovery error/warning in the app
  save API response and render it in the app editor so the user sees "MCP server
  X unreachable: <error>" instead of a silently empty tool list.
- **Add a re-discovery / test-connection path.** A small endpoint that runs
  `InitializeMCPClientSkill` for a given MCP config from the API server and
  returns success + discovered tool count, or the exact error. This is the
  "confirm in 2 minutes" tool the team asked for, runnable without server-log
  access.
- **Improve runtime logging.** In `mcp_skill.go` / `mcp_client.go`, log
  connection start, target URL, chosen transport, and any init/call error at
  `Info`/`Error` so an operator can see the outbound attempt for a session.
- **Document the egress requirement and transport rules** in the docs repo.

### Key files (Part A)
- `api/pkg/agent/skill/mcp/mcp_client.go` — connection + transport selection.
- `api/pkg/agent/skill/mcp/mcp_skill.go` — skill registration + runtime
  `Execute` (add logging).
- `api/pkg/tools/validation.go:150-187` — config-time discovery; promote the
  `Warn` into a surfaced error/warning on the save response.
- `api/pkg/store/store_apps.go:291-308` — `AssistantMCP` → `ToolTypeMCP`
  conversion (note: drops stdio fields; HTTP/SSE only).
- `api/pkg/controller/inference_agent.go:164-166` — where MCP skills register
  during a session.
- App editor MCP section in `frontend/src/` — render the surfaced
  discovery error and a "test connection" button.

---

## Part B — Long-lived API tokens

### Root cause analysis

Account `hl-` API keys are persistent and have **no expiry**: the `ApiKey` type
(`types/types.go:907`) has only `Created`, and the auth middleware `hl-` branch
(`auth_middleware.go:184-243`) never checks expiry. So a true `hl-` key cannot
time out. A mid-session 401 therefore comes from one of:

- The automation used a **short-lived JWT/OIDC token** copied from the browser
  (Helix regular JWT = 7 days default `REGULAR_AUTH_TOKEN_VALIDITY`; OIDC =
  Keycloak-controlled, often much shorter). Both are accepted on the same Bearer
  header, so the mistake is easy and silent.
- The key was **regenerated** (the Regenerate button deletes the old key,
  invalidating it instantly for any other running process).
- An **ephemeral scoped key** (`SessionID`/`SpecTaskID`/`ProjectID` on `ApiKey`)
  was used; these are created internally for sessions/spec-tasks and are not
  durable personal credentials.

### Design decisions

- **The long-lived token IS the account `hl-` key** — no new token type is
  needed. The fix is to make this unambiguous and prevent accidental use of a
  JWT.
- **Clarify the account UI.** Label the persistent key, state it does not expire,
  visually separate it from session/browser auth, and add a copy snippet for
  `Authorization: Bearer hl-...` / `HELIX_API_KEY` (a snippet already exists;
  ensure it copies the `hl-` value, not a cookie/JWT).
- **Add a guard/warning against JWT-as-automation.** The middleware already
  detects stale Helix JWTs under OIDC (`auth_middleware.go` `looksLikeHelixJWT` /
  `ErrHelixTokenWithOIDC`). Ensure the 401 message for an expired Bearer JWT is
  clear ("this looks like a session token, not an `hl-` API key") so the failure
  mode is self-explaining.
- **Confirm the meta.helix.ml behavior.** Verify on the actual install whether
  `/account` surfaces an `hl-` key or an OIDC token; if it surfaces the wrong
  thing, that is the concrete bug to fix.
- **Do not introduce configurable expiry on `hl-` keys.** They are intentionally
  permanent; adding TTL would worsen this use case. Naming + multiple keys
  (already supported via `ApiKey.Name`) covers rotation needs.

### Key files (Part B)
- `api/pkg/types/types.go:907` — `ApiKey` (no expiry; confirms persistence).
- `api/pkg/server/auth_middleware.go:184-262` — `hl-` vs JWT/OIDC validation;
  improve the expired-JWT 401 message.
- `api/pkg/system/apikey.go` — `hl-` key generation.
- `api/pkg/controller/handlers.go:30-89` — create/delete API key.
- `frontend/src/components/account/ApiKeysSettings.tsx` /
  `frontend/src/services/userService.ts:86` — account UI + key fetch/copy.
- `api/pkg/config/config.go:564,578` — `AUTH_PROVIDER`,
  `REGULAR_AUTH_TOKEN_VALIDITY` (context only; not changed).
- Docs repo — "Long-lived API tokens for automation" page.

---

## Testing strategy
- **MCP discovery surfacing:** save an app with an unreachable MCP URL; assert the
  save response/UI shows the discovery error and `Tools[]` is empty with a
  visible warning. Save with a reachable mock MCP; assert tools populate and a
  session issues `initialize`/`tools/list`/`tools/call` to the mock.
- **Test-connection endpoint:** unit + integration test against a mock MCP server
  (reachable → tool count; unreachable → exact error string).
- **Transport selection:** unit-test that `*sse` URLs select SSE and others
  select Streamable HTTP.
- **Tokens:** integration test that an `hl-` key authenticates indefinitely (no
  expiry path), and that an expired JWT returns the clarified 401 message.
- **Docs:** verify the deployment-egress and long-lived-token pages render and are
  accurate.
