# Requirements — Notion Integration for Helix

## Problem

Users want to drive Helix agents from inside Notion — both **outbound** ("when content in Notion changes, run an agent") and **inbound** ("show the running agent inside the Notion page itself"). Today there is no Notion connector, no Notion-aware trigger, and the existing iframe embed (`/embed/task/:taskId`, used by Gatewaze — `helix/frontend/src/pages/EmbedTaskPage.tsx`, route at `helix/frontend/src/router.tsx:480`) hasn't been verified against Notion's embed pipeline (Iframely + frame-ancestors checks).

This is a **discovery + scoped MVP** task: prove the integration works end-to-end with the smallest credible slice, then decide what (if anything) to harden into a product surface.

## Scope

This task delivers a working **demo loop** plus a written-up assessment of the gaps before we'd take it to GA. Concretely:

1. **Notion → Helix trigger**: a webhook-driven trigger that fires a Helix agent when a watched page or database row changes in Notion.
2. **Helix → Notion embed**: a Helix task / spec-task page that renders cleanly inside a Notion `/embed` block.
3. **Findings doc** (`design/tasks/002021_investigate-notion/findings.md`, written during implementation): what works, what doesn't, recommended GA shape.

## End-to-end onboarding (the user's-eye view)

The integration touches three surfaces — Helix (where the trigger config lives), Notion's OAuth consent (one-time install), and the Notion database itself (column setup + Automation). The onboarding wizard in Helix walks the user through all three. **Helix is not in the Notion integration catalog**; we use a Helix-owned **public Notion integration** that's installed via a direct OAuth URL we hand the user. (Notion permits this — public integrations are fully functional before/without catalog listing; catalog listing is a separate marketing surface.)

The script:

