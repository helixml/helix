# Helix Jobs API Walkthrough

This document walks through the APIs Phil would use to set up and run a "job" — an autonomous agent with a defined role, running against a Helix project.

**Convention:** Each API call is marked with its status:
- **EXISTS** — available today, no changes needed
- **PROPOSED** — new or modified, needs implementation

All requests use `Authorization: Bearer $TOKEN` and `Content-Type: application/json`.

---

## Overview

A job maps 1:1 to a Helix project. The project provides:
- Agent configuration (which LLM, which runtime)
- MCP servers (tools the agent can use)
- Startup script (install dependencies in the container)
- Secrets (GitHub token, API keys, etc.)
- A git repo with a `helix-specs` branch where job state files live in a `job/` folder

A "run" is an unmanaged session — a desktop agent session that isn't managed by the spec task orchestrator.

---

## Step 1: Create a Project

Use the declarative YAML apply endpoint to create or update a project. This is idempotent — calling it again with the same name updates the existing project.

**`PUT /api/v1/projects/apply`** — EXISTS

```bash
curl -X PUT "$HELIX_URL/api/v1/projects/apply" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "organization_id": "org_123",
    "name": "code-reviewer",
    "spec": {
      "description": "Reviews repositories for architecture issues",
      "technologies": ["Go", "Python"],
      "guidelines": "Focus on security and maintainability patterns.",

      "repositories": [
        {
          "url": "https://github.com/myorg/my-repo",
          "branch": "main",
          "primary": true
        }
      ],

      "startup": {
        "script": "#!/bin/bash\nset -e\napt-get update && apt-get install -y python3-pip\npip install pandas"
      },

      "agent": {
        "name": "Code Reviewer",
        "model": "claude-sonnet-4-6",
        "provider": "anthropic",
        "runtime": "claude_code",
        "tools": {
          "web_search": true
        }
      }
    }
  }'
```

**Response:**
```json
{
  "project_id": "prj_abc123",
  "agent_app_id": "app_def456",
  "created": true
}
```

Save `project_id` — you'll use it for everything else.

---

## Step 2: Add Secrets

Store credentials that get injected as environment variables into the agent's container.

**`POST /api/v1/projects/{project_id}/secrets`** — EXISTS

```bash
curl -X POST "$HELIX_URL/api/v1/projects/prj_abc123/secrets" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "GITHUB_TOKEN",
    "value": "ghp_xxxxxxxxxxxx"
  }'
```

**Response:**
```json
{
  "id": "sec_789",
  "name": "GITHUB_TOKEN",
  "project_id": "prj_abc123",
  "created": "2026-04-24T09:00:00Z"
}
```

The secret is encrypted (AES-256-GCM) at rest. It will be available as the environment variable `GITHUB_TOKEN` inside the agent's container.

---

## Step 3: Write Job State Files

Job state files live in the `job/` folder on the `helix-specs` branch of the project's primary repo. These files define the agent's persona, track its work, and persist between runs.

Use the git file contents API to write files. Content must be base64-encoded.

**`PUT /api/v1/git/repositories/{repo_id}/contents`** — EXISTS

### 3a. Write the persona/prompt file

```bash
curl -X PUT "$HELIX_URL/api/v1/git/repositories/repo_xyz/contents" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "job/prompt.md",
    "branch": "helix-specs",
    "message": "Add job prompt",
    "content": "'$(echo -n '# Code Reviewer

## Role
You are a senior architect reviewing repositories for quality issues.

## What To Do
1. Clone the repository
2. Review the codebase for architecture anti-patterns, security issues, and maintainability concerns
3. Update `job/findings.md` with your findings
4. Update `job/tasks.md` with any recommended actions

## Rules
- Be specific — cite file paths and line numbers
- Prioritize security issues over style
- Append to findings.md, never overwrite previous entries
' | base64 -w 0)'"
  }'
```

### 3b. Write initial state files

