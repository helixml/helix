# Implementation Tasks

## Types (`api/pkg/types/project.go`)

- [x] Add `ProjectStartup` struct (`Install`, `Start`)
- [x] Add `Startup *ProjectStartup` to `ProjectSpec`
- [x] Add `StartupInstall`, `StartupStart` to `Project` DB model
- [x] Add `ProjectRepositorySpec` struct (`URL`, `Branch`, `Primary` bool)
- [x] Add `ProjectKanban` struct with `WIPLimits *ProjectWIPLimits`
- [x] Add `ProjectWIPLimits` struct (`Planning`, `Implementation`, `Review` int)
- [x] Add `ProjectTaskSpec` struct (`Title` string, `Description` string)
- [x] Add `Repository *ProjectRepositorySpec` (singular shorthand) to `ProjectSpec`
- [x] Add `Repositories []ProjectRepositorySpec` to `ProjectSpec`
- [x] Add `Kanban *ProjectKanban` to `ProjectSpec`
- [x] Add `Tasks []ProjectTaskSpec` to `ProjectSpec`
- [x] Add `ValidateRepositories() error` method on `ProjectSpec`
- [x] Add `ResolvedRepositories() []ProjectRepositorySpec` method (normalises singular/plural → always returns a slice)

## Store (`api/pkg/store/`)

- [x] Check if `ListGitRepositories` supports URL filtering; if not, add `GetGitRepositoryByExternalURL(ctx, orgID, url)` helper

## Server (`api/pkg/server/project_handlers.go`)

- [x] Add `PUT /api/v1/projects/apply` endpoint (idempotent upsert by name+org)
- [x] Wire `startup` fields through on create and update
- [x] Call `spec.ValidateRepositories()` in `applyProject` handler — return 400 on validation error
- [x] For each resolved repository: find-or-create `GitRepository` by `ExternalURL`, attach to project with `AttachRepositoryToProject`
- [x] Set primary repository with `SetProjectPrimaryRepository` after all repos are attached
- [x] Map `spec.Kanban.WIPLimits` → `project.Metadata.BoardSettings.WIPLimits` on create and update
- [x] For each task in `spec.Tasks`: list existing project tasks, create task in Planning column only if title not already present (idempotent by title)

## CLI (`api/pkg/cli/app/apply.go`)

- [x] Add `kind:` dispatcher routing `Project` to `runApplyProject()`
- [x] `runApplyProject()` creates/updates agent app and calls `ApplyProject`
- [x] `ApplyProject` client method
- [x] Agent idempotency: look up by name, update or create
- [x] Verify `Repositories` and `Kanban` flow through `ProjectApplyRequest.Spec` automatically (no extra wiring needed beyond types)

## K8s Operator — Bug fixes

- [x] Add `HELIX_TLS_SKIP_VERIFY` support (in AIApp controller)
- [ ] Fix GPTScript conversion bug
- [ ] Fix Knowledge source conversion
- [ ] Add `OrganizationID` to `AIAppSpec`
- [ ] Add status fields to `AIAppStatus`

## K8s Operator — Project CRD

- [x] `Project`, `ProjectSpec`, `ProjectStatus`, `ProjectList` types
- [x] Scheme registration
- [x] Full reconciler with finalizer, status reporting
- [x] Registration in `operator/cmd/main.go`
- [x] Deepcopy methods in `zz_generated.deepcopy.go`
- [x] Add `ProjectRepositorySpec`, `ProjectKanban`, `ProjectWIPLimits`, `ProjectTaskSpec` to operator `ProjectSpec` CRD type
- [x] Map operator `Repositories`, `Kanban`, and `Tasks` through to `ProjectApplyRequest` in reconciler
- [ ] Run `make generate manifests` (requires `controller-gen`)

## Examples

- [x] `examples/project.yaml` with startup block
- [x] Update `examples/project.yaml` to show multi-repo + kanban + tasks

## Testing setup

- [ ] Install `kind`: `go install sigs.k8s.io/kind@latest`
- [ ] Install `kubectl`: download binary or snap
- [ ] Install `controller-gen`: `go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest`
- [ ] Verify `helix apply` end-to-end: build CLI, apply project YAML against `localhost:8080`, check idempotency
- [ ] Verify `kubectl apply` end-to-end: `kind create cluster`, install CRDs, run operator locally, apply project YAML, check `kubectl get projects` status
