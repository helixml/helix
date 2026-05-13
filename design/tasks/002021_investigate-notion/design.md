# Design — Notion Integration for Helix

## Approach

Reuse Helix's existing webhook-trigger plumbing (`webhookTriggerHandler` + `Trigger.AzureDevOps` pattern) and existing OAuth-provider abstraction (`OAuthProviderTypeAtlassian`-style). Add a new trigger type and a new OAuth provider type. For the embed direction, exercise the existing `/embed/task/:taskId` route from inside Notion and confirm/relax frame-ancestor headers if needed.

This is intentionally a copy-the-Azure-DevOps-trigger task. The novelty is in (a) Notion's HMAC verification and aggregated-webhook semantics, and (b) the Notion-specific OAuth flow (HTTP-Basic token endpoint, no scopes, capabilities-driven consent).

## High-Level Architecture

```
┌──────────────┐  webhook    ┌──────────────────────────┐
│   Notion     │────────────▶│  POST /api/v1/webhooks/  │
│  (workspace) │  X-Notion-  │     {trigger_id}          │
└──────────────┘  Signature  │  webhookTriggerHandler    │
       ▲                     │     ↓                     │
       │                     │  trigger.ProcessWebhook   │
       │                     │     ↓ (Notion case)       │
       │                     │  notion.ProcessWebhook    │
       │  optional reply      │     ↓ HMAC verify        │
       │  (Notion API call)  │     ↓ fetch page via      │
       └─────────────────────┤        OAuth connection   │
                             │     ↓ run agent           │
                             │     ↓ (optional) post    │
                             │        reply comment      │
                             └──────────────────────────┘

┌────────────────────────────────────────────────────────┐
│  Notion page → /embed block → https://app.helix.ml/   │
│                  embed/task/{id}?access_token=...      │
│                  (existing EmbedTaskPage)              │
└────────────────────────────────────────────────────────┘
```

## Key Decisions

### 1. Trigger pattern: webhook, not poll (for the MVP)

Notion's webhooks are aggregated (debounced, ~1 min worst-case delay, no per-property diff). They're inferior to polling for "exact text changed" detection but **good enough** for "row toggled" / "comment posted" / "page edited" — which is what user story 1 actually needs.

We start webhook-only because:
- Reuses existing Helix webhook infra one-for-one (handler, trigger config, executions table, UI).
- Polling adds a new background loop, state for last-seen timestamps per page, and burns the 3 req/sec rate limit even when nothing's changed.
- The findings doc decides whether to add polling later. If we do, it's a separate `notion.Poller` goroutine analogous to `cron.Trigger`, sharing the same `notion.ProcessChange` code path.

**Not viable:** observing Toggle *block* expand/collapse — Notion doesn't expose this via API. Documented as a hard "no" so future work doesn't chase it.

### 2. OAuth provider: extend the existing abstraction

Notion fits cleanly into `oauth/provider.go` `Provider` interface:
- `GetAuthorizationURL` → `https://api.notion.com/v1/oauth/authorize?owner=user&...`
- `CompleteAuthorization` → `POST https://api.notion.com/v1/oauth/token` with **HTTP Basic** (`client_id:client_secret`) — this is the one quirk vs the existing OAuth2 implementation; the generic OAuth2 provider currently sends client creds in the body. We add a `useBasicAuth` flag on the provider config.
- `GetUserInfo` → `GET /v1/users/me`, parse `bot.owner.user` for ID + name.
- `MakeAuthorizedRequest` → adds `Authorization: Bearer <token>` and `Notion-Version: 2025-09-03` header (Notion requires the version header on every request).

Add `OAuthProviderTypeNotion = "notion"` and a `case` in `oauth2.go:326`'s `GetUserInfo` switch. No scopes — Notion uses capabilities (configured in the integration's developer portal, not at consent time). The MVP requires the integration to have **Read content + Read user information + Insert comments** capabilities; documented in the findings doc.

### 3. HMAC signature verification

Notion sends `X-Notion-Signature: sha256=<hex>` on every delivery. The secret is the `verification_token` returned during webhook subscription setup, stored in the trigger config (encrypted, alongside other trigger secrets like Slack tokens — `helix/api/pkg/types/types.go:1666`).

```go
mac := hmac.New(sha256.New, []byte(triggerCfg.Notion.VerificationToken))
mac.Write(rawBody)
expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Notion-Signature"))) {
    return errInvalidSignature
}
```

The `webhookTriggerHandler` already reads the body once before dispatch (`helix/api/pkg/server/webhook_trigger_handlers.go:27`); we pass it through as-is. Adding a `Headers http.Header` field to the `ProcessWebhook` interface is the cleanest way — the existing Azure path ignores it.