**One-time per workspace (Helix admin):**
1. In Helix, on a Helix App's **Triggers** tab, click **Add Trigger → Notion**. Helix shows the wizard.
2. **Wizard step 1 — Connect to Notion.** Click "Connect to Notion". Browser redirects to Notion's standard OAuth consent screen, where the user picks which pages/databases to share with Helix. (Notion grants are page-level, per-user — the customer admin shares the specific database they want Helix to manage; child pages of that database come along.) Notion redirects back to Helix with the OAuth code; Helix exchanges it for a long-lived token and stores an `OAuthConnection` row.
3. **Wizard step 2 — Pick the database.** Helix calls `POST /v1/search` filtered to databases — only the databases the user just shared appear in the dropdown. User picks one.
4. **Wizard step 3 — Pick the action column and option mapping.** Helix fetches the database schema and lists every `select` and `status` column. User picks one (e.g. `Go/NoGo`). Helix then lists that column's options and the user picks: "create" option (e.g. `Go`) and "cancel" option (e.g. `NoGo`). User can also pick optional `Prompt` and `Result` rich-text columns from the same dropdown (or leave empty). Helix saves the mapping.
5. **Wizard step 4 — Pick the target Helix project.** Spectasks created from this database land in this project.
6. **Wizard step 5 — Wire the Notion Automations.** Helix shows: a webhook URL, a generated shared secret, and step-by-step instructions (with screenshots) to create **two automations** in Notion (or one with two branches if Notion supports it — verify in discovery):
   - Open the database → click the ⚡ icon top-right → "+ New automation".
   - **Automation A (create):** trigger = "When `<action column>` is set to `<create option>`" (e.g. `Go`); action = "Send webhook" → paste Helix URL; headers = `X-Helix-Webhook-Secret: <secret>` and `X-Helix-Source: notion-automation` and `X-Helix-Action: create`; body = (the action column's value, the Prompt column if configured, page ID auto-included).
   - **Automation B (cancel):** trigger = "When `<action column>` is set to `<cancel option>`" (e.g. `NoGo`); same URL/secret; header `X-Helix-Action: cancel`.
   - **Optionally**, add a `Run on Helix` Button-property column with action "Send webhook", same URL, `X-Helix-Source: notion-button`, `X-Helix-Action: create`. Per-row "kick off now" button.
7. The wizard's **Test setup** button POSTs a synthetic Automation payload (both `create` and `cancel` directions) through the URL so the user can confirm wiring without touching real rows.

**Recurring (anyone in the team):**
8. In Notion, the team uses the database as they always have. When a row is ready, flip `Go/NoGo` from `NoGo` (or unset) to `Go`. Notion fires Automation A → Helix creates a spectask in the configured project, appends a Helix `embed` block to that row's page body. Click into the row → see the live Helix UI rendered inline.
9. When the agent finishes, Helix writes the result summary into the `Result` column (if configured). The embed block stays in place; clicking it shows the finished task. **Helix never touches the `Go/NoGo` column** — the user owns that.
10. To stop a running spectask, flip `Go/NoGo` back to `NoGo`. Notion fires Automation B → Helix cancels the spectask and best-effort removes the embed block.

**Note on who-installs-what:** the Helix admin runs steps 1–7 once. Step 8 onward is everyday team use — no further Helix touchpoint per row. The OAuth grant and Notion Automation persist across rows.

**Alternative auth mode (documented for completeness, not the default):** if the customer prefers not to OAuth into a third-party Helix integration, they can create a **Notion internal integration** in their own developer portal, share the database with it manually, and paste the static token into Helix's wizard step 1 instead of clicking "Connect to Notion". Same flow from step 2 onward. The findings doc decides whether we keep both modes.

## How a Notion change maps to a Helix action

This is the central UX decision. Helix needs to know, from a webhook payload, *what to do* — start a spectask, stop one, just write a result back, etc. We anchor the MVP on a **convention**: one Notion database = one Helix project; each row = one candidate spectask.

**Notion has two purpose-built primitives we lean on** (both POST to an arbitrary webhook URL with selected row fields, both on Notion paid plans):

- **Database Automations** (`https://www.notion.com/help/database-automations`) — per-database rules: "when `Status` changes to X, send webhook to URL Y with these property fields". This is the **implicit / event-driven** path.
- **Button property** (column type — `https://www.notion.com/help/buttons`) — a per-row clickable button with a "Send webhook" action. This is the **explicit / user-initiated** path.

Both are dramatically better than the general API webhook subscription for our use case: the user picks exactly which property changes fire, and the payload contains the row fields we need so we don't have to GET-and-diff. (We still support the API webhook subscription as a fallback for free-plan workspaces and for `comment.created` events, which Automations don't cover.)

### The MVP convention (one row = one spectask)

