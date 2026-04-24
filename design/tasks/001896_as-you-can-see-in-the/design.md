# Design: Helix API Gaps for "Helix Jobs"

## Current State of the Helix API

The Helix API already provides substantial infrastructure. Here's what exists and what's missing for the Jobs use case.

### What Already Works

| Capability | API | Notes |
|---|---|---|
| **Create sessions without spec tasks** | `POST /api/v1/sessions/chat` | Sessions are independent of the Kanban workflow. No `SpecTaskID` is required. |
| **Continue existing sessions** | `POST /api/v1/sessions/chat` with `session_id` | Appends new messages to existing conversation. |
| **External agent (Zed) sessions** | `agent_type: "zed_external"` in chat request | Launches autonomous desktop container with Zed IDE. No hard timeout. |
| **Streaming output** | `stream: true` in chat request | Two formats: OpenAI-compatible SSE (flat text deltas) and response entries format (structured `EntryPatches` via WebSocket, supports in-place content modification). Jobs must use the response entries format — see note below. |
| **Project-scoped MCP servers** | `Project.Skills.MCPs` | MCPs configured per project via YAML or API. Three transports: HTTP, SSE, stdio. |
| **Cron scheduling** | `CronTrigger` on Apps | Runs agent sessions on schedule. Min 90s interval. Uses `gocron`. |
| **Webhook triggers** | Discord, Slack, Teams, Azure DevOps | Platform-specific webhook receivers exist. |
| **Secrets per project** | `POST /api/v1/projects/{id}/secrets` | AES-256-GCM encrypted. Injected as env vars into containers. |
| **Exploratory sessions** | `POST /api/v1/projects/{id}/exploratory-session` | Desktop session tied to project, no spec task. |
| **Session resume** | `POST /api/v1/sessions/{id}/resume` | Restarts a paused external agent. |
| **Trigger execution history** | `TriggerExecution` records | Stores output, status, session ID per execution. |
| **Email notifications** | `EventCronTriggerComplete/Failed` | Email sent on cron trigger completion. |

### Streaming Format: Response Entries, Not OpenAI-Compatible

The Jobs system must use the **response entries** streaming format, not the OpenAI-compatible SSE format. Agent responses are structured — they interleave text and tool calls, and content can be modified after it's been sent (e.g., a tool call's status changes from "In Progress" to "Completed"). The OpenAI-compatible format only streams flat text deltas with no way to update previous content.

The response entries format is already used by external agent (Zed) sessions:
- Streams `EntryPatches` via WebSocket pubsub, with per-entry typed deltas (`type: "text"` or `"tool_call"`)
- Supports in-place modification: same `MessageID` = replace content, different `MessageID` = new entry
- Includes tool metadata (`ToolName`, `ToolStatus`) as first-class fields
- Throttled at 50ms for frontend publishes, 5s for DB writes
- DB stores final state as `Interaction.ResponseEntries` (JSONB array of `ResponseEntry` objects)

Since Jobs will use external agent sessions (`agent_type: "zed_external"`), this format is already the default. No changes needed — just noting the requirement so the Jobs frontend consumes `EntryPatches` rather than OpenAI SSE chunks.

**Key files:** `api/pkg/server/wsprotocol/accumulator.go` (ResponseEntry type, MessageAccumulator), `api/pkg/server/websocket_external_agent_sync.go` (patch publishing logic).

### Identified Gaps

#### Gap 1: No "unmanaged" session mode (bypass spec task orchestrator)
**Problem:** Sessions created via the API are indistinguishable from spec-task-managed sessions. Phil needs sessions that exist outside the spec task orchestrator — no Kanban board, no planning/review lifecycle — but are still fully functional (desktop streaming, embedded session viewer, etc.).

**Current state:** `SessionMetadata` has `SessionRole` (planning/implementation/coordination/exploratory) and a `system_session` flag used internally. But there's no API-exposed way to mark a session as unmanaged, and the frontend doesn't filter by role. Sessions can already be created without a `SpecTaskID`, but the UI doesn't differentiate them.

**Proposed fix:** Add `"managed": false` or `"session_role": "job"` to `SessionChatRequest`. Sessions with this role bypass the spec task orchestrator but remain fully viewable — desktop streaming and the embedded session viewer (iframe embed) work as normal. The session list endpoint (`GET /sessions`) gains a `role` or `exclude_roles` query parameter so the Jobs UI can list its own sessions and the main Helix UI can filter them out. This is purely a filtering/categorization concern, not a visibility restriction.

**Files:** `api/pkg/types/types.go` (SessionChatRequest, SessionMetadata), `api/pkg/server/session_handlers.go` (list handler filtering).

#### Gap 2: Cron triggers can't start external agent sessions
**Problem:** Phil's jobs need full desktop/Zed agents (for git operations, running code, etc.), but the cron trigger system only creates inference sessions.

