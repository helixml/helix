# Declarative Projects with `helix apply`

`helix apply -f project.yaml` applies a declarative project specification to a running Helix instance. The same YAML file works with both the `helix` CLI and `kubectl apply -f` (via the Kubernetes operator).

---

## Quick start

```bash
# Build or download the CLI
CGO_ENABLED=0 go build -o /tmp/helix .

# Set credentials
export HELIX_URL=http://localhost:8080
export HELIX_API_KEY=<your-api-key>

# Apply a project spec
helix apply -f project.yaml

# Scope to an organization (by ID or name)
helix apply -f project.yaml --organization my-org
```

The command is **idempotent**: running it again updates the project in place and returns the same project ID. Tasks are only ever added — existing tasks are never modified or deleted.

---

## YAML structure

Every project file is a Kubernetes-style resource document.

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-project          # used as the idempotency key

spec:
  # ── Basic fields ─────────────────────────────────────────────────────────
  description: "Full-stack web application"
  technologies: [React, Go, PostgreSQL]
  guidelines: |
    Use conventional commits. All PRs require tests.

  # ── Repository attachment ─────────────────────────────────────────────────
  # Option A — single repo (shorthand)
  repository:
    url: "https://github.com/org/my-api"
    branch: main

  # Option B — multiple repos (mutually exclusive with 'repository')
  repositories:
    - url: "https://github.com/org/my-api"
      branch: main
      primary: true          # exactly one must be primary when using this form
    - url: "https://github.com/org/my-frontend"
      branch: main

  # ── Startup commands ──────────────────────────────────────────────────────
  startup:
    install: "npm install"   # run once after the repo is cloned
    start:   "npm start"     # entry point for the running process

  # ── Kanban board settings ─────────────────────────────────────────────────
  kanban:
    wip_limits:
      planning:       5      # 0 = unlimited
      implementation: 3
      review:         3

  # ── Seed tasks (demos / templates only — omit in production) ─────────────
  tasks:
    - title: "Set up CI pipeline"
      description: "Configure GitHub Actions"   # optional
    - title: "Add authentication"

  # ── Inline agent ─────────────────────────────────────────────────────────
  agent:
    name: "Project Assistant"
    model: claude-sonnet-4-6
    provider: anthropic
    system_prompt: |
      You are a coding assistant for this project.
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
```

---

## Field reference

### `metadata`

| Field  | Required | Description |
|--------|----------|-------------|
| `name` | Yes      | Project name. Used as the idempotency key — re-applying with the same name updates the existing project. |

### `spec.description`, `spec.technologies`, `spec.guidelines`

Plain string fields written to the project. `technologies` is a list of strings. `guidelines` is a multi-line string shown to agents working on tasks in this project.

---

### `spec.repository` (singular shorthand)

Use when the project has exactly one repository.

```yaml
spec:
  repository:
    url: "https://github.com/org/my-api"
    branch: main              # defaults to "main" if omitted
```

The repository is automatically marked as the primary.

| Field    | Required | Description |
|----------|----------|-------------|
| `url`    | Yes      | Public clone URL of the repository |
| `branch` | No       | Default branch. Defaults to `main`. |

---

### `spec.repositories` (multi-repo list)

Use when the project spans multiple repositories. Mutually exclusive with `spec.repository`.

```yaml
spec:
  repositories:
    - url: "https://github.com/org/frontend"
      branch: main
      primary: true
    - url: "https://github.com/org/backend"
      branch: develop
```

**Rules:**
- If there is only one entry, `primary: true` is implied.
- If there are two or more entries, exactly one must have `primary: true`.

| Field     | Required                    | Description |
|-----------|-----------------------------|-------------|
| `url`     | Yes                         | Clone URL of the repository |
| `branch`  | No                          | Default branch. Defaults to `main`. |
| `primary` | Required when multiple repos | Marks this repo as the project's default for agents and startup commands. |

Repositories are looked up by URL before being created. Applying the same URL twice attaches the existing repository rather than creating a duplicate.

---

### `spec.startup`

Startup commands run inside the **primary repository** when an agent session begins.

```yaml
spec:
  startup:
    install: "go mod download"   # dependency installation
    start:   "go run ./cmd/api"  # process entry point
```

| Field     | Description |
|-----------|-------------|
| `install` | Runs once after cloning. Use for dependency installation. |
| `start`   | Starts the main process (dev server, API, etc.). |

---

### `spec.kanban`

Controls the Work-In-Progress (WIP) limits for each column of the Kanban board.

```yaml
spec:
  kanban:
    wip_limits:
      planning:       5    # max tasks in the Planning column (0 = unlimited)
      implementation: 3
      review:         3
```

| Field                        | Default | Description |
|------------------------------|---------|-------------|
| `wip_limits.planning`        | 0       | Max tasks in Planning. `0` means unlimited. |
| `wip_limits.implementation`  | 0       | Max tasks in Implementation. |
| `wip_limits.review`          | 0       | Max tasks in Review. |

---

### `spec.tasks`

Seeds the Kanban board with initial tasks. Intended for demos and shareable project templates. **Omit this field in production YAMLs.**

```yaml
spec:
  tasks:
    - title: "Set up CI pipeline"
      description: "Configure GitHub Actions for build, test, and deploy"
    - title: "Add authentication"
```

**Idempotency:** Tasks are matched by title. Re-applying the YAML will not create duplicates. Tasks are never deleted or modified by `helix apply` — only new titles are added. `description` is optional.

| Field         | Required | Description |
|---------------|----------|-------------|
| `title`       | Yes      | Unique task title within the project. Used as the idempotency key. |
| `description` | No       | Optional longer description. |

---

### `spec.agent`

Defines an inline Helix App agent linked to this project. The agent is created or updated each time the YAML is applied, with the agent name as the idempotency key.

```yaml
spec:
  agent:
    name: "Project Assistant"
    model: claude-sonnet-4-6
    provider: anthropic
    system_prompt: |
      You are a coding assistant. Follow the project guidelines.
    tools:
      web_search: true
      browser: false
      calculator: false
    mcps:
      - name: github
        transport: stdio
        command: npx
        args: ["-y", "@modelcontextprotocol/server-github"]
        env:
          GITHUB_TOKEN: "${GITHUB_TOKEN}"
    knowledge:
      - name: docs
        source:
          web:
            urls: ["https://docs.example.com"]
```

---

## Idempotency summary

| Resource       | Idempotency key         | Behaviour on re-apply |
|----------------|-------------------------|-----------------------|
| Project        | `metadata.name` + org   | Fields updated in place |
| Repository     | `url` within org        | Existing repo reused, re-attached |
| Kanban limits  | (part of project)       | Overwritten each apply |
| Task           | `title` within project  | Only new titles added; existing tasks untouched |
| Agent app      | `agent.name`            | Updated in place |

---

## Using with `kubectl`

The same YAML file works with the Kubernetes operator. Apply it to a cluster that has the Helix CRDs installed:

```bash
kubectl apply -f project.yaml
kubectl get projects
kubectl describe project my-project
```

The operator reads `HELIX_URL`, `HELIX_API_KEY`, and `HELIX_ORGANIZATION_ID` from the environment, then calls the same `PUT /api/v1/projects/apply` endpoint that the CLI uses.

---

## Getting an API key

Log in to the Helix UI, navigate to **Account → API Keys**, and create a key. Then:

```bash
export HELIX_URL=http://localhost:8080
export HELIX_API_KEY=hl-xxxxxxxxxxxx

# Optional: find your organization ID
helix organizations list

# Apply
helix apply -f project.yaml --organization <org-id-or-name>
```