### 4. Initial verification handshake

When Notion first asks the user to subscribe a URL, it `POST`s a `{ "verification_token": "..." }` body and waits for the user to paste that token back into the integration setup. We surface this in the trigger configuration UI: paste-the-token field with a "verify" button, the same shape Azure DevOps' webhook setup uses today.

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
    Enabled            bool     `json:"enabled,omitempty"`
    VerificationToken  string   `json:"verification_token"` // HMAC secret from Notion
    OAuthConnectionID  string   `json:"oauth_connection_id,omitempty"` // To fetch page content / post replies
    WatchPageIDs       []string `json:"watch_page_ids,omitempty"`      // Optional filter
    WatchDatabaseIDs   []string `json:"watch_database_ids,omitempty"`  // Optional filter
    EventTypes         []string `json:"event_types,omitempty"`         // Optional filter (page.properties_updated, etc)
    ReplyAsComment     bool     `json:"reply_as_comment,omitempty"`    // Off by default
    PromptTemplate     string   `json:"prompt_template,omitempty"`     // Go template; vars: .EventType, .Page, .Properties, .Comment
}
```

Add `TriggerTypeNotion TriggerType = "notion"` to `helix/api/pkg/types/types.go:2625`.

This is a JSONB column; no migration needed — the trigger config row is already `gorm:"jsonb"` (`helix/api/pkg/types/types.go:2653`).

### No new tables

`TriggerConfiguration` and `TriggerExecution` are reused. Per-page "last seen state" caching for de-dup goes in a small in-memory map in the `notion` package, keyed by page ID; lost on restart, which is fine — at worst a duplicate execution fires.

## Component Layout

| New file                                                  | Purpose                                            |
| --------------------------------------------------------- | -------------------------------------------------- |
| `api/pkg/trigger/notion/notion.go`                        | `New()`, `ProcessWebhook(ctx, cfg, headers, body)` |
| `api/pkg/trigger/notion/events.go`                        | Notion event payload structs (`PageEvent`, `CommentEvent`) + signature verify helper |
| `api/pkg/trigger/notion/render.go`                        | Format Notion event → prompt string (mirrors `azure/render.go`) |
| `api/pkg/trigger/notion/notion_test.go`                   | HMAC verification, dedup, prompt-rendering tests   |
| `api/pkg/oauth/notion.go`                                 | `notionProvider` implementing `Provider` (HTTP-Basic token, `Notion-Version` header) |

| Edited file                                               | Change                                             |
| --------------------------------------------------------- | -------------------------------------------------- |
| `api/pkg/types/types.go`                                  | Add `NotionTrigger`, `TriggerTypeNotion`, plumb into `Trigger` |
| `api/pkg/types/oauth.go`                                  | Add `OAuthProviderTypeNotion`                      |
| `api/pkg/oauth/oauth2.go`                                 | Notion case in `GetUserInfo`; HTTP-Basic flag     |
| `api/pkg/trigger/trigger.go`                              | `case triggerConfig.Trigger.Notion != nil:` in `ProcessWebhook` |
| `api/pkg/server/webhook_trigger_handlers.go`              | Pass `r.Header` through to `trigger.ProcessWebhook` |
| `frontend/src/components/app/triggers/...` (existing trigger config UI) | Add Notion trigger config form (verification token, page IDs, prompt template, OAuth connection picker) |
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
| Notion webhook delays of ~1 min make the demo feel sluggish           | High       | Document. If unacceptable, plan polling layer in v2.                                        |
| `page.properties_updated` doesn't say *which* property → extra GETs   | Certain    | One GET per delivery + 3 req/sec rate limit is fine for demo. Cache per-page for de-dup.   |
| Notion embed via Iframely refuses arbitrary URLs                      | Medium     | Add OG meta tags to `/embed/*`. Fall back: instruct users to use Notion's "link preview" instead of `/embed`. |
| Helix dev domain not reachable from Notion (must be public HTTPS)     | Medium     | Use ngrok / cloud preview env for testing. Documented in findings.                          |
| Token refresh path untested (Notion tokens are long-lived)            | Low        | No-op refresh now; revisit when first token actually expires (Notion has only just enabled refresh tokens for new public integrations as of late 2024). |
| Aggregated-webhook dedup is in-memory → duplicates after API restart  | Low        | Acceptable for MVP. Document. Move to Redis pubsub if it bites.                             |
| Reply-as-comment posts loop back as `comment.created` events → loop   | Medium     | When posting, set a sentinel marker (e.g. comment author = our bot user) and skip own bot's comments in the webhook handler. |

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
