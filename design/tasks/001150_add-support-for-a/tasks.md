# Implementation Tasks

## New types

- [ ] Add `ProjectCRD`, `ProjectSpec`, `ProjectAgentSpec`, `ProjectAgentTools` to `api/pkg/types/project.go`
- [ ] Add `ToAppHelixConfig() *AppHelixConfig` conversion method on `ProjectAgentSpec` that maps simplified agent fields to `AssistantConfig` (model, provider, system_prompt, tools, MCPs, knowledge)

## `helix apply` — Project kind

- [ ] Add `kind:` dispatcher to `api/pkg/cli/app/apply.go`: peek at `kind` field from YAML and route `Project` to `applyProject()`, everything else to existing app logic
- [ ] Add project client methods to `api/pkg/client/` interface: `CreateProject`, `UpdateProject`, `GetProjectByName` (new `project.go` file)
- [ ] Implement `applyProject()`: parse project YAML → look up project by `(name, org_id)` → create or update → if `spec.agent` present, create/update agent app via `ToAppHelixConfig()` → link agent app as `DefaultHelixAppID` on project → print project ID and agent app ID
- [ ] Add `--template` flag to `helix apply`: when set, also registers the project as an org sample project template via `PUT /api/v1/sample-projects/simple`
- [ ] Add `PUT /api/v1/sample-projects/simple` upsert endpoint in `api/pkg/server/sample_projects_handlers.go` and register in routes

## K8s Operator — Bug fixes (AIApp)

- [ ] Fix GPTScript conversion bug in `operator/internal/controller/aiapp_controller.go`: loop iterates `assistant.Zapier` instead of `assistant.GPTScripts` and appends to wrong slice
- [ ] Fix Knowledge conversion: add loop to convert `assistant.Knowledge` from operator CRD types to `types.AssistantKnowledge`
- [ ] Add `HELIX_TLS_SKIP_VERIFY` env var support (TODO already marked in code)
- [ ] Add `OrganizationID` field to `AIAppSpec` in `operator/api/v1alpha1/aiapp_types.go` and pass through during create/update
- [ ] Add `Ready`, `AppID`, `LastSynced`, `Message` to `AIAppStatus` and populate after each reconcile

## K8s Operator — Project CRD

- [ ] Create `operator/api/v1alpha1/project_types.go` with `Project`, `ProjectSpec` (matching `api/pkg/types/project.go` schema), `ProjectStatus`, `ProjectList`
- [ ] Register `Project` and `ProjectList` in the scheme builder (`groupversion_info.go`)
- [ ] Create `operator/internal/controller/project_controller.go`: reconcile Project CRD → create/update Helix project + agent app, use `k8s.<namespace>.<name>` as project name, handle finalizer for deletion
- [ ] Register `ProjectReconciler` in `operator/cmd/main.go`
- [ ] Run `make generate manifests` in `operator/` to produce updated deepcopy and CRD YAML files

## Examples

- [ ] Add `examples/project.yaml` showing a full project with inline agent (model, system_prompt, tools, one MCP)
- [ ] Update operator README with `Project` CRD docs and required RBAC
