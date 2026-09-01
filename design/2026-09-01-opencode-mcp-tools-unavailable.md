# opencode: "Model tried to call unavailable tool 'helix-tasks_list_secrets'"

## Symptom (reported)

A spec task started by a helix-org bot could not call ANY MCP tool for 20+
minutes. Every call failed with:

```
The arguments provided to the tool are invalid: Model tried to call unavailable
tool 'helix-tasks_list_secrets'. Available tools: bash, chrome-devtools_click, ...
```

Affected `helix-tasks_*`, `kodit_*` and `ask_human`. The agent itself kept
working (LLM calls succeeded).

## Harness identification

The error text is not Helix's and not Zed's. It comes from **opencode**
(pinned at `OPENCODE_VERSION=1.18.18` in `sandbox-versions.txt`), which is the
`code_agent_runtime` both org bots in the test org use:

* `Model tried to call unavailable tool '<x>'. Available tools: <list>.` is the
  Vercel AI SDK's `NoSuchToolError`, thrown from `parseToolCall` when
  `tools[toolName] == null`.
* `The arguments provided to the tool are invalid: <err>` is opencode's builtin
  `invalid` tool, which is what a failed tool call is recorded as — this is why
  the reporter saw "the call is logged as arriving under tool name `invalid`".

MCP tool ids in opencode are `sanitize(server) + "_" + sanitize(tool)`
(`McpCatalog.toolName`), hence `helix-tasks_list_secrets`,
`kodit_kodit_repositories`, `chrome-devtools_click`.

## How opencode gets its MCP servers

Helix's opencode config (`api/cmd/settings-sync-daemon/opencode.go`) contains
**no** `mcp` section. The servers arrive over ACP instead:

1. `api/pkg/external-agent/zed_config.go` writes `context_servers` into Zed's
   `settings.json` (`helix-tasks`, `helix-session`, `helix-desktop`, `kodit`,
   plus stdio `chrome-devtools`).
2. Zed forwards them on `session/new` / `session/load` / `session/resume`
   (`crates/agent_servers/src/acp.rs::mcp_servers_for_project`).
3. opencode registers each one **once**, via `sdk.mcp.add(...)`.

## Root cause

opencode's MCP registration is one-shot and failure is silent + permanent:

* the ACP registration helper wraps each `mcp.add` in `Effect.ignore`, and
  records the server in a per-session "already registered" `Set`, so it is
  never retried for the life of the session;
* `MCP.create` logs `WARN "server unavailable" key=<name> type=remote
  status=failed` and stores `status: failed`;
* `MCP.tools()` skips any server that is not `connected`, so its tools never
  enter the AI SDK `tools` map;
* nothing tells the model. Helix's own prompts (e.g.
  `api/pkg/org/domain/seedprompts/seedprompts.go`) explicitly instruct the
  agent to call `list_secrets` / `get_secret`, so the model keeps calling tools
  that are not in the map → `NoSuchToolError`, forever.

Therefore **any transient failure to reach the Helix API during the few seconds
in which opencode boots strips every Helix MCP tool from the agent for the whole
session**, while the LLM path (which retries every turn) recovers and looks
healthy. That asymmetry is exactly the reported symptom.

Reproduced locally — `/home/retro/work/.opencode-state/opencode/log/opencode.log`:

```
level=WARN message="server unavailable" key=helix-desktop type=remote status=failed
level=WARN message="server unavailable" key=helix-session type=remote status=failed
level=WARN message="server unavailable" key=kodit        type=remote status=failed
level=WARN message="server unavailable" key=helix-tasks  type=remote status=failed
```

...after which the agent ran on with zero Helix MCP tools. `chrome-devtools`
(stdio, no network) was unaffected — matching the reporter's observation that
`chrome-devtools_*` was in the available list.

## Ruled out

* **Missing `tools` capability in the `initialize` response.** Probed all three
  Helix MCP endpoints with a real session-scoped key; every one advertises
  `tools: {listChanged: true}`.
* **Registry/dispatcher disagreement.** The AI SDK builds `availableTools` from
  `Object.keys()` of the *same* object it just failed to index, and opencode
  never stores an `undefined` value in it. The reporter's claim that the name
  appeared in the printed list is not reliable (the quoted list was elided).

