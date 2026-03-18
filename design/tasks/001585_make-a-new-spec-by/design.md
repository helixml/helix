# Design: Declarative Project YAML

## Architecture Overview

A single `Project` CRD kind covers all use cases. The file is self-contained: repositories, Kanban board settings, startup config, and agent inline. Works identically with `helix apply -f` and `kubectl apply -f`.

---

## Existing Internals We Build On

### Multi-repo (already implemented)
- `Project.DefaultRepoID` — the primary repo ID
- `ProjectRepository` — junction table `(project_id, repository_id, organization_id)`
- `AttachRepositoryToProject(ctx, projectID, repoID)` — idempotent, writes junction table
- `SetProjectPrimaryRepository(ctx, projectID, repoID)` — sets `DefaultRepoID`
- `GitRepository.ExternalURL` — used to look up an existing repo by URL

### Kanban board settings (already implemented)
- `Project.Metadata` (JSONB) → `ProjectMetadata.BoardSettings.WIPLimits`
- `WIPLimits.Planning`, `WIPLimits.Review`, `WIPLimits.Implementation` (ints)

### Startup (added in previous iteration)
- `Project.StartupInstall`, `Project.StartupStart` — new DB columns
- Always applies to the primary repository

---

## Repository Block Design

### Singular shorthand (`repository`)

For the common single-repo case:
```yaml
spec:
  repository:
    url: "https://github.com/org/my-api"
    branch: main
```
Pre-processing: convert to `repositories: [{url: ..., branch: ..., primary: true}]` before validation.

### Multi-repo array (`repositories`)

```yaml
spec:
  repositories:
    - url: "https://github.com/org/frontend"
      branch: main
      primary: true
    - url: "https://github.com/org/backend"
      branch: main
```

### Validation rules
1. `repository` and `repositories` are mutually exclusive
2. If `len(repositories) == 1`, `primary: true` is implied
3. If `len(repositories) > 1` and exactly one has `primary: true` → OK
4. If `len(repositories) > 1` and zero or multiple have `primary: true` → validation error

### Type: `ProjectRepository` (YAML type, not the DB junction type)

```go
type ProjectRepositorySpec struct {
    URL     string `yaml:"url" json:"url"`           // required; external clone URL
    Branch  string `yaml:"branch,omitempty" json:"branch,omitempty"` // defaults to "main"
    Primary bool   `yaml:"primary,omitempty" json:"primary,omitempty"`
}
```

### Applying repositories

On `applyProject`:
1. For each `ProjectRepositorySpec` in the resolved list:
   a. Look up existing `GitRepository` by `ExternalURL == url` within the org
   b. If not found: create a new external `GitRepository` (`RepoType: "code"`, `IsExternal: true`)
   c. Attach to project (idempotent — `AttachRepositoryToProject`)
2. Set the primary one: `SetProjectPrimaryRepository(ctx, projectID, primaryRepoID)`

**Key constraint:** We do not clone repos during `apply` — repos are registered and linked. Cloning happens when a spec task starts.

---

## Kanban Block Design

### YAML type

```go
type ProjectKanban struct {
    WIPLimits *ProjectWIPLimits `yaml:"wip_limits,omitempty" json:"wip_limits,omitempty"`
}

type ProjectWIPLimits struct {
    Planning       int `yaml:"planning,omitempty" json:"planning,omitempty"`
    Implementation int `yaml:"implementation,omitempty" json:"implementation,omitempty"`
    Review         int `yaml:"review,omitempty" json:"review,omitempty"`
}
```

### Mapping to internal type

`ProjectWIPLimits` → `Project.Metadata.BoardSettings.WIPLimits`:

```go
if spec.Kanban != nil && spec.Kanban.WIPLimits != nil {
    project.Metadata.BoardSettings.WIPLimits.Planning       = spec.Kanban.WIPLimits.Planning
    project.Metadata.BoardSettings.WIPLimits.Implementation = spec.Kanban.WIPLimits.Implementation
    project.Metadata.BoardSettings.WIPLimits.Review         = spec.Kanban.WIPLimits.Review
}
```

`Project.Metadata` is already stored as JSONB — no migration needed for WIP limit values. The `ProjectMetadata` struct already has `BoardSettings.WIPLimits` fields.

---

## Full YAML Schema

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-project

spec:
  description: "..."
  technologies: []
  guidelines: |
    ...

  # Option A: single repo shorthand
  repository:
    url: "https://github.com/org/repo"
    branch: main

  # Option B: multi-repo list (mutually exclusive with 'repository')
  repositories:
    - url: "https://github.com/org/primary-repo"
      branch: main
      primary: true          # required when multiple repos
    - url: "https://github.com/org/other-repo"
      branch: main

  # Startup runs in the primary repository root
  startup:
    install: "npm install"   # run once after clone
    start: "npm start"       # entry point for the running process

  # Kanban board WIP limits
  kanban:
    wip_limits:
      planning: 5            # 0 = unlimited
      implementation: 3
      review: 3

  # Inline agent (creates/updates a linked Helix App)
  agent:
    name: "Project Assistant"
    model: claude-sonnet-4-6
    provider: anthropic
    system_prompt: |
      ...
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

## File-by-File Changes

