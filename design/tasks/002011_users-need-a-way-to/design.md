# Design — In-App Error Reporting

## Approaches Considered

We need a single button that works the same in three deployment shapes: SaaS (`app.helix.ml`), Helm/k8s with internet egress, and Helm/k8s air-gapped.

| Approach | Pros | Cons |
|---|---|---|
| **A. Crisp chat (same channel as Mac app)** | Already wired — `frontend/index.html:68-79` loads the same Crisp widget with the same `CRISP_WEBSITE_ID` (`d69e5955-…`) as the Mac app. Same triage channel for desktop + web reports. Zero new infra. | ~10 KB per text message → must chunk or send the bundle as a file. Air-gapped customers can't reach `client.crisp.chat`. |
| **B. New API endpoint → central Helix-hosted ingest** | Structured, queryable. | Needs new ingest service + storage + retention. Splits triage between Crisp (Mac) and a new ticketing inbox (web/helm). |
| **C. Auto-file GitHub issues** | Public + transparent. | Privacy nightmare for logs/prompts; requires per-user GH auth. |

**Chosen: A (Crisp).** Per review feedback, we're keeping Crisp as the upstream — it's the existing support channel, the widget is already loaded in the same frontend that ships to SaaS *and* helm installs, and unifying with the Mac app's triage queue is more valuable than building a parallel ingest pipeline. Bundle-too-big and air-gapped are handled by the downloadable-bundle escape hatch (see "Transport" below).

## High-Level Architecture

```
        ┌─────────────────────────────────────────────────────────────────┐
        │ Frontend                                                        │
        │  Help menu / SpecTask page / Session page / ErrorBoundary       │
        │             │                                                   │
        │             ▼                                                   │
        │  ReportIssueDialog                                              │
        │   1. POST /api/v1/feedback/preview  ──── assembled bundle ──┐   │
        │   2. user reviews + edits description                       │   │
        │   3. on Send:                                               │   │
        │      a. format compact summary (≤9 KB) for Crisp            │   │
        │      b. $crisp.push(['do','message:send',['text',summary]]) │   │
        │      c. trigger browser download of full bundle.json        │   │
        │      d. open Crisp window so user can drag/drop the file    │   │
        │             │                                               │   │
        │             ▼                              fallback ▼       │   │
        │   Crisp not loaded (air-gapped)?  → only download the bundle    │
        └─────────────┼───────────────────────────────────────────────────┘
                      │ /preview
        ┌─────────────▼───────────────────────────────────────────────────┐
        │ API server                                                      │
        │  feedback_handlers.go                                           │
        │   └─ collector.go  (assembles spec-task / session / log bundle) │
        └─────────────────────────────────────────────────────────────────┘
                      ▲                                          ▲
                      │ /preview                                 │
        helix support report --task <id> ─────────┘  (CLI writes bundle.json directly)
```

The API only ever **assembles** the bundle. The browser is the one that talks to Crisp (using the already-loaded `$crisp` widget), and the browser is the one that triggers the download. The API never makes outbound calls of its own.

## Where Things Live

