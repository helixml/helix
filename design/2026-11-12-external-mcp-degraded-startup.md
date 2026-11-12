# External MCP degraded startup

Date: 2026-11-12
Status: implemented locally; review and rollout pending

## Finding

Meta bot `b-security-review-coordinator` could not start because its
project-level `helix-cyber` MCP connection returned HTTP 401. The previous
external-agent startup path treated that upstream failure as fatal, so the
bot waited for the MCP readiness deadline and then failed the waiting turn.
The user-facing error exposed the timeout, internal proxy URL, and nested HTTP
500/401 details instead of explaining what the user could fix.

## Fix

The external MCP proxy now starts in a degraded mode when the upstream client
cannot be created or its tool list cannot be loaded. Session ownership,
session-scoped identity headers, and configuration lookup checks remain in
place. A degraded proxy is a valid stateless MCP server with a
`helix_connection_status` tool and model-visible instructions. The instructions
tell the bot which connection is unavailable, distinguish rejected credentials
from a general connection failure, keep unrelated work moving, and avoid
claiming dependent work completed.

Raw startup errors remain in server logs. The degraded instructions and status
responses omit the internal URL, HTTP status chain, and response body.

Connection setup is bounded to three seconds. A successful client stops that
timer and keeps its background context alive for the normal cached connection;
the timeout does not tear down healthy SSE or Streamable HTTP clients.
Degraded proxies are not cached, so a later request retries the upstream
connection and can recover after credentials or service health are fixed.
Healthy status checks probe the existing client with a bounded context without
cancelling it. If the connection recovers after the coding agent initialized
in degraded mode, the agent still needs to reconnect so the restored tool
schemas are registered.

## Recovery

For the already-failed Meta request, after repairing the Cyber credential or
service, use `Restart` on the failed message in the bot chat queue. The session restart path recreates the agent
container while preserving the conversation and workspace. Do not use the
bot-page `Restart org bot` action for this recovery; that action creates a new
bot session and can discard the current conversation context.

A newly degraded bot may have no failed queue entry. Its preserving reconnect
operation is `POST /api/v1/sessions/{id}/restart-agent`; this change does not add
a new UI action for that operation or restart active work automatically.

## Validation and limits

The full server and prompt test packages passed. Focused regressions were
rerun after the final copy edit. Coverage includes rejected
credentials, continued degraded status, tool-list failures, bounded setup,
successful client lifetime, readiness `OPTIONS` handling, recovery, upstream
status-tool name collisions, diagnostic redaction, and session-owner
isolation. The core settings-sync MCP readiness tests also passed. A
negative-control run with the owner check removed failed the owner-isolation
regression, confirming that the authorization guard is exercised.

No Meta deployment, production configuration change, credential update,
desktop image rebuild, or live bot recovery was performed. The exact wording
shown in a live bot response remains model-generated and was not observed in a
live run.
