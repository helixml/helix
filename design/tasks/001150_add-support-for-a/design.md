# Design: Declarative Project YAML

## Architecture Overview

A single `Project` kind covers all use cases: defining a live project, and sharing it as a template. The file is self-contained: project metadata, startup configuration, and default agent inline. The same YAML works for `helix apply -f` and `kubectl apply -f`.

---

## Existing Startup Script Mechanism

The codebase already has a `StartupScript` field on `Project`:
```go
StartupScript string `json:"startup_script" gorm:"-"`
```

**Key constraint:** `gorm:"-"` means it is **never persisted to the database**. It is always loaded on-demand from `.helix/startup.sh` in the primary code repository via `LoadStartupScriptFromCodeRepo()`. This means the current mechanism:
- Requires a git repo to be connected to the project first
- Requires committing `.helix/startup.sh` to that repo
- Is not declarable in a project YAML without a pre-existing connected repo

**Our approach:** Add two new persistent DB fields `StartupInstall` and `StartupStart` to the `Project` model. These are set from the YAML and stored directly in the database — no git roundtrip required. They represent the declarative, YAML-specified startup config. The existing `.helix/startup.sh` transient mechanism remains untouched for backward compatibility.

---

## Idempotency Analysis

All three application paths are idempotent. Here is the key lookup for each:

### Server (`PUT /api/v1/projects/apply`)
```go
// Lists projects for org, finds by name, updates in-place
for _, p := range existingProjects {
    if p.Name == req.Name {
        existing = p; break
    }
}
// if existing != nil → update; else → create
```
Key: `(OrganizationID, Name)`

### CLI (`runApplyProject` → `applyProjectAgent`)
```go
// Lists apps for org, finds by agent name, updates in-place
for _, existing := range existingApps {
    if existing.Config.Helix.Name == appConfig.Name {
        // update
    }
}
// else create
```
Key: `(OrganizationID, agent.name)` — defaults to `"<project-name> Assistant"`

### Operator (`reconcileProjectAgent`)
Identical logic to CLI. **Improvement needed:** on first reconcile it searches by name and stores the `AgentAppID` in status. On subsequent reconciles it should use the stored ID to update directly, avoiding a full list+search on every loop:

```go
// Improvement: try by stored ID first
if project.Status.AgentAppID != "" {
    existing, err := r.helix.GetApp(ctx, project.Status.AgentAppID)
    if err == nil {
        existing.Config.Helix = *appConfig
        updated, err := r.helix.UpdateApp(ctx, existing)
        return updated.ID, err
    }
    // ID not found → fall through to name search (e.g. after manual deletion)
}
// fall back to name search
```

---

## New `startup` Block

### YAML schema addition

```yaml
spec:
  startup:                          # optional
    install: "npm install"          # run once after clone to install dependencies
    start: "npm start"              # entry point: start the application
```

Both fields are optional strings. `install` is run before `start` (dependency installation). `start` is the long-running process that launches the application. Both commands run in the root of the cloned `github_repo_url` repository.

### Type changes

**`api/pkg/types/project.go`** — add to `ProjectSpec`:
```go
type ProjectStartup struct {
    Install string `yaml:"install,omitempty" json:"install,omitempty"`
    Start   string `yaml:"start,omitempty" json:"start,omitempty"`
}
```
Add `Startup *ProjectStartup` to `ProjectSpec`.

**`api/pkg/types/project.go`** — add to the DB `Project` type:
```go
StartupInstall string `json:"startup_install,omitempty" gorm:"column:startup_install"`
StartupStart   string `json:"startup_start,omitempty" gorm:"column:startup_start"`
```

**DB migration:** add `startup_install` and `startup_start` columns to the `projects` table.

### Wire-through in `applyProject` server handler

When a `ProjectApplyRequest` carries `Spec.Startup`, copy to the project model:
```go
if spec.Startup != nil {
    existing.StartupInstall = spec.Startup.Install
    existing.StartupStart   = spec.Startup.Start
}
```

### Operator

Add `Startup` to the operator's `ProjectSpec` CRD type and pass through to `ProjectApplyRequest`.

---

## File-by-File Changes (incremental from existing implementation)

### `api/pkg/types/project.go`
- Add `ProjectStartup` struct
- Add `Startup *ProjectStartup` to `ProjectSpec`
- Add `StartupInstall`, `StartupStart` string fields to the `Project` DB model (with `gorm:"column:..."` tags)

### `api/pkg/server/project_handlers.go`
- `applyProject`: copy `Startup` fields from request spec to project model on create and update

### DB migration
- Add migration adding `startup_install` and `startup_start` columns to `projects` table (follow existing GORM auto-migrate or numbered migration pattern)

### `api/pkg/cli/app/apply.go`
- Pass `Spec.Startup` through in `ProjectApplyRequest` (already included in `Spec`, so no extra change needed if `ProjectApplyRequest.Spec` is `ProjectSpec`)

### `operator/api/v1alpha1/project_types.go`
- Add `ProjectStartup` struct mirroring the API type
- Add `Startup *ProjectStartup` to `ProjectSpec`

### `operator/internal/controller/project_controller.go`
- Improve `reconcileProjectAgent`: check `Status.AgentAppID` first (look up by ID), fall back to name search
- Pass `Spec.Startup.Install` and `Spec.Startup.Start` in `ProjectApplyRequest`

### `examples/project.yaml`
- Add `startup` block to the example

---

## Full YAML Schema (updated)

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-project              # required; idempotency key within org
spec:
  description: "..."            # optional
  github_repo_url: "..."        # optional; repo to clone for agent work
  default_branch: main          # optional, defaults to "main"
  technologies: []              # optional; tags shown in UI
  guidelines: |                 # optional; AI agent guidelines
    ...

  startup:                      # optional; how to run the project's code
    install: "npm install"      # run once after clone (dependency installation)
    start: "npm start"          # entry point command (relative to repo root)

  agent:                        # optional; inline agent linked to this project
    name: "..."                 # optional; defaults to "<project-name> Assistant"
    description: "..."          # optional
    model: claude-sonnet-4-6    # required if agent block present
    provider: anthropic         # optional; auto-detected
    system_prompt: |            # optional
      ...
    tools:                      # optional; all default to false
      web_search: false
      browser: false
      calculator: false
    mcps:                       # optional; Model Context Protocol servers
      - name: github
        transport: stdio
        command: npx
        args: ["-y", "@modelcontextprotocol/server-github"]
        env:
          GITHUB_TOKEN: "${GITHUB_TOKEN}"
    knowledge:                  # optional; RAG knowledge sources
      - name: project-docs
        source:
          web:
            urls: ["https://docs.example.com"]
```

---

## Patterns Found in Codebase

- `Project.StartupScript` is `gorm:"-"` — always loaded from `.helix/startup.sh` in git; do NOT use this field for declarative config. Add new `StartupInstall`/`StartupStart` DB columns instead.
- Agent idempotency key is `Config.Helix.Name` matched within org — consistent across CLI and operator
- Operator should prefer `Status.AgentAppID` on subsequent reconciles to avoid O(n) list+search on every loop
- `ProjectApplyRequest.Spec` carries the full `ProjectSpec` including `Startup` — no extra wiring needed in CLI if the types are correct
