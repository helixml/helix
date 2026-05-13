# Design — Notion Integration for Helix

## Approach

Reuse Helix's existing webhook-trigger plumbing (`webhookTriggerHandler` + `Trigger.AzureDevOps` pattern) and existing OAuth-provider abstraction (`OAuthProviderTypeAtlassian`-style). Add a new trigger type and a new OAuth provider type. For the embed direction, exercise the existing `/embed/task/:taskId` route from inside Notion and confirm/relax frame-ancestor headers if needed.

The product surface is anchored on a **convention** (see requirements.md "How a Notion change maps to a Helix action"): one Notion database = one Helix project; each row = one spectask; a `Status` column drives the action via Notion **Database Automations** (event-driven) and a `Run` **Button property** drives the explicit on-demand path. Both Notion features POST to an arbitrary webhook URL with selected row fields — we do not need the general API webhook subscription for the primary path. We keep the API webhook subscription as a secondary path for free-plan workspaces and for free-form `comment.created` / `page.content_updated` events.

This is intentionally a copy-the-Azure-DevOps-trigger task on the plumbing side. The novelty is in (a) the action-dispatch layer that maps `Status` values to spectask lifecycle calls and writes back to Notion, and (b) the Notion-specific OAuth flow (HTTP-Basic token endpoint, no scopes, capabilities-driven consent).

## High-Level Architecture

```
Primary path — Notion Database Automation / Button:

┌─────────────────────────┐  webhook (selected fields)  ┌───────────────────────────┐
│  Notion DB              │──────────────────────────-─▶│ POST /api/v1/webhooks/    │
│  Automation /           │  X-Helix-Signature          │   {trigger_id}            │
│  Run-button property    │                             │ webhookTriggerHandler     │
└─────────────────────────┘                             │     ↓                     │
        ▲                                               │ notion.ProcessWebhook     │
        │  PATCH /v1/pages/{id} writeback              │     ↓ verify shared secret│
        │  (Status, Helix Task URL, Result)            │     ↓ classify shape      │
        │                                               │     ↓ dispatch action:    │
        │                                               │       create / cancel /   │
        │                                               │       no-op               │
        │                                               │     ↓                     │
        └───────────────────────────────────────────────┤   spectask service        │
                                                        │     ↓                     │
                                                        │   on completion:          │
                                                        │   notion.WriteResultBack  │
                                                        └───────────────────────────┘

Secondary path — Notion API webhook subscription (free plan / comments):

┌──────────────┐  X-Notion-Signature  ┌──────────────────────────┐
│  Notion      │─────────────────────▶│  POST /api/v1/webhooks/  │
│  workspace   │  HMAC-SHA256         │     {trigger_id}          │
└──────────────┘                      │  notion.ProcessWebhook    │
                                      │     ↓ HMAC verify         │
                                      │     ↓ fetch page          │
                                      │     ↓ run agent (no row   │
                                      │        write-back)        │
                                      └──────────────────────────┘

Embed direction:

┌────────────────────────────────────────────────────────┐
│  Notion page → /embed block →                          │
│   https://app.helix.ml/embed/task/{id}?access_token=…  │
│   (existing EmbedTaskPage)                             │
└────────────────────────────────────────────────────────┘
```

## Key Decisions

### 1. Trigger source: Database Automation + Button (primary), API webhook (secondary)

Notion has two purpose-built features for "send a webhook when the user does X" (both paid-plans-only):
- **Database Automation**: per-database rule, e.g. "when `Status` changes → send webhook to URL with these fields". Event-driven. No aggregation. Payload is exactly the row fields the user picked.
- **Button property**: per-row clickable button, action = "Send webhook". User-initiated. Same payload shape as Automation.

These are **strictly better** than the general API webhook subscription for our convention because they let the user pre-filter what fires (only `Status` changes, not every property edit) and the payload arrives with the row fields baked in (no GET-and-diff). They're also faster — no aggregation delay.

The general API webhook subscription stays as a **secondary path** for: (a) free-plan workspaces (Notion gates Send-webhook actions to paid plans), (b) `comment.created` events (not surfaced by Automations), (c) coarse `page.content_updated` for free-form non-database pages.

