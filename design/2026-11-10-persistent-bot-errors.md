# Persistent bot errors

Date: 2026-11-10
Status: implemented locally; review and rollout pending

## Findings

The bot detail API derived `activation_error` from the newest failed
activation audit row. A later successful chat turn did not change that row,
so the UI kept showing an old timeout after the bot had recovered.

Meta bot `b-security-review-coordinator` is a separate live failure. Session
`ses_01m6nyxm7w3c5wee6f5k22pb5m` was blocked while starting the required
`helix-cyber` MCP server at `https://cyber.helix.ml/mcp`. The project
`prj_01m6nyxm3t6nvmzywbkvcnmsvt` has no authorization header and no OAuth
provider. Cyber requires a PocketBase user token; a Helix API key does not
authenticate this endpoint. The replacement turn created at 15:22 UTC was
reported as a startup error at 15:25 UTC after the 180-second gate.

## Fix

`Activations.LatestError` now checks the current bot session for the latest
genuine successful interaction. It qualifies only when the interaction is
complete, has a non-zero completion timestamp, and has no error. Its creation
time must be strictly after the failed activation ended. The activation audit
row remains unchanged. Synthetic model-handoff interactions marked complete
without a completion timestamp cannot clear an error. PostgreSQL and memory
stores share the `CompletedSuccessfully` interaction filter.

The desktop MCP readiness loop now probes again after its ten-second backoff
and reuses the existing idempotent startup-error callback while the dependency
is unavailable. This reports the current failure to replacement waiting turns,
while a 200 readiness response still releases the gate and launches the agent.

## Validation and limits

Focused activation, API, memory-store, server, and disposable PostgreSQL tests
passed. The shell regression and `bash -n` checks passed. No production
configuration, desktop restart, image rebuild, or live bot recovery was done.
The Meta bot still needs a valid Cyber PocketBase token configured before it
can run.