```bash
# Initialize findings log
curl -X PUT "$HELIX_URL/api/v1/git/repositories/repo_xyz/contents" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "job/findings.md",
    "branch": "helix-specs",
    "message": "Initialize findings log",
    "content": "'$(echo -n '# Findings Log
' | base64 -w 0)'"
  }'

# Initialize task list
curl -X PUT "$HELIX_URL/api/v1/git/repositories/repo_xyz/contents" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "path": "job/tasks.md",
    "branch": "helix-specs",
    "message": "Initialize task list",
    "content": "'$(echo -n '# Tasks
' | base64 -w 0)'"
  }'
```

These files will be checked out into `~/work/helix-specs/job/` when the agent starts. Any changes the agent makes will be auto-committed back after the session ends.

---

## Step 4: Start a Run (Ad Hoc Task)

A "run" is an unmanaged session — a desktop agent that executes against the project's configuration.

**`POST /api/v1/sessions/chat`** — EXISTS (with proposed `session_role` field)

```bash
curl -X POST "$HELIX_URL/api/v1/sessions/chat" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "project_id": "prj_abc123",
    "agent_type": "zed_external",
    "stream": false,
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "text",
            "text": "Review the codebase following the instructions in ~/work/helix-specs/job/prompt.md. Update the findings and tasks files."
          }
        ]
      }
    ]
  }'
```

**Response:**
```json
{
  "id": "ses_run001",
  "project_id": "prj_abc123",
  "created": "2026-04-24T10:00:00Z",
  "config": {
    "agent_type": "zed_external",
    "streaming": true
  },
  "interactions": [
    {
      "id": "int_001",
      "state": "waiting",
      "prompt_message": "Review the codebase..."
    }
  ]
}
```

Save the session `id` — use it to check status, view in the UI, or stop the agent.

### What happens behind the scenes

1. Helix creates a desktop container with the project's startup script and secrets
2. The `helix-specs` branch is checked out into `~/work/helix-specs/`
3. The agent reads `job/prompt.md` and starts working
4. The agent can modify files in `job/` (findings, tasks, etc.)
5. When the session ends, Helix auto-commits changes back to the `helix-specs` branch

### PROPOSED: `session_role` field

To distinguish job sessions from regular sessions, we propose adding a `session_role` field:

```json
{
  "project_id": "prj_abc123",
  "agent_type": "zed_external",
  "session_role": "job",
  "messages": [...]
}
```

This lets the Jobs UI filter its sessions separately from the main Helix UI. **Status: PROPOSED** — not yet implemented.

---

## Step 5: Check Run Status

### 5a. Poll session state

**`GET /api/v1/sessions/{session_id}`** — EXISTS

```bash
curl "$HELIX_URL/api/v1/sessions/ses_run001" \
  -H "Authorization: Bearer $TOKEN"
```

Check `interactions[-1].state`:
- `"waiting"` — agent is still working
- `"complete"` — agent finished
- `"error"` — agent hit an error

### 5b. List sessions for a project

**`GET /api/v1/sessions?project_id={project_id}`** — EXISTS

```bash
curl "$HELIX_URL/api/v1/sessions?project_id=prj_abc123&page_size=10" \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "sessions": [
    {
      "id": "ses_run001",
      "name": "Code review run",
      "created": "2026-04-24T10:00:00Z",
      "updated": "2026-04-24T10:15:00Z"
    }
  ],
  "page": 0,
  "page_size": 10,
  "total_count": 1,
  "total_pages": 1
}
```

### PROPOSED: Filter by session role

```bash
curl "$HELIX_URL/api/v1/sessions?project_id=prj_abc123&session_role=job" \
  -H "Authorization: Bearer $TOKEN"
```

**Status: PROPOSED** — `session_role` query parameter not yet exposed.

---

## Step 6: View a Run in the UI

Open the session in the Helix UI to see the desktop stream and chat:

```
$HELIX_URL/orgs/{org_id}/session/{session_id}
```

This shows the full desktop stream (what the agent sees) and the chat/interaction history. The `EmbeddedSessionView` and `ExternalAgentDesktopViewer` components only need a session ID — no spec task object required.

---

