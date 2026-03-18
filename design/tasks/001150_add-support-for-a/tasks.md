# Implementation Tasks

## New types

- [x] Add `ProjectCRD`, `ProjectSpec`, `ProjectAgentSpec`, `ProjectAgentTools` to `api/pkg/types/project.go`
- [x] Add `ToAppHelixConfig() *AppHelixConfig` conversion method on `ProjectAgentSpec`
- [ ] Add `ProjectStartup` struct (`Install`, `Start` string fields) to `api/pkg/types/project.go`
- [ ] Add `Startup *ProjectStartup` to `ProjectSpec`
- [ ] Add `StartupInstall`, `StartupStart` string fields to the `Project` DB model with `gorm:"column:..."` tags

## `helix apply` — Project kind

- [x] Add `kind:` dispatcher to `api/pkg/cli/app/apply.go`
- [x] Add `ApplyProject` client method (`api/pkg/client/project.go`)
- [x] Implement `runApplyProject()` with agent create/update + project upsert
- [ ] Verify `Startup` fields flow through automatically (they will once `ProjectSpec` has them, since `ProjectApplyRequest.Spec` is `ProjectSpec`)
- [ ] (Deferred) Add `--template` flag for registering org sample project templates

## Server

- [x] Add `PUT /api/v1/projects/apply` endpoint — idempotent upsert by name+org
- [ ] Copy `Spec.Startup.Install` / `Spec.Startup.Start` to project model fields in `applyProject` handler (create and update paths)
- [ ] Add DB migration for `startup_install` and `startup_start` columns on `projects` table

## K8s Operator — Bug fixes (AIApp)

- [x] Fix GPTScript conversion bug (documented as unsupported; `types.AssistantConfig` has no `GPTScripts` field)
- [x] Fix Knowledge source conversion
- [x] Add `HELIX_TLS_SKIP_VERIFY` env var support
- [x] Add `OrganizationID` to `AIAppSpec`
- [x] Add `Ready`, `AppID`, `LastSynced`, `Message` to `AIAppStatus`

## K8s Operator — Project CRD

- [x] Create `operator/api/v1alpha1/project_types.go` with `Project`, `ProjectSpec`, `ProjectStatus`, `ProjectList`
- [x] Register `Project` and `ProjectList` in scheme builder
- [x] Create `operator/internal/controller/project_controller.go` with full reconciler
- [x] Register `ProjectReconciler` in `operator/cmd/main.go`
- [x] Add deepcopy methods to `zz_generated.deepcopy.go`
- [ ] Add `ProjectStartup` to operator's `ProjectSpec` CRD type
- [ ] Pass `Startup` fields through in `ProjectApplyRequest` from operator reconciler
- [ ] Improve `reconcileProjectAgent`: on subsequent reconciles, use `Status.AgentAppID` to update directly (look up by ID first, fall back to name search if ID not found/deleted)
- [ ] Run `make generate manifests` in `operator/` (requires `controller-gen`; do in CI or dev environment)

## Examples

- [x] Add `examples/project.yaml`
- [ ] Add `startup` block to `examples/project.yaml`
- [ ] Update operator README with `Project` CRD docs and required RBAC
