# Design: Helix API Gaps for "Helix Jobs"

## Current State of the Helix API

The Helix API already provides substantial infrastructure. Here's what exists and what's missing for the Jobs use case.

### What Already Works

| Capability | API | Notes |
|---|---|---|
| **Create sessions without spec tasks** | `POST /api/v1/sessions/chat` | Sessions are independent of the Kanban workflow. No `SpecTaskID` is required. |
| **Continue existing sessions** | `POST /api/v1/sessions/chat` with `session_id` | Appends new messages to existing conversation. |
| **External agent (Zed) sessions** | `agent_type: "zed_external"` in chat request | Launches autonomous desktop container with Zed IDE. No hard timeout. |
| **Streaming output** | `stream: true` in chat request | SSE streaming with `text/event-stream`. Also blocking mode. |
| **Project-scoped MCP servers** | `Project.Skills.MCPs` | MCPs configured per project via YAML or API. Three transports: HTTP, SSE, stdio. |
| **Cron scheduling** | `CronTrigger` on Apps | Runs agent sessions on schedule. Min 90s interval. Uses `gocron`. |
| **Webhook triggers** | Discord, Slack, Teams, Azure DevOps | Platform-specific webhook receivers exist. |
| **Secrets per project** | `POST /api/v1/projects/{id}/secrets` | AES-256-GCM encrypted. Injected as env vars into containers. |
| **Exploratory sessions** | `POST /api/v1/projects/{id}/exploratory-session` | Desktop session tied to project, no spec task. |
| **Session resume** | `POST /api/v1/sessions/{id}/resume` | Restarts a paused external agent. |
| **Trigger execution history** | `TriggerExecution` records | Stores output, status, session ID per execution. |
| **Email notifications** | `EventCronTriggerComplete/Failed` | Email sent on cron trigger completion. |

### Identified Gaps

#### Gap 1: No "headless" session flag
**Problem:** All sessions appear in the UI session list. Phil needs background agent sessions that don't clutter the user's workspace.

**Current state:** `SessionMetadata` has `SessionRole` (planning/implementation/coordination/exploratory) and a `system_session` flag used internally. But there's no API-exposed "headless" or "background" flag, and the frontend doesn't filter by role.

**Proposed fix:** Add `"headless": true` to `SessionChatRequest`. Sessions with `headless=true` get `SessionMetadata.SessionRole = "job"`. The session list endpoint (`GET /sessions`) gains a `exclude_roles=job` query parameter (defaulting to excluding jobs). This keeps jobs queryable but hidden from the main UI.

**Files:** `api/pkg/types/types.go` (SessionChatRequest, SessionMetadata), `api/pkg/server/session_handlers.go` (list handler filtering).

#### Gap 2: Cron triggers can't start external agent sessions
**Problem:** Phil's jobs need full desktop/Zed agents (for git operations, running code, etc.), but the cron trigger system only creates inference sessions.

**Current state:** `trigger_cron.go` calls `startChatSessionHandler` with basic prompt. The `CronTrigger` type has no `AgentType` field. It always creates a standard LLM inference session.

**Proposed fix:** Add `agent_type` field to `CronTrigger`. When set to `"zed_external"`, the cron executor creates a desktop agent session with the project's agent config. Also add `project_id` to `CronTrigger` so the agent inherits project MCP/startup/secrets config.

**Files:** `api/pkg/types/types.go` (CronTrigger), `api/pkg/trigger/cron/trigger_cron.go` (execution logic).

#### Gap 3: No generic webhook trigger
**Problem:** Phil sends HTTP POSTs from his phone automation app. Current webhooks are platform-specific (Discord, Slack, etc.). There's no generic "send a prompt via HTTP and get a session" endpoint.

**Current state:** `TriggerConfiguration` has a `WebhookURL` field but it's only used for Azure DevOps inbound. No generic webhook receiver exists.

**Proposed fix:** Add a generic webhook endpoint: `POST /api/v1/apps/{id}/webhook` that accepts `{"prompt": "...", "project_id": "..."}` and creates a headless session. Authenticates via API key. Returns `{"session_id": "..."}`. This is simple — it's essentially `startChatSessionHandler` wrapped with trigger execution logging.

**Files:** `api/pkg/server/app_trigger_handlers.go` (new handler), `api/pkg/types/types.go` (new trigger type or extend existing).

#### Gap 4: No persistent agent working directory between runs
**Problem:** Phil's agents maintain state in markdown files (task lists, knowledge notes, questions). Each cron run needs to see the output of previous runs. Currently, containers are ephemeral — destroyed after each session.

**Current state:** The filestore system (`api/pkg/filestore/`) supports per-user and per-app file storage. Golden cache (`DockerCacheState`) provides pre-built container snapshots per project. But there's no persistent working directory that survives between sessions.

