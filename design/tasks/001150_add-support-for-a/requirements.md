# Requirements: Declarative Project & Project Template Support

## Overview

Add support for `project.yaml` and `project_template.yaml` files that can be applied via `helix apply -f`, and update the k8s operator to handle these new resource kinds along with fixing existing issues.

---

## User Stories

### 1. Declarative Project Management

**As a developer**, I want to define my Helix project in a `project.yaml` file so that I can version-control my project configuration and apply it idempotently.

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
  default_helix_app_id: app_abc123
  guidelines: |
    Use conventional commits.
    All PRs require tests.
```

**Acceptance Criteria:**
- `helix apply -f project.yaml` creates or updates a project by name
- Requires `--organization` flag to scope the project to an org (personal orgs are not supported)
- On update, project fields are merged/overwritten (name is the unique key)
- Output prints the project ID on success
- Errors if required fields (name) are missing

---

### 2. Declarative Project Template (Sample Project)

**As an org admin**, I want to define a `project_template.yaml` that adds a new sample project to my organization's list of available templates, so teams can fork it.

```yaml
apiVersion: helix.ml/v1alpha1
kind: ProjectTemplate
metadata:
  name: fastapi-service
spec:
  description: "FastAPI microservice template"
  github_repo: "myorg/fastapi-template"
  default_branch: main
  technologies:
    - Python
    - FastAPI
    - PostgreSQL
  difficulty: intermediate
  category: api
  task_prompts:
    - title: "Add health check endpoint"
      prompt: "Add a /health endpoint that returns service status and DB connectivity"
      type: feature
      priority: high
    - title: "Add authentication middleware"
      prompt: "Implement JWT authentication middleware for protected routes"
      type: feature
      priority: high
```

**Acceptance Criteria:**
- `helix apply -f project_template.yaml` creates or updates an org-scoped sample project template
- Templates are stored in the database (not hardcoded), scoped to an organization or globally
- `helix project samples` lists both built-in and org-defined templates
- Users can `helix project fork <template-id>` from org templates
- `--organization` flag scopes the template to that org; omitting it requires admin and makes it global

---

### 3. K8s Operator: Project CRD

**As a platform engineer**, I want to manage Helix projects as Kubernetes CRDs so that I can use GitOps tooling to deploy and manage project configuration.

**Acceptance Criteria:**
- A `Project` CRD exists in the operator (`helix.ml/v1alpha1`, kind `Project`)
- The operator reconciles `Project` resources with the Helix API
- Deletion of a k8s `Project` resource soft-deletes the Helix project (or is configurable)
- The project name is namespaced: `k8s.<namespace>.<name>` (consistent with AIApp pattern)

---

### 4. K8s Operator: Bug Fixes & Improvements

**As a platform engineer**, I want the operator to correctly convert all AIApp fields and have better observability.

**Acceptance Criteria:**
- Fix: `GPTScripts` are currently not converted (the loop mistakenly iterates `Zapier` again)
- Fix: `Knowledge` sources are not converted from operator CRD to Helix types
- Add: `AIAppStatus` exposes `Ready`, `AppID`, and `LastSyncedAt` as status conditions
- Add: Support `HELIX_TLS_SKIP_VERIFY` environment variable (already noted as TODO in code)
- Add: `OrganizationID` field in `AIAppSpec` so apps can be org-scoped from k8s

---

## Out of Scope

- Declarative management of SpecTasks via YAML (handled separately)
- Importing projects from GitHub metadata automatically
- Multi-document YAML files (multiple resources in one file separated by `---`)