**Not viable:** observing Toggle *block* expand/collapse — Notion doesn't expose this via API. Documented as a hard "no" so future work doesn't chase it.

### 1b. Action dispatch from a row webhook

The primary-path payload contains the page ID and the property values the user selected. Helix's dispatch logic:

```go
switch row.Status {
case cfg.StatusValueReady:    // e.g. "Ready"
    if existingSpecTask(row.PageID) != nil { return noop }
    spectask := createSpecTask(cfg.TargetProjectID, row.Title, row.Prompt,
                                externalRef("notion", row.PageID))
    notion.PatchPage(row.PageID, map[string]any{
        cfg.StatusColumn:    cfg.StatusValueRunning,           // "Running"
        cfg.HelixTaskColumn: spectaskURL(spectask),
    })
case cfg.StatusValueCancelled:
    if st := existingSpecTask(row.PageID); st != nil {
        cancelSpecTask(st)
    }
case "_button_run":           // synthetic value for Button property webhook
    spectask := createSpecTask(...) // same as Ready
}
```

Spectask completion (existing internal Helix event, hooked via `spec_driven_task_service`) calls `notion.WriteResultBack(externalRef, finalStatus, summary)` which `PATCH`es `Status: Done` and `Result: <summary>`. The mapping from spectask → Notion row lives on the spectask as a new `external_trigger_ref` field (`{"type":"notion","page_id":"…","trigger_config_id":"…"}`).

### 1c. Differentiating Automation vs Button vs API payloads

The three payload shapes are distinguishable:
- **Automation/Button**: contains a Helix-set custom header (we tell the user to add `X-Helix-Source: notion-button` or `notion-automation` when configuring the action — Notion supports custom headers on webhook actions).
- **API subscription**: contains `X-Notion-Signature` and Notion's own envelope (`{"type": "page.properties_updated", ...}`).

`notion.ProcessWebhook` switches on the header to pick the verification + parser. Both code paths converge on the same dispatch function once the row data is in hand (the API path GETs the page first to obtain the same row fields).

### 2. OAuth provider: extend the existing abstraction

Notion fits cleanly into `oauth/provider.go` `Provider` interface:
- `GetAuthorizationURL` → `https://api.notion.com/v1/oauth/authorize?owner=user&...`
- `CompleteAuthorization` → `POST https://api.notion.com/v1/oauth/token` with **HTTP Basic** (`client_id:client_secret`) — this is the one quirk vs the existing OAuth2 implementation; the generic OAuth2 provider currently sends client creds in the body. We add a `useBasicAuth` flag on the provider config.
- `GetUserInfo` → `GET /v1/users/me`, parse `bot.owner.user` for ID + name.
- `MakeAuthorizedRequest` → adds `Authorization: Bearer <token>` and `Notion-Version: 2025-09-03` header (Notion requires the version header on every request).

Add `OAuthProviderTypeNotion = "notion"` and a `case` in `oauth2.go:326`'s `GetUserInfo` switch. No scopes — Notion uses capabilities (configured in the integration's developer portal, not at consent time). The MVP requires the integration to have **Read content + Read user information + Insert comments** capabilities; documented in the findings doc.

### 3. Webhook authentication (two schemes)

**Primary path (Database Automation / Button):** Notion's webhook actions don't sign payloads — they just POST. We use a **shared secret** in a custom header (`X-Helix-Webhook-Secret: <secret>`) that the user copies from the Helix trigger config UI into the Notion automation's headers. Constant-time compare on receipt.

We also include the trigger ID in the URL path (`/api/v1/webhooks/{trigger_id}`) — same as today — so the secret only authenticates *this specific trigger config*. Compromising one trigger doesn't affect others.

**Secondary path (API webhook subscription):** Notion sends `X-Notion-Signature: sha256=<hex>` on every delivery. The secret is the `verification_token` returned during webhook subscription setup, stored in the trigger config (encrypted, alongside other trigger secrets like Slack tokens — `helix/api/pkg/types/types.go:1666`).