### `api/pkg/types/project.go`
- Add `ProjectRepositorySpec` struct (`URL`, `Branch`, `Primary` fields)
- Add `ProjectKanban` struct with `WIPLimits *ProjectWIPLimits`
- Add `ProjectWIPLimits` struct (`Planning`, `Implementation`, `Review` ints)
- Add to `ProjectSpec`:
  - `Repository  *ProjectRepositorySpec   yaml:"repository"`  (singular shorthand)
  - `Repositories []ProjectRepositorySpec  yaml:"repositories"`
  - `Kanban       *ProjectKanban           yaml:"kanban"`
- Add `ValidateRepositories() error` method on `ProjectSpec` — enforces single-primary rule
- Add `ResolvedRepositories() []ProjectRepositorySpec` — normalises singular/plural

### `api/pkg/server/project_handlers.go` (`applyProject`)
1. Call `spec.ValidateRepositories()` — return 400 on error
2. Get `resolvedRepos := spec.ResolvedRepositories()`
3. For each repo: find-or-create `GitRepository` by `ExternalURL`, attach to project
4. Set primary with `SetProjectPrimaryRepository`
5. Map `spec.Kanban` → `project.Metadata.BoardSettings.WIPLimits`

### `api/pkg/store/` (no new store methods needed)
- `AttachRepositoryToProject`, `SetProjectPrimaryRepository`, `ListGitRepositories` already exist
- May need a `GetGitRepositoryByURL(ctx, orgID, url) (*GitRepository, error)` helper if not present

### `operator/api/v1alpha1/project_types.go`
- Mirror the new types: `ProjectRepositorySpec`, `ProjectKanban`, `ProjectWIPLimits`
- Add `Repository *ProjectRepositorySpec` and `Repositories []ProjectRepositorySpec` to `ProjectSpec`
- Add `Kanban *ProjectKanban` to `ProjectSpec`

### `operator/internal/controller/project_controller.go`
- Map operator `ProjectSpec.Repositories` → `types.ProjectSpec.Repositories` in `applyReq`
- Map operator `ProjectSpec.Kanban` similarly

### `operator/api/v1alpha1/zz_generated.deepcopy.go`
- Add deepcopy for `ProjectRepositorySpec`, `ProjectKanban`, `ProjectWIPLimits`
- Update `ProjectSpec.DeepCopyInto` for new slice and pointer fields

### `examples/project.yaml`
- Update to show multi-repo + kanban

---

## Store Helper: GetGitRepositoryByURL

Check if `ListGitRepositories` supports URL filtering. If not, add:
```go
func (s *PostgresStore) GetGitRepositoryByExternalURL(ctx context.Context, orgID, url string) (*types.GitRepository, error)
```
This is needed by `applyProject` to look up an existing repo before creating.

---

## Testing Plan

### Testing `helix apply`

Prerequisites:
```bash
# Running Helix stack at localhost:8080 (already available in dev environment)
# Build the CLI
cd /home/retro/work/helix/api && CGO_ENABLED=0 go build -o /tmp/helix .

# Set credentials from .env.usercreds
export HELIX_URL=http://localhost:8080
export HELIX_API_KEY=$(grep HELIX_API_KEY .env.usercreds | cut -d= -f2-)
```

Test sequence:
```bash
# First apply — creates project + agent
/tmp/helix apply -f examples/project.yaml --organization <org-id>

# Re-apply unchanged — should return same IDs (idempotency)
/tmp/helix apply -f examples/project.yaml --organization <org-id>

# Update model and re-apply — agent updated, project unchanged
# edit examples/project.yaml: change model
/tmp/helix apply -f examples/project.yaml --organization <org-id>

# Verify via API
curl -H "Authorization: Bearer $HELIX_API_KEY" $HELIX_URL/api/v1/projects
```

### Testing `kubectl apply` (operator)

Install required tools in dev environment:
```bash
# kind (Kubernetes in Docker)
go install sigs.k8s.io/kind@latest
# or: curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64 && chmod +x ./kind && mv ./kind /usr/local/bin/

# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl && mv kubectl /usr/local/bin/

# controller-gen (for CRD manifest generation)
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
```

Test sequence:
```bash
# Create cluster
kind create cluster --name helix-test

# Generate CRD manifests
cd /home/retro/work/helix/operator && make manifests

# Install CRDs
kubectl apply -f config/crd/bases/

# Run operator locally (out-of-cluster, pointing at localhost:8080)
HELIX_URL=http://localhost:8080 \
HELIX_API_KEY=$HELIX_API_KEY \
go run ./cmd/main.go &

# Apply project
kubectl apply -f ../examples/project.yaml

# Check status
kubectl get projects
kubectl describe project my-fullstack-app
# Should show Ready=true, ProjectID=..., AgentAppID=...
```

---

## Patterns Found in Codebase

- `GitRepository` is found by `ExternalURL` field — use this for idempotent repo lookup-or-create
- `AttachRepositoryToProject` is already idempotent (writes to junction table, handles duplicates)
- `Project.Metadata` is JSONB — `BoardSettings.WIPLimits` already exists, no migration needed
- The singular `repository` vs plural `repositories` normalisation should happen in a method on `ProjectSpec`, not in the handler
- Operator `ProjectSpec` must mirror API `ProjectSpec` field-for-field so the same YAML file works for both