A Notion database becomes Helix-managed by designating one **action column** (a dropdown — Notion `select` OR `status` type, doesn't matter) and one optional **result column** (rich text). Helix's setup wizard shows the user how to wire the Automation. **Helix never writes back to the action column** — the user owns that, which means there is no possibility of a write-back loop and no constraint on what the action-column option names are.

| Column            | Notion type   | Direction                  | Meaning                                                                            |
| ----------------- | ------------- | -------------------------- | ---------------------------------------------------------------------------------- |
| `Title` (built-in) | title        | input                      | Spectask name.                                                                     |
| `Prompt` (optional) | rich text  | input                      | Spectask prompt. If empty, Helix falls back to the row's page body (block content). |
| **Action column** | `select` or `status` | input             | Drives the action via Automation. **User-named, user-controlled.** Customer's example: a `Go/NoGo` select column with two options. Alternative: a `Status` workflow column with `Backlog`/`Ready`/`Done` etc. Helix is told which option triggers create vs. which triggers cancel during setup. |
| `Result` (optional) | rich text  | output                     | Helix writes the agent's final output summary on completion. If absent, the result is left only in Helix. |
| `Run` (optional)  | button        | input                      | Manual "Run on Helix" button — sends webhook → starts spectask regardless of action column. |

**Where the live embed lives:** when Helix creates a spectask for a row, it appends a Notion `embed` block to that row's **page body** via `PATCH /v1/blocks/{page_id}/children` pointing at `https://app.helix.ml/embed/task/{task_id}`. The user opens the row in Notion → sees the live Helix UI rendered inside the row's page. No manual paste, no separate URL-column step.

(Notion has no inline-embed *property* type — embedding only works as a block in a page body. Each database row IS a Notion page, so this works cleanly. The user's row-list view shows whatever columns they've configured; clicking into a row reveals the embedded Helix UI in the body.)

**Action mapping (customer's Go/NoGo example):**
- `Go/NoGo: <unset> → Go` (Automation fires) → Helix creates a spectask in the configured project; appends the embed block to the row's page body.
- `Go/NoGo: Go → NoGo` (Automation fires) → Helix cancels the in-flight spectask referenced by the `ExternalTriggerRef`; optionally removes the embed block (best effort — see below).
- `Run` button pressed → Helix creates a spectask immediately, ignoring the action column.
- Spectask completes → Helix writes the summary to `Result` (if configured). Helix does **not** touch the action column, so the user can decide whether to flip `Go → NoGo` themselves.

**Embed-block lifecycle on cancel/completion:**
- On cancel: best-effort remove the embed block (we recorded its block ID at creation time on the spectask). If removal fails (user moved/edited it), log and move on — we don't want a Notion edit failure to block cancellation.
- On completion: leave the embed block in place. The Helix task URL still resolves to the finished task with its final state, and the user may want to reopen it.

**Why this shape:**
- **Type-agnostic action column** — `Go/NoGo` (select) and `Backlog/Ready/Done` (status) and any other dropdown shape all work. The user names the options; Helix learns the mapping during setup. Customers who already have a `Go/NoGo` workflow don't have to re-architect.
- **Helix never writes the action column** → no loop risk, no need to constrain what option names exist, no setup-wizard validation that "the trigger value isn't the writeback value".
- **Embed-in-body is the right place** — Notion's UX makes the row a page, the embed block belongs in the page body where the user can read context and watch the agent work alongside.
- **One-row-one-spectask** is the simplest mental model that survives translation back to a Helix project view.
- **The convention is opt-in per-database** — users can have other Notion databases Helix doesn't touch.

### What this does *not* try to do

- **Free-form text edits triggering an agent** (the original framing of "when text changes inside Notion"). The general API webhook gives only `page.content_updated` (aggregated, no diff). We can support it as a coarse "the page was edited" trigger for the comment / page-edit user stories, but the *primary* path is the structured database convention because it gives the user predictable control over what fires when.
- **Watching every property change.** Only `Status` (or the `Run` button) drives action. Other property changes are ignored unless the user explicitly extends the Automation.
- **Auto-creating the database / columns.** The MVP user creates the database and columns manually following Helix's setup docs. A "Helix template database" downloadable from Notion is a v2 nice-to-have.

### Alternative considered (and rejected for the MVP)

**"Helix watches the whole workspace"**: a single integration grant, Helix figures out which page / row to act on. Rejected because:
- Notion's general webhook is coarse and doesn't tell us which property changed → forces a GET on every event, wastes the 3 req/sec rate limit.
- No clear way for the user to say "this page should trigger Helix, that one shouldn't" without a convention layer anyway — and once you have a convention, you may as well lean on the structured Database Automation primitive.

## User Stories

**1. PM driving agents from an existing Notion database (the customer's flow)**
> As a PM with a Notion database that already has a `Go/NoGo` dropdown column on each row, I want to flip a row to `Go` and have Helix automatically create a spectask in the linked project and embed the live Helix UI directly inside the row's page body — so I can read the row, watch the agent work, and see results in one Notion view. Flipping back to `NoGo` cancels the spectask. Helix never touches my `Go/NoGo` column — I own that.

**2. Knowledge worker triggering an agent on demand (button-driven)**
> As an engineer with a one-off row of work, I want to click a `Run on Helix` button on that row and have Helix immediately start the spectask, so I don't have to use the action-column mechanism for ad-hoc runs.

**3. Watching the agent inside the Notion row, not in a separate tab**
> As a user who triggered a Helix spectask from a Notion row, I want to click into that row in Notion and see the live Helix UI (logs, video stream of the desktop sandbox, planning artifacts) embedded directly in the row's page body — without having to copy a URL or paste anything. Helix should add the embed automatically when it starts the spectask.

**4. Operator wiring up the integration once**
> As a workspace admin, I want to install a Helix Notion integration once (OAuth grant on the database the team uses), then point Helix's setup wizard at our existing `Go/NoGo` (or other dropdown) column and have Helix learn the mapping — so individual users don't each have to manage tokens or webhook URLs, and we don't have to redesign our existing database schema.

**5. Coarse "page edited / commented" trigger (secondary)**
> As a user with a free-form Notion page (no database), I want Helix to receive a coarse "this page was edited" or "a comment was added" event so I can wire less-structured agent flows. (Best-effort — no per-property diff, ~1 min latency.)

## Acceptance Criteria

### Notion → Helix (trigger)
- [ ] A new `Trigger.Notion` type alongside `Trigger.AzureDevOps` (`helix/api/pkg/types/types.go:1714`) — webhook-driven, configured per Helix App, gives the user a `WebhookURL` they paste into a Notion **Database Automation** or **Button property** "Send webhook" action (and into the general Notion API webhook subscription as a fallback).
- [ ] A `NotionTrigger` config carries: a webhook **shared secret**, an `OAuthConnectionID` for write-back calls, the **target Helix project ID**, the **Notion database ID** this trigger is bound to, the **column mapping** (which column is the action-driving dropdown; which optional rich-text column is the prompt; which optional rich-text column is the result), and the **action mapping** (which dropdown option triggers create, which triggers cancel). The action column may be `select` or `status` type — both are first-class.
- [ ] `webhookTriggerHandler` (`helix/api/pkg/server/webhook_trigger_handlers.go:14`) routes Notion-typed trigger configs into a new `notion.ProcessWebhook(ctx, triggerConfig, headers, payload)`.
- [ ] **Two payload shapes** are handled:
  - **Notion Database Automation / Button webhook payload** (the primary path) — JSON the user configured in the Automation: includes the page ID, the new property values for the fields they selected, and a Helix shared-secret header for verification. Helix dispatches the action (create / cancel / no-op) based on the `Status` value or the explicit "this is a Run-button press" flag.
  - **Notion API webhook subscription payload** (the secondary path, for `comment.created` / coarse `page.content_updated` / free-plan workspaces): HMAC-SHA256 verified via `X-Notion-Signature` against the subscription's `verification_token`. Triggers the agent with the page content as input; no row-state writeback.
- [ ] On action column → "create" option (e.g. `Go`) or Run-button press: create a Helix spectask in the trigger's `target_project_id`; append a Notion `embed` block to the source row's page body pointing at `https://app.helix.ml/embed/task/{task_id}?access_token={…}`; record the inserted block ID on the spectask. Helix does **not** write the action column.
- [ ] On action column → "cancel" option (e.g. `NoGo`) when an `ExternalTriggerRef` exists for that page: cancel the in-flight spectask; best-effort delete the previously inserted embed block (log on failure, do not block cancellation).
- [ ] On spectask completion (existing internal Helix event): if the trigger has a `Result` column configured, `PATCH` the row's `Result` rich-text property with a summary. Leave the embed block in place. The mapping from spectask → Notion row is stored on the spectask (new field `external_trigger_ref`) at creation time, including the inserted block ID for later cleanup.
- [ ] Idempotency: re-receiving the same "create" event for a page that already has a live spectask is a no-op (look up by `ExternalTriggerRef.NotionPageID`). Re-receiving "cancel" for a page with no live spectask is a no-op.
- [ ] Aggregated webhooks (Notion API path only, ~1 min latency) are de-duplicated: a follow-up event on the same page within N seconds replaces the queued execution rather than starting a second one. (Database Automations / Button webhooks don't aggregate, so this only applies to the secondary path.)
- [ ] One `TriggerExecution` row per processed delivery, surfaced in the existing per-app trigger executions UI, including the Notion event source (Automation / Button / API), database ID, and page ID.

### Setup UX
- [ ] The Helix trigger configuration page tells the user, step-by-step:
  1. Install the Helix Notion integration on the workspace via the OAuth "Connect to Notion" button and grant access to the target database.
  2. **Pick the action column** — Helix shows every `select` and `status` column in the database; the user picks which one drives the trigger. (Customer's case: their existing `Go/NoGo` select column.)
  3. **Pick the option-to-action mapping** — Helix shows the dropdown's options; the user picks which option means "create" (e.g. `Go`) and which means "cancel" (e.g. `NoGo`).
  4. **Pick the optional prompt and result columns** — Helix lists rich-text columns; the user picks one for the prompt (or leaves empty to use the page body) and one for the result (or leaves empty to suppress writeback).
  5. **Pick the target Helix project**.
  6. Create a Database Automation in Notion (Helix shows copy-paste URL/secret/headers): trigger = "When `<action column>` is set to `<create option>`" → action = "Send webhook" → URL = Helix-provided.
  7. Create a second Automation for the cancel direction: trigger = "When `<action column>` is set to `<cancel option>`".
  8. (Optional) add a `Run` button column with a "Send webhook" action to the same URL but with `X-Helix-Source: notion-button`.
- [ ] Helix exposes a "Test setup" button that sends a synthetic event through the same path so the user can verify wiring without touching real rows.

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
- [ ] Notion API gaps we hit (e.g. no per-property diff in webhooks → forced to GET-and-compare for the secondary path).
- [ ] Whether the **structured database convention** is the right primary primitive, or whether users instead want free-form page triggers.
- [ ] How Database Automations / Button webhooks behave under load (latency, retries, rate limits — Notion's general 3 req/sec applies to the read-back PATCHes, not the inbound webhook).
- [ ] Recommendation for GA: keep both Database Automation + Button paths, drop one, or add the API webhook subscription as well.
- [ ] Friction in the manual setup wizard — does this need a downloadable Notion template database?

## Out of Scope (this task)

- A first-party Notion app/listing in the Notion integration gallery (requires a security review at Notion's end — separate workstream).
- Notion as a **knowledge source** for RAG (different code path, `helix/api/pkg/rag/`); this task is about triggers + embed only.
- Polling fallback for fine-grained text-edit detection (called out in findings; deferred until we know whether webhooks alone are sufficient).
- Auto-installing the Notion integration on the user's workspace from inside Helix — for the MVP, the user runs through the standard Notion OAuth consent screen.
- Auto-creating the convention columns in the user's database — the MVP user creates them manually following Helix's setup wizard. A downloadable Notion template database is a v2 nice-to-have.
- Inline auto-rendered embed of the Helix UI in a row (Notion has no embed-property type — only embed *blocks* in page bodies; the user pastes the URL into an embed block manually).
- Toggle-block expand/collapse events: Notion does not expose these via the API, so they cannot be observed. Note in findings, do not attempt to work around.
- Free-plan workspace support for the primary (Automation/Button) path — Notion gates "Send webhook" actions to paid plans. Free-plan users get the secondary (API webhook subscription) path with reduced functionality.

## Target customer

Initial target customer is on the **Notion Business plan**, so the primary-path features (Database Automations + Button "Send webhook" actions) are available. Free-plan parity is **not** a release blocker for v1 — the secondary API-webhook path exists only to cover event types Automations don't surface (`comment.created`, free-form page edits), not to back-fill missing plan capabilities.

## Open Questions

- **Public vs internal Notion integration**: do we want a single Helix-owned **public integration** (in the Notion directory, per-workspace OAuth), or per-Helix-admin **internal integrations**? The MVP picks internal-integration because it's faster to ship and doesn't need a Notion review; findings doc revisits.
- **Bot-comment loop avoidance**: if we ever post comments back to Notion (out of scope for the MVP convention but possible via the secondary-path), how do we tag our own bot's comments so the resulting `comment.created` webhook doesn't loop? Author-ID match is the obvious answer; confirm during implementation.
- **What to do when the user maps Helix to a database that doesn't have the convention columns**: hard-fail at setup time, or auto-create? MVP hard-fails with a clear error in the setup wizard.
