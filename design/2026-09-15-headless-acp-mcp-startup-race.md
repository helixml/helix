# Headless ACP MCP startup race

## Evidence

A newly dispatched headless PR review task, created by `b-pr-coordinator`
under its bound project, started OpenCode without the `helix-tasks` tools. Its
OpenCode session exposed only local tools, despite the bot/project binding
that supplies the task's Helix tool surface. The coordinator is configured to
dispatch `ubuntu-desktop` tasks as a temporary mitigation.

## Cause and fix

Zed builds the MCP server list sent in ACP `session/new` in
`mcp_servers_for_project`. For HTTP servers, it previously required runtime
configuration from `configuration_for_server`. The context-server manager
populates that state asynchronously, so a headless session could start first
and receive no HTTP MCP servers. OpenCode registers the servers during session
startup; omitted servers remained unavailable for that session.

The first fix was merged in [Zed PR #97](https://github.com/helixml/zed/pull/97),
but its stale base would have delivered a Zed revision 21 commits behind fork
main. [PR #98](https://github.com/helixml/zed/pull/98) reverted it. The same
fix was reapplied from current fork main in [PR #99](https://github.com/helixml/zed/pull/99),
merged as `b47763fe69497f009524d7e21db3aae83497e9d8`.

The merged fix reads configured HTTP server URLs and headers directly from
settings when building the ACP list. The existing runtime configuration path
remains for stdio servers. Regression test
`mcp_servers_include_configured_http_servers_before_running` checks that an
HTTP server and its authorization header are included before runtime
configuration exists.

## Rollout and validation

Pin `ZED_COMMIT` in `sandbox-versions.txt` to the merged fix, then run
`./stack build-zed release` and `./stack build-ubuntu`. Start a new headless PR
review task and verify its OpenCode session exposes and can call `helix-tasks`
tools. Keep the coordinator on `ubuntu-desktop` until that end-to-end check
passes.

Live headless validation has not passed yet: the merged Zed commit must be
included in a rebuilt sandbox image and exercised by a new headless task.
