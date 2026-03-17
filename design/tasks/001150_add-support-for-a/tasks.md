# Implementation Tasks

## New types

- [x] Add `ProjectCRD`, `ProjectSpec`, `ProjectAgentSpec`, `ProjectAgentTools` to `api/pkg/types/project.go`
- [x] Add `ToAppHelixConfig() *AppHelixConfig` conversion method on `ProjectAgentSpec` that maps simplified agent fields to `AssistantConfig` (model, provider, system_prompt, tools, MCPs, knowledge)

## `helix apply` — Project kind

- [x] Add `kind:` dispatcher to `api/pkg/cli/app/apply.go`: peek at `kind` field from YAML and route `Project` to `runApplyProject()`, everything else to existing app logic
- [x] Add `ApplyProject` client method to `api/pkg/client/` interface (new `project.go` file calling `PUT /api/v1/projects/apply`)
- [x] Implement `runApplyProject()`: parse project YAML → resolve org → if `spec.agent` present, create/update agent app via `ToAppHelixConfig()` → call `ApplyProject` with agent app ID linked → print project ID and agent app ID
- [ ] Add `--template` flag to `helix apply`: when set, also registers the project as an org sample project template (deferred)
- [ ] Add `PUT /api/v1/sample-projects/simple` upsert endpoint (deferred with `--template` flag)

## Server

- [x] Add `PUT /api/v1/projects/apply` endpoint in `api/pkg/server/project_handlers.go` — idempotent upsert by name+org; registered before `/projects/{id}` to avoid routing conflict

## K8s Operator — Bug fixes (AIApp)

- [x] Fix GPTScript conversion bug in `operator/internal/controller/aiapp_controller.go` (was iterating `Zapier` — documented as unsupported since `types.AssistantConfig` has no `GPTScripts` field; Zapier conversion left correct)
- [x] Fix Knowledge conversion: add loop to convert `assistant.Knowledge` from operator CRD types to `types.AssistantKnowledge`
- [x] Add `HELIX_TLS_SKIP_VERIFY` env var support (was TODO'd in code)
- [x] Add `OrganizationID` field to `AIAppSpec` in `operator/api/v1alpha1/aiapp_types.go` and pass through during create/update
- [x] Add `Ready`, `AppID`, `LastSynced`, `Message` to `AIAppStatus` and populate after each reconcile

## K8s Operator — Project CRD

- [x] Create `operator/api/v1alpha1/project_types.go` with `Project`, `ProjectSpec`, `ProjectAgentSpec`, `ProjectAgentTools`, `ProjectStatus`, `ProjectList`
- [x] Register `Project` and `ProjectList` in the scheme builder (`groupversion_info.go` / `init()`)
- [x] Create `operator/internal/controller/project_controller.go`: reconcile Project CRD → create/update Helix project + agent app, use `k8s.<namespace>.<name>` as project name, handle finalizer for deletion, full status reporting
- [x] Register `ProjectReconciler` in `operator/cmd/main.go`
- [x] Manually added deepcopy methods to `zz_generated.deepcopy.go` (`make generate` requires `controller-gen` binary not present in this environment)
- [ ] Run `make generate manifests` in `operator/` to produce CRD YAML in `operator/config/crd/bases/` (requires `controller-gen`; should be done in CI or dev environment with the tool installed)

## Examples

- [x] Add `examples/project.yaml` showing a full project with inline agent (model, system_prompt, tools, one MCP)
- [ ] Update operator README with `Project` CRD docs and required RBAC
