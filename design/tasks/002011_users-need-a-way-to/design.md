# Design — In-App Error Reporting

## Approaches Considered

We need a single button that works the same in three deployment shapes: SaaS (`app.helix.ml`), Helm/k8s with internet egress, and Helm/k8s air-gapped.

| Approach | Pros | Cons |
|---|---|---|
| **A. Crisp chat (mirror Mac app)** | Already wired in `frontend/src/components/app/TriggerCrisp.tsx`. Zero backend. | 10 KB cap per message kills container logs; no Crisp on air-gapped/enterprise; no structured storage; relies on a third party. |
| **B. New API endpoint → central Helix-hosted ingest** | Structured, queryable, deployment-agnostic. | Air-gapped breaks; needs new ingest service; outbound egress assumption. |
| **C. Hybrid: API collects → tries upload → falls back to downloadable bundle** | One UX, three transports. Reuses the `LaunchpadURL` outbound-call pattern that `PingService` already established. Air-gapped works via download. | More UI surface than A. |
| **D. Auto-file GitHub issues** | Public + transparent. | Privacy nightmare for logs/prompts; requires per-user GH auth. |

**Chosen: C (hybrid).** It's the only one that satisfies all three deployments with one user-facing surface, and it lets us reuse the already-proven Mac-app diagnostic-collection pattern.

## High-Level Architecture

```
                    ┌─────────────────────────────────────────────────┐
                    │ Frontend                                        │
                    │  Help menu / SpecTask page / ErrorBoundary      │
                    │             │                                   │
                    │             ▼                                   │
                    │  ReportIssueDialog (description + preview)      │
                    │             │                                   │
                    │             ▼  POST /api/v1/feedback/report     │
                    └─────────────┼───────────────────────────────────┘
                                  │
                    ┌─────────────▼───────────────────────────────────┐
                    │ API server                                      │
                    │  feedback_handlers.go                           │
                    │   ├─ collector.go   (gathers all context)       │
                    │   └─ uploader.go    (try POST → fall back)      │
                    │             │                                   │
                    │   ┌─────────┴────────────┐                      │
                    │   ▼                      ▼                      │
                    │  outbound enabled?      disabled / unreachable  │
                    │   POST upstream          return bundle as JSON  │
                    └───┼──────────────────────────────────────────┼──┘
                        │                                          │
                        ▼                                          ▼
            https://feedback.helix.ml                 Browser save-as dialog
            (out-of-scope ingest service)             or CLI file write
```

## Where Things Live