**Proposed fix (two options):**

**Option A — Git-backed persistence (recommended):** The agent's working directory is a git repo. At session start, clone the repo (or a specific branch). At session end, commit and push changes. This gives versioning for free and aligns with Phil's existing markdown-in-git approach. The Helix API already handles git cloning for spec tasks — reuse that machinery.

**Option B — Filestore-backed volume mount:** Create a persistent filestore path per project/job. Mount it into the container at a known path (e.g., `/workspace/state/`). Survives container restarts. Simpler but no versioning.

**Decision:** Option A (git-backed) is preferred. It matches Phil's existing pattern and the Helix codebase already has git clone/push infrastructure. The agent's markdown files live in a repo, and Helix auto-commits on session completion.

**Files:** `api/pkg/external-agent/hydra_executor.go` (container setup), `api/pkg/hydra/devcontainer.go` (mount points).

#### Gap 5: Cron prompt from file reference
**Problem:** Phil's agent prompts are long markdown files, not short inline strings. The current `CronTrigger.Input` is a string field — awkward for multi-page prompts.

**Current state:** `CronTrigger` has `Input string` for the prompt. It's passed directly to the session.

**Proposed fix:** Add `InputFile` field to `CronTrigger` — a path relative to a project repository (e.g., `agents/researcher/prompt.md`). At execution time, read the file from the repo's default branch and use its contents as the prompt. Falls back to `Input` if `InputFile` is empty.

**Files:** `api/pkg/types/types.go` (CronTrigger), `api/pkg/trigger/cron/trigger_cron.go`.

#### Gap 6: No webhook callback on session completion
**Problem:** Phil needs to know when an agent finishes without polling. Current notifications are email-only.

**Current state:** `notification.go` supports email only. `EventCronTriggerComplete` sends email.

**Proposed fix:** Add `CallbackURL` field to `CronTrigger` and to `SessionChatRequest`. On session completion, POST to the callback URL with `{"session_id": "...", "status": "success|error", "output": "..."}`. Implement as a new `WebhookNotifier` alongside the existing `EmailNotifier`.

**Files:** `api/pkg/notification/` (new webhook notifier), `api/pkg/types/types.go` (add CallbackURL fields).

#### Gap 7: Output retrieval is indirect
**Problem:** After a cron-triggered session completes, getting the output requires chaining two queries: trigger execution → session ID → session interactions → last response. There's no single-call way to get "what did this job produce?"

**Current state:** `TriggerExecution.Output` captures the response string. But the full structured output (tool calls, file changes) requires fetching the full session.

**Proposed fix:** Add `GET /api/v1/sessions/{id}/output` endpoint that returns the last interaction's response message, any file artifacts from the filestore, and the session status. Single call, no chaining needed.

**Files:** `api/pkg/server/session_handlers.go` (new handler).

## Architecture Decision: Build External vs. Extend Helix

From the transcript, Luke and Phil agreed:
- Phil builds the Jobs frontend (HTMX + Go) as a **separate folder in the Helix monorepo**
- Luke provides **clean APIs** that Phil consumes
- The Jobs system uses Helix as the **agent execution layer via REST API**
- Each "job" maps to a **Helix project** (for MCP config, startup scripts, secrets)

This means the API gaps above are the **contract between the two systems**. The Jobs frontend will call these endpoints. No changes needed to the spec task workflow, Kanban, or existing frontend.

## Codebase Patterns Discovered

- **Route registration**: All routes in `api/pkg/server/server.go` lines 646-1363, grouped by `authRouter`, `adminRouter`, `runnerRouter`.
- **Handler pattern**: Each handler is a method on `HelixAPIServer`, registered via `r.HandleFunc(path, handler).Methods(...)`.
- **Type definitions**: All in `api/pkg/types/types.go` (2500+ lines) and `api/pkg/types/project.go`.
- **Session creation**: `startChatSessionHandler` in `session_handlers.go` handles both new and continued sessions.
- **Cron system**: `api/pkg/trigger/cron/trigger_cron.go` uses `gocron` and reconciles every 10s.
- **Secrets**: AES-256-GCM encrypted, stored in Postgres, injected as env vars via `GetProjectSecretsAsEnvVars()`.
- **External agents**: Launched via `externalAgentExecutor.StartDesktop()`, connect back via WebSocket at `/api/v1/external-agents/sync`.

## Constraints

- Minimum cron frequency is 90 seconds (enforced in `trigger_cron.go`).
- Container state is ephemeral — golden cache only preserves the base image, not runtime state.
- The `Session` type is large and deeply coupled — avoid adding many new fields. Use `SessionMetadata` (JSON) for extensibility.
- MCP secret substitution (`${VAR}` syntax in YAML) exists in config but isn't fully implemented for runtime injection yet.
