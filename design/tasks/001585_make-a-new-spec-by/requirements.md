# Requirements: Declarative Project YAML

## Overview

A single `project.yaml` that declaratively defines a Helix project: its repositories (one or many, one primary), Kanban board settings, startup configuration, and inline agent. Works identically with `helix apply -f` and `kubectl apply -f`.

---

## User Stories

### 1. Single-repository project (shorthand)

The common case — one repo, stated simply:

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-api
spec:
  repository:
    url: "https://github.com/org/my-api"
    branch: main
```

**Acceptance Criteria:**
- `repository` (singular) is a shorthand for a single primary repository
- Equivalent to `repositories: [{url: ..., branch: ..., primary: true}]`
- Cannot specify both `repository` and `repositories`

---

### 2. Multi-repository project

```yaml
spec:
  repositories:
    - url: "https://github.com/org/frontend"
      branch: main
      primary: true
    - url: "https://github.com/org/backend"
      branch: main
    - url: "https://github.com/org/shared-lib"
      branch: main
```

**Acceptance Criteria:**
- If only one repository is listed, `primary: true` is implied
- If multiple repositories are listed and none has `primary: true`, applying returns an error: "exactly one repository must be designated primary"
- If multiple have `primary: true`, applying returns an error
- All listed repositories are cloned when a spec task starts
- The primary repository is the working directory; `startup.install` and `startup.start` run there
- Non-primary repositories are cloned alongside the primary (sibling directories)

---

### 3. Startup block applies to the primary repository

```yaml
spec:
  startup:
    install: "npm install"   # run in primary repo root after cloning
    start: "npm start"       # entry point; agents attach to this running process
```

**Acceptance Criteria:**
- `startup` always applies to the primary repository
- Commands run in the root of the primary repository
- Default branch is the primary repository's configured branch
- Only one startup block per project (not per-repository)

---

### 4. Kanban board settings

```yaml
spec:
  kanban:
    wip_limits:
      planning: 5
      implementation: 3
      review: 3
```

**Acceptance Criteria:**
- `kanban.wip_limits` maps to `Project.Metadata.BoardSettings.WIPLimits` (existing internal type)
- All WIP limit fields are optional; omitting leaves existing values unchanged on update
- Setting a limit to `0` means unlimited

---

### 5. Full example

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-fullstack-app
spec:
  description: "Full-stack web application"
  technologies: [React, Go, PostgreSQL]
  guidelines: |
    Use conventional commits. All PRs require tests.

  repositories:
    - url: "https://github.com/org/my-api"
      branch: main
      primary: true
    - url: "https://github.com/org/my-frontend"
      branch: main

  startup:
    install: "go mod download"
    start: "go run ./cmd/server"

  kanban:
    wip_limits:
      planning: 5
      implementation: 3
      review: 3

  agent:
    name: "Project Assistant"
    model: claude-sonnet-4-6
    provider: anthropic
    system_prompt: |
      You are a coding assistant for this full-stack application.
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

---

### 6. Idempotency

Re-applying the same or updated YAML must never create duplicates.

| Resource | Idempotency key |
|---|---|
| Project | `(metadata.name, organization_id)` |
| Agent app | `(agent.name, organization_id)` |
| Repository attachment | URL match within org; attach is idempotent |
| k8s Project | `k8s.<namespace>.<name>` |

On update: only fields present in the YAML are updated. Repositories are reconciled (attach new, leave existing untouched).

---

### 7. k8s operator support

`kubectl apply -f project.yaml` works identically to `helix apply -f project.yaml`:
- Creates/updates the project, attaches repositories, links agent
- Namespaced project name: `k8s.<namespace>.<name>`
- `ProjectStatus` reports `Ready`, `ProjectID`, `AgentAppID`, `LastSynced`

---

### 8. Testing approach

#### Testing `helix apply`

Requirements: Go build environment, running Helix instance (local stack at `localhost:8080`), org + API key.

```bash
# Build CLI
cd api && CGO_ENABLED=0 go build -o /tmp/helix .

# Set credentials
export HELIX_URL=http://localhost:8080
export HELIX_API_KEY=<key from .env.usercreds>

# Apply a project
/tmp/helix apply -f examples/project.yaml --organization <org-id>
```

#### Testing `kubectl apply` (operator)

Requirements: Docker, [kind](https://kind.sigs.k8s.io/) or k3s, kubectl.

```bash
# Install kind
go install sigs.k8s.io/kind@latest

# Create local cluster
kind create cluster --name helix-test

# Build operator image
cd operator && docker build -t helix-operator:dev .

# Load into kind
kind load docker-image helix-operator:dev --name helix-test

# Install CRDs
kubectl apply -f config/crd/bases/

# Deploy operator (with HELIX_URL + HELIX_API_KEY set)
kubectl apply -f config/deploy/

# Apply project
kubectl apply -f examples/project.yaml
kubectl get projects
kubectl describe project my-fullstack-app
```

**Minimum tooling to install in dev environment:**
- `docker` (already present)
- `kind`: `go install sigs.k8s.io/kind@latest` or `curl` install
- `kubectl`: snap/apt or direct binary download
- `controller-gen`: `go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest` (for CRD manifest generation)

---

## Out of Scope

- Per-repository startup scripts (startup always applies to primary)
- Automatic repo cloning at apply time (repos are registered, cloning happens when a spec task starts)
- `--template` flag for org sample project templates (deferred)
- Multi-document YAML (`---` separated resources)
