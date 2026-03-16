# Implementation Tasks

## `helix apply` — Project kind support

- [ ] Add kind-based dispatcher to `api/pkg/cli/app/apply.go`: peek at `kind` field from YAML and route to `applyProject()`, `applyProjectTemplate()`, or existing app logic
- [ ] Add `ProjectCRD` and `ProjectTemplateCRD` types to `api/pkg/types/project.go` (CRD wrappers with `apiVersion`, `kind`, `metadata`, `spec`)
- [ ] Add `CreateProject`, `UpdateProject`, `GetProjectByName`, `ListProjects` methods to `api/pkg/client/` (new `project.go` file implementing the `Client` interface)
- [ ] Implement `applyProject()` in the apply command: look up project by `(name, org_id)`, create or update via API client, print project ID
- [ ] Implement `applyProjectTemplate()` in the apply command: upsert template via API, print template ID

## Server — ProjectTemplate persistence

- [ ] Add `OrganizationID`, `CreatedAt`, `UpdatedAt`, `ID` GORM fields to `types.SampleSpecProject` so it can be persisted to DB
- [ ] Create `api/pkg/store/project_templates.go` with `CreateProjectTemplate`, `UpdateProjectTemplate`, `GetProjectTemplateByName`, `ListProjectTemplates(orgIDs []string)` CRUD methods
- [ ] Add DB migration for `project_templates` table (follow existing migration pattern in `api/pkg/store/`)
- [ ] Update `GET /api/v1/sample-projects/simple` handler to merge built-in hardcoded templates with DB-stored org templates for the requesting user
- [ ] Add `PUT /api/v1/sample-projects/simple` endpoint (upsert by name+org) to `sample_projects_handlers.go` and register route in `routes.go`
- [ ] Update fork handler to handle both built-in and DB-stored templates

## K8s Operator — Bug fixes

- [ ] Fix GPTScript conversion bug in `operator/internal/controller/aiapp_controller.go`: loop iterates `assistant.Zapier` instead of `assistant.GPTScripts` and appends to wrong slice
- [ ] Fix Knowledge conversion: add loop to convert `assistant.Knowledge` from operator CRD types to `types.AssistantKnowledge`
- [ ] Add `HELIX_TLS_SKIP_VERIFY` env var support in `SetupWithManager` (TODO already in code)
- [ ] Add `OrganizationID` field to `AIAppSpec` in `operator/api/v1alpha1/aiapp_types.go` and pass it through during app create/update
- [ ] Update `AIAppStatus` with `Ready bool`, `AppID string`, `LastSynced string`, `Message string` fields and populate them after reconcile (success and error cases)
- [ ] Run `make generate manifests` in `operator/` to regenerate deepcopy and CRD manifests after type changes

## K8s Operator — Project CRD

- [ ] Create `operator/api/v1alpha1/project_types.go` with `Project`, `ProjectSpec`, `ProjectStatus`, `ProjectList` types matching the project.yaml schema
- [ ] Register `Project` and `ProjectList` in `groupversion_info.go` scheme builder
- [ ] Create `operator/internal/controller/project_controller.go` with `ProjectReconciler`: upsert project via Helix API using `k8s.<namespace>.<name>` naming, handle finalizer for deletion
- [ ] Register `ProjectReconciler` with manager in `operator/cmd/main.go`
- [ ] Run `make generate manifests` to produce the `Project` CRD YAML in `operator/config/crd/bases/`

## Documentation & Examples

- [ ] Add `examples/project.yaml` sample file showing all supported fields
- [ ] Add `examples/project_template.yaml` sample file showing template with task prompts
- [ ] Update operator README with `Project` CRD documentation and required RBAC
