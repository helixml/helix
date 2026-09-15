# Headless ACP MCP startup race

## Evidence

A newly dispatched headless PR review task, created by `b-pr-coordinator`
under its bound project, started OpenCode without the `helix-tasks` tools. Its
OpenCode session exposed only local tools, despite the bot/project binding
that supplies the task's Helix tool surface. The coordinator is now configured
to dispatch `ubuntu-desktop` tasks as a temporary mitigation.

## Cause and fix

Zed builds the MCP server list sent in ACP `session/new` in
`mcp_servers_for_project`. For HTTP servers, it previously required a runtime
configuration from `configuration_for_server`. The context-server manager
populates that state asynchronously, so a headless session could start first
and receive no HTTP MCP servers. OpenCode registers the servers during session
startup; omitted servers remained unavailable for that session.

Zed commit `07198ce107c94b95a39a158a7dc148dc26387a12` reads configured HTTP
server URLs and headers directly from project settings when building the ACP
list. The existing runtime configuration path remains for stdio servers.
Regression test `mcp_servers_include_configured_http_servers_before_running`
checks that an HTTP server is included with its URL and authorization header
before runtime configuration exists.

## Rollout and validation

Bump `ZED_COMMIT` in `sandbox-versions.txt` to the fix, then run
`./stack build-zed release` and `./stack build-ubuntu`. Start a new headless PR
review task and verify its OpenCode session exposes and can call `helix-tasks`
tools. Keep the coordinator on `ubuntu-desktop` until that end-to-end check
passes.

The live fix has not yet been verified: the Zed commit must first be included
in a rebuilt sandbox image and exercised by a new headless task.
