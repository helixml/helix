# DeepSeek Harness (`dsh`) as a code agent runtime

Adds `deepseek_harness` alongside `claude_code` / `codex_cli` / `qwen_code` /
`goose_code` / `opencode` / `zed_agent` as a selectable coding harness, with a
mark and label in the org **Providers → Coding harnesses** policy surface.

Upstream: <https://github.com/deepseek-ai/deepseek-harness>. Structurally this
follows the opencode change (`https://github.com/helixml/helix/pull/3018`),
with one significant difference described below.

## The one structural difference from every other harness

Every other harness we ship exposes ACP as a subcommand of its product CLI:
`claude-code-acp`, `codex acp`, `goose acp`, `opencode acp`, `qwen --experimental-acp`.

**`dsh` does not.** Its published CLI (`@deepseek-ai/dsh`) offers `dsh web`,
`dsh --profile <name>`, `dsh --profile headless "job"`, and `dsh plugin` — and
nothing else. The ACP server is a *separate cordis composition*: the transport
plugin `@deepseek-ai/dsh-acp` mounted by the app package
`@deepseek-ai/dsh-acp-demo`, whose bin speaks JSON-RPC on stdio and is
configured by a `cordis.yml` naming every plugin the agent should have (LLM
adapter, sandbox, bash executor, filesystem, approval policy, compaction,
model-facing tools).

So the integration owns a composition file, not just a command line:

| Piece | Lives at | Why there |
|---|---|---|
| Manifest | `desktop/shared/dsh/package.json` + `package-lock.json` | The exact plugin tree, so `npm ci` is reproducible |
| Composition | `desktop/shared/dsh/cordis.yml` | A repo file, so plugin choices are reviewable in a diff and testable outside the image |
| Launcher | `desktop/shared/dsh/dsh-acp` | A stable command name for `agent_servers`, and the one place the composition path is spelled |
| Install | `Dockerfile.ubuntu-helix` (`dsh-build` stage) | npm tree incl. a natively-built node-pty, copied root-owned into the runtime image |
| Per-session values | `api/cmd/settings-sync-daemon/deepseek_harness.go` | Base URL, model, API key, `$DSH_HOME`, sessions root |

The composition is static and env-driven. A configuration change is a YAML
diff someone can read, not a Go string builder — which is the opposite of the
opencode approach (`OPENCODE_CONFIG_CONTENT`, a marshalled struct) and is
justified by dsh's config surface being an order of magnitude larger.

## Three settings are load-bearing

**`llm-pi-ai` route with a hand-declared model.** Every provider Helix exposes
is reached through the one OpenAI-compatible proxy endpoint, so there is a
single route and the model keeps its provider prefix (`openai/gpt-5`) — that
prefix is what the proxy routes on. `apiKeyEnv: HELIX_API_KEY` is a credential
*reference* resolved per request, so no secret is written into the composition
or a session log.

**`sandbox-policy: danger-full-access` + `approval: never`.** A headless spec
task has nobody to answer `session/request_permission`; under the narrower
`workspace-write` mode the agent would stall on its first write outside the
workspace until the turn timed out. The container *is* the sandbox — one
session, one workspace — so this is dsh's equivalent of `--yolo` for qwen and
`permission: "allow"` for opencode.

**Node 24 across the image.** dsh requires `^22.19.0 || >=24.0.0`; the desktop
image shipped Node 20, which is also out of maintenance, so the shared
interpreter moves to the Node 24 LTS line rather than dsh carrying a private
one. Every Node consumer in the image moves with it: Qwen Code,
chrome-devtools-mcp, the GitHub MCP server, the Drone CI MCP server, and the
Claude Code / Codex ACP wrappers Zed bootstraps through npm.

Qwen Code is still *built* against `node:20-slim` and only runs on the image's
interpreter. Its three bundled native modules — `@rollup/rollup-linux-x64-gnu`,
`@lydell/node-pty-linux-x64`, `@teddyzhu/clipboard-linux-x64-gnu` — were each
loaded under Node 24 (`process.dlopen`, MODULE_VERSION 137) before the bump and
all succeeded, so they are N-API and ABI-independent.