| Concern | New file (Helix repo) |
|---|---|
| Bundle data type | `api/pkg/types/feedback.go` — `FeedbackReport`, `FeedbackContext` |
| Collector (assembles bundle) | `api/pkg/feedback/collector.go` |
| HTTP handler | `api/pkg/server/feedback_handlers.go` (POST `/api/v1/feedback/preview`) |
| CLI | `api/pkg/cli/support/report.go` — `helix support report --task <id> --session <id> --output <file>` |
| Frontend dialog | `frontend/src/components/feedback/ReportIssueDialog.tsx` |
| Frontend Crisp helper | `frontend/src/utils/crispReport.ts` — `sendReportToCrisp(summary, bundle)` |
| Frontend context (open from anywhere) | `frontend/src/contexts/ReportIssueContext.tsx` |
| ErrorBoundary integration | extend existing `frontend/src/components/system/ErrorBoundary.tsx` (don't fork) |

No new helm values, no new outbound URL, no `uploader.go` — all transport is browser-side via the Crisp widget that's already loaded for every install.

## Data Model

```go
// api/pkg/types/feedback.go
type FeedbackReport struct {
    ReportID         string            `json:"report_id"`         // ULID, generated client-side for dedup
    SubmittedAt      time.Time         `json:"submitted_at"`
    DeploymentID     string            `json:"deployment_id"`     // sha256(LICENSE_KEY)[:16] — same scheme as PingService
    Edition          string            `json:"edition"`           // "saas" | "server" | "mac-desktop" | ""
    HelixVersion     string            `json:"helix_version"`     // build SHA
    Reporter         FeedbackReporter  `json:"reporter"`
    UserDescription  string            `json:"user_description"`  // free text
    Browser          *BrowserInfo      `json:"browser,omitempty"` // populated when source=frontend
    Source           string            `json:"source"`            // "frontend" | "cli" | "errorboundary"
    Context          FeedbackContext   `json:"context"`
}

type FeedbackContext struct {
    SessionContext   *SessionContext   `json:"session_context,omitempty"`
    SpecTaskContext  *SpecTaskContext  `json:"spec_task_context,omitempty"`
    ServerLogs       string            `json:"server_logs,omitempty"`        // last ~200 lines from API
    SandboxLogs      string            `json:"sandbox_logs,omitempty"`       // hydra-collected when sandbox attached
    FrontendErrors   []FrontendError   `json:"frontend_errors,omitempty"`    // from sessionStorage
    CurrentURL       string            `json:"current_url,omitempty"`
}

type SpecTaskContext struct {
    TaskID, Name, Type, Priority    string
    Status                          types.SpecTaskStatus
    StatusUpdatedAt                 *time.Time
    HelixAppID, ExternalAgentID     string
    BranchName, LastPushCommitHash  string
    RequirementsSpec                string  // truncated to 16 KB
    TechnicalDesign                 string  // truncated to 16 KB
    ImplementationPlan              string  // truncated to 16 KB
    RecentInteractions              []InteractionSummary
    AgentWorkState                  types.AgentWorkState
}
```

## Collector Behaviour

`collector.Collect(ctx, req CollectRequest) (*FeedbackReport, error)`

1. Build the deployment ID from `cfg.LicenseKey` using the same hash as `version.PingService` so reports correlate with telemetry.
2. If `req.SpecTaskID != ""`: load the SpecTask; populate `SpecTaskContext`. Look up the linked `PlanningSessionID` to also fill `SessionContext`.
3. If `req.SessionID != ""`: list last N (default 20) interactions via the existing `store.ListInteractions(ListInteractionsQuery{SessionID})`, summarize each (id, prompt, response, state, created_at, error). Truncate per-message bodies to 4 KB.
4. Pull last 200 lines of API stdout from a small in-memory ring buffer that we add to the existing logger setup (cheap; no log file dependency).
5. If a sandbox is attached to the session/task, ask hydra for the last 100 lines from the container (reuse `hydra.SandboxOps.Logs`).
6. Strip env-var-shaped secrets from any embedded config dump (regex on `*_KEY`, `*_TOKEN`, `*_SECRET`, `*_PASSWORD`).
7. Cap total payload at 1 MB; truncate logs first if over.

## Frontend → Crisp Transport

`frontend/src/utils/crispReport.ts` exposes `sendReportToCrisp(report, opts)`. The flow:

1. **Build a compact summary** (≤9 KB, mirroring the Mac app's 9 KB budget at `for-mac/frontend/src/components/SettingsPanel.tsx:226`):
   - `report.user_description` (the free-text the user typed)
   - one-line header: `Report {report_id} — {edition} — deployment {deployment_id} — Helix {version}`
   - if spec-task context: `Task {id} — {name} — status={status} (since {status_updated_at}) — agent={helix_app_id}/{external_agent_id} — branch={branch_name}`
   - if session context: `Session {id} — {N} interactions — last error: {…}`
   - footer: `Full diagnostic bundle (XX KB) attached as helix-feedback-{report_id}.json — please drop the file into this chat.`
2. **Set Crisp user identity** from `useAccount()` — same `user:email` / `user:nickname` calls already done in `frontend/src/contexts/account.tsx:312-315`.
3. **Send the summary**: `$crisp.push(['do', 'message:send', ['text', summary]])`. Open the chat: `$crisp.push(['do', 'chat:show'])` then `['do', 'chat:open']`.
4. **Trigger a browser download** of the full `FeedbackReport` JSON as `helix-feedback-{report_id}.json`. The Crisp chat panel has a built-in paperclip — the user drags the freshly-downloaded file in. We considered `$crisp.push(['do','message:send',['file',{url,…}]])` but that requires the file to be at a public URL we'd have to host; download + drag-drop avoids that.
5. **Fallbacks**:
   - If `$crisp` is undefined or `client.crisp.chat` failed to load (air-gapped) → skip steps 2-3, just trigger the download and show a toast "Crisp chat unavailable — please attach the downloaded file to your support ticket."
   - If `cfg.Feedback.Disabled` (admin-side env var) → same as air-gapped: download only, never call `$crisp`.

Per-message text is capped at 9 KB — same number the Mac app uses; if the summary somehow blows past it we truncate (it shouldn't — there's no log content in the summary, only IDs and status).

## API Surface

```
POST /api/v1/feedback/preview
  body: FeedbackReportRequest {
      session_id?:          string,
      spec_task_id?:        string,
      user_description:     string,
      include_server_logs:  bool,     // user-toggleable in preview
      include_sandbox_logs: bool,
      browser?:             BrowserInfo,
      frontend_errors?:     []FrontendError,
      current_url?:         string,
  }
  response: 200 FeedbackReport   // the assembled bundle, ready for download + Crisp summary
```

Only one endpoint — submission happens entirely in the browser via Crisp. The API's job is to assemble, not to send.

Auth: requires a logged-in user (use the standard `requireUser` middleware). Org-scoped resources (sessions, spec tasks) go through the same `authorizeUserToResource()` check that other handlers use, so a user can't pull another org's task into their report.

## Operator config

A single env var: `HELIX_FEEDBACK_DISABLED=true` makes the dialog download-only and skip the Crisp call. Useful for customers who don't want their users hitting `client.crisp.chat`. Plumbed through `api/pkg/config/config.go` and exposed to the frontend via the existing `/api/v1/config` bootstrap so the dialog can render the right copy.

No `HELIX_FEEDBACK_URL`, no Crisp website ID override (helm installs ship the same `frontend/index.html` with the same hard-coded `CRISP_WEBSITE_ID` as SaaS — that's the existing Helix-team triage queue and we want all reports landing there).

## Frontend UX

```
┌─ Report Issue ──────────────────────────────────────────┐
│  What went wrong? (optional)                            │
│  ┌──────────────────────────────────────────────────┐   │
│  │ The agent got stuck after pushing the PR…        │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  Include:                                               │
│   ☑ Spec task context (this task: 002011)               │
│   ☑ Session interactions (last 20)                      │
│   ☑ Server logs (last 200 lines)                        │
│   ☑ Sandbox logs                                        │
│   ☑ Recent frontend errors (3 captured)                 │
│                                                         │
│  Preview ▾                                              │
│  ┌──────────────────────────────────────────────────┐   │
│  │ {                                                │   │
│  │   "report_id": "01J…",                           │   │
│  │   "deployment_id": "a1b2c3…",                    │   │
│  │   …                                              │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  Sending this will:                                     │
│   • open the Helix support chat with a summary          │
│   • download the full bundle for you to drag in         │
│              [ Cancel ]   [ Send Report ]               │
└─────────────────────────────────────────────────────────┘
```

- Description first, then **always** show the preview (collapsible) — mirrors the Mac app pattern in `SettingsPanel.tsx:309-323` where the diagnostics textarea is read-only and visible.
- On Send: open Crisp + send summary, then trigger the bundle download. Toast: "Support chat opened — please drag the downloaded file in to share full diagnostics."
- The dialog component is opened by `useReportIssue()` hook backed by `ReportIssueContext`; trigger surfaces just call `openReportDialog({ specTaskId, sessionId, frontendErrors })`.

## Key Decisions

**Crisp is the upstream — same channel as the Mac app.** Confirmed via review. The web `frontend/index.html:68-79` already loads Crisp with the same `CRISP_WEBSITE_ID` (`d69e5955-fa6a-4fe6-b1bb-c87cf7515d09`) the Mac app uses, so SaaS, helm, and Mac reports all land in the same triage queue. No new ingest infrastructure to build, run, or secure.

**Collection happens server-side, not in the browser.** The browser doesn't have access to container logs, the database, or the spec-task `PlanningSessionID`. Doing the collection in the API also means a single code path produces the bundle for both the web dialog and the CLI command.

**Bundle is JSON, not a `.zip`.** Single-file is easier to copy-paste, email, and drag into a Crisp chat. Logs are inline strings. If we ever attach binary artifacts (heap dumps, screenshots) we switch to `.zip`. The 1 MB cap keeps it under typical attachment limits — both Crisp's file uploader and email.

**Crisp text message + dragged-in file, not chunked text messages.** Crisp's per-message text cap is ~10 KB (the Mac app uses 9 KB at `SettingsPanel.tsx:226`). Chunking the bundle into 5-10 sequential messages would spam the support inbox and is rate-limited. Sending a compact summary text + a dragged-in JSON attachment keeps one message + one file per report and lets the support engineer expand the bundle in their tool of choice.

**Why not auto-upload via `$crisp.push(['do','message:send',['file',{url}]])`.** Crisp's file-message format requires a publicly fetchable URL. We'd have to host the bundle somewhere temporarily — defeats the point of avoiding new infrastructure. Drag-drop is a one-second user step and uses Crisp's native file uploader.

**License key is hashed, never sent.** Same `sha256(LICENSE_KEY)[:16]` scheme as `PingService` — gives the support engineer a stable deployment identifier (correlate with telemetry) without leaking the key. License keys themselves stay in the customer's cluster.

**Air-gapped fallback is download-only, no Crisp call.** When `client.crisp.chat` can't load (or admin sets `HELIX_FEEDBACK_DISABLED=true`), the dialog skips the `$crisp` calls entirely and just downloads the bundle. The user attaches it to whatever support channel they have (email, ticketing portal). Same UI, fewer steps.

**No automatic submission of the bundle from CLI.** `helix support report` writes the file; the operator decides whether to attach it to a Crisp message, an email, or a ticket. Avoids surprise outbound traffic from a debugging command.

## Notes for Future Implementation

- If Crisp ever exposes a hosted-file API that doesn't need a public URL (or we add a short-lived signed-upload endpoint somewhere), revisit step 4 in "Frontend → Crisp Transport" and replace the drag-drop step with a programmatic file send.
- The in-memory log ring buffer is a minor addition to the existing logger setup. If we later want full log retrieval, we can swap it for a tail of the structured log file once one exists.
- The Mac app's `CollectDiagnostics()` (in `for-mac/app.go:1120`) should eventually be refactored to call the same `/api/v1/feedback/preview` endpoint and use the same summary format, so the support team sees a consistent shape across sources. Out of scope for v1.
- Spec task agent configuration (`HelixAppID` → resolve to App row → include name + model) requires loading the App; reuse `store.GetApp()` with org-scoped auth.
