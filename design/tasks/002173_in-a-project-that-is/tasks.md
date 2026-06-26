# Implementation Tasks: External MCP Server Connectivity and Long-Lived API Tokens for Agent Automation

## Part A — External MCP connectivity

- [ ] Confirm on the live install: during a session against the external-MCP agent, check API-server logs for the outbound MCP `initialize`/`tools/list` attempt and capture the exact error (timeout / refused / TLS / 4xx).
- [ ] Inspect the saved app's `tools[]` / `assistant.MCPs[].Tools` for the agent — verify whether `Tools[]` is empty (proves discovery failed at save time) vs. populated (proves a runtime/egress issue).
- [ ] Surface MCP discovery failures: change `api/pkg/tools/validation.go:170-187` so an `InitializeMCPClientSkill` error is returned to the caller (or attached as a visible warning on the save response) instead of only `log.Warn`.
- [ ] Render the MCP discovery error/warning in the app editor MCP section (`frontend/src/`) so the user sees "MCP server unreachable: <error>" and that no tools were discovered.
- [ ] Add a "test MCP connection" / re-discovery endpoint that runs `InitializeMCPClientSkill` for a given MCP config from the API server and returns success + tool count or the exact error; wire a button in the app editor.
- [ ] Add runtime connection logging in `api/pkg/agent/skill/mcp/mcp_client.go` and `mcp_skill.go` (target URL, chosen transport, init/call errors) so per-session MCP attempts are observable.
- [ ] Document transport selection rules (URL ending in `sse` → SSE, else Streamable HTTP) and how to avoid mismatches.
- [ ] Document the deployment egress requirement: the **API server container** must reach the external MCP URL; include a verification command (e.g. exec into the API container and curl the tunnel URL).
- [ ] Add/verify an integration test: reachable mock MCP populates `Tools[]` and a session issues `initialize`/`tools/list`/`tools/call`; unreachable URL yields a surfaced error and empty `Tools[]`.

## Part B — Long-lived API tokens

- [ ] Confirm on the live install whether `<install>/account` surfaces a persistent `hl-` key or an OIDC/JWT token; record the result.
- [ ] Verify the failing automation token's prefix — `hl-` (persistent, cannot expire) vs. a JWT (expires). This pins the actual root cause.
- [ ] Clarify the account UI (`frontend/src/components/account/ApiKeysSettings.tsx`): label the persistent key, state that `hl-` keys do not expire, and ensure the copy snippet copies the `hl-` value (not a cookie/JWT).
- [ ] Improve the expired-Bearer-JWT 401 message in `api/pkg/server/auth_middleware.go` to say it looks like a session token, not an `hl-` API key (guide users to the account key).
- [ ] Add a warning in the account UI/docs that "Regenerate" instantly invalidates the old key and will break running automation.
- [ ] Write a docs page "Long-lived API tokens for automation": use the account `hl-` key as `Authorization: Bearer hl-...` / `HELIX_API_KEY`; do not use browser/OIDC JWTs; rotate by creating a second named key before deleting the old one.
- [ ] Add an integration test asserting an `hl-` key authenticates with no expiry path, and an expired JWT returns the clarified 401.

## Cross-cutting

- [ ] Update relevant docs in `/home/retro/work/docs/` for both MCP egress and long-lived tokens.
- [ ] If diagnosis confirms a platform egress block, file/confirm the deployment fix for the API server container's outbound network access (separate from code changes).
