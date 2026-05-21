# Requirements: Integrate Goose AI Agent into Zed via ACP

## Background

Goose (now hosted by AAIF, originally by Block) is an open-source AI agent
that natively supports the Agent Client Protocol (ACP) — the same protocol
Zed uses to talk to Claude Code and Qwen Code in Helix today. Adding Goose
gives Helix users another agent option without re-inventing any of the Zed
plumbing.

The Goose CLI exposes ACP via `goose acp` (stdio JSON-RPC). Configuring it
in Zed is a single `agent_servers` entry; provider/model are overridden per
instance with `GOOSE_PROVIDER` and `GOOSE_MODEL` env vars.

Helix already routes each user's chosen runtime through
`api/cmd/settings-sync-daemon/main.go`, which writes
`~/.config/zed/settings.json` for the active session. Adding Goose follows
the same `case "goose_code":` pattern as Qwen and Claude Code.

## User Stories

### US-1: As a Helix user, I want to choose Goose as my code agent
So that I can use Goose's tools and extensions from inside Zed in my Helix
session, the same way I can already use Qwen Code or Claude Code today.

**Acceptance Criteria**
- Project Settings → Code Agent Runtime offers "Goose" alongside the
  existing options (Zed Agent, Qwen Code, Claude Code).
- Onboarding flow lets a new user pick Goose with no extra steps.
- When Goose is selected, opening Zed shows a "Goose" thread option in
  the agent panel and a fresh thread starts a working Goose session
  bound to the user's configured LLM provider/model.

### US-2: As an operator, I want Goose pre-installed in the desktop image
So that sessions start instantly with no per-session download.

**Acceptance Criteria**
- `goose --version` works inside a fresh `helix-ubuntu` container.
- The installed version is pinned (`GOOSE_VERSION`) so rebuilds are
  reproducible and CI doesn't break on upstream releases.
- Goose telemetry/auto-update are disabled (consistent with how Qwen and
  Gemini are configured in `Dockerfile.ubuntu-helix`).

### US-3: As a Helix user, my LLM provider/model selection drives Goose
So that I don't have to configure Goose separately — it picks up the
provider, model, base URL, and API key Helix already knows about.

**Acceptance Criteria**
- `GOOSE_PROVIDER` and `GOOSE_MODEL` are derived from `CodeAgentConfig`
  and written into the Zed `agent_servers.goose.env` block by the
  settings-sync-daemon.
- An API key supplied by Helix (proxied via `BaseURL`) is forwarded to
  Goose via the provider's expected env var (`OPENAI_API_KEY`,
  `ANTHROPIC_API_KEY`, …).
- Localhost rewriting (`rewriteLocalhostURL`) is applied to base URLs so
  Goose can reach the Helix API proxy from inside the container.

### US-4: As an operator, I need a decision on Sway
The user explicitly asked: "do we need to install goose in helix-ubuntu
(and sway, although increasingly I'm thinking we should just delete sway)".

**Acceptance Criteria**
- This task installs Goose in `helix-ubuntu` only.
- `helix-sway` is left untouched in this PR. A separate task can either
  delete `Dockerfile.sway-helix` + `desktop/sway-config/` outright, or
  mirror the Goose install. Recommendation in `design.md` is to delete
  Sway in a follow-up — it's already gated behind
  `HELIX_EXPERIMENTAL_DESKTOPS` and is not part of the production path.

## Out of Scope

- Deleting `helix-sway` (separate task — surface the recommendation only).
- A Helix-hosted Goose extension registry or recipe library.
- Custom Goose extensions / MCP server bundling beyond what Helix already
  installs for Zed (`chrome-devtools-mcp`, `server-github`, Kodit, etc.).
  Goose auto-picks up MCPs declared in Zed's `context_servers`, so the
  existing set is reused for free.
- Goose Desktop (Electron app) — Helix users interact with Goose through
  Zed; the Desktop app would compete for the window manager and isn't
  needed.

## Notes for Future Agents

- The Goose docs live at `https://goose-docs.ai`. The ACP-clients page
  (`/docs/guides/acp-clients`) is the authoritative integration reference.
- Goose's source repo moved to `github.com/aaif-goose/goose`.
- Goose currently works best with Claude 4 models (per upstream docs), but
  any provider it supports (OpenAI, Gemini, OpenRouter, Tetrate, Ollama)
  can be wired through Helix's existing OpenAI-compatible proxy by setting
  `GOOSE_PROVIDER=openai` + `OPENAI_BASE_URL=<helix-proxy>`.
