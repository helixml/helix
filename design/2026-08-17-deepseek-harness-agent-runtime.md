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
| Composition | `desktop/shared/dsh/cordis.yml` | A repo file, so plugin choices are reviewable in a diff and testable outside the image |
| Launcher | `desktop/shared/dsh/dsh-acp` | A stable command name for `agent_servers`, and the thing that selects the private Node |
| Install | `Dockerfile.ubuntu-helix` (`dsh-build` stage) | npm tree + its own Node, copied root-owned into the runtime image |
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

**Its own Node interpreter.** dsh requires `^22.19.0 || >=24.0.0`; the desktop
image ships Node 20 for qwen-code and the MCP servers. Moving that shared Node
to satisfy one harness would put every other npm consumer on an untested
runtime, so dsh gets a private interpreter at `/opt/helix/dsh/bin/node`. It is
copied out of `node:24-bookworm-slim`, whose glibc is older than the runtime
image's — glibc is forward compatible, so the binary keeps working.

`node-pty` (a hard dependency of `dsh-subprocess-local`, the bash executor's
process plumbing) publishes no prebuild for Node 24 and is compiled in the
build stage; the toolchain does not reach the runtime image.

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
- `require('node-pty')` succeeds inside the built image, confirming the native
  module survived npm 11's blocked-lifecycle-scripts default

**Automated.** `cmd/settings-sync-daemon` (agent_servers shape, env contents,
localhost rewriting, and the three defer-without-credentials cases);
`pkg/server` runtime-selection table gains a `deepseek_harness` row;
`AgentHarness.test.tsx` asserts the official mark renders. Required builds
pass: `go build ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/
./api/cmd/settings-sync-daemon/` and `cd frontend && yarn build`.

## What is NOT verified

- **Never run under Zed.** Every turn above was driven by a hand-written ACP
  client, not by Zed's ACP client against a live spec task. The desktop image
  has not been rebuilt and no session has used this runtime.
- **The Helix proxy path is proven only up to the proxy.** Requests from dsh
  reached this dev stack's `/v1/chat/completions` and came back with a Helix
  error (`failed to check balance: … org_id not specified`) that every API key
  on the box reproduces via plain `curl` — a stack billing-config issue, not an
  integration one. The turns above therefore ran against `api.openai.com`
  through the same `llm-pi-ai` route. The Helix-specific hop that remains
  unexercised is provider-prefixed model routing under a working org.
- **No `--version`-style smoke test in the runtime layer**, only `node --version`.

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

## Version pinning

`DSH_VERSION` in `Dockerfile.ubuntu-helix` is an exact version, never a range:
upstream states dsh is in developer preview and that there **will** be
compatibility-breaking changes. Note also that the `@latest` dist-tag on the
plugin packages is a `0.0.1-rc.1` placeholder — real releases are published
under `@next` — which is why every package is requested at an explicit version
rather than a tag.

Unlike opencode, there is **no admin version-override setting**. opencode has
one because it ships a single self-contained binary with published digests;
dsh is a 15-package npm tree with a native module, so a runtime rollout would
mean running `npm install` and a native build inside a session container. If
that is wanted later it belongs in the image build, not in
settings-sync-daemon.
