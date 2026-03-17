# Requirements: Declarative Project YAML

## Overview

Add a single `project.yaml` format that declaratively defines a Helix project, including its inline agent configuration. The same file works for both `helix apply -f project.yaml` (CLI) and `kubectl apply -f project.yaml` (k8s operator). Sharing a project template is simply sharing a `project.yaml` file.

---

## User Stories

### 1. Define and apply a project

**As a developer**, I want to declare my entire Helix project — including its agent — in a single YAML file so I can version-control it and apply it idempotently.

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
  agent:
    name: "Project Assistant"
    description: "Helps work on this project"
    model: claude-sonnet-4-6
    provider: anthropic
    system_prompt: |
      You are a coding assistant for this Go/PostgreSQL project.
      Follow the project guidelines and use conventional commits.
    tools:
      web_search: true
      browser: false
    mcps:
      - name: github
        transport: stdio
        command: npx
        args: ["-y", "@modelcontextprotocol/server-github"]
        env:
          GITHUB_TOKEN: "${GITHUB_TOKEN}"
    knowledge:
      - name: project-docs
        source:
          web:
            urls:
              - https://docs.example.com
```

**Acceptance Criteria:**
- `helix apply -f project.yaml` creates or updates a project by name
- Requires `--organization` flag to scope the project to an org (personal orgs are not supported)
- On update, project fields are merged/overwritten (name is the unique key within the org)
- The inline `agent` block creates or updates a Helix App (agent) linked to the project as its default agent
- Agent name is used as idempotency key: existing agent with same name is updated, otherwise created
- Output prints the project ID and agent app ID on success
- Errors if required fields (`metadata.name`, `spec.agent.model`) are missing

---

### 2. Use a project YAML as a template

**As an org admin**, I want to share a `project.yaml` file that others can fork to create their own project from a common starting point.

**Acceptance Criteria:**
- A `project.yaml` file, applied with `--template` flag, registers the project as a sample/template available to org members
- `helix project samples` lists org-registered templates alongside built-in ones
- `helix project fork <template-name>` creates a new project from a registered template
- Without `--template`, `helix apply -f project.yaml` creates a live project (not a template listing)

---

### 3. K8s operator: apply via kubectl

**As a platform engineer**, I want to `kubectl apply -f project.yaml` and have the operator create the Helix project and its agent.

**Acceptance Criteria:**
- A `Project` CRD exists in the operator (`helix.ml/v1alpha1`, kind `Project`)
- The operator reconciles `Project` resources with the Helix API (create/update/delete)
- Project name is namespaced as `k8s.<namespace>.<name>` (consistent with AIApp pattern)
- Agent defined in `spec.agent` is created/updated linked to the project
- `ProjectStatus` reports `Ready`, `ProjectID`, `AgentAppID`, `LastSyncedAt`

---

### 4. K8s Operator: Bug Fixes

**Acceptance Criteria:**
- Fix: GPTScript conversion loop iterates `assistant.Zapier` instead of `assistant.GPTScripts`
- Fix: Knowledge sources not converted from operator CRD types to Helix API types
- Add: `HELIX_TLS_SKIP_VERIFY` env var support (already TODO'd in code)
- Add: `OrganizationID` field in `AIAppSpec` so apps can be org-scoped from k8s
- Add: `AIAppStatus` exposes `Ready`, `AppID`, `LastSyncedAt`, `Message`

---

## Out of Scope

- Multi-document YAML (multiple resources in one file separated by `---`)
- Declarative SpecTask management via YAML
- Automatic import from GitHub repo metadata
