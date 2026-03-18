# Requirements: Declarative Project YAML

## Overview

A single `project.yaml` format that declaratively defines a Helix project, its inline agent, and how to start the project's code stack. The same file works for both `helix apply -f project.yaml` (CLI) and `kubectl apply -f project.yaml` (k8s operator). Sharing a project template is simply sharing a `project.yaml` file.

---

## User Stories

### 1. Define and apply a project

**As a developer**, I want to declare my entire Helix project — including its agent and how to run the code — in a single YAML file so I can version-control it and apply it idempotently.

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-backend-api
spec:
  description: "Backend API project"
  github_repo_url: "https://github.com/org/repo"
  default_branch: main
  technologies:
    - Go
    - PostgreSQL
  guidelines: |
    Use conventional commits. All PRs require tests.

  startup:
    install: "go mod download"
    start: "go run ./cmd/server"

  agent:
    name: "Project Assistant"
    model: claude-sonnet-4-6
    provider: anthropic
    system_prompt: |
      You are a coding assistant for this Go/PostgreSQL project.
    tools:
      web_search: true
    mcps:
      - name: github
        transport: stdio
        command: npx
        args: ["-y", "@modelcontextprotocol/server-github"]
        env:
          GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

**Acceptance Criteria:**
- `helix apply -f project.yaml` creates or updates a project by name (idempotent)
- Requires `--organization` flag (personal orgs are not supported)
- Re-applying with changed fields updates in place — never duplicates
- The inline `agent` block creates or updates a Helix App linked to the project (idempotent by agent name within org)
- Output prints project ID and agent app ID
- Errors if required fields (`metadata.name`, org) are missing

---

### 2. Startup: run the code stack

**As a developer**, I want to declare how my project's code stack is started so that Helix agents can launch the application and debug it while it runs.

The `startup` block has two fields:

| Field | Purpose |
|---|---|
| `startup.install` | Shell command to install dependencies, run once after cloning. E.g. `npm install`, `go mod download`, `pip install -r requirements.txt` |
| `startup.start` | Entry point command to start the application, run in the repo root. E.g. `npm start`, `go run ./cmd/server`, `python app.py` |

**Acceptance Criteria:**
- `startup.install` and `startup.start` are stored persistently on the project (DB columns, not git file)
- Both fields are optional; agents can still work without them
- The `start` command is a relative shell command run in the root of the cloned `github_repo_url` repository
- `install` is run once before `start` (e.g. after a fresh clone)
- The startup configuration is available to Helix agents via the project API so they know how to launch the stack

---

### 3. Idempotency guarantee

**As a developer**, I want re-applying the same or updated YAML to be safe — it must update the existing project and agent, never create duplicates.

**Idempotency keys:**

| Resource | Key |
|---|---|
| Project | `(metadata.name, organization_id)` |
| Agent app | `(agent.name, organization_id)` — defaults to `"<project-name> Assistant"` |
| k8s Project | `k8s.<namespace>.<metadata.name>` |

**Acceptance Criteria:**
- Applying twice with no changes: no-op (same IDs returned)
- Applying with changed `spec.agent.model`: agent is updated in place, same app ID
- Applying with changed `spec.guidelines`: project is updated in place, same project ID
- k8s operator: `kubectl apply` on an updated Project spec updates the Helix project and agent; uses `Status.AgentAppID` for subsequent reconciles instead of re-searching by name

---

### 4. K8s operator: apply via kubectl

**As a platform engineer**, I want to `kubectl apply -f project.yaml` and have the operator create the Helix project and agent idempotently.

**Acceptance Criteria:**
- `Project` CRD exists (`helix.ml/v1alpha1`, kind `Project`)
- Operator reconciles create/update/delete with Helix API
- Project name namespaced as `k8s.<namespace>.<name>`
- `spec.startup` fields are passed through to the Helix project
- `ProjectStatus` reports `Ready`, `ProjectID`, `AgentAppID`, `LastSyncedAt`
- On subsequent reconciles, uses `Status.AgentAppID` to update the agent directly (not re-search by name)

---

### 5. K8s Operator: Bug Fixes

**Acceptance Criteria:**
- Fix: GPTScript conversion loop was iterating `Zapier` instead of `GPTScripts` (resolved: documented as unsupported since `types.AssistantConfig` has no `GPTScripts` field)
- Fix: Knowledge sources not converted from operator CRD types to Helix API types
- Add: `HELIX_TLS_SKIP_VERIFY` env var support
- Add: `OrganizationID` field in `AIAppSpec`
- Add: `AIAppStatus` exposes `Ready`, `AppID`, `LastSyncedAt`, `Message`

---

## Out of Scope

- Writing startup config back to `.helix/startup.sh` in the git repo (startup is stored as DB fields, not git files)
- Multi-document YAML (`---` separated resources in one file)
- Declarative SpecTask management via YAML
- `--template` flag for org sample project templates (deferred)
