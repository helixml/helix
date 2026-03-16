# Design: Declarative Project & Project Template Support

## Architecture Overview

The implementation extends the existing `helix apply` dispatch pattern and k8s operator to handle two new resource kinds: `Project` and `ProjectTemplate`.

---

## Current State

### `helix apply`
- Located at `api/pkg/cli/app/apply.go`
- Only supports `AIApp` (via `apps.NewLocalApp()` → `config/yaml_processor.go`)
- `api/pkg/cli/model/apply.go` handles `Model` kind separately under `helix model apply`

### `helix apply` dispatch today
There is no central dispatcher — each sub-command (`helix apply` vs `helix model apply`) handles its own kind. The app apply command does not inspect `kind:` at all; it assumes everything is an AIApp.

### Sample Projects
- Hardcoded in `api/pkg/server/sample_projects_handlers.go` as `SAMPLE_PROJECTS` slice
- `SampleSpecProject` type exists in `api/pkg/types/simple_spec_task.go` but is separate from the hardcoded `SampleProject` struct in the server package
- No database persistence for sample projects

### K8s Operator
- Only handles `AIApp` CRD (`operator/api/v1alpha1/aiapp_types.go`)
- Known bug: GPTScript conversion loop incorrectly iterates `assistant.Zapier` instead of `assistant.GPTScripts`
- Knowledge sources not converted from operator CRD types to Helix API types
- `AIAppStatus` is empty; no status reporting
- TODO comment for `HELIX_TLS_SKIP_VERIFY` in `SetupWithManager`

---

## Design Decisions

### Decision 1: Central `kind` dispatcher in `helix apply`

Rather than creating separate `helix project apply` and `helix project-template apply` sub-commands, extend the existing `helix apply -f` to inspect the `kind:` field and route accordingly.

**Rationale:** Users expect a single `helix apply` entry point (Kubernetes-style UX). Routing by kind is idiomatic.

**Implementation:** In `api/pkg/cli/app/apply.go`, after reading the YAML, peek at `kind` field. Route to the appropriate handler:
- `AIApp` → existing app logic
- `Project` → new project handler
- `ProjectTemplate` → new project template handler

The `yaml_processor.go` already extracts `kind` from CRD-style YAMLs.

### Decision 2: `ProjectTemplate` stored in the database, scoped by org

Currently sample projects are hardcoded. The new approach: `ProjectTemplate` resources are stored in a new `project_templates` DB table. Built-in templates are seeded at startup. Org-scoped templates are stored with `organization_id`. The list endpoint returns built-in + org-scoped templates for the requesting user.

**Rationale:** Allows orgs to define their own templates without requiring code changes or redeployment. Built-in templates remain for backward compatibility.

### Decision 3: Reuse existing `SampleSpecProject` type for `ProjectTemplate`

The `types.SampleSpecProject` type in `api/pkg/types/simple_spec_task.go` already has most fields needed. Extend it with `OrganizationID` and `CreatedAt`/`UpdatedAt` for DB persistence. Add a GORM model to make it persistable.

**Rationale:** Avoids creating a redundant type. The existing CLI and fork endpoints can be reused with minimal changes.

### Decision 4: Project name as idempotency key

When applying a `project.yaml`, look up existing projects by `(name, organization_id)`. If found: update. If not: create. This mirrors how `helix apply` for AIApps uses the app name.

### Decision 5: Operator `Project` CRD is additive

Add a new `Project` CRD to the operator alongside `AIApp`. The reconciler follows the same pattern: namespace the project name as `k8s.<namespace>.<name>`, use a finalizer for cleanup.

---

## File-by-File Changes

### New/Modified in `api/`

| File | Change |
|---|---|
| `api/pkg/cli/app/apply.go` | Add kind-based dispatch; add `applyProject()` and `applyProjectTemplate()` functions |
| `api/pkg/types/project.go` | Add `ProjectTemplateCRD` type (CRD wrapper for YAML apply) |
| `api/pkg/types/simple_spec_task.go` | Add `OrganizationID`, GORM fields to `SampleSpecProject` for DB persistence |
| `api/pkg/store/project_templates.go` | New: CRUD for `SampleSpecProject` table |
| `api/pkg/server/sample_projects_handlers.go` | Update list/fork endpoints to include DB-stored templates; add create/update handlers |
| `api/pkg/server/routes.go` | Add PUT `/api/v1/sample-projects/simple/:id` for upsert |
| `api/pkg/client/client.go` | Add `CreateProject`, `UpdateProject`, `GetProjectByName`, `ListProjects` to client interface |
| `api/pkg/client/project.go` | New: implement project client methods |