**Current state:** `trigger_cron.go` calls `startChatSessionHandler` with basic prompt. The `CronTrigger` type has no `AgentType` field. It always creates a standard LLM inference session.

**Proposed fix:** Add `agent_type` field to `CronTrigger`. When set to `"zed_external"`, the cron executor creates a desktop agent session with the project's agent config. Also add `project_id` to `CronTrigger` so the agent inherits project MCP/startup/secrets config.

**Files:** `api/pkg/types/types.go` (CronTrigger), `api/pkg/trigger/cron/trigger_cron.go` (execution logic).

#### Gap 3: No persistent agent working directory between runs
**Problem:** Phil's agents maintain state in markdown files (task lists, knowledge notes, questions). Each cron run needs to see the output of previous runs. Currently, containers are ephemeral — destroyed after each session.

**Current state:** The filestore system (`api/pkg/filestore/`) supports per-user and per-app file storage. Golden cache (`DockerCacheState`) provides pre-built container snapshots per project. But there's no persistent working directory that survives between sessions.

**Design: One job = one project, state on the helix-specs branch.**

Each job maps 1:1 to a Helix project. The project's primary git repo already has a `helix-specs` branch (used by the spec task flow for design docs in per-task subdirectories). For jobs, the agent's markdown state files (persona definition, task lists, knowledge notes, questions, append-only log) live as **top-level files in the `helix-specs` branch** — no per-task subdirectories needed since there's only one job per project.

This reuses existing infrastructure:
- The `helix-specs` branch already exists on every project repo
- Helix already has git clone/push machinery for spec tasks
- The agent's state is automatically versioned in git

**What Helix needs to add:**
1. **Auto-restore on session start:** When an unmanaged (job) session starts for a project, the existing machinery checks out the `helix-specs` branch into `~/work/helix-specs` — same as it does for spec tasks. The agent works directly from there.
2. **Auto-commit on session end:** When the session completes, commit and push any changes to the state files back to the `helix-specs` branch. This should be transparent to the agent — Helix handles it, so we don't rely on the agent remembering to commit.
3. **Job naming:** The project name serves as the job name (1:1 mapping). No separate job naming needed.

**Files:** `api/pkg/external-agent/hydra_executor.go` (container setup), `api/pkg/hydra/devcontainer.go` (mount points), existing git clone/push infrastructure in spec task service.

#### Gap 4: Cron prompt from file reference
**Problem:** Phil's agent prompts are long markdown files, not short inline strings. The current `CronTrigger.Input` is a string field — awkward for multi-page prompts.

**Current state:** `CronTrigger` has `Input string` for the prompt. It's passed directly to the session.

**Proposed fix:** Add `InputFile` field to `CronTrigger` — a path relative to a project repository (e.g., `agents/researcher/prompt.md`). At execution time, read the file from the repo's default branch and use its contents as the prompt. Falls back to `Input` if `InputFile` is empty.

**Files:** `api/pkg/types/types.go` (CronTrigger), `api/pkg/trigger/cron/trigger_cron.go`.

#### Gap 5: No webhook callback on session completion
**Problem:** Phil needs to know when an agent finishes without polling. Current notifications are email-only.

**Current state:** `notification.go` supports email only. `EventCronTriggerComplete` sends email.

**Proposed fix:** Add `CallbackURL` field to `CronTrigger` and to `SessionChatRequest`. On session completion, POST to the callback URL with `{"session_id": "...", "status": "success|error", "output": "..."}`. Implement as a new `WebhookNotifier` alongside the existing `EmailNotifier`.

**Files:** `api/pkg/notification/` (new webhook notifier), `api/pkg/types/types.go` (add CallbackURL fields).

#### Gap 6: Output retrieval is indirect
**Problem:** After a cron-triggered session completes, getting the output requires chaining two queries: trigger execution → session ID → session interactions → last response. There's no single-call way to get "what did this job produce?"

**Current state:** `TriggerExecution.Output` captures the response string. But the full structured output (tool calls, file changes) requires fetching the full session.

**Proposed fix:** Add `GET /api/v1/sessions/{id}/output` endpoint that returns the last interaction's response message, any file artifacts from the filestore, and the session status. Single call, no chaining needed.

**Files:** `api/pkg/server/session_handlers.go` (new handler).

## Architecture Decision: Build External vs. Extend Helix

From the transcript, Luke and Phil agreed:
- Phil builds the Jobs frontend (HTMX + Go) as a **separate folder in the Helix monorepo**
- Luke provides **clean APIs** that Phil consumes
- The Jobs system uses Helix as the **agent execution layer via REST API**
- **One job = one Helix project** (1:1 mapping). The project provides: agent config, MCP servers, startup script, secrets, and the primary git repo
- The project name serves as the job name
- Job state (persona markdown, task lists, notes, log) lives as top-level files on the `helix-specs` branch of the project's primary repo
- Helix auto-restores state files at session start and auto-commits at session end

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