| Concern | New file (Helix repo) |
|---|---|
| Bundle data type | `api/pkg/types/feedback.go` — `FeedbackReport`, `FeedbackContext` |
| Collector (assembles bundle) | `api/pkg/feedback/collector.go` |
| Uploader (POST or return bundle) | `api/pkg/feedback/uploader.go` |
| HTTP handlers | `api/pkg/server/feedback_handlers.go` (POST `/api/v1/feedback/report`) |
| Config | extend `api/pkg/config/config.go` with `Feedback` block |
| CLI | `api/pkg/cli/support/report.go` — `helix support report --task <id> --session <id> --output <file>` |
| Frontend dialog | `frontend/src/components/feedback/ReportIssueDialog.tsx` |
| Frontend context (open from anywhere) | `frontend/src/contexts/ReportIssueContext.tsx` |
| ErrorBoundary integration | extend existing `frontend/src/components/system/ErrorBoundary.tsx` (don't fork) |
| Helm chart env passthrough | `charts/helix-controlplane/values.yaml` — add `controlplane.feedback.{url,disabled}` |

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

## Uploader Behaviour

`uploader.Submit(ctx, report) (*SubmitResult, error)` returns either `{ReferenceID, Mode: "uploaded"}` or `{Bundle: []byte, Mode: "bundle"}`.

- If `cfg.Feedback.Disabled` is true → always return Bundle.
- Else POST JSON to `cfg.Feedback.URL` with a 10 s timeout, License-Key-Hash header, and 1 retry. On any failure → return Bundle with the error attached as a warning.
- The bundle is the same JSON the upstream would have received, served to the browser as `attachment; filename="helix-feedback-{report_id}.json"`.

## API Surface

```
POST /api/v1/feedback/report
  body: FeedbackReportRequest {
      session_id?:       string,
      spec_task_id?:     string,
      user_description:  string,
      include_server_logs:  bool,    // user-toggleable in preview
      include_sandbox_logs: bool,
      browser?:          BrowserInfo,
      frontend_errors?:  []FrontendError,
      current_url?:      string,
  }
  response: 200 {
      mode: "uploaded" | "bundle",
      reference_id?: string,        // when uploaded
      bundle?:       json,          // when bundle (sent inline, max 1 MB)
      warnings?:     []string,
  }

POST /api/v1/feedback/preview
  same body as /report; returns the assembled FeedbackReport WITHOUT submitting.
  Used by the dialog to populate the preview.
```

Auth: requires a logged-in user (use the standard `requireUser` middleware). Org-scoped resources (sessions, spec tasks) go through the same `authorizeUserToResource()` check that other handlers use, so a user can't pull another org's task into their report.

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
│  Please review — submission will share this with Helix. │
│              [ Cancel ]   [ Send Report ]               │
└─────────────────────────────────────────────────────────┘
```

- Description first, then **always** show the preview (collapsible) — mirrors the Mac app pattern in `SettingsPanel.tsx:309-323` where the diagnostics textarea is read-only and visible.
- Submission shows a snackbar with the reference ID when uploaded, or triggers a download when in bundle mode.
- The dialog component is opened by `useReportIssue()` hook backed by `ReportIssueContext`; trigger surfaces just call `openReportDialog({ specTaskId, sessionId, frontendErrors })`.

## Key Decisions

**Use the existing Launchpad pattern, not Crisp, for the upstream.** The macOS app uses Crisp because it's a desktop app with a chat widget already loaded. The web frontend has Crisp too, but Crisp's 10 KB message cap can't hold container logs, and self-hosted operators may not have Crisp configured at all. The `PingService` outbound-to-Launchpad pattern (`api/pkg/version/ping.go`) is already approved by enterprise/on-prem deployments and uses the same `deployment_id` we want to attach.

**Collection happens server-side, not in the browser.** The browser doesn't have access to container logs, the database, or the spec-task `PlanningSessionID`. Doing the collection in the API also means a single code path produces the bundle for both the web dialog and the CLI command.

**Bundle is JSON, not a `.zip`.** Single-file is easier to copy-paste and email. Logs are inline strings. If we ever attach binary artifacts (heap dumps, screenshots) we switch to `.zip`, but v1 doesn't need them. The 1 MB cap keeps it under typical email-attachment limits.

**License key is hashed, never sent.** Same `sha256(LICENSE_KEY)[:16]` scheme as `PingService` — gives Helix a stable deployment identifier without leaking the key. License keys themselves stay in the customer's cluster.

**Outbound reporting is opt-out, not opt-in, for SaaS; opt-in (configurable) for self-hosted.** SaaS already has a privacy policy covering operational data. Self-hosted operators set `HELIX_FEEDBACK_URL` (defaults to `https://feedback.helix.ml/v1/report`) and can set `HELIX_FEEDBACK_DISABLED=true` to force download-only. Air-gapped customers will set the latter.

**No automatic submission of the bundle from CLI.** `helix support report` writes the file; the operator decides whether to upload it. Avoids surprise outbound traffic from a debugging command.

## Notes for Future Implementation

- The actual `feedback.helix.ml` ingest service is a separate piece of infra (out of scope here). The contract is documented in `Data Model` above so the receiving end can be built independently. For early dev, point `HELIX_FEEDBACK_URL` at a stub (e.g. webhook.site) to verify the wire format.
- The in-memory log ring buffer is a minor addition to the existing logger setup. If we later want full log retrieval, we can swap it for a tail of the structured log file once one exists.
- The Mac app's `CollectDiagnostics()` should eventually be refactored to call this same API endpoint instead of going to Crisp directly — out of scope for v1 but worth noting so we don't drift.
- Spec task agent configuration (`HelixAppID` → resolve to App row → include name + model) requires loading the App; reuse `store.GetApp()` with org-scoped auth.
