# Requirements — Notion Integration for Helix

## Problem

Users want to drive Helix agents from inside Notion — both **outbound** ("when content in Notion changes, run an agent") and **inbound** ("show the running agent inside the Notion page itself"). Today there is no Notion connector, no Notion-aware trigger, and the existing iframe embed (`/embed/task/:taskId`, used by Gatewaze — `helix/frontend/src/pages/EmbedTaskPage.tsx`, route at `helix/frontend/src/router.tsx:480`) hasn't been verified against Notion's embed pipeline (Iframely + frame-ancestors checks).

This is a **discovery + scoped MVP** task: prove the integration works end-to-end with the smallest credible slice, then decide what (if anything) to harden into a product surface.

## Scope

This task delivers a working **demo loop** plus a written-up assessment of the gaps before we'd take it to GA. Concretely:

1. **Notion → Helix trigger**: a webhook-driven trigger that fires a Helix agent when a watched page or database row changes in Notion.
2. **Helix → Notion embed**: a Helix task / spec-task page that renders cleanly inside a Notion `/embed` block.
3. **Findings doc** (`design/tasks/002021_investigate-notion/findings.md`, written during implementation): what works, what doesn't, recommended GA shape.

## User Stories

**1. Project manager driving agents from a Notion task list**
> As a PM with a Notion database of "AI tasks", I want to check a row's status (e.g. flip a `Status` property to `Ready for Helix`) and have Helix automatically pick up the row, run the configured agent, and write the result back as a comment on the page — so my team's planning surface (Notion) and execution surface (Helix) stay synced without copy-paste.

**2. Knowledge worker editing a spec inside Notion**
> As an engineer who keeps design notes in Notion, I want a button or saved view that runs a Helix agent against the current page's content (review, summarise, generate tasks) and posts results back as a child block / comment, so I don't have to leave Notion to interact with Helix.

**3. Embedding the live agent inside Notion**
> As a user who has triggered a Helix spec task from Notion, I want to paste the task URL into a Notion embed block and see the live Helix UI (logs, video stream of the desktop sandbox, planning artifacts) inside Notion — so the Notion page becomes a single-pane-of-glass for "request → agent doing the work → result".

**4. Operator wiring up the integration once for the workspace**
> As a workspace admin, I want to install a Helix Notion integration once (OAuth grant on selected pages/databases), then point Notion at a webhook URL Helix gives me, so individual users don't each have to manage tokens.

## Acceptance Criteria

### Notion → Helix (trigger)
- [ ] A new `Trigger.Notion` type alongside `Trigger.AzureDevOps` (`helix/api/pkg/types/types.go:1714`) — webhook-driven, configured per Helix App, gives the user a `WebhookURL` to paste into Notion's webhook subscription UI.
- [ ] A `NotionTrigger` config carries: the Notion `verification_token` (HMAC-SHA256 secret used to verify deliveries), an optional `OAuthConnectionID` to fetch full page content with, an optional `WatchPageID` / `WatchDatabaseID` filter, and a `PromptTemplate` describing how to format the change event for the agent.
- [ ] `webhookTriggerHandler` (`helix/api/pkg/server/webhook_trigger_handlers.go:14`) routes Notion-typed trigger configs into a new `notion.ProcessWebhook(ctx, triggerConfig, payload)` analogous to `azure.ProcessWebhook`.
- [ ] HMAC verification: reject requests where `X-Notion-Signature` doesn't match `HMAC-SHA256(verification_token, raw_body)`. Reject deliveries older than 5 min (replay protection) using the timestamp in the payload.
- [ ] At minimum the following Notion event types are handled and produce a sensible prompt:
  - `page.properties_updated` — fetch the page, compare to last-seen state cached in the trigger execution row, surface the changed property names + new values to the agent.
  - `page.content_updated` — fetch and pass the page content (or a diff against `last_edited_time`-filtered prior fetch).
  - `comment.created` — pass the comment body and let the agent reply.
- [ ] Aggregated webhooks (Notion debounces and may delay up to ~1 min) are de-duplicated: a follow-up event on the same page within N seconds replaces the queued execution rather than starting a second one.
- [ ] Optional: agent can write a reply back to the Notion page as a comment using the OAuth connection (gated behind a `ReplyAsComment` config flag — read-only by default).
- [ ] One `TriggerExecution` row per processed delivery, surfaced in the existing per-app trigger executions UI, including the Notion event type and page/database ID.

### OAuth provider
- [ ] A new `OAuthProviderTypeNotion = "notion"` value (`helix/api/pkg/types/oauth.go:13`).
- [ ] Notion-specific auth: `https://api.notion.com/v1/oauth/authorize` (with `owner=user`, `response_type=code`), token endpoint `https://api.notion.com/v1/oauth/token` (HTTP Basic auth with `client_id:client_secret`). User info parsed from `/v1/users/me`.
- [ ] Token refresh: Notion access tokens are long-lived; the refresh path is wired but expected to be a no-op for the MVP. Note this in the findings doc.
- [ ] Admin-installable from the `Providers` page (`helix/frontend/src/pages/Providers.tsx`) like the other custom providers.

### Helix → Notion (embed)
- [ ] The existing `/embed/task/:taskId` page renders correctly when loaded inside a Notion embed block (manual verification, screenshot in `screenshots/`).
- [ ] Server response headers do **not** set `X-Frame-Options: DENY/SAMEORIGIN` for the `/embed/*` route; `Content-Security-Policy: frame-ancestors` (if set anywhere) explicitly allows `https://*.notion.so` and `https://*.notion.site`. Default-deny embedding for non-`/embed/*` routes is preserved.
- [ ] The embed URL accepts `?access_token=` (already supported — `helix/frontend/src/hooks/useApi.ts:62`) so a user pasting the URL into Notion can authenticate the embedded view.
- [ ] Findings doc captures: Iframely behaviour (does Notion proxy our URL through Iframely?), whether the embed survives Notion page reloads, and whether the desktop video stream works inside Notion's frame.

### Findings doc (deliverable, written during implementation)
- [ ] What worked end-to-end vs what was a workaround.
- [ ] Notion API gaps we hit (e.g. no per-property diff in webhooks → forced to GET-and-compare).
- [ ] Whether `page.content_updated` granularity is enough for "trigger when text changes" — or if polling is mandatory.
- [ ] Recommendation for GA: stay webhook-only, add polling, or skip Notion altogether.
- [ ] Cost/rate-limit estimate (Notion default: 3 req/sec per integration).

## Out of Scope (this task)

- A first-party Notion app/listing in the Notion integration gallery (requires a security review at Notion's end — separate workstream).
- Notion as a **knowledge source** for RAG (different code path, `helix/api/pkg/rag/`); this task is about triggers + embed only.
- Polling fallback for fine-grained text-edit detection (called out in findings; deferred until we know whether webhooks alone are sufficient).
- Auto-installing the Notion integration on the user's workspace from inside Helix — for the MVP, the user runs through the standard Notion OAuth consent screen.
- Bidirectional sync of Helix tasks ↔ Notion database rows (one-shot trigger only).
- Toggle-block expand/collapse events: Notion does not expose these via the API, so they cannot be observed. Note in findings, do not attempt to work around.

## Open Questions

- Do we want one shared Notion **public integration** (Helix-owned, in the Notion directory) with per-workspace OAuth, or do we let each Helix admin register their own internal integration? The OAuth-provider abstraction can support both; the MVP picks one and the findings doc revisits.
- Where does the prompt template live — on the `TriggerConfiguration`, or on the `App`? The Azure DevOps trigger uses the App's system prompt; we'll mirror that unless we hit a reason not to.