## Step 7: Stop a Run

**`DELETE /api/v1/sessions/{session_id}/stop-external-agent`** — EXISTS

```bash
curl -X DELETE "$HELIX_URL/api/v1/sessions/ses_run001/stop-external-agent" \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "message": "external Zed agent stopped",
  "session_id": "ses_run001"
}
```

After stopping, Helix auto-commits any file changes the agent made back to the `helix-specs` branch.

---

## Step 8: Read Job State Files

After a run completes, read the agent's updated state files.

**`GET /api/v1/git/repositories/{repo_id}/contents`** — EXISTS

```bash
# Read findings
curl "$HELIX_URL/api/v1/git/repositories/repo_xyz/contents?path=job/findings.md&branch=helix-specs" \
  -H "Authorization: Bearer $TOKEN"
```

**Response:**
```json
{
  "path": "job/findings.md",
  "content": "IyBGaW5kaW5ncyBMb2cK..."
}
```

The `content` field is base64-encoded. Decode it to see the agent's output.

---

## Recurring Jobs: Configure a Cron Trigger

For recurring jobs (check email every morning, review repos weekly), set up a cron trigger on the project's app.

### PROPOSED: Cron trigger with `agent_type` and `project_id`

The current cron trigger system only creates inference sessions. To create external agent (Zed) sessions, we propose adding `agent_type` to the `CronTrigger`:

```json
{
  "enabled": true,
  "schedule": "0 9 * * 1-5",
  "input": "Review the codebase following ~/work/helix-specs/job/prompt.md",
  "agent_type": "zed_external",
  "project_id": "prj_abc123",
  "emails": ["phil@example.com"]
}
```

This would be configured via the App API (`PUT /api/v1/apps/{app_id}`), updating the app's trigger configuration.

**Status: PROPOSED** — `agent_type` field on CronTrigger not yet implemented.

### PROPOSED: Cron prompt from file reference

Instead of inline prompt text, reference a file in the `helix-specs` branch:

```json
{
  "enabled": true,
  "schedule": "0 9 * * 1-5",
  "input_file": "job/prompt.md",
  "agent_type": "zed_external",
  "project_id": "prj_abc123"
}
```

**Status: PROPOSED** — `input_file` field not yet implemented.

---

## Summary: What Exists vs. What's Proposed

| Step | API | Status |
|------|-----|--------|
| Create project | `PUT /api/v1/projects/apply` | **EXISTS** |
| Add secrets | `POST /api/v1/projects/{id}/secrets` | **EXISTS** |
| Write job files to git | `PUT /api/v1/git/repositories/{id}/contents` | **EXISTS** |
| Start a run (session) | `POST /api/v1/sessions/chat` with `project_id` + `agent_type` | **EXISTS** |
| `session_role` field on session | `session_role: "job"` in chat request | **PROPOSED** |
| Check run status | `GET /api/v1/sessions/{id}` | **EXISTS** |
| List project sessions | `GET /api/v1/sessions?project_id=...` | **EXISTS** |
| Filter sessions by role | `GET /api/v1/sessions?session_role=job` | **PROPOSED** |
| View run in UI | `$HELIX_URL/orgs/{org}/session/{id}` | **EXISTS** |
| Stop a run | `DELETE /api/v1/sessions/{id}/stop-external-agent` | **EXISTS** |
| Read job state files | `GET /api/v1/git/repositories/{id}/contents` | **EXISTS** |
| Auto-commit state on session end | Transparent git commit/push | **PROPOSED** |
| Cron with `agent_type` | `agent_type` on CronTrigger | **PROPOSED** |
| Cron with file reference | `input_file` on CronTrigger | **PROPOSED** |
| Session output endpoint | `GET /api/v1/sessions/{id}/output` | **PROPOSED** |
| Webhook callback on completion | `CallbackURL` on CronTrigger/SessionChatRequest | **PROPOSED** |

Most of the workflow works today. The proposed additions are about making it cleaner (session role filtering, file-based prompts) and more automated (auto-commit, cron for external agents, completion callbacks).