`node-pty` (a hard dependency of `dsh-subprocess-local`, the bash executor's
process plumbing) publishes no prebuild for Node 24 and is compiled in the
`dsh-build` stage; the toolchain does not reach the runtime image. That native
module is the one thing a future Node major bump would silently break, so the
runtime layer `require()`s it as a build gate: the stage's Node major and the
image's must agree, and the build fails loudly if they drift.

## MCP servers cannot ride ACP for this harness

Zed puts the project's context servers in `session/new.mcpServers`
(`mcp_servers_for_project`, zed `crates/agent_servers/src/acp.rs:4601`). dsh's
ACP transport **rejects a non-empty list outright**:

```
session/new params: { cwd: '/home/retro/work', mcpServers: [ [Object] x3 ] }
-32602 Invalid params: mcpServers is not supported
```

Session creation fails, Zed surfaces nothing to the UI, and the task simply
hangs. This is stated in the upstream README ("empty `additionalDirectories`
and `mcpServers` are accepted, non-empty values reject") and was still missed
here, because every pre-Zed probe passed `mcpServers: []`.

Zed has no per-agent filter and stdio MCP carries no ACP capability bit to gate
on, so the only Helix-side lever is to withhold the servers:
`contextServersForZed()` returns an empty map for this runtime alone.

The capability is **moved, not dropped**. `writeDeepSeekHarnessMCPConfig`
renders the same servers as `@deepseek-ai/dsh-mcp-client` loader entries — one
per server, Zed's `command` shape becoming stdio and its `url` shape becoming
streamable-http — into a JSON file the composition pulls in with
`@deepseek-ai/cordis-plugin-include`. The model sees the same
`mcp__<server>__<tool>` names it would have either way.

Three details are load-bearing:

- **The include path is a literal, not `!!js process.env.…`.** The loader
  resolves an include's `path` *without* evaluating js tags, so an env
  reference arrives as `""` and fails with `extension "" not supported`. The
  path is therefore duplicated between `cordis.yml` and
  `DeepSeekHarnessMCPConfigPath`, and the comments on both point at each other.
- **The file is written 0400, via temp + rename.** `cordis-plugin-include`
  writes entries back when the file is writable; read-only keeps the daemon the
  sole writer, and the atomic install means the agent never reads a partial
  file.
- **It is rewritten even when there are no servers.** A stale file from a
  previous agent would otherwise leak that agent's servers *and its bearer
  tokens* into this session.

The residual cost of this route: Zed's own agent panel in a dsh session has no
MCP tools, because `context_servers` is shared state. Fixing that properly
means teaching Zed not to send `mcpServers` to agents that cannot take them,
which is a cross-repo change.

## Verification

**Live, against the artifacts that ship.** The `dsh-build` Docker stage was
built, the repo's `cordis.yml` and `dsh-acp` wrapper copied in, and a real ACP
turn driven over stdio by a minimal client:

- `initialize` → `protocolVersion: 1`, `agentInfo: deepseek-harness-acp`
- `session/new` → session id
- `session/prompt` "read marker.txt and reply with its contents" → the agent
  used its own filesystem tool and returned the file's exact contents,
  `stopReason: end_turn`
- a second run outside Docker wrote a file and ran `cat` through the bash
  executor; the file was verified on disk afterwards, so the tools really ran
  rather than being described
- with a real Helix `helix-session` MCP server mounted through the include
  file, the agent called `mcp__helix-session__current_session` and returned the
  live session id — a value only the MCP server could supply, so the mounted
  MCP path is confirmed rather than merely loading without error
- `require('node-pty')` succeeds inside the built image, confirming the native
  module survived npm 11's blocked-lifecycle-scripts default; this is now a
  build-time gate rather than a manual check

**Automated.** `cmd/settings-sync-daemon` (agent_servers shape, env contents,
localhost rewriting, and the three defer-without-credentials cases);
`pkg/server` runtime-selection table gains a `deepseek_harness` row;
`AgentHarness.test.tsx` asserts the official mark renders. Required builds
pass: `go build ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/
./api/cmd/settings-sync-daemon/` and `cd frontend && yarn build`.

## What is NOT verified

- **No successful turn under Zed yet.** Two live spec tasks have run. The
  first, on a stale image, failed with `Custom agent server 'dsh' is not
  registered`. The second registered and spawned the agent correctly, then
  died on the `mcpServers` rejection above. Both failures are fixed, but a
  green Zed→dsh turn has not been observed.
- **The Helix proxy path is proven only up to the proxy.** Requests from dsh
  reached this dev stack's `/v1/chat/completions` and came back with a Helix
  error (`failed to check balance: … org_id not specified`) that every API key
  on the box reproduces via plain `curl` — a stack billing-config issue, not an
  integration one. The turns above therefore ran against `api.openai.com`
  through the same `llm-pi-ai` route. The Helix-specific hop that remains
  unexercised is provider-prefixed model routing under a working org.
- **Non-dsh Node consumers are unexercised under Node 24.** qwen-code's native
  modules were checked directly, but chrome-devtools-mcp, the GitHub and Drone
  MCP servers, and the Claude Code / Codex ACP wrappers have only been built,
  not run, on the new interpreter.

## Known limitations of the upstream ACP server

These are properties of `@deepseek-ai/dsh-acp`, which describes itself as
automation-only, and they are worse than what the other harnesses give Zed:

- **No streaming.** Only *committed* assistant messages go on the wire, one
  `agent_message_chunk` per block at end of turn. Zed will show nothing until
  the turn finishes.
- **No tool-call updates.** Tool activity, reasoning, plans, and titles stay
  off the protocol, so Zed's thread view shows the answer with no visible work.
- **No `session/load`.** Fresh sessions only — resume, fork, and list are
  unsupported.
- **Text-only prompts.** A hand-declared model has no catalogue entry, so
  `promptCapabilities.image` is false and an image prompt is refused before it
  is sent. Declaring `defaultInput: [text, image]` would over-claim for models
  whose endpoint does not serve images, which fails mid-turn *after* the
  message is durable — a worse failure than a refusal.
- **No reasoning effort.** A hand-declared model declares no
  `reasoningEfforts`, so no reasoning parameter is sent and the provider's own
  default applies. Helix's per-model effort table
  (`api/pkg/model/reasoning_efforts.go`) is not plumbed into the composition;
  sending a level a model rejects is a hard 400 that aborts the turn (see the
  `qwen3.8-27b` case in CLAUDE.md), so sending nothing is the safe default.

## Version pinning is a lockfile, not a version string

Naming exact versions in the Dockerfile is **not** enough, and this bit us
mid-development. `dsh-acp-demo` pulls its plugin set in as *peer*
dependencies, which npm resolves by range (`^0.1.0-rc.6`). When upstream cut
`0.1.0-rc.7`, npm satisfied that peer with rc.7 while our explicit pins stayed
at rc.6, and the install died on a conflict between the two:

```
peer @deepseek-ai/dsh-user-approval@"^0.1.0-rc.7" from @deepseek-ai/dsh-acp@0.1.0-rc.7
  peer @deepseek-ai/dsh-acp@"^0.1.0-rc.6" from @deepseek-ai/dsh-acp-demo@0.1.0-rc.6
```

The first build had only passed because it ran before rc.7 was published, and
the second reused a cached layer — so the pinning looked like it worked right
up until a layer was invalidated. That is the worst kind of non-reproducible
build.

`desktop/shared/dsh/package.json` + `package-lock.json` now pin the whole
181-package tree, peers included, and the image installs with `npm ci`, which
either reproduces that tree exactly or fails. To move dsh: edit the versions in
`package.json`, run `npm install` there to refresh the lock, and re-test the
composition with a live turn — upstream is in developer preview and states
there **will** be compatibility-breaking changes.

Note also that the `@latest` dist-tag on the plugin packages is a `0.0.1-rc.1`
placeholder; real releases are published under `@next`, which is why the
manifest names explicit versions rather than a tag.

Unlike opencode, there is **no admin version-override setting**. opencode has
one because it ships a single self-contained binary with published digests;
dsh is a 15-package npm tree with a native module, so a runtime rollout would
mean running `npm install` and a native build inside a session container. If
that is wanted later it belongs in the image build, not in
settings-sync-daemon.