## Local environment note

The first local reproduction was aggravated by version skew in this dev stack:
`helix-sandbox-nvidia-1` was 9 days old and predates the sandbox egress work
(`602e8c55a`, `9b287f71e`, `a7ce030bc`, 2026-08-29..31) that introduced
`helix-api.internal:18080`, while the API had hot-reloaded to current `main`
and was emitting that hostname. Rebuilt with `./stack build-sandbox`. The
reporter's branch (`df768fe61`, 2026-08-26) predates that change, so their
failure had a different trigger — but the same permanent-failure mechanism.

## Fix

The agent's MCP registration is one-shot, so the addresses we write into
`settings.json` must be proven reachable *before* Zed launches the agent.
Nothing checked this: `start-zed-core.sh` gated only on `settings.json`
existing, so a container whose context-server origin did not resolve booted
happily into a tool-less session.

1. `api/cmd/settings-sync-daemon/mcp_readiness.go` — after the initial sync,
   the daemon extracts the distinct `scheme://host:port` of every
   URL-addressed `context_server` **from the settings it just merged** and
   probes each one (unauthenticated `GET /api/v1/config`, no side effects),
   retrying for 60s. Any HTTP status counts: only a transport error is a
   failure, because that is the only thing that breaks MCP registration.
   Success writes `/tmp/helix-mcp-endpoints-ready`; failure writes
   `/tmp/helix-mcp-endpoints-unreachable` with the failing URL and calls the
   existing `reportAgentStartupError`, so it surfaces in the session UI.

   It reads the URLs back out of the settings rather than trusting
   `HELIX_API_URL` on purpose: those two can disagree (a control plane that has
   rolled forward to `helix-api.internal:18080` against an older sandbox host
   that still sets `HELIX_API_URL=http://api:8080` and never pins the proxy
   hostname). In that state Zed's websocket and inference both work over the
   env address and *only* the MCP servers are dead — the exact failure being
   fixed, and one a `HELIX_API_URL` probe would miss.

2. `desktop/shared/start-zed-core.sh` — new `wait_for_mcp_endpoints` step
   between `wait_for_zed_config` and the Zed launch. Waits for the ready
   marker; on the unreachable marker it prints the recorded reason and exits,
   matching the existing `wait_for_zed_config` fatal behaviour.

### Verified

* Healthy container (`ubuntu-external-01m1e14w0ffjmqbx8v62y37dkk`, image
  `bb8589`): daemon logs `MCP readiness: all 1 context server origin(s)
  reachable`, `wait_for_mcp_endpoints` proceeds, Zed starts, and opencode logs
  no `server unavailable` — the MCP servers connect.
* Same container, `helix-api.internal` pointed at a dead port while the daemon
  syncs over the gateway IP — i.e. the exact skew that produced the original
  failure: the probe fails, the reason (with the failing URL) is recorded, and
  the gate exits 1 instead of starting a tool-less agent.
* Same container with the API unreachable entirely: the initial sync fails,
  `helixSettings` is nil, and the gate refuses to declare readiness rather than
  passing vacuously (regression test:
  `TestVerifyMCPEndpoints/unsynced_settings_never_mark_ready`).
* `go test ./cmd/settings-sync-daemon/` green.

### What this does and does not cover

Covers: the agent never starts into a session whose MCP origins are
unreachable, and if they are, the operator gets the failing URL immediately
instead of a silent 20-minute tool outage.

Does not cover: an MCP server that fails to register even though its origin
answered (e.g. the API dies in the few seconds between the probe and
`session/new`). Repairing that requires either a retry inside the agent or
Zed re-pushing `mcpServers` to a live ACP session
(`crates/agent_servers/src/acp.rs::mcp_servers_for_project` is only called from
`session/new|load|resume`). Both are outside this repo.

### Also seen, not fixed here

`opencode` fails its plugin bootstrap on every start with
`EACCES: permission denied, open '/home/retro/.npm/_cacache/...'` while
fetching `@opencode-ai/plugin` (46 occurrences across the historical session
logs on this host). Unrelated to the MCP surface, but it is a real permissions
defect in the desktop image.
