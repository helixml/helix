# Helix Project YAML — Demo Guide

This guide shows how to use `helix apply -f project.yaml` (and `kubectl apply`) to declaratively
create and manage Helix projects. We use [robot-hq](https://github.com/binocarlos/robot-hq) as
a worked example — a full-stack React/Go/PostgreSQL app with a Kanban board, startup scripts,
and a Claude Code agent.

---

## What a project YAML does

A single `project.yaml` file in your repository declares everything Helix needs to set up your
development environment:

| Section | What it creates |
|---------|----------------|
| `repository` | Clones the repo and tracks it in Helix |
| `startup` | Runs `install` once on first boot, `start` on every boot |
| `kanban` | Creates a Kanban board with WIP limits |
| `tasks` | Seeds initial tickets onto the board |
| `agent` | Creates a Claude Code desktop agent wired to the project |

---

## The robot-hq project.yaml

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: robot-hq
spec:
  description: "Full-stack web application"
  guidelines: |
    Use conventional commits. All PRs require tests.
  repository:
    url: "https://github.com/binocarlos/robot-hq"
    default_branch: main
  startup:
    install: "docker compose build"
    start:   "docker compose up -d"
  kanban:
    wip_limits:
      planning:       5
      implementation: 3
      review:         3
  tasks:
    - title: "Add a second real-time graph to the home page"
      description: "..."
    - title: "Add tests for the API"
      description: "..."
  agent:
    name: "Project Assistant"
    runtime: claude_code        # Claude Code CLI — handles context compaction automatically
    model: claude-opus-4-6
    provider: anthropic
    tools:
      web_search: true
      browser: true
```

> **`runtime: claude_code`** is the recommended default. It uses the Claude Code CLI inside the
> Zed desktop container, which handles context compaction automatically. The alternative
> `runtime: zed` (Zed's built-in agent panel) does not handle compaction.

---

## Option 1 — helix apply (CLI)

### Prerequisites

- Helix CLI installed (`helix` binary in PATH)
- `HELIX_URL` and `HELIX_API_KEY` set, or logged in via `helix auth login`

### Run it

```bash
# Apply from a local file
helix apply -f project.yaml

# Apply directly from GitHub
helix apply -f https://raw.githubusercontent.com/binocarlos/robot-hq/main/project.yaml
```

Helix apply is **idempotent** — run it again to update an existing project. It will not duplicate
tasks or repositories.

### What happens

1. Project record created (or updated) in Helix
2. Repository cloned asynchronously into Helix's git store
3. `.helix/startup.sh` written to the `helix-specs` branch once the clone completes
4. Kanban board created with the configured WIP limits
5. Tasks seeded onto the board (skipped if they already exist)
6. A Claude Code agent app created and linked to the project

---

## Option 2 — kubectl apply (Kubernetes operator)

The Helix operator watches for `Project` custom resources and calls the same `applyProject` API
that `helix apply` uses. This lets you manage Helix projects alongside your other Kubernetes
resources using standard GitOps tooling (Argo CD, Flux, etc.).

### Prerequisites

- A Kubernetes cluster (`kind`, `k3s`, EKS, GKE, etc.)
- `kubectl` configured to talk to it
- The Helix operator installed (see below)

### Quick start with kind

```bash
# Create a local cluster
kind create cluster --name helix

# Install the Project CRD
kubectl apply -f https://raw.githubusercontent.com/helixml/helix/main/operator/config/crd/bases/app.aispec.org_projects.yaml

# Run the operator locally (separate terminal)
cd operator
HELIX_URL=https://your-helix-instance.example.com \
HELIX_API_KEY=your-api-key \
go run ./cmd/main.go \
  --health-probe-bind-address=:8091 \
  --metrics-bind-address=:8092
```

### Apply the robot-hq project

The operator uses `apiVersion: app.aispec.org/v1alpha1` (the Kubernetes CRD group):

```yaml
# robot-hq-project.yaml
apiVersion: app.aispec.org/v1alpha1
kind: Project
metadata:
  name: robot-hq
  namespace: default
spec:
  description: "Full-stack web application"
  guidelines: |
    Use conventional commits. All PRs require tests.
  repository:
    url: "https://github.com/binocarlos/robot-hq"
    default_branch: main
  startup:
    install: "docker compose build"
    start:   "docker compose up -d"
  kanban:
    wip_limits:
      planning:       5
      implementation: 3
      review:         3
  tasks:
    - title: "Add a second real-time graph to the home page"
      description: "..."
    - title: "Add tests for the API"
      description: "..."
  agent:
    name: "Project Assistant"
    runtime: claude_code
    model: claude-opus-4-6
    provider: anthropic
    tools:
      web_search: true
      browser: true
```

```bash
kubectl apply -f robot-hq-project.yaml

# Watch the operator reconcile
kubectl get projects -w

# Check details and status
kubectl describe project robot-hq
```

The operator sets `.status.ready = true` and `.status.project_id` once the project is created
in Helix.

### Deploy the operator in-cluster

For production, deploy the operator as a pod with a Secret holding your Helix credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: helix-credentials
  namespace: helix-operator-system
stringData:
  HELIX_URL: "https://your-helix-instance.example.com"
  HELIX_API_KEY: "your-api-key"
  HELIX_ORGANIZATION_ID: "your-org-id"   # optional
```

Then use the Helm chart or kustomize manifests from `operator/config/` to install the operator.

---

## Agent runtime options

The `agent.runtime` field controls which CLI runs inside the Zed desktop container:

| Value | Description |
|-------|-------------|
| `claude_code` | Claude Code CLI **(default, recommended)** |
| `zed` | Zed's built-in agent panel (no automatic compaction) |
| `qwen_code` | Qwen Code CLI |
| `gemini_cli` | Gemini CLI |
| `codex_cli` | OpenAI Codex CLI |

If `runtime` is omitted, `claude_code` is used.

---

## Startup script behaviour

The `startup` block is converted to a shell script and stored in two places:

1. **`.helix/startup.sh`** in the `helix-specs` orphan branch of the cloned repo (primary)
2. **`HELIX_STARTUP_INSTALL` / `HELIX_STARTUP_START`** env vars injected into the container (fallback)

The fallback ensures startup commands run even on the first task, before the async clone has
finished writing the script to git.

---

## Verifying the result

After applying, log into your Helix instance and navigate to **Projects**. You should see:

- The `robot-hq` project in the list
- A Kanban board with three columns and the configured WIP limits
- Two tasks in the **Planning** column
- A **Project Assistant** agent linked to the project (Claude Code, claude-opus-4-6)

You can also verify via the API:

```bash
curl -s -H "Authorization: Bearer $HELIX_API_KEY" \
  $HELIX_URL/api/v1/projects | jq '.[] | {id, name}'
```