```go
mac := hmac.New(sha256.New, []byte(triggerCfg.Notion.VerificationToken))
mac.Write(rawBody)
expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Notion-Signature"))) {
    return errInvalidSignature
}
```

The `webhookTriggerHandler` already reads the body once before dispatch (`helix/api/pkg/server/webhook_trigger_handlers.go:27`); we pass headers and body through to `notion.ProcessWebhook`. Adding a `Headers http.Header` field to the `ProcessWebhook` interface is the cleanest way — the existing Azure path ignores it.

### 4. Initial subscription handshake (secondary path only)

When Notion first asks the user to subscribe a URL via the API webhook subscription flow, it `POST`s a `{ "verification_token": "..." }` body and waits for the user to paste that token back into the integration setup. We surface this in the trigger configuration UI as a paste-the-token field with a "verify" button — same shape Azure DevOps' webhook setup uses today. (No handshake needed for the primary path: the user pastes the URL + secret into Notion themselves.)

### 5. Embed: validate before changing anything

Before modifying any frame-options handling we first **load `/embed/task/:taskId` inside a Notion embed block** and screenshot the result. The path already exists, the `?access_token=` flow already works (Gatewaze uses it). Likely outcomes:
- It just works (Helix doesn't currently set restrictive frame-ancestors → Notion → Iframely renders it).
- It's blocked by a CSP we haven't checked → relax CSP for `/embed/*` only, allowlist `*.notion.so` + `*.notion.site`.
- Iframely refuses because it can't extract metadata → add Open Graph tags to the embed page or contact Iframely.

The findings doc captures which one happened and what we did about it.

## Schema Changes

### `types.Trigger` adds a Notion field

```go
// helix/api/pkg/types/types.go (~line 1714)
type Trigger struct {
    Discord     *DiscordTrigger     `json:"discord,omitempty"`
    Slack       *SlackTrigger       `json:"slack,omitempty"`
    Teams       *TeamsTrigger       `json:"teams,omitempty"`
    Cron        *CronTrigger        `json:"cron,omitempty"`
    Crisp       *CrispTrigger       `json:"crisp,omitempty"`
    AzureDevOps *AzureDevOpsTrigger `json:"azure_devops,omitempty"`
    Notion      *NotionTrigger      `json:"notion,omitempty"` // new
}

type NotionTrigger struct {
    Enabled bool `json:"enabled,omitempty"`

    // Auth
    SharedSecret      string `json:"shared_secret"`                      // Primary path (Automation/Button) — Helix-generated, copied into Notion's webhook headers
    VerificationToken string `json:"verification_token,omitempty"`       // Secondary path — HMAC secret from Notion subscription handshake
    OAuthConnectionID string `json:"oauth_connection_id,omitempty"`      // For PATCH-back to row, GET page content

    // Convention binding (primary path)
    NotionDatabaseID string             `json:"notion_database_id,omitempty"`  // The DB this trigger is bound to
    TargetProjectID  string             `json:"target_project_id,omitempty"`   // Helix project to create spectasks in
    ColumnMapping    NotionColumnMap    `json:"column_mapping,omitempty"`      // Maps convention names → user's actual column names
    ActionMapping    NotionActionMap    `json:"action_mapping,omitempty"`      // Status values → Helix actions

    // Secondary path (free-form pages / comments)
    WatchPageIDs   []string `json:"watch_page_ids,omitempty"`
    EventTypes     []string `json:"event_types,omitempty"` // e.g. ["comment.created", "page.content_updated"]
    PromptTemplate string   `json:"prompt_template,omitempty"`
}

type NotionColumnMap struct {
    Status    string `json:"status,omitempty"`     // default "Status"
    Prompt    string `json:"prompt,omitempty"`     // default "Prompt"
    HelixTask string `json:"helix_task,omitempty"` // default "Helix Task"
    Result    string `json:"result,omitempty"`     // default "Result"
}

type NotionActionMap struct {
    StatusValueReady     string `json:"status_value_ready,omitempty"`     // default "Ready"
    StatusValueRunning   string `json:"status_value_running,omitempty"`   // default "Running"
    StatusValueDone      string `json:"status_value_done,omitempty"`      // default "Done"
    StatusValueCancelled string `json:"status_value_cancelled,omitempty"` // default "Cancelled"
}
```

Add `TriggerTypeNotion TriggerType = "notion"` to `helix/api/pkg/types/types.go:2625`.

This is a JSONB column; no migration needed — the trigger config row is already `gorm:"jsonb"` (`helix/api/pkg/types/types.go:2653`).

### Spectask `external_trigger_ref`

To complete the round-trip (spectask → write back to the originating Notion row), the spectask needs to remember where it came from. Add a JSONB `external_trigger_ref` column on the spec_tasks table:

```go
type ExternalTriggerRef struct {
    Type              string `json:"type"`                // "notion"
    TriggerConfigID   string `json:"trigger_config_id"`
    NotionPageID      string `json:"notion_page_id,omitempty"`
    NotionDatabaseID  string `json:"notion_database_id,omitempty"`
}
```

This is generic enough to extend later (e.g. ADO work-item refs).

### No new tables

`TriggerConfiguration` and `TriggerExecution` are reused. The new `external_trigger_ref` column on spec_tasks is added via the existing GORM AutoMigrate. Per-page "last seen state" caching for de-dup (secondary path only) goes in a small in-memory map in the `notion` package, keyed by page ID; lost on restart, which is fine — at worst a duplicate execution fires.

## Component Layout

| New file                                                  | Purpose                                            |
| --------------------------------------------------------- | -------------------------------------------------- |
| `api/pkg/trigger/notion/notion.go`                        | `New()`, `ProcessWebhook(ctx, cfg, headers, body)` — branches on payload shape |
| `api/pkg/trigger/notion/dispatch.go`                      | `dispatchRowEvent(cfg, row)` — Status/Button → create/cancel; spectask-completion hook → row write-back |
| `api/pkg/trigger/notion/client.go`                        | Thin Notion API client: `GetPage`, `PatchPage`, post-comment (uses OAuth connection) |
| `api/pkg/trigger/notion/events.go`                        | Payload structs for both shapes + signature verify helpers |
| `api/pkg/trigger/notion/render.go`                        | Format Notion event → prompt string (secondary path only; the primary path uses `Prompt` column directly) |
| `api/pkg/trigger/notion/notion_test.go`                   | Signature verification, dispatch logic, write-back, dedup |
| `api/pkg/oauth/notion.go`                                 | `notionProvider` implementing `Provider` (HTTP-Basic token, `Notion-Version` header) |

| Edited file                                               | Change                                             |
| --------------------------------------------------------- | -------------------------------------------------- |
| `api/pkg/types/types.go`                                  | Add `NotionTrigger`, `NotionColumnMap`, `NotionActionMap`, `TriggerTypeNotion`, `ExternalTriggerRef`; plumb `Notion` into `Trigger`; `external_trigger_ref` field on `SpecTask` |
| `api/pkg/types/oauth.go`                                  | Add `OAuthProviderTypeNotion`                      |
| `api/pkg/oauth/oauth2.go`                                 | Notion case in `GetUserInfo`; HTTP-Basic flag, `Notion-Version` header |
| `api/pkg/trigger/trigger.go`                              | `case triggerConfig.Trigger.Notion != nil:` in `ProcessWebhook` |
| `api/pkg/server/webhook_trigger_handlers.go`              | Pass `r.Header` through to `trigger.ProcessWebhook` |
| `api/pkg/services/spec_driven_task_service.go`            | On spectask completion, if `ExternalTriggerRef.Type == "notion"`, invoke `notion.WriteResultBack` |
| `frontend/src/components/app/triggers/...` (existing trigger config UI) | Add Notion trigger config form: project picker, database ID, column mapping (4 fields), action mapping (4 status values), shared-secret display + setup wizard |
| `frontend/src/pages/Providers.tsx`                        | Add "Notion" preset for the OAuth provider create form |

## Embed Path

No new code expected. Verify:

1. Generate a Helix API key tied to a test user.
2. Build URL: `https://app.helix.ml/embed/task/{task_id}?access_token={key}`.
3. In a Notion test page, `/embed` block, paste the URL.
4. Screenshot result. If it loads, attach to `screenshots/01-notion-embed.png`.
5. Check response headers on `/embed/*` for `X-Frame-Options` and CSP `frame-ancestors`. If restrictive, relax for that path only (server middleware gating on `strings.HasPrefix(r.URL.Path, "/embed/")`).

If the desktop video stream doesn't work in Notion's iframe (likely candidates: WebSocket origin checks, autoplay policies, fullscreen API gated to top-level frames), capture browser console logs in the findings doc.

## Risks & Mitigations

| Risk                                                                  | Likelihood | Mitigation                                                                                  |
| --------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------- |
| Database Automation / Button is paid-plan-only — free users excluded from primary path | Certain    | Document. Free-plan users get the secondary API webhook path (less functionality). Findings doc decides whether free-plan parity matters. |
| Helix's row write-back (`PATCH /v1/pages`) triggers the user's own Automation → loop | High | Status changes Helix makes (`Running`, `Done`) must not match the trigger value (`Ready`). Setup wizard enforces this via UI validation; default mapping makes this impossible. |
| User adds the convention columns with the wrong types (e.g. `Status` as text not status) | High | Setup wizard fetches the database schema via the OAuth connection and validates types before saving the trigger config; surfaces a clear error otherwise. |
| Setup is fiddly — three steps in Notion (install integration, add columns, add automation) | High | Step-by-step wizard with copy-paste blocks for the URL + secret + headers. v2: downloadable Notion template DB. |
| Helix dev domain not reachable from Notion (must be public HTTPS)     | Medium     | Use ngrok / cloud preview env for testing. Documented in findings.                          |
| Notion embed via Iframely refuses arbitrary URLs                      | Medium     | Add OG meta tags to `/embed/*`. Fall back: instruct users to use Notion's "link preview" instead of `/embed`. |
| Secondary-path `page.properties_updated` doesn't say which property → extra GETs | Certain (secondary path only) | One GET per delivery + 3 req/sec rate limit is fine for demo volume. Cache per-page for de-dup. |
| Aggregated-webhook dedup (secondary path) is in-memory → duplicates after API restart | Low | Acceptable for MVP. Document.                                                              |
| Token refresh path untested (Notion tokens are long-lived)            | Low        | No-op refresh now; revisit when first token actually expires.                              |

## Testing Strategy

- **Unit (Go)**: HMAC sig verification (good token, bad token, replay); event dispatch by type; prompt template rendering; dedup of two webhooks within N seconds.
- **Integration (manual, captured in findings)**: end-to-end loop with a real Notion workspace — toggle a checkbox → see Helix session created → verify reply comment appears.
- **Embed (manual)**: paste embed URL into Notion, screenshot, verify desktop stream works.
- **Frontend**: trigger config form renders, validates verification token, submits.

No CI requirement for the Notion side beyond unit tests — we can't drive a real Notion workspace from Drone without secrets we don't want in CI. Manual sign-off in the findings doc is the gate.

## Notes for Future Implementers

- **No official Notion Go SDK.** Use `github.com/jomei/notionapi` for the few calls we need (page fetch, comment create, users-me). Don't take it as a hard dep yet — for the MVP, hand-rolled `http.Client` calls against 3 endpoints is fewer LOC than wiring the SDK.
- **`Notion-Version` header is mandatory** on every API call. Forgetting it returns a confusing 400. Pin to `2025-09-03` (current as of May 2026).
- **OAuth `owner=user`** is required for non-admin installs. Without it, Notion forces an admin to authorize, which excludes most users.
- **Notion Embed != Notion Link Preview** — the former takes any HTTPS URL, the latter requires a registered link-preview integration (out of scope).
- **Existing Gatewaze embed flow is the closest analogue** — read `helix/frontend/src/components/external-agent/DesktopStreamViewer.tsx:1309` for the fullscreen handling.
