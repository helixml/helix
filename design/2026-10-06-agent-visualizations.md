# Agent Visualizations (inline HTML renders) in AgentChat

Date: 2026-10-06
Status: implementing

## Goal

Let a coding agent (Zed Agent, Claude Code, Codex, Qwen, Goose, …) answer with a
self-contained **HTML page** — a chart, table, diagram, collage, or mockup —
rendered **inline in the AgentChat timeline**, above its final text reply.
Reverse-engineered from T3 Code's "visual replies" feature (pingdotgg/t3code
PRs #15968 and #16283).

## How T3 does it (summary)

1. An MCP tool `html_render` takes a complete HTML document + title + height.
2. The server injects a small **bootstrap** into the document `<head>`: a
   `<style id>` with themed CSS custom properties and a `<script>` that (a) reads
   the active theme from the URL fragment before first paint, (b) follows live
   theme changes posted via `postMessage` (MCP-Apps `ui/notifications/host-context-changed`),
   (c) reports its real content height back to the host (`ui/notifications/size-changed`),
   and (d) routes link clicks to the host (`ui/open-link`) instead of navigating.
3. The prepared page is stored as a thread attachment; the client shows it in a
   **sandboxed iframe** (`sandbox="allow-scripts allow-forms"`, *no*
   `allow-same-origin`, so the page runs in an opaque origin and cannot touch
   the app's session/storage). The frame auto-sizes to the page.
4. Theme variables mirror the app's own design tokens, so pages match light/dark
   and custom themes live.

## Helix implementation

Mirror the shape, reuse existing Helix infrastructure, add no new service.

### Server (`api/pkg/visualization`, new pure package)

- `bootstrap.go`: the ported theme/bootstrap logic — `Theme`, default light/dark
  palettes, `InjectBootstrap(html)`, clamp helpers, tool-name + guide constants,
  and the `Reference` struct. Pure and unit-tested; no deps on server/store.
- The bootstrap is injected at **publish time** so the stored file is
  self-contained and renders themed even on a direct open.

### Server (`mcp_backend_session.go`)

- Add an `html_render` tool to the existing **session MCP backend**, which is
  already wired to every external-agent session as the `helix-session` context
  server (`zed_config.go`). So every harness gets the tool automatically with no
  per-harness change.
- Handler: resolve current session + owner from context, validate/clamp inputs,
  inject the bootstrap, write the HTML to the session's filestore folder at
  `visualizations/<viz_id>.html`, and return a tool-result string carrying a
  machine-readable marker the frontend can find regardless of how each harness
  wraps tool output:

  ```
  HELIX_VISUALIZATION_V1 {"id":"viz_…","title":"…","height":480}
  ```

  (The marker is robust to ACP/harness formatting differences; we do not parse
  free-form tool-call content.)

- `NewSessionMCPBackend` gains the `*controller.Controller` (for `Filestore` +
  `GetFilestoreSessionPath` + presign config).

### Server serving endpoint

- `GET /api/v1/sessions/{id}/visualizations/{viz_id}` on `subRouter` (same auth
  surface as `getSession`: bearer / `access_token` cookie / `access_token` query
  — so an iframe loads it with the SPA's cookie). Authorizes via
  `authorizeUserToSession(ActionGet)`, reads the stored HTML from filestore, and
  serves it as `text/html` with `X-Content-Type-Options: nosniff` and a
  restrictive CSP. Isolation is primarily the client-side iframe sandbox.
- `deleteSession` best-effort removes the session's `visualizations/` folder so
  pages are deleted with the thread.

### Frontend

- `visualizationTheme.ts`: map the active MUI/Helix theme → the CSS variables the
  bootstrap expects; build the URL fragment and the `host-context-changed`
  message; readers for `size-changed` / `open-link`.
- `VisualizationFrame.tsx`: the sandboxed iframe (theme via fragment +
  postMessage, auto-size from size-changed, link open on host), sized to the
  reported content height.
- `InteractionInference.tsx`: detect completed `tool_call` entries whose content
  carries the `HELIX_VISUALIZATION_V1` marker, parse the reference, and render a
  `VisualizationFrame` inline instead of the normal tool-call row.

## Scope

- v1 ships `html_render` (inline display) only. T3's `html_preview`
  (headless-Chrome self-check screenshot) is deliberately **out of scope** — it
  requires bundling a Chrome for Testing download and a public-egress proxy. The
  bootstrap's live `size-changed` reporting means the client fits the frame to
  the page without the server pre-measuring heights, so preview is not needed
  for correct rendering. Tracked as a follow-up.

## Availability (which agents can publish)

Rendering is universal — spec-task detail, the org-bot chat panel, and ordinary
chat all go through the same `AgentChat → EmbeddedSessionView → Interaction →
InteractionInference` path, so a published visualization shows inline everywhere.

Whether an agent *has* the `html_render` tool depends on whether its Zed config
includes the `helix-session` context server (which hosts the tool):

| Surface | Gets `html_render`? | Why |
|---|---|---|
| Spec tasks | **Yes, automatic** | `GenerateZedMCPConfig` always wires `helix-session`; spec-task sessions have no `BotInstance`, so `ApplyBotInstanceProfile(nil)` strips nothing. |
| Ordinary chat sessions | **Yes, automatic** | Same base config, no instance profile. |
| Org bots | **Opt-in** | Org-bot instances run `ApplyBotInstanceProfile`, which deletes every context server not in `profile.MCPServers`. `DefaultBotInstanceProfile()` keeps only `chrome-devtools`. |

To enable visualizations for an org bot, add `helix-session` to the bot's
instance-profile `MCPServers` (e.g. via `helix org bots profile`/the instance
profile editor, or `--mcp …,helix-session` in `bots_build`). This is consistent
with the org design philosophy ("instances are minimal by default"); we do not
auto-grant it. Note that enabling `helix-session` also grants the session-
navigation tools (`session_toc`, `search_session`, …); there is no way to grant
only `html_render` today. A future split onto its own always-wired context
server would decouple the two — tracked as a follow-up.

## Security notes

- The iframe is sandboxed without `allow-same-origin`; the page cannot read the
  app origin's cookies/storage or make credentialed same-origin requests.
- Helix's SPA auth is bearer/cookie, and the serving route authorizes the
  session, so one user cannot load another's visualization.
- Pages may still load remote http(s) resources (CDN libs/images), as in T3.
