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

### Server (`mcp_backend_visualization.go`)

- An `html_render` tool on a dedicated **visualization MCP backend**, wired into
  every agent config as the `helix-viz` context server (`zed_config.go`). Every
  harness gets the tool from the same config with no per-harness change. See
  Availability below for why it is not a `helix-session` tool.
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

### Server serving endpoint

- `GET /api/v1/sessions/{id}/visualization?viz_id=…` on `subRouter` (same auth
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

Rendering is universal — spec-task detail, the org-bot chat panel, embedded
chats and ordinary chat all go through `AgentChat → EmbeddedSessionView →
Interaction → InteractionInference`.

`html_render` lives on its **own** MCP backend (`/api/v1/mcp/visualization`),
wired into every agent config as the `helix-viz` context server. It started out
as a tool on `helix-session`, but org-bot instances strip every context server
their profile doesn't list (`DefaultBotInstanceProfile()` keeps only
`chrome-devtools`), so bots could not visualize — and enabling `helix-session`
for a bot would also have granted session navigation. Found on a live
chief-of-staff session whose served config was `[chrome-devtools, helix]`; the
agent fell back to writing a local HTML file and screenshotting it.

| Surface | Gets `html_render`? | Mechanism |
|---|---|---|
| Spec tasks, ordinary chat | Yes | `GenerateZedMCPConfig` always adds `helix-viz` |
| Org bots | Yes | `ApplyBotInstanceProfile` always keeps `helix-viz` |

Auth surfaces that had to change with it:

- **Bot-instance keys** are fail-closed per MCP backend
  (`auth_bot_instance_key.go`). `visualization` is allowed only when the request
  names the key's own `session_id`. This must ship with the config change: the
  sandbox readiness gate OPTIONS-probes every remote context server before
  launching Zed and fails on 401/403, so a denied `helix-viz` would stop every
  bot instance from starting.
- **Embed keys** (`auth_embed_key.go`) allow `GET
  /api/v1/sessions/{id}/visualization` for the key's own session only. The
  serving route takes `viz_id` as a query parameter so the suffix-based embed
  matcher can scope it.
- The settings-sync-daemon treats `helix-viz` as Helix-owned so a stale on-disk
  entry can never override the API's config (takes effect with the next
  `build-ubuntu`; not needed for the server to work).

A bot instance serves untrusted users and reads untrusted pages, so a
prompt-injected bot can publish attacker-chosen HTML into its own chat. It runs
in the sandboxed opaque-origin iframe and can only open links on a real user
click — no capability beyond what the bot's chat text already has.

## Security notes

- The iframe is sandboxed without `allow-same-origin`; the page cannot read the
  app origin's cookies/storage or make credentialed same-origin requests.
- Helix's SPA auth is bearer/cookie, and the serving route authorizes the
  session, so one user cannot load another's visualization.
- Pages may still load remote http(s) resources (CDN libs/images), as in T3.