### New/Modified in `operator/`

| File | Change |
|---|---|
| `operator/api/v1alpha1/aiapp_types.go` | Add `OrganizationID` to `AIAppSpec`; add status fields to `AIAppStatus` |
| `operator/api/v1alpha1/project_types.go` | New: `Project` CRD type definition |
| `operator/api/v1alpha1/zz_generated.deepcopy.go` | Regenerate (run `make generate`) |
| `operator/internal/controller/aiapp_controller.go` | Fix GPTScript conversion bug; fix Knowledge conversion; add status updates; add TLS skip verify |
| `operator/internal/controller/project_controller.go` | New: Project reconciler |
| `operator/cmd/main.go` | Register new `Project` controller with manager |
| `operator/config/crd/bases/` | New CRD manifest for `Project` (run `make manifests`) |

---

## YAML Schemas

### project.yaml
```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-project
spec:
  description: "..."
  github_repo_url: "https://github.com/org/repo"
  default_branch: main
  technologies: [Go, PostgreSQL]
  status: active                    # optional, defaults to "active"
  default_helix_app_id: app_xxx     # optional
  guidelines: |                     # optional
    ...
```

### project_template.yaml
```yaml
apiVersion: helix.ml/v1alpha1
kind: ProjectTemplate
metadata:
  name: my-template
spec:
  description: "..."
  github_repo: "org/repo"
  default_branch: main
  technologies: [Python, FastAPI]
  difficulty: intermediate           # beginner | intermediate | advanced
  category: api                      # web | api | mobile | data | ai
  task_prompts:
    - title: "Add endpoint"
      prompt: "Implement a /health endpoint..."
      type: feature
      priority: high
```

---

## API Changes

### New endpoint: `PUT /api/v1/sample-projects/simple` (upsert by name)

Used by `helix apply` to create or update a project template.

Request body: `types.SampleSpecProject`
Response: `types.SampleSpecProject` with ID

### Modified: `GET /api/v1/sample-projects/simple`

Returns built-in templates + org-scoped templates for the requesting user's organizations.

---

## Operator Bug Fix Details

In `aiapp_controller.go`, the current code:
```go
// Convert GPTScripts
for _, zapier := range assistant.Zapier {   // BUG: should be assistant.GPTScripts
    helixAssistant.Zapier = append(...)     // BUG: should append to GPTScripts
}
```

Fix:
```go
// Convert GPTScripts
for _, script := range assistant.GPTScripts {
    helixAssistant.GPTScripts = append(helixAssistant.GPTScripts, types.AssistantGPTScript{
        Name:        script.Name,
        Description: script.Description,
        File:        script.File,
        Content:     script.Content,
    })
}

// Convert Knowledge
for _, k := range assistant.Knowledge {
    helixAssistant.Knowledge = append(helixAssistant.Knowledge, types.AssistantKnowledge{
        Name: k.Name,
        // ... map fields
    })
}
```

Status reporting:
```go
type AIAppStatus struct {
    Ready       bool   `json:"ready"`
    AppID       string `json:"appID,omitempty"`
    LastSynced  string `json:"lastSynced,omitempty"`
    Message     string `json:"message,omitempty"`
}
```

---

## Patterns Found in Codebase

- **CRD dispatch pattern**: Each CLI sub-command owns its own apply logic. Extend `helix apply` to route by `kind:` field.
- **Idempotency key**: App name used as unique key for `helix apply`. Use `(name, org_id)` for projects.
- **K8s naming**: `k8s.<namespace>.<name>` prefix used by operator for all resources — follow same convention for Projects.
- **Client interface**: All API interactions go through `client.Client` interface in `api/pkg/client/`. New project methods should be added there.
- **Sample projects seeding**: Built-in samples are in-memory. New DB-stored templates should fall back gracefully to built-ins if DB is empty.
