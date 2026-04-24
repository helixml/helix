# Requirements: Helix API Gaps for "Helix Jobs" Task System

## Context

Phil and Luke are prototyping a new **task-based system** ("Helix Jobs") that builds on Helix primitives. The system supports two types of work:

1. **Ad hoc tasks** — one-off agent executions triggered externally (webhooks from phone, Slack, etc.)
2. **Recurring jobs** — cron-like agent responsibilities tied to a "role" (check email, review repos, file research notes)

Each agent has three separate concerns:
- **Persona** — markdown files defining the agent's role, rules, knowledge
- **Agent runtime** — the LLM engine (Claude Code, Codex, Qwen Code)
- **Sandbox** — the container environment with tools (Python, MCP servers, etc.)

Phil's prototype runs Claude Code in Docker with `--output-format stream-json`, reading prompts from markdown files and writing state back to markdown. Luke proposes rebuilding this on top of the Helix API so that Helix handles agent execution, sandboxing, and MCP integration.

## User Stories

### US-1: Jobs UI in Helix (hidden developer page)
**As** Phil (or another developer prototyping jobs),
**I want** a minimal `/jobs` page in Helix that lets me create/select projects, configure job files, and start/stop runs,
**So that** I can prototype and test the Jobs system through a UI while also seeing the equivalent API calls to integrate into my own system.

**Note:** This page is not published in the nav bar and not publicly accessible to users. It's a developer tool for prototyping. It can look rough — functionality over aesthetics.

**Acceptance Criteria:**
- [ ] `/jobs` route exists in the Helix frontend, not linked from the nav bar
- [ ] **Project management:** Create a new project or select an existing project. Copies over existing project configuration, skills/MCPs, startup script, secrets.
- [ ] **Job file editing:** Within a selected project, show three text boxes corresponding to files that get written to the `helix-specs` branch inside the `job/` folder (e.g., persona/prompt, task list, notes — the exact file names can be decided during implementation)
- [ ] **Run management:** Start and stop runs (unmanaged agent sessions running against the selected project). Show run status and link to the job detail view (embedding `EmbeddedSessionView` + `ExternalAgentDesktopViewer` components) for viewing the desktop stream and chat.
- [ ] **Cron configuration:** UI to configure cron triggers that kick off job runs on a schedule
- [ ] **API call display:** At each interaction point, show the equivalent API call (curl/JSON) so Phil can see how to replicate it in his system

### US-2: Start an unmanaged agent session via API (backend)
**As** an external system (Helix Jobs frontend),
**I want** to start an agent session with a prompt and project config, without going through the spec task orchestrator,
**So that** I can run agent work programmatically without Kanban state management.

**Note:** "Unmanaged" means not managed by the spec task orchestrator (no Kanban board, no planning/review workflow). The session itself is still fully functional — it supports desktop streaming, the embedded session viewer, and all normal session features. It just isn't part of the spec task lifecycle.

**Debugging/testing in the Helix UI:** Job sessions should reuse the existing viewer components — `EmbeddedSessionView` (chat/interactions) and `ExternalAgentDesktopViewer` (desktop stream) — which both only require a `sessionId` prop. No SpecTask object is needed. The session list API already supports `project_id` filtering, so all sessions for a job's project can be listed. The only new filtering needed is by `session_role` so the main UI and Jobs UI can each show the sessions relevant to them.

**Acceptance Criteria:**
- [ ] POST `/api/v1/sessions/chat` accepts a flag (e.g. `"managed": false` or `"session_role": "job"`) that creates a session outside the spec task orchestrator
- [ ] The session still uses the project's agent config, MCP servers, startup script, and secrets
- [ ] The session is viewable in the existing Helix UI, reusing the `EmbeddedSessionView` and `ExternalAgentDesktopViewer` components (both only require a session ID, no SpecTask object needed)
- [ ] The session appears in the sessions sidebar and is discoverable via `GET /api/v1/sessions?project_id=...`
- [ ] The session can be either streaming (SSE) or blocking (synchronous JSON response)
- [ ] The session ID is returned immediately so the caller can poll for results
- [ ] The session list endpoint adds `session_role` filtering so the Jobs UI can list its own sessions and the main Helix UI can exclude them if desired

### US-3: Run a long-running autonomous agent
**As** an external orchestrator,
**I want** to start a Zed/desktop agent session that runs autonomously for minutes or hours with a prompt defined in markdown,
**So that** agents can perform complex multi-step tasks (clone repos, run code, create PRs).

**Acceptance Criteria:**
- [ ] The agent receives the full markdown prompt as its initial input
- [ ] The agent has access to project-configured MCP servers and secrets (GitHub token, etc.)
- [ ] The agent runs until it completes or is explicitly stopped — no hard timeout
- [ ] Progress can be observed via the existing WebSocket sync or session interaction polling

### US-4: Trigger agent sessions on a schedule
**As** a user defining agent "roles",
**I want** to configure periodic agent runs (cron-style) that execute with a specific prompt and project context,
**So that** agents can perform ongoing responsibilities (check email, review code, file notes).

**Acceptance Criteria:**
- [ ] Existing cron trigger system (`/api/pkg/trigger/cron/`) supports triggering external agent (Zed) sessions, not just inference sessions
- [ ] Cron triggers can target a specific project (for MCP + startup script config)
- [ ] Cron trigger input can reference a markdown file path (in a repo) rather than inline prompt text
- [ ] Trigger execution history is queryable via API with session IDs and outputs

### US-5: Agent file persistence between runs
**As** an agent running a recurring job,
**I want** to read and write markdown files that persist between runs,
**So that** I can maintain state (task lists, knowledge notes, questions for the user).

**Note:** One job maps 1:1 to one Helix project. The project's primary repo already has a `helix-specs` branch. Job state files (persona, tasks, notes, log) live as files inside a `job/` folder on that branch — no per-task subdirectories like the spec task flow.

**Acceptance Criteria:**
- [ ] On session start, Helix checks out the `helix-specs` branch into `~/work/helix-specs` (reusing existing machinery)
- [ ] On session completion, Helix auto-commits and pushes any changes back to the `helix-specs` branch
- [ ] This is transparent to the agent — Helix handles restore/commit, not the agent
- [ ] State files are versioned in git (change history preserved automatically)

### US-6: Retrieve agent output after completion
**As** an external system or UI,
**I want** to query the final output of a completed agent session,
**So that** I can display results, send notifications, or feed output into other workflows.

**Acceptance Criteria:**
- [ ] GET endpoint returns the final interaction response for a given session ID
- [ ] For cron-triggered sessions, the trigger execution record links to the session and captures output
- [ ] Output includes both text responses and any file artifacts the agent produced

### US-7: Notification on task completion
**As** an external system,
**I want** to receive a webhook callback when an agent session completes,
**So that** I can take action on the results without polling.

**Acceptance Criteria:**
- [ ] Completion webhook URL can be configured per trigger or per session
- [ ] Webhook payload includes session ID, status (success/error), and summary
- [ ] Falls back to existing email notification if no webhook configured

## Out of Scope (for now)

- Phil's full HTMX frontend (he builds this separately; US-1 is just a minimal developer page in Helix)
- Agent persona marketplace or sharing
- Multi-agent orchestration (agents talking to each other)
- Custom container images per job (use startup scripts for now)
